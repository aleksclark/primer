# cmd/studio-server

Curriculum Studio process binary entrypoint (platform S1).

```bash
export STUDIO_DATABASE_URL='postgres://studio:***@127.0.0.1:5432/curriculum_studio?sslmode=disable'
go run ./cmd/studio-server
# or: make -C .. studio-build && ../bin/studio-server
```

## Behavior (S1)

- Loads `STUDIO_*` config only (no bare `DATABASE_URL` fallback)
- Validates Studio DB isolation (refuses LMS/TV/Identity DB names)
- Applies embedded goose migrations to Studio DB (`studio_goose_db_version`)
- Serves Huma+chi under `/studio/v1` (health, ready) + `/metrics`
- Structured JSON logs with DSN/secret redaction
- Graceful SIGINT/SIGTERM shutdown

## Related

- Reserved hook path `cmd/studio-api/` remains for contracts/OpenAPI gen wiring.
- Domain routes, authz, SPA land in later S* waves.
