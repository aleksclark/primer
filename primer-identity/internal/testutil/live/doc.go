// Package live holds opt-in live-provider qualification tests.
//
// These tests are excluded from the default suite (//go:build live_stytch).
// They call the real Stytch test API through the production adapter boundary
// only. They do not implement the full IB8-E10 browser + webhook drill and
// must never be labelled as that gate.
//
// Enable with:
//
//	make identity-live-stytch
//
// Credentials are loaded only from IDENTITY_STYTCH_* (optionally via
// IDENTITY_LIVE_STYTCH_ENV_FILE). Secrets are never logged.
package live
