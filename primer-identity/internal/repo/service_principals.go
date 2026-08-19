package repo

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aleksclark/primer/identity/internal/domain"
)

type ServiceCredential struct {
	ID, ServicePrincipalID, OAuthClientID             uuid.UUID
	SubjectRef                                        string
	SecretHash                                        []byte
	PepperVersion                                     int16
	AllowedResources, AllowedAudiences, AllowedScopes []string
	NotAfter                                          *time.Time
}

func GetActiveServiceCredential(ctx context.Context, q Querier, clientID uuid.UUID) (*ServiceCredential, error) {
	if clientID == uuid.Nil {
		return nil, wrapf("get service credential", domain.ErrInvalid)
	}
	out := &ServiceCredential{}
	err := q.QueryRow(ctx, `
SELECT c.id,c.service_principal_id,c.oauth_client_id,p.subject_ref,c.secret_hash,c.pepper_version,c.allowed_resources,c.allowed_audiences,c.allowed_scopes,p.disabled_at
FROM oauth_service_credentials c
JOIN oauth_service_principals p ON p.id=c.service_principal_id
JOIN oauth_clients oc ON oc.id=c.oauth_client_id
WHERE c.oauth_client_id=$1 AND oc.enabled AND p.enabled AND c.revoked_at IS NULL
ORDER BY c.created_at DESC LIMIT 1`, clientID).Scan(&out.ID, &out.ServicePrincipalID, &out.OAuthClientID, &out.SubjectRef, &out.SecretHash, &out.PepperVersion, &out.AllowedResources, &out.AllowedAudiences, &out.AllowedScopes, &out.NotAfter)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, wrapf("get service credential", domain.ErrNotFound)
	}
	return out, wrapf("get service credential", err)
}

func CreateServiceGrant(ctx context.Context, q Querier, principalID, clientID uuid.UUID, resource, audience string, scopes []string, now, notAfter time.Time) (uuid.UUID, error) {
	if principalID == uuid.Nil || clientID == uuid.Nil || resource == "" || audience == "" || len(scopes) == 0 || !notAfter.After(now) {
		return uuid.Nil, wrapf("create service grant", domain.ErrInvalid)
	}
	if _, err := q.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, principalID.String()+"|"+clientID.String()+"|"+resource+"|"+audience); err != nil {
		return uuid.Nil, wrapf("create service grant", err)
	}
	var id uuid.UUID
	err := q.QueryRow(ctx, `SELECT id FROM oauth_grants WHERE service_principal_id=$1 AND oauth_client_id=$2 AND resource_uri=$3 AND audience=$4 AND subject_class='service' AND status='active' FOR UPDATE`, principalID, clientID, resource, audience).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, wrapf("create service grant", err)
	}
	err = q.QueryRow(ctx, `INSERT INTO oauth_grants(account_id,service_principal_id,oauth_client_id,provider_session_association_id,resource_uri,audience,scopes,subject_class,status,granted_at,not_after) VALUES(NULL,$1,$2,NULL,$3,$4,$5,'service','active',$6,$7) RETURNING id`, principalID, clientID, resource, audience, scopes, now, notAfter).Scan(&id)
	return id, wrapf("create service grant", err)
}
