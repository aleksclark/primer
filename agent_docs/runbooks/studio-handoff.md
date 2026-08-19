# Curriculum Studio → primer-agents Handoff

## Status: Deferred

This document describes the integration path for Studio to call primer-agents
**when Studio is ready**. Phase 7 does not implement Studio S19, any `/mcp`
endpoint, or any Stytch BFF change. The handoff is contractual only.

## What is available

| Artifact | Location | Purpose |
|----------|----------|---------|
| Generated Go client | `primer-agents/client/go/` | Go façade over generated types |
| Generated TypeScript types | `primer-agents/client/ts/types.gen.ts` | openapi-typescript types |
| TypeScript client facade | `primer-agents/client/ts/client.ts` | `AgentsClient` with SSE helper |
| OpenAPI spec (OAS 3.1) | `primer-agents/openapi.yaml` | Authoritative contract |

## Authentication

- Audience: `aud=primer-agents`
- Issuer: Primer Identity (`IDENTITY_ISSUER`)
- Token type: short-lived ES256 access JWT, no raw Stytch tokens
- Required scope for run/session creation: `agents:runs:write agents:sessions:write`
- Studio must obtain an Identity service credential with the above audience/scopes.
  **This credential does not exist yet** and must be reviewed before Studio calls
  the service in production.

## Opaque caller context

Studio passes an opaque `callerContext` string when creating sessions. primer-agents
stores it under the signed owner namespace without querying Studio's database.

```go
sess, err := agentsClient.CreateSession(ctx, agentsclient.CreateSessionJSONRequestBody{
    Profile:       "admin",           // server-verified; Studio cannot supply budget fields
    CallerContext: &studioWorkspaceID, // opaque; primer-agents never reads LMS/Studio DB
})
```

## What Studio must NOT do

- Pass raw Stytch tokens or SessionJWTs.
- Construct handwritten HTTP calls to `/agents/v1/`. Use the generated client only.
- Implement Studio S19 or `/mcp` through this service (deferred to a later phase).
- Share credentials with the LMS BFF or workstation callers.

## Integration checklist (when Studio is ready)

- [ ] Identity client grant reviewed and issued for `aud=primer-agents`
- [ ] Studio imports `primer-agents/client/go` or `client/ts` (no handwritten DTOs)
- [ ] Studio local authorization happens before `CreateSession`/`CreateRun`
- [ ] Import gate passes: no raw fetch / duplicate DTO scan
- [ ] E2E: Studio → agents service with real JWKS and separate DB
