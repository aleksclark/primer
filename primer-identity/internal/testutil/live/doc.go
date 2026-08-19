// Package live holds opt-in live-provider qualification tests.
//
// These tests are excluded from the default suite (//go:build live_stytch).
// They call the real Stytch test API through the production adapter boundary
// and exercise the signed webhook verification boundary using the pinned
// Svix library. They do not implement the full IB8-E10 browser + webhook
// drill and must never be labelled as that gate.
//
// Coverage areas:
//   - Adapter session authentication (project-id/secret credential proof)
//   - Adapter revalidation (token-free provider query)
//   - Broker construction and StartLogin (email OTP, magic-link, SSO URLs)
//   - Svix webhook signature verification (valid, tampered, wrong secret,
//     stale timestamp, missing headers, project mismatch)
//   - Secret redaction in String/Format paths
//
// Enable with:
//
//	make identity-live-stytch
//
// Credentials are loaded only from IDENTITY_STYTCH_* (optionally via
// IDENTITY_LIVE_STYTCH_ENV_FILE). Secrets are never logged.
//
// Optional env vars for extended coverage:
//   - IDENTITY_LIVE_STYTCH_SESSION_TOKEN: exercises full session authenticate
//     + revalidate happy path
//   - IDENTITY_LIVE_STYTCH_WEBHOOK_SECRET: uses the real Stytch Dashboard
//     webhook secret instead of a deterministic test secret
package live
