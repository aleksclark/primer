# cmd/openapi-gen

Offline OpenAPI emitter for Curriculum Studio authoring REST.

**Wave ownership:** Contracts C6 fills this binary. C1 only reserves the path.

## Purpose

- Load Huma handler registrations without listening on a port
- Emit OpenAPI 3.1 to a build-only path (never committed as live SoT)
- After C7 handoff, emitted OpenAPI becomes the live authoring contract;
  `contracts/openapi/v1/curriculum-studio.yaml` remains the immutable baseline

## Non-goals (C1)

- No handler implementation
- Does not write LMS `web/openapi.yaml` or `tv-web/openapi.yaml`
