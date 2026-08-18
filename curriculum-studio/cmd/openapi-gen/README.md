# cmd/openapi-gen

Offline OpenAPI emitter for Curriculum Studio authoring REST.

**Wave ownership:** Contracts C6 owns this binary and the shared Huma
registration path. C1 only reserved the path.

Run it from `curriculum-studio/` with:

```bash
go run ./cmd/openapi-gen -out contracts/.tmp/openapi.emitted.yaml
# or: make contracts-openapi-emit
```

The command constructs `internal/api.NewWithPinger(nil, ...)`; it never binds a
listener or opens a database connection. The output path is build-only and
ignored by git.

## Purpose

- Load Huma handler registrations without listening on a port
- Emit OpenAPI 3.1 to a build-only path (never committed as live SoT)
- After C7 handoff, emitted OpenAPI becomes the live authoring contract;
  `contracts/openapi/v1/curriculum-studio.yaml` remains the immutable baseline

## Non-goals (C1)

- No handler implementation
- Does not write LMS `web/openapi.yaml` or `tv-web/openapi.yaml`
