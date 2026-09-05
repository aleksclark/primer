package db

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ParentLink struct {
	Issuer, Subject, TenantID, ActorRef, HouseholdName string
	CreateHousehold                                    bool
}

// BootstrapParent is an operator-only, additive identity link. It never guesses
// identity from email or rebinds/reactivates an existing canonical identity.
func BootstrapParent(ctx context.Context, pool *pgxpool.Pool, in ParentLink) error {
	u, err := url.Parse(in.Issuer)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("issuer must be an exact HTTPS issuer URL")
	}
	if _, err := uuid.Parse(in.TenantID); err != nil {
		return errors.New("tenant-id must be an explicit local UUID")
	}
	if strings.TrimSpace(in.Subject) == "" || strings.TrimSpace(in.ActorRef) == "" || (in.CreateHousehold && strings.TrimSpace(in.HouseholdName) == "") {
		return errors.New("subject, actor-ref, and (when creating) household-name are required")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return errors.New("bootstrap database unavailable")
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, in.Issuer+"\n"+in.Subject); err != nil {
		return err
	}
	if in.CreateHousehold {
		if _, err = tx.Exec(ctx, `INSERT INTO tenants(id,name) VALUES($1,$2) ON CONFLICT DO NOTHING`, in.TenantID, in.HouseholdName); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO parent_memberships(tenant_id,subject_ref,role) VALUES($1,$2,'admin') ON CONFLICT DO NOTHING`, in.TenantID, in.ActorRef); err != nil {
			return err
		}
	}
	var active bool
	if err = tx.QueryRow(ctx, `SELECT revoked_at IS NULL FROM parent_memberships WHERE tenant_id=$1 AND subject_ref=$2 AND role='admin' FOR UPDATE`, in.TenantID, in.ActorRef).Scan(&active); err != nil || !active {
		return errors.New("existing active local admin membership required; use explicit create-household for initial provisioning")
	}
	var tenant, actor string
	var revoked bool
	err = tx.QueryRow(ctx, `SELECT tenant_id,subject_ref,revoked_at IS NOT NULL FROM parent_identities WHERE issuer=$1 AND subject=$2 FOR UPDATE`, in.Issuer, in.Subject).Scan(&tenant, &actor, &revoked)
	if err == nil {
		if tenant != in.TenantID || actor != in.ActorRef || revoked {
			return errors.New("refusing to rebind or reactivate existing identity")
		}
		return tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO parent_identities(issuer,subject,tenant_id,subject_ref) VALUES($1,$2,$3,$4)`, in.Issuer, in.Subject, in.TenantID, in.ActorRef); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_records(tenant_id,subject_ref,action) VALUES($1,$2,'parent.identity_linked')`, in.TenantID, in.ActorRef); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
