# Codebase Improvement Opportunities

Review of the LMS server (`server/`) and admin SPA (`web/`) focused on DRY,
separation of concerns, and intuitive naming. Items marked **done** were
implemented on the `dry-cleanup` branch; the rest remain for follow-up.

## Done on this branch

### 1. Derive repository column lists from domain tags
**Was:** every resource listed columns twice — once as `db` tags on the domain
struct, again as a hand-maintained `Columns` slice in `repo/resources.go`.
**Now:** `NewResource` reflects `db` tags from `T` when `Columns` is empty, so
the domain struct is the single source of truth for selected columns.

### 2. Factory association helper
**Was:** every factory with foreign keys repeated
`if _, ok := merged["x_id"]; !ok { merged["x_id"] = Other(...).ID }`.
**Now:** `ensureFK` centralizes that pattern.

### 3. Admin column builders
**Was:** each resource page hand-wrote nearly identical column defs (id chip,
badge, short UUID, date, plain text) with mixed sort-key conventions.
**Now:** shared builders in `web/src/lib/columns.tsx` (`idCol`, `textCol`,
`badgeCol`, `codeCol`, `shortIdCol`, `dateCol`, `createdCol`, `gradeCol`) so
pages declare intent, not markup. Sort keys always use the DB column name.

### 4. API client owns mutations
**Was:** `mutate` lived next to the `useList` React hook, mixing transport with
component state.
**Now:** `mutate` (and shared error extraction) live in `web/src/api/client.ts`;
the hook only manages list state.

### 5. Small cleanups
- `slices.Contains` for sortable/filterable checks in `repo/list.go`
- `maps.Copy` in factory merge
- Removed dead `fieldDefsSanity` export
- `go mod tidy` so test deps are direct

## Remaining opportunities

### High impact

#### A. Shared enums across layers
Enum strings (`role`, `approach`, `item_type`, mastery `status`, …) are
duplicated in:
- domain struct tags (`enum:"…"`)
- create/update API bodies
- admin select `options` arrays
- SQL comments / CHECKs (where present)

**Direction:** define Go `const` blocks (or a small `domain/enums.go`) and
generate OpenAPI enums from them. Frontend options should come from the
generated OpenAPI schema (`components.schemas.*` enum members) rather than
hand-typed arrays.

#### B. Create/Update body duplication
`api/resources.go` mirrors almost every domain field twice more (create value
types, update pointer types) with the same validation tags. Adding a field
requires three edits.

**Direction (pick one):**
1. Codegen create/update structs from domain + a small annotation set
   (`create:"required"`, `update:"-"` for immutable FKs).
2. Use a single partial-update body of pointers for both create and update,
   validating required fields in a resource-specific hook.
3. Keep hand-written bodies but share embedded field groups
   (`type namedFields struct { Name string … }`) to cut repetition.

#### C. Resource registration catalog
`registerAll` repeats the same `RegisterCRUD[T, C, U](…, singular, plural, path)`
shape fourteen times. A typed catalog (or one `ResourceSpec[T,C,U]` value per
resource) would make “add a resource” a single table row and keep API path
naming consistent with the SPA routes.

#### D. Frontend form fields from OpenAPI
Field defs in `pages/resources.tsx` restate labels, required flags, and select
options already present in the OpenAPI schema. A thin helper that builds
`FieldDef[]` from a schema component (with label overrides) would keep the
admin UI in lockstep with the API.

### Medium impact

#### E. `standard_prerequisites` has no API surface
The join table exists in the migration but has no domain type, repo, CRUD, or
admin page. Wire it through the same generic stack when prerequisite editing
is needed.

#### F. Naming consistency: evidence plural
API path is `/mastery-evidence` while the operation plural is
`mastery-evidences` and the table is `mastery_evidence`. Prefer one plural
form end-to-end (`mastery-evidence` uncountable, or `mastery-evidences`
everywhere) and document the convention for multi-word resources.

#### G. Sort/filter column names in the SPA
List query `sort` / `filter` use **database** column names (`first_name`),
while JSON bodies use **camelCase** (`firstName`). The admin builders now keep
sort keys on the DB form, but this dual vocabulary is easy to get wrong.
Options: accept camelCase in the list API (map to columns server-side), or
document the dual scheme in the OpenAPI descriptions more prominently.

#### H. `useList` still uses raw `fetch`
`openapi-fetch` client is constructed but list/mutate bypass it. Routing both
through the typed client would catch path/body drift at compile time.

### Lower impact / polish

#### I. Extract CORS middleware
`corsMiddleware` in `api/api.go` is fine for now; if more HTTP middleware
appears, move to `internal/httpx` or similar.

#### J. Page envelope duplication
`repo.Page[T]` and `api.PageBody[T]` are structurally identical. Sharing one
type (or a thin alias) would remove a mapping step in `RegisterCRUD`.

#### K. Test helper for list page decode
Integration tests repeatedly decode the same `{ items, totalCount, limit,
offset }` shape. A `decodePage[T](t, body)` helper would shrink `api_test.go`.

#### L. Domain stringly-typed statuses
Statuses and kinds are plain `string` with enum tags. Branded string types
(`type MasteryStatus string`) improve call-site clarity without fighting the
DB layer.

## Suggested priority

1. **A** (shared enums) — prevents silent drift between API and UI.
2. **B** or **C** — reduces the cost of every new field/resource.
3. **D** + **H** — keep the SPA honest against the OpenAPI contract.
4. **E** when product needs prerequisite editing.
5. **F/G/J/K/L** as polish when touching those areas.
