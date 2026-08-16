# Curriculum Studio MCP endpoint — design decisions

**Status:** Decision-complete plan (docs only; no production MCP code in this commit)
**Audience:** Platform, contracts, identity, database, and delivery orchestrators
**Must-cite with:** [foundation crosswalk](./curriculum-studio-foundation-crosswalk.md),
[product plan](./primer-curriculum-studio-product-plan.md),
[identity design](./primer-identity-service-design.md),
[platform plan](./curriculum-studio-platform/),
[contracts plan](./curriculum-studio-contracts/),
[delivery roadmap](./curriculum-studio-delivery/)

---

## 1. Outcome

Curriculum Studio exposes an **authenticated Streamable HTTP MCP endpoint** on
the same Studio deployable so external curriculum-planning agents can discover
workspaces/curricula, search standards/resources, edit draft plan graphs,
validate, and propose publish — **on behalf of an authorized user** — without
duplicating REST/gRPC business logic or bypassing Studio tenancy/authz.

---

## 2. Authoritative protocol decisions

| ID | Decision |
| --- | --- |
| M1 | Target MCP specification revision **`2026-07-28`** (Streamable HTTP). Cite official transport/tools/auth sections at implementation time; if the published pin label differs slightly, freeze the exact revision string in the Phase 19/C12 gate notes before mass build. |
| M2 | **Transport:** Streamable HTTP **single endpoint** (`POST` JSON-RPC). Response may be plain JSON **or** request-scoped SSE on the same request. **No** implicit MCP protocol sessions / resumption / session IDs in this revision. Cross-call continuity uses **explicit opaque resource/draft IDs** returned by tools. |
| M3 | **SDK pin:** official Go SDK `github.com/modelcontextprotocol/go-sdk` **`v1.7.0`**. Plan uses `mcp.NewStreamableHTTPHandler` (or successor name) in **stateless** mode as supported by that pin. **Qualification gate:** implementers must confirm the exact constructor/options API against the pinned module before mass build (spike evidence in platform Phase 19 / contracts Phase 12). |
| M4 | **Deployable boundary:** same Studio binary/process. Dedicated route **`/mcp`** (not under REST `/studio/v1`). No third service, no third database. HTTPS in production; loopback HTTP allowed for local/E2E only. |
| M5 | **Capabilities (initial server):** **tools only**. No roots, sampling, or logging capability advertisement (deprecated / out of initial scope for the 2026 revision posture). Long operations may emit **request-scoped progress** over SSE when the SDK/spec supports it on the same request. |
| M6 | **Tool results:** every tool returns `structuredContent` matching a code-defined output schema **plus** a short text fallback for clients that only render text. |
| M7 | **Auth:** per-request `Authorization: Bearer <JWT>` issued by **Primer Identity**. Audience **`curriculum-studio`**. Subject is human (`identity:<uuid>`) or service (`identity:svc:<id>`). Workspace-scoped permissions enforced in Studio on **every** tool call and **every** opaque draft/resource handle. MCP is **never** a token issuer, OP, or refresh endpoint. |
| M8 | **OAuth client flow:** MCP clients that require OAuth protected-resource metadata / authorization-server discovery obtain tokens **through Primer Identity** (resource = Studio MCP URL; `aud=curriculum-studio`). Studio may publish protected-resource metadata that **points at Identity**; Studio does not host authorize/token. |
| M9 | **Tool inventory ownership:** tool names, input/output JSON Schemas, and authorization filters are **code-defined** next to the MCP adapter (contracts Phase 12 + platform Phase 19). They are a **third surface SoT** separate from OpenAPI and protobuf — **no DTO duplication** of domain structs; adapters map tool I/O ↔ existing application services. |
| M10 | **Human-in-the-loop:** draft planning mutations may proceed within granted workspace role/scope. **Publish, share, export-of-published, and destructive** actions require MCP elicitation / `InputRequiredResult` when the pinned SDK+spec support it; otherwise tools return an **actionable error** directing the user to Studio UI confirmation/step-up. **Never silently publish.** |
| M11 | **Security transport controls:** strict Origin allowlist (reject missing/disallowed); body/header protocol-version parity; honor current `_meta` / `MCP-Protocol-Version` (and any required `Mcp-Method` / `Mcp-Name` validation per pin); forbid sensitive custom `x-mcp-header` secret channels; bounded body size, concurrency, timeouts, and rate limits; cancel in-flight work on client disconnect; disable proxy buffering for SSE; generic public errors; redacted audit logs. |
| M12 | **Domain path:** tools call the **same application/domain services** as REST/gRPC. Never raw repositories from the MCP adapter, never cross-DB access, never duplicated business rules. |
| M13 | **Test clients:** official Go SDK client **and** at least one real external Streamable HTTP client (Hermes/mcporter or equivalent). |

---

## 3. Endpoint and package placement (planned)

```text
curriculum-studio/
  cmd/studio-server/          # mounts /mcp alongside /studio/v1 + gRPC
  internal/mcp/               # Streamable HTTP handler, tool registry, authz filter
  internal/mcp/tools/         # thin tool handlers → app services
  # schemas live as Go types / JSON Schema next to tools (not OpenAPI, not proto)
```

| Concern | Path / rule |
| --- | --- |
| Public URL | `https://<studio-host>/mcp` (prod TLS) |
| Local | `http://127.0.0.1:<port>/mcp` |
| REST remains | `/studio/v1/**` |
| gRPC remains | integration service (machine/Primer) |
| MCP does not replace | UI authoring or Primer gRPC materialize |

---

## 4. Authentication and authorization

### 4.1 Token validation (every request)

1. Require `Authorization: Bearer`.
2. Validate JWT locally via Identity JWKS: `iss`, `aud=curriculum-studio`, `exp`/`nbf`, `kid`, signature.
3. Map `sub` → `subject_ref` (`identity:<uuid>` or `identity:svc:<id>`).
4. **Do not** trust role/workspace claims inside the JWT as authorization SoT.
5. Load Studio `workspace_memberships` (and service scope map) for subsequent tool calls.
6. Reject wrong audience, expired, unknown kid, missing membership, and cross-workspace opaque IDs (IDOR → not-found/forbidden with no leak).

### 4.2 Identity responsibilities (not Studio)

| Identity owns | Studio owns |
| --- | --- |
| OP, clients, JWKS, mint/revoke | JWT validate, workspace RBAC |
| Protected-resource registration / AS metadata for MCP clients | Resource metadata document that **references** Identity (if required) |
| Human delegation / service principals | Tool allow-list filtered by membership + scope |
| Token revocation / short TTL | Re-validate on every MCP HTTP request (stateless) |

### 4.3 Suggested scopes (Identity-issued; enforced in Studio)

| Scope (illustrative) | Enables |
| --- | --- |
| `studio:mcp` | Base MCP connect / tools/list |
| `studio:read` | Discovery + search + read plan graph + findings |
| `studio:draft` | Create curriculum/draft, graph patches, validate |
| `studio:publish` | Generate publish proposal; still requires human confirmation path |
| `materialize:write` | Unrelated Primer gRPC path (not initial MCP tools) |

Exact scope strings freeze in Identity client registration + Studio authz map during **I7 / IB3** and the applicable **I12 / IB8** integration chain; **I6 / IB2** supplies the Primer JWT/JWKS bridge and **I8 / IB4** is the signed webhook/two-plane revocation gate. Names above are planning vocabulary.

### 4.4 Opaque handles

Tools never accept raw internal UUIDs from untrusted clients without workspace binding. Prefer already-contracted opaque prefixed IDs (`ws_`, `cur_`, `prev_`, …). Every handle is re-authorized on use.

---

## 5. Initial tool inventory

Authorization-filtered but **deterministic** `tools/list` (stable sort; same principal+memberships → same list).

| Tool name | Class | App service | Notes |
| --- | --- | --- | --- |
| `studio.workspaces.list` | read | workspaces | Names-first |
| `studio.curricula.list` | read | plan | Workspace-scoped |
| `studio.curricula.get` | read | plan | |
| `studio.standards.search` | read | standards | |
| `studio.resources.search` | read | resources | Metadata only |
| `studio.drafts.create` | mutate (draft) | plan | Create curriculum and/or draft revision |
| `studio.drafts.get_graph` | read | plan | Plan graph read |
| `studio.drafts.patch_graph` | mutate (draft) | plan | Optimistic concurrency + idempotency key |
| `studio.drafts.validate` | mutate/read | validation | Persist report via existing path |
| `studio.drafts.get_findings` | read | validation | Coverage/findings |
| `studio.publish.propose` | mutate (gated) | plan/publish | Creates proposal / dry-run; does not publish |
| `studio.publish.confirm` | mutate (step-up) | plan/publish | Requires elicitation or prior UI step-up token; never silent |

**Out of initial inventory:** materialize-for-Primer, webhook admin, destructive workspace delete, cross-tenant admin, roots/sampling.

### 5.1 Human confirmation matrix

| Action | MCP behavior |
| --- | --- |
| List/search/read | Proceed if authorized |
| Draft create / graph patch / validate | Proceed if `author+` (or equivalent) and scopes allow |
| Publish proposal | Allowed; returns proposal id + summary; no immutability flip |
| Publish confirm | Elicitation / `InputRequiredResult` **or** error `publish_confirmation_required` with Studio UI deep link |
| Share / export published / destructive | Same step-up class as publish |

---

## 6. Transport security checklist (implementation must prove)

- [ ] Origin allowlist configured; disallowed/missing Origin rejected (browser-like clients)
- [ ] `MCP-Protocol-Version` (and body `_meta` version if present) match supported set
- [ ] Header/body method-name parity checks per pin (`Mcp-Method` / `Mcp-Name` when required)
- [ ] No secret transport via custom `x-mcp-*` headers
- [ ] Max body bytes; max concurrent MCP requests per subject/IP; request timeouts
- [ ] Context cancellation on client disconnect aborts domain work where safe
- [ ] SSE: `X-Accel-Buffering: no` / equivalent; flush policy documented
- [ ] Public errors generic; audit_events + logs redact tokens/PII
- [ ] Rate limit returns protocol-safe error without stack traces

---

## 7. Persistence mapping (no new DB unless required)

| MCP concern | Existing Studio tables / phases |
| --- | --- |
| Authz | `workspace_memberships`, tenants/workspaces (D3 / S2–S3) |
| Draft/graph/publish | plan graph + publish immutability (D5 / S6–S8) |
| Validation findings | validation reports (D6 / S7) |
| Idempotent tool mutations | `idempotency_keys` (D11) with scope e.g. `mcp:<tool>` |
| Optimistic concurrency | existing revision/graph version columns/ETags (D5 / S6) |
| Audit | `audit_events` (D12) — record tool name, subject_ref, workspace_id, opaque ids, outcome |
| Publish step-up evidence | Prefer existing publish audit + optional short-lived confirmation record **only if** UI/MCP cannot share idempotency row; if additive table needed, **database track** owns `0000N` migration (label ownership explicitly) |

**Default:** no new migration for MCP. Additive migration only if confirmation/nonce storage cannot fit `idempotency_keys` + `audit_events`.

---

## 8. Contract surface rules

| Surface | SoT | MCP relationship |
| --- | --- | --- |
| OpenAPI `/studio/v1` | Huma / hand baseline | Unchanged; MCP is not REST |
| Protobuf integration | `.proto` | Unchanged; Primer materialize stays gRPC |
| MCP tools | Pinned MCP spec + code-defined tool schemas | **Third surface**; no mirroring OpenAPI DTOs or proto messages as MCP schemas |

Contracts Phase 12 owns: schema lint/conformance harness, tool inventory freeze, official client + external client matrix, negative protocol tests.
Platform Phase 19 owns: handler mount, authz filter, tool → app service wiring, security middleware, E2E against real Studio domain.

---

## 9. Testing matrix (must pass before claiming MCP done)

| ID | Case |
| --- | --- |
| MCP-T1 | Official Go SDK client: initialize + tools/list + read tool |
| MCP-T2 | External Streamable HTTP client (Hermes/mcporter): same |
| MCP-T3 | Disconnect/cancel mid long tool (progress/SSE) |
| MCP-T4 | Protocol version / header mismatch → safe error |
| MCP-T5 | Origin attack (disallowed Origin) rejected |
| MCP-T6 | IDOR on draft handle across workspaces |
| MCP-T7 | Wrong `aud` JWT rejected |
| MCP-T8 | Idempotent replay of patch_graph |
| MCP-T9 | Concurrent patch → conflict (optimistic concurrency) |
| MCP-T10 | Publish confirm without elicitation/step-up fails closed |
| MCP-T11 | tools/list deterministic + authz-filtered |
| MCP-T12 | Audit row present; secrets absent from logs |

---

## 10. Official references (implementers)

- MCP specification (pinned revision **2026-07-28**): Streamable HTTP transport; tools; authorization / protected resource patterns as published for that revision.
- Go SDK: `github.com/modelcontextprotocol/go-sdk` **v1.7.0** — qualify `NewStreamableHTTPHandler` / stateless options before mass build.
- Primer Identity design: single-audience JWT, JWKS, BFF/OP, client_credentials, revocation.
- Foundation crosswalk L1–L4: one Studio deployable + DB; Identity authenticates; Studio authorizes.

---

## 11. Explicit non-goals

- MCP as a second product deployable or database
- Studio-minted MCP API keys or long-lived static tokens
- Implicit session resume / multi-node sticky MCP sessions in this revision
- Replacing Studio UI for final publish governance
- Duplicating OpenAPI or protobuf as MCP DTO catalogs
- Cross-DB reads of LMS/Identity
- Silent publish from an agent

---

## 12. Traceability

| Artifact | Update |
| --- | --- |
| Product plan | MCP ownership + workflow + roadmap note |
| Foundation crosswalk | Flow row + contract surface row + L-decision |
| Platform | Phase 19 |
| Contracts | Phase 12 |
| Identity design + phase 12 | Protected resource / MCP client notes |
| Database index | MCP mapping section (no phase renumber) |
| Delivery | Waves `S19` / `C12` / `X7` (and supporting deps) |
| LikeC4 | Agent actor, `mcp_adapter` component, `studio_mcp` view |

**Identity dependency rule:** credential-free test Identity/JWKS may prove local transport only. Delegated MCP writes and delivery X7 require **I6 / IB2 + I8 / IB4**; require **I7 / IB3 + applicable I12 / IB8** when Identity client registration, BFF mediation, or publish confirmation is in scope. MCP accepts Primer JWTs only and re-authorizes every request against local Studio membership; it rejects raw Stytch bearer/SessionJWT material.


## Stytch-backed identity reconciliation (authoritative)

Stytch B2B is upstream human authentication/session authority. Primer Identity is its sole SDK/API client and downstream Primer token broker: it validates opaque Stytch sessions, maps exact `(project_id, organization_id, member_id)` to a local account, and issues only short-lived single-audience Primer JWTs/JWKS. Studio, LMS, TV, MCP, product APIs, and browser JS never receive, store, forward, log, or validate a Stytch session token, SessionJWT, tuple, or role.

Stytch organization/member roles are eligibility hints only. Studio workspace membership, LMS educator roles, and TV device authentication remain local systems of record; a Stytch organization does not create a Studio tenant/workspace. Provisioning is explicit invite/admin only, and distinct cross-org tuples stay distinct personas without email merge/linking. IA is library-only and unapproved; production auth waits for IA-R and IB1–IB4, with signed webhook/cache+grant revocation as a hard BFF/MCP gate.

Required E2Es: mapped tuple to local membership; valid no-membership token denied; same-email cross-org isolation; Stytch admin-like role denied without local role; raw Stytch bearer rejected; outage fails closed/no negative cache; signed webhook forgery/replay/dedupe/out-of-order; explicit revocation bound; LMS local-role dual run; and no token/provider payload in audit logs.
