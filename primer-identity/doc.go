// Package identity is the root of the Primer Identity Go module.
//
// Module path (frozen): github.com/aleksclark/primer/identity
// Physical root: primer-identity/
//
// I1 delivers the service shell: cmd/identity-server, IDENTITY_ config,
// embedded goose migrations (identity_goose_db_version), health/ready, and an
// isolated Postgres test harness. OAuth/OIDC, accounts, JWKS, and sessions
// land in later I* waves.
package identity
