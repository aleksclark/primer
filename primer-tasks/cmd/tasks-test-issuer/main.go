// Command tasks-test-issuer is a protocol-compatible, explicitly test-only
// OAuth/OIDC issuer. It uses its own Postgres database for authorization-code
// replay protection; it is never selected by a production Tasks configuration.
package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func issuerSigningSecret() []byte {
	return []byte(env("TASKS_TEST_ISSUER_SECRET", "primer-tasks-test-issuer-secret"))
}

func main() {
	dsn := os.Getenv("TASKS_TEST_ISSUER_DATABASE_URL")
	if dsn == "" {
		panic("TASKS_TEST_ISSUER_DATABASE_URL is required")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		panic(err)
	}
	defer pool.Close()
	_, err = pool.Exec(context.Background(), `CREATE TABLE IF NOT EXISTS tasks_test_oauth_codes (code_hash bytea PRIMARY KEY, subject_ref text NOT NULL, redirect_uri text NOT NULL, client_id text NOT NULL, challenge text NOT NULL, expires_at timestamptz NOT NULL, consumed_at timestamptz)`)
	if err != nil {
		panic(err)
	}
	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, map[string]string{"status": "ok"}) })
	r.Get("/.well-known/openid-configuration", metadata)
	r.Get("/oauth/authorize", authorize(pool))
	r.Post("/oauth/token", token(pool))
	addr := env("TASKS_TEST_ISSUER_HOST", "0.0.0.0") + ":" + env("TASKS_TEST_ISSUER_PORT", "8091")
	if err := http.ListenAndServe(addr, r); err != nil {
		panic(err)
	}
}

func authorize(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("response_type") != "code" || q.Get("code_challenge_method") != "S256" || q.Get("state") == "" || q.Get("redirect_uri") == "" || q.Get("client_id") == "" {
			http.Error(w, "invalid authorization request", 400)
			return
		}
		sub := q.Get("login_hint")
		if sub == "" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			base := "/issuer/oauth/authorize?"
			qA := url.Values{}
			for k, values := range q {
				for _, value := range values {
					qA.Add(k, value)
				}
			}
			qA.Set("login_hint", "parent-a")
			qB := url.Values{}
			for k, values := range q {
				for _, value := range values {
					qB.Add(k, value)
				}
			}
			qB.Set("login_hint", "parent-b")
			_, _ = fmt.Fprintf(w, "<!doctype html><title>Test Identity</title><h1>Choose test parent</h1><a href=\"%s%s\">Parent A</a><a href=\"%s%s\">Parent B</a>", base, qA.Encode(), base, qB.Encode())
			return
		}
		code := random(32)
		exp := time.Now().Add(2 * time.Minute)
		_, err := pool.Exec(r.Context(), `INSERT INTO tasks_test_oauth_codes(code_hash,subject_ref,redirect_uri,client_id,challenge,expires_at) VALUES($1,$2,$3,$4,$5,$6)`, hash(code), sub, q.Get("redirect_uri"), q.Get("client_id"), q.Get("code_challenge"), exp)
		if err != nil {
			http.Error(w, "issuer unavailable", 503)
			return
		}
		redirect, err := url.Parse(q.Get("redirect_uri"))
		if err != nil {
			http.Error(w, "invalid redirect", 400)
			return
		}
		v := redirect.Query()
		v.Set("code", code)
		v.Set("state", q.Get("state"))
		redirect.RawQuery = v.Encode()
		http.Redirect(w, r, redirect.String(), 302)
	}
}
func token(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil || r.Form.Get("grant_type") != "authorization_code" {
			writeJSONStatus(w, 400, map[string]string{"error": "invalid_request"})
			return
		}
		code, verifier := r.Form.Get("code"), r.Form.Get("code_verifier")
		var sub, redirect, client, challenge string
		err := pool.QueryRow(r.Context(), `UPDATE tasks_test_oauth_codes SET consumed_at=now() WHERE code_hash=$1 AND consumed_at IS NULL AND expires_at>now() AND redirect_uri=$2 AND client_id=$3 AND challenge=$4 RETURNING subject_ref,redirect_uri,client_id,challenge`, hash(code), r.Form.Get("redirect_uri"), r.Form.Get("client_id"), pkceChallenge(verifier)).Scan(&sub, &redirect, &client, &challenge)
		if err != nil {
			writeJSONStatus(w, 400, map[string]string{"error": "invalid_grant"})
			return
		}
		claims := map[string]any{"iss": issuer(), "sub": sub, "aud": client, "iat": time.Now().Unix(), "exp": time.Now().Add(5 * time.Minute).Unix()}
		id := signedJWT(claims)
		writeJSON(w, map[string]any{"access_token": random(32), "token_type": "Bearer", "expires_in": 300, "id_token": id, "scope": "openid profile"})
	}
}
func metadata(w http.ResponseWriter, r *http.Request) {
	base := issuer()
	writeJSON(w, map[string]any{"issuer": base, "authorization_endpoint": base + "/oauth/authorize", "token_endpoint": base + "/oauth/token", "jwks_uri": base + "/.well-known/jwks.json", "response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code"}, "code_challenge_methods_supported": []string{"S256"}})
}
func signedJWT(claims map[string]any) string {
	h := b64([]byte(`{"alg":"HS256","typ":"JWT"}`))
	p, _ := json.Marshal(claims)
	ps := b64(p)
	mac := hmac.New(sha256.New, issuerSigningSecret())
	mac.Write([]byte(h + "." + ps))
	return h + "." + ps + "." + b64(mac.Sum(nil))
}
func pkceChallenge(v string) string {
	h := sha256.Sum256([]byte(v))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
func random(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b64(b)
}
func b64(b []byte) string  { return base64.RawURLEncoding.EncodeToString(b) }
func hash(v string) []byte { h := sha256.Sum256([]byte(v)); return h[:] }
func issuer() string {
	return strings.TrimRight(env("TASKS_TEST_ISSUER_URL", "http://test-issuer:8091"), "/")
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func writeJSON(w http.ResponseWriter, v any) { writeJSONStatus(w, 200, v) }
func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
