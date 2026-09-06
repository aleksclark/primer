package api

import (
	"errors"
	"net/http"
	"strings"

	"git.clark.team/aleksclark/authstack/auth"
	"git.clark.team/aleksclark/authstack/authhttp"
	"github.com/jackc/pgx/v5"
)

// Only parent HTTP requests accept Clerk sessions. Student cookies and device
// bearer tokens retain their existing, separate authorization implementations.
func (s *Server) parentBoundary(next http.Handler) http.Handler {
	if s.Auth.Mode != "clerk" {
		return next
	}
	protected := authhttp.AuthenticateWithPolicy(s.ParentAuthenticator, s.ParentPolicy)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := auth.PrincipalFromContext(r.Context())
		if !ok || p.Kind != auth.PrincipalHuman || p.Credential != auth.CredentialSession || p.SessionID == "" {
			w.Header().Set("WWW-Authenticate", "Bearer")
			problem(w, 401, "unauthorized", "parent session required")
			return
		}
		// Authorize before decoding requests. Household and actor are resolved from
		// the local membership ledger, never Clerk org/email/role or request data.
		if r.URL.Path != "/auth/logout" {
			if _, err := s.clerkScope(r); err != nil {
				s.parentError(w, err)
				return
			}
		}
		next.ServeHTTP(w, r)
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/health" || path == "/openapi.json" || path == "/openapi.yaml" || path == "/auth/login" || path == "/auth/callback" || strings.HasPrefix(path, "/student/") || strings.HasPrefix(path, "/device/") || strings.HasPrefix(path, "/management-device/") {
			next.ServeHTTP(w, r)
			return
		}
		protected.ServeHTTP(w, r)
	})
}

func (s *Server) clerkScope(r *http.Request) (scope, error) {
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.SessionID == "" {
		return scope{}, auth.ErrUnauthenticated
	}
	var sc scope
	err := s.DB.QueryRow(r.Context(), `SELECT i.tenant_id,i.subject_ref
 FROM parent_identities i JOIN parent_memberships m ON m.tenant_id=i.tenant_id AND m.subject_ref=i.subject_ref
 WHERE i.issuer=$1 AND i.subject=$2 AND i.revoked_at IS NULL AND m.revoked_at IS NULL AND m.role='admin'
 AND NOT EXISTS (SELECT 1 FROM parent_session_revocations r WHERE r.issuer=i.issuer AND r.session_id=$3)`, p.Issuer, string(p.Subject), p.SessionID).Scan(&sc.Tenant, &sc.Subject)
	if errors.Is(err, pgx.ErrNoRows) {
		return scope{}, auth.ErrForbidden
	}
	if err != nil {
		return scope{}, auth.ErrUnavailable
	}
	return sc, nil
}

func (s *Server) parentError(w http.ResponseWriter, err error) {
	if s.Auth.Mode == "clerk" {
		if errors.Is(err, auth.ErrForbidden) {
			problem(w, 403, "denied", "parent membership or session is not permitted")
			return
		}
		if errors.Is(err, auth.ErrUnavailable) {
			problem(w, 503, "unavailable", "parent authorization unavailable")
			return
		}
		w.Header().Set("WWW-Authenticate", "Bearer")
	}
	problem(w, 401, "unauthorized", "parent session required")
}

func (s *Server) clerkLogout(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.PrincipalFromContext(r.Context())
	if !ok || p.SessionID == "" {
		s.parentError(w, auth.ErrUnauthenticated)
		return
	}
	if _, err := s.DB.Exec(r.Context(), `INSERT INTO parent_session_revocations(issuer,session_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, p.Issuer, p.SessionID); err != nil {
		problem(w, 503, "unavailable", "unable to revoke local parent session")
		return
	}
	jsonOK(w, map[string]string{"status": "signed_out"})
}
