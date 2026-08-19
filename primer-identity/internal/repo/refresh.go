package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/identity/internal/domain"
)

// RefreshRedemption is the locked family/grant state plus the newly created
// current token. The caller signs and audits while the same transaction is open.
var ErrRefreshScopeWidening = errors.New("refresh scope widening")

type RefreshRedemption struct {
	Family      *domain.OAuthRefreshFamily
	Previous    *domain.OAuthRefreshToken
	Next        *domain.OAuthRefreshToken
	Grant       *domain.OAuthGrant
	Association *domain.ProviderSessionAssociation
}

func RedeemRefreshToken(ctx context.Context, q Querier, hashes [][]byte, clientID uuid.UUID, resource string, requested []string, nextHash []byte, pepper int16, now time.Time) (*RefreshRedemption, error) {
	if clientID == uuid.Nil || len(hashes) == 0 || resource == "" || len(nextHash) != 32 || pepper <= 0 || now.IsZero() {
		return nil, wrapf("redeem refresh token", domain.ErrInvalid)
	}
	var prev domain.OAuthRefreshToken
	var family domain.OAuthRefreshFamily
	found := false
	for _, hash := range hashes {
		err := q.QueryRow(ctx, `
SELECT t.id,t.family_id,t.token_hash,t.pepper_version,t.sequence,t.issued_at,t.expires_at,t.consumed_at,t.revoked_at,t.replaced_by_id,t.reuse_detected_at,
       f.id,f.grant_id,f.oauth_client_id,f.resource_uri,f.status,f.absolute_expires_at,f.idle_expires_at,f.last_rotated_at,f.revoked_at,f.expired_at,f.reuse_detected_at,f.revoke_reason_code,f.version
FROM oauth_refresh_tokens t JOIN oauth_refresh_families f ON f.id=t.family_id
WHERE t.token_hash=$1 AND f.oauth_client_id=$2 AND f.resource_uri=$3
FOR UPDATE OF f,t`, hash, clientID, resource).Scan(
			&prev.ID, &prev.FamilyID, &prev.TokenHash, &prev.PepperVersion, &prev.Sequence, &prev.IssuedAt, &prev.ExpiresAt, &prev.ConsumedAt, &prev.RevokedAt, &prev.ReplacedByID, &prev.ReuseDetectedAt,
			&family.ID, &family.GrantID, &family.OAuthClientID, &family.ResourceURI, &family.Status, &family.AbsoluteExpiresAt, &family.IdleExpiresAt, &family.LastRotatedAt, &family.RevokedAt, &family.ExpiredAt, &family.ReuseDetectedAt, &family.RevokeReasonCode, &family.Version)
		if err == nil {
			found = true
			break
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, wrapf("redeem refresh token", err)
		}
	}
	if !found {
		return nil, wrapf("redeem refresh token", domain.ErrNotFound)
	}
	grant, err := GetOAuthGrant(ctx, q, family.GrantID)
	if err != nil {
		return nil, err
	}
	if grant.Status != "active" || family.Status != domain.RefreshFamilyStatusActive || prev.ConsumedAt != nil || prev.RevokedAt != nil || !prev.ExpiresAt.After(now) || !family.IdleExpiresAt.After(now) || !family.AbsoluteExpiresAt.After(now) {
		if prev.ConsumedAt != nil || prev.RevokedAt != nil {
			if err := markRefreshReuse(ctx, q, family.ID, now); err != nil {
				return nil, err
			}
		} else if !prev.ExpiresAt.After(now) || !family.IdleExpiresAt.After(now) || !family.AbsoluteExpiresAt.After(now) {
			if err := markRefreshExpired(ctx, q, family.ID, now); err != nil {
				return nil, err
			}
		}
		return nil, wrapf("redeem refresh token", domain.ErrConflict)
	}
	if err := validateRefreshScope(grant.Scopes, requested); err != nil {
		return nil, err
	}
	_, assoc, err := LoadConsumeBindings(ctx, q, grant.ID)
	if err != nil {
		return nil, err
	}
	nextID := uuid.New()
	// Seed the successor revoked so the partial one-live index remains valid;
	// clear it only after the predecessor points at an existing row.
	var next domain.OAuthRefreshToken
	if err = q.QueryRow(ctx, `
INSERT INTO oauth_refresh_tokens(id,family_id,token_hash,pepper_version,sequence,issued_at,expires_at,revoked_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8)
RETURNING id,family_id,token_hash,pepper_version,sequence,issued_at,expires_at,consumed_at,revoked_at,replaced_by_id,reuse_detected_at`, nextID, family.ID, nextHash, pepper, prev.Sequence+1, now, family.IdleExpiresAt, now).Scan(&next.ID, &next.FamilyID, &next.TokenHash, &next.PepperVersion, &next.Sequence, &next.IssuedAt, &next.ExpiresAt, &next.ConsumedAt, &next.RevokedAt, &next.ReplacedByID, &next.ReuseDetectedAt); err != nil {
		return nil, wrapf("redeem refresh token", err)
	}
	if _, err = q.Exec(ctx, `UPDATE oauth_refresh_tokens SET consumed_at=$2,replaced_by_id=$3 WHERE id=$1 AND consumed_at IS NULL AND revoked_at IS NULL`, prev.ID, now, next.ID); err != nil {
		return nil, wrapf("redeem refresh token", err)
	}
	if _, err = q.Exec(ctx, `UPDATE oauth_refresh_tokens SET revoked_at=NULL WHERE id=$1`, next.ID); err != nil {
		return nil, wrapf("redeem refresh token", err)
	}
	if _, err = q.Exec(ctx, `UPDATE oauth_refresh_families SET last_rotated_at=$2,idle_expires_at=LEAST($3,absolute_expires_at),version=version+1 WHERE id=$1`, family.ID, now, now.Add(domain.MaxRefreshIdle)); err != nil {
		return nil, wrapf("redeem refresh token", err)
	}
	return &RefreshRedemption{Family: &family, Previous: &prev, Next: &next, Grant: grant, Association: assoc}, nil
}

func validateRefreshScope(granted, requested []string) error {
	if len(requested) == 0 {
		return nil
	}
	have := make(map[string]struct{}, len(granted))
	for _, scope := range granted {
		have[scope] = struct{}{}
	}
	for _, scope := range requested {
		if _, ok := have[scope]; !ok {
			return fmt.Errorf("%w: %w", domain.ErrConflict, ErrRefreshScopeWidening)
		}
	}
	return nil
}

func markRefreshExpired(ctx context.Context, q Querier, familyID uuid.UUID, now time.Time) error {
	reason := "expired"
	if _, err := q.Exec(ctx, `UPDATE oauth_refresh_tokens SET revoked_at=$2 WHERE family_id=$1 AND consumed_at IS NULL AND revoked_at IS NULL`, familyID, now); err != nil {
		return wrapf("mark refresh expired", err)
	}
	if _, err := q.Exec(ctx, `UPDATE oauth_refresh_families SET status='expired',expired_at=$2,revoke_reason_code=$3,version=version+1 WHERE id=$1 AND status='active'`, familyID, now, reason); err != nil {
		return wrapf("mark refresh expired", err)
	}
	if _, err := q.Exec(ctx, `UPDATE oauth_grants g SET status='expired',revoked_at=$2,revoke_reason_code=$3,version=g.version+1 FROM oauth_refresh_families f WHERE f.id=$1 AND g.id=f.grant_id AND g.status='active'`, familyID, now, reason); err != nil {
		return wrapf("mark refresh expired", err)
	}
	return nil
}

func markRefreshReuse(ctx context.Context, q Querier, familyID uuid.UUID, now time.Time) error {
	reason := "refresh_reuse"
	if _, err := q.Exec(ctx, `UPDATE oauth_refresh_tokens SET revoked_at=$2 WHERE family_id=$1 AND consumed_at IS NULL AND revoked_at IS NULL`, familyID, now); err != nil {
		return wrapf("mark refresh reuse", err)
	}
	if _, err := q.Exec(ctx, `UPDATE oauth_refresh_families SET status='reuse_detected',revoked_at=$2,reuse_detected_at=$2,revoke_reason_code=$3,version=version+1 WHERE id=$1`, familyID, now, reason); err != nil {
		return wrapf("mark refresh reuse", err)
	}
	if _, err := q.Exec(ctx, `UPDATE oauth_grants g SET status='revoked',revoked_at=$2,revoke_reason_code=$3,version=g.version+1 FROM oauth_refresh_families f WHERE f.id=$1 AND g.id=f.grant_id AND g.status='active'`, familyID, now, reason); err != nil {
		return wrapf("mark refresh reuse", err)
	}
	return nil
}
