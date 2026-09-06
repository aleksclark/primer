# Export artifact storage (S13)

PostgreSQL stores export metadata, `obj:exports/<workspace>/<export>.<ext>`, and
SHA-256 checksums, never rendered bytes. Both backends use the same relative
keys. Each artifact has a `<key>.manifest.json` sidecar containing export ID,
plan revision ID, optional materialization run ID, `created_by` subject ref,
creation timestamp, format, MIME type, and checksum.

## Configuration

| Environment variable | Default / meaning |
|---|---|
| `STUDIO_ARTIFACT_STORE` | `fs`; choices `fs`, `s3` |
| `STUDIO_ARTIFACT_STORE_DIR` | `./studio-artifacts`; FS root (use a persistent volume in deployments) |
| `STUDIO_ARTIFACT_S3_ENDPOINT` | Required for S3, e.g. `https://s3.us-east-1.amazonaws.com` or local MinIO URL |
| `STUDIO_ARTIFACT_S3_BUCKET` | Required, private pre-provisioned bucket |
| `STUDIO_ARTIFACT_S3_REGION` | `us-east-1` |
| `STUDIO_ARTIFACT_S3_ACCESS_KEY` / `STUDIO_ARTIFACT_S3_SECRET_KEY` | Optional pair; absent uses the MinIO client's IAM credential provider |

Production S3 endpoints require HTTPS. The service does not create buckets or
public policies. Back up the artifact root/bucket **and** the Studio database;
changing backends requires copying objects with unchanged keys. Never expose
an FS root through a static/public HTTP mount.

## API and formats

- `POST /studio/v1/revisions/{revisionId}/exports`: author/admin/owner only.
  Accepts `markdown`, `pdf`, `docx`, `csv_coverage`, `json_bundle`, `ical`.
  The last two descriptive wire names map to DB formats `csv` and `json`.
- Optional `materializationId` must identify a ready run of that same revision
  and workspace. Its items are included in Markdown, PDF, DOCX, and JSON.
- `GET /studio/v1/exports/{exportId}` returns status, references, checksum,
  actor, source revision/run, and download/manifest routes when ready.
- `GET .../{exportId}/download` and `GET .../{exportId}/manifest` require active
  workspace membership on **every request** (including viewers). They read
  bytes through Store.Get, not a public URL. Downloads verify the checksum.
- FS SignURL returns `ErrSigningUnsupported`, never a raw filesystem path.
  S3 SignURL creates a bounded bearer-capability URL; API downloads deliberately
  proxy bytes instead so membership revocation takes effect immediately.

CSV is a fresh, persisted deterministic validation report, with report identity,
status, findings, and spreadsheet formula protection. JSON is explicitly a
**planning download subset**, not the Primer protobuf MaterializationBundle or
SessionSpec contract; Primer integration belongs to CurriculumIntegrationService.
DOCX is generated with godocx. PDF retains the basic Helvetica renderer.
iCal emits VEVENTs only for `calendar_window`, `testing_window`, and `blackout`
constraints with RFC3339 `start`/`end` and optional `title` payload fields.
It converts times to UTC, escapes/folds lines, and rejects invalid dated windows.
Undated policies do not become invented appointments; an empty calendar is valid.

Creation is synchronous: both artifact and manifest Put must succeed before
`ready`. A render/store failure persists `failed`, returns HTTP 503 with the
export ID, exposes a safe error message on metadata GET, and logs the cause.
Object cleanup is best-effort, with errors retained in the logged cause. A
process crash or ambiguous DB completion error may leave a requested row/orphan
object; no cross-DB/object-store transaction or background reconciliation is
claimed. Export POST does not yet implement idempotency-key replay.

## Verification

```sh
GOWORK=off go test ./internal/artifacts -count=1   # FS + real MinIO testcontainer
GOWORK=off go test ./internal/export -run 'Formats|Provenance|StoreFail' -count=1
```

The HTTP tests use real PostgreSQL, signed JWTs, and FS storage. The MinIO test
is unconditional (no build tag or Docker-unavailable skip), verifies persistence
with a new client, signed/unsigned access, MIME type, deletion, and bucket failure.
Generated clients come from Huma emission; immutable contract baselines and
protobuf source shapes are unchanged. The Go client normalizer lowers the
OpenAPI 3.1 binary `contentMediaType` annotation into its existing 3.0 binary form.
