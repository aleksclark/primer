package api

import (
	"context"
	"os"
	"primer-tasks/internal/jobs"
	"primer-tasks/internal/repo"
)

// externalSecretResolver is deliberately a development/test adapter. A
// production deployment must replace it with a SecretResolver backed by its
// secret manager; task revisions and catalog responses contain only refs.
func externalSecretResolver(env string) jobs.SecretResolver {
	secret := os.Getenv("TASKS_EXTERNAL_VERIFIER_SECRET")
	if secret == "" {
		if env == "production" {
			return nil
		}
		secret = "fixture-development-secret"
	}
	ref := envOr("TASKS_EXTERNAL_VERIFIER_SECRET_REF", "fixture")
	version := envOr("TASKS_EXTERNAL_VERIFIER_SECRET_VERSION", "1")
	return jobs.StaticSecretResolver{ref + ":" + version: []byte(secret), ref: []byte(secret)}
}

// StartExternalWorker owns delivery outside the HTTP request lifetime. The
// worker uses Postgres leases and therefore may be started by every Tasks
// process in a multi-worker deployment.
func (s *Server) StartExternalWorker(ctx context.Context) {
	if s.ExternalSecrets == nil {
		return
	}
	worker := jobs.NewExternalWorker(repo.NewExternalRepository(s.DB), repo.NewVerifierCatalogRepository(s.DB), s.ExternalSecrets)
	go worker.Run(ctx)
}
