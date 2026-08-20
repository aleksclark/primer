package api

import (
	"context"
	"encoding/json"
	"os"

	"primer-tasks/internal/jobs"
	"primer-tasks/internal/repo"
)

// externalSecretResolver is the deployment adapter for an administrator- or
// secret-manager-provided version bundle. TASKS_EXTERNAL_VERIFIER_SECRET_BUNDLE
// is a JSON object keyed by "secretRef:secretVersion"; the single-secret
// variables remain a development/local fallback. Plaintext secrets stay in
// process configuration and never enter task revisions, catalog responses,
// envelopes, clients, or logs.
func externalSecretResolver(env string) jobs.SecretResolver {
	secrets := map[string][]byte{}
	if bundle := os.Getenv("TASKS_EXTERNAL_VERIFIER_SECRET_BUNDLE"); bundle != "" {
		var values map[string]string
		if json.Unmarshal([]byte(bundle), &values) != nil {
			return nil
		}
		for key, value := range values {
			if key != "" && value != "" {
				secrets[key] = []byte(value)
			}
		}
	}
	secret := os.Getenv("TASKS_EXTERNAL_VERIFIER_SECRET")
	if secret == "" && len(secrets) == 0 {
		if env == "production" {
			return nil
		}
		secret = "fixture-development-secret"
	}
	if secret != "" {
		ref := envOr("TASKS_EXTERNAL_VERIFIER_SECRET_REF", "fixture")
		version := envOr("TASKS_EXTERNAL_VERIFIER_SECRET_VERSION", "1")
		secrets[ref+":"+version] = []byte(secret)
		secrets[ref] = []byte(secret)
	}
	if len(secrets) == 0 {
		return nil
	}
	return jobs.StaticSecretResolver(secrets)
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
