// Package live contains opt-in live operational proof tests for the TV admin
// identity boundary. These tests exercise real HTTP round trips with JWKS
// endpoints and write sanitized evidence to /var/tmp/primer-ib8-tv-admin-*.
//
// Run with:
//
//	TV_LIVE_IDENTITY_PROOF=1 go test -tags live_stytch -run TestLiveTVIdentityProof -v ./server/internal/tv/api/live/ -count=1
package live
