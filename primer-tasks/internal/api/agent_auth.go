package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"git.clark.team/aleksclark/authstack/auth"
	"github.com/jackc/pgx/v5"
	"primer-tasks/internal/domain/parent"
)

const agentBearerProtocol = "primer-tasks.v1.bearer."

type agentAuthKey struct{}

// The verifier closure holds a credential only in process memory. It is never
// serialized. Authstack alone verifies expiry; its normalized Principal does
// not expose exp, and Tasks must not decode provider claims to invent one.
type agentAuthorization struct {
	check                    func(context.Context) error
	verifyProvider           func(context.Context) error
	issuer, subject, session string
	bffHash                  []byte
	deadline                 time.Time
}

func agentBearer(r *http.Request) (string, error) {
	var token string
	for _, protocol := range strings.Split(r.Header.Get("Sec-WebSocket-Protocol"), ",") {
		protocol = strings.TrimSpace(protocol)
		if strings.HasPrefix(protocol, agentBearerProtocol) {
			if token != "" {
				return "", auth.ErrUnauthenticated
			}
			token = strings.TrimPrefix(protocol, agentBearerProtocol)
			if token == "" || len(token) > 8192 || strings.ContainsAny(token, " \t\r\n") {
				return "", auth.ErrUnauthenticated
			}
		}
	}
	if token == "" {
		return "", auth.ErrUnauthenticated
	}
	return token, nil
}

// Browser WebSockets cannot set Authorization. Accept one ephemeral subprotocol
// credential only on the parent socket route, before the existing Authstack
// middleware. Accept negotiates only primer-tasks.v1, never this input protocol.
func (s *Server) agentUpgradeRequest(r *http.Request) (*http.Request, error) {
	if !s.agentOriginAllowed(r) || !s.validCSRF(r) {
		return nil, auth.ErrUnauthenticated
	}
	token, err := agentBearer(r)
	if err != nil {
		return nil, err
	}
	clone := r.Clone(r.Context())
	clone.Header.Set("Authorization", "Bearer "+token)
	return clone, nil
}

func (s *Server) socketAuthorization(r *http.Request, sc scope) (*agentAuthorization, error) {
	if s.Auth.Mode == "clerk" {
		token, err := agentBearer(r)
		if err != nil {
			return nil, err
		}
		principal, ok := auth.PrincipalFromContext(r.Context())
		if !ok {
			return nil, auth.ErrUnauthenticated
		}
		a := &agentAuthorization{issuer: principal.Issuer, subject: string(principal.Subject), session: principal.SessionID}
		a.verifyProvider = func(ctx context.Context) error {
			p, err := s.ParentAuthenticator.Authenticate(ctx, auth.Credential{Token: token}, s.ParentPolicy)
			if err != nil || p.Kind != auth.PrincipalHuman || p.Credential != auth.CredentialSession || p.Issuer != a.issuer || string(p.Subject) != a.subject || p.SessionID == "" || p.SessionID != a.session {
				return auth.ErrUnauthenticated
			}
			return nil
		}
		a.check = func(ctx context.Context) error {
			if err := a.verifyProvider(ctx); err != nil {
				return err
			}
			req := r.Clone(auth.WithPrincipal(ctx, principal))
			current, err := s.clerkScope(req)
			if err != nil || current != sc {
				return auth.ErrForbidden
			}
			return nil
		}
		return a, a.check(r.Context())
	}
	cookie, err := r.Cookie("tasks_parent")
	if err != nil {
		return nil, err
	}
	a := &agentAuthorization{bffHash: hash(cookie.Value)}
	a.check = func(ctx context.Context) error {
		current, err := s.parentScope(r.Clone(ctx))
		if err != nil || current != sc {
			return auth.ErrUnauthenticated
		}
		return nil
	}
	return a, a.check(r.Context())
}

func checkAgentAuthorization(ctx context.Context) error {
	if a, ok := ctx.Value(agentAuthKey{}).(*agentAuthorization); ok {
		if !a.deadline.IsZero() && !time.Now().Before(a.deadline) {
			return auth.ErrUnauthenticated
		}
		return a.check(ctx)
	}
	return nil // Non-Clerk internal fixtures; durable Clerk rows require it below.
}

// Shared locks serialize local revocation with effects and private frame writes.
// Lock in membership -> identity/session order, then use NEW statements for all
// authorization predicates. A locking SELECT may have taken its MVCC snapshot
// before waiting: notably Clerk logout locks identity but inserts a different
// session-revocation row, so NOT EXISTS in that locking SELECT would be stale.
func lockAgentParent(ctx context.Context, tx pgx.Tx, tenant, actor string) error {
	if err := lockAgentParentRows(ctx, tx, tenant, actor); err != nil {
		return err
	}
	return checkLockedAgentParent(ctx, tx, tenant, actor)
}

func lockAgentParentRows(ctx context.Context, tx pgx.Tx, tenant, actor string) error {
	var locked string
	if err := tx.QueryRow(ctx, `SELECT subject_ref FROM parent_memberships WHERE tenant_id=$1 AND subject_ref=$2 FOR SHARE`, tenant, actor).Scan(&locked); err != nil {
		return parent.ErrInvalidContext
	}
	a, hasCredential := ctx.Value(agentAuthKey{}).(*agentAuthorization)
	if hasCredential {
		if a.issuer != "" {
			if err := tx.QueryRow(ctx, `SELECT subject FROM parent_identities WHERE issuer=$1 AND subject=$2 FOR SHARE`, a.issuer, a.subject).Scan(&locked); err != nil {
				return parent.ErrInvalidContext
			}
		} else {
			if err := tx.QueryRow(ctx, `SELECT subject_ref FROM bff_sessions WHERE handle_hash=$1 FOR SHARE`, a.bffHash).Scan(&locked); err != nil {
				return parent.ErrInvalidContext
			}
		}
	}
	return nil
}

// Called only while membership and identity/session row locks remain held.
func checkLockedAgentParent(ctx context.Context, tx pgx.Tx, tenant, actor string) error {
	a, hasCredential := ctx.Value(agentAuthKey{}).(*agentAuthorization)
	// These statements start AFTER every authority row lock was acquired.
	var allowed bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM parent_memberships WHERE tenant_id=$1 AND subject_ref=$2 AND revoked_at IS NULL AND role='admin')`, tenant, actor).Scan(&allowed); err != nil || !allowed {
		return parent.ErrInvalidContext
	}
	if hasCredential {
		if a.issuer != "" {
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM parent_identities i WHERE i.issuer=$1 AND i.subject=$2 AND i.tenant_id=$4 AND i.subject_ref=$5 AND i.revoked_at IS NULL AND NOT EXISTS(SELECT 1 FROM parent_session_revocations r WHERE r.issuer=i.issuer AND r.session_id=$3))`, a.issuer, a.subject, a.session, tenant, actor).Scan(&allowed); err != nil || !allowed {
				return parent.ErrInvalidContext
			}
			if a.verifyProvider == nil {
				return parent.ErrInvalidContext
			}
			// Verify expiry using Authstack, without another pooled DB connection
			// while holding this transaction's locks. No JWT parsing in Tasks.
			if err := a.verifyProvider(ctx); err != nil {
				return err
			}
		} else {
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM bff_sessions WHERE handle_hash=$1 AND tenant_id=$2 AND subject_ref=$3 AND session_kind='parent' AND expires_at>clock_timestamp() AND revoked_at IS NULL)`, a.bffHash, tenant, actor).Scan(&allowed); err != nil || !allowed {
				return parent.ErrInvalidContext
			}
		}
		if !a.deadline.IsZero() && !time.Now().Before(a.deadline) {
			return auth.ErrUnauthenticated
		}
	}
	return nil
}

func bindAgentRunAuthorization(ctx context.Context, tx pgx.Tx, tenant, run string) error {
	a, ok := ctx.Value(agentAuthKey{}).(*agentAuthorization)
	if !ok {
		return nil
	}
	if a.issuer == "" {
		_, err := tx.Exec(ctx, `INSERT INTO agent_run_parent_authority(tenant_id,run_id,bff_hash) VALUES($1,$2,$3)`, tenant, run, a.bffHash)
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO agent_run_parent_authority(tenant_id,run_id,issuer,subject,session_id) VALUES($1,$2,$3,$4,$5)`, tenant, run, a.issuer, a.subject, a.session)
	return err
}

func checkDurableAgentAuthorization(ctx context.Context, tx pgx.Tx, tenant, actor, run string) error {
	var issuer, subject, session string
	var bffHash []byte
	err := tx.QueryRow(ctx, `SELECT COALESCE(issuer,''),COALESCE(subject,''),COALESCE(session_id,''),bff_hash FROM agent_run_parent_authority WHERE tenant_id=$1 AND run_id=$2`, tenant, run).Scan(&issuer, &subject, &session, &bffHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	// A fresh credential may refresh expiry for the SAME issuer subject/session,
	// but neither another actor nor a revoked original session can consume a run.
	a, ok := ctx.Value(agentAuthKey{}).(*agentAuthorization)
	if !ok || a.issuer != issuer || a.subject != subject || a.session != session || !bytes.Equal(a.bffHash, bffHash) {
		return parent.ErrInvalidContext
	}
	return lockAgentParent(ctx, tx, tenant, actor)
}
