# Implementation Plan: Video As Instruction

Turns the [Virtual TV Channel](../tv-channel.md) concept into a concrete
system: an Android/Android TV client, a content-access backend ("TV server"),
a Jellyfin media source, and Primer integration for curriculum-driven
scheduling and instructional-hours reporting.

```
┌──────────────┐   pull metadata,    ┌──────────────┐
│   Jellyfin    │◄──stream media─────│  Android app  │
│ (media+meta)  │    (direct play)   │ (tablet + TV) │
└──────┬───────┘                     └──────┬───────┘
       │ admin API token                    │ device API
       ▼                                    ▼
┌─────────────────────────────────────────────────────┐
│                     TV SERVER (Go)                   │
│  catalog · availability · watch-once ledger ·        │
│  schedule/EPG · play grants · heartbeats · metrics   │
└──────┬───────────────────────────────────┬──────────┘
       │ admin SPA (React/shadcn)           │ push instructional hours
       ▼                                    ▼
   Parent admin                    ┌──────────────┐
   Primer (schedule mgmt) ────────►│  Primer LMS   │
                                   └──────────────┘
```

## Guiding decisions

| # | Decision | Choice | Rationale |
|---|----------|--------|-----------|
| 1 | Service topology | New `tv-server` binary in the existing Go module (`server/cmd/tv-server`, `server/internal/tv/...`), **separate PostgreSQL database** | Reuses the proven Huma/chi/pgx + generic CRUD + testcontainer stack (`internal/repo`, `internal/api`) without coupling LMS and TV schemas |
| 2 | Media delivery | App streams **directly from Jellyfin** (direct play URLs); TV server issues single-use, short-lived **play grants** and never proxies bytes | RK3318 box + LAN: avoid doubling traffic through Go; access control lives at grant issuance; Jellyfin credentials never reach the client |
| 3 | Client platform | Single Kotlin app, Jetpack Compose + Compose for TV, Media3/ExoPlayer, `minSdk 28` | One codebase, two form factors (touch tablet / D-pad leanback); Android 9 box is the floor |
| 4 | Schedule authority | Server-authoritative clock and schedule; client asks "what's on now + offset" and seeks | No client-side trust for a no-skip channel; box RTCs drift |
| 5 | Primer integration | TV server **pushes** watch sessions to a new Primer ingest endpoint; Primer **calls** the TV admin API to manage schedules/availability. One shared secret per direction: `SERVICE_TOKEN` for TV→LMS, `TV_ADMIN_API_KEY` for LMS→TV | Matches guidelines 9–10; each service owns its own data, and a credential that only ever travels one way cannot be replayed the other |
| 6 | Admin SPA | `tv-web/` — Vite + React + shadcn + openapi-typescript client, same build pipeline as `web/` | Guideline 8; copy the existing `ResourcePage`/column-builder patterns |
| 7 | Content acquisition | Out of scope. TV server only sees what Jellyfin already has | Guideline 11 |
| 8 | Schedule timezone | Entries stay `timestamptz` (instants); the **server** buckets them into calendar days in one configured IANA zone (`TV_CHANNEL_TIMEZONE`, default `America/Chicago`) | "Tuesday morning" is a household fact, not a UTC or per-client one. Bucketing in UTC would split a Tennessee evening across two days; bucketing per client would give the tablet and the box different EPGs. Day boundaries are computed by adding a calendar day, so DST does not shift them |
| 9 | Schedule conflicts | Rejected by an application-level guard inside the INSERT/UPDATE's own `WHERE`, not a PostgreSQL `EXCLUDE` constraint | A slot's end is `airs_at + media_items.runtime_seconds` — a column in *another* table that the Jellyfin sync rewrites. An exclusion constraint would need a denormalised runtime kept in step by triggers: a large, drift-prone mechanism for a grid one parent edits. The conditional write still decides the common case in one statement against one snapshot |

**Known limitation (accepted):** under `READ COMMITTED`, two *simultaneous*
inserts can each pass the overlap guard without seeing the other's uncommitted
row, so a concurrent double-write could still produce an overlap. One parent
edits this grid, so the window is theoretical; the channel degrades gracefully
rather than breaking, because `AiringAt` orders and takes a single row. Closing
it properly means a denormalised `ends_at` plus an `EXCLUDE` constraint — see
row 9. Revisit if scheduling is ever automated or multi-user.

## Hardware notes (T9 / RK3318 box)

- Quad A53, 4GB RAM, Android 9 (API 28), arm64 + armeabi-v7a. Ship both ABIs.
- Hardware decode: H.264, H.265/HEVC up to 4K. **Direct play only** — never
  trigger Jellyfin transcoding (CPU-poor NAS + box can't handle it).
  Pre-validate library encodings; flag incompatible items in the admin UI.
- Typically no Google Play — sideload via ADB. Plan an in-app self-update
  check against the TV server (phase 5).
- Leanback UI required: D-pad focus navigation, no touch assumptions, 10-foot
  typography, TV overscan-safe margins.

## Data model (tv-server database)

| Table | Purpose | Key columns |
|-------|---------|-------------|
| `devices` | Paired clients | name, kind (`tablet`,`tv_box`), pairing token hash, last_seen_at |
| `media_items` | Curated subset of Jellyfin library | jellyfin_item_id (unique), title/duration/image cache, `class` (`educational`,`entertainment`,`mixed`), subject_tags[], standard_codes[], quality_notes, direct_play_ok |
| `availability_windows` | On-demand rotation | media_item_id, starts_at, ends_at, max_plays (null = unlimited) |
| `play_ledger` | Watch-once enforcement | media_item_id, device_id, availability_window_id, consumed_at |
| `schedule_entries` | Programmed channel grid | media_item_id, airs_at, join_in_progress (bool), block label (`morning`,`midday`,`afternoon`,`evening`) |
| `play_grants` | Single-use playback authorizations | media_item_id, device_id, mode (`on_demand`,`programmed`), stream URL params, issued_at, expires_at, consumed_at |
| `playback_sessions` | Metrics + reporting source | grant_id, started_at, ended_at, watched_seconds, max_position_seconds, completed |
| `primer_reports` | Idempotent export log | playback_session_id (unique), reported_at, primer_ref (the LMS log ID) |

Rules encoded server-side:
- **Entertainment = one play** per item per availability window (ledger row
  consumes it on session completion or >80% watched).
- **Rotation**: on-demand catalog = media items with an active availability
  window and unconsumed ledger allowance. Windows are created by parent/Primer.
- **Programmed mode**: `GET /now` resolves the current schedule entry and
  offset; grants for programmed mode carry the required start offset. Missed
  slot = missed (no catch-up grants).

## API surface (tv-server, Huma-generated OpenAPI)

**Device API** (paired-device token auth):
- `POST /devices/pair` — exchange pairing code (shown in admin) for token
- `GET /catalog` — on-demand items currently available to this device
- `GET /schedule?day=` — EPG for one calendar day in the channel timezone;
  `GET /now` — current program + offset, or the gap and what follows it
- `POST /media/{id}/grant?mode=` — request play grant → `{streamUrl, startOffset, mode, grantId}`
  or 403. `mode=programmed` requires the item to be airing right now
- `POST /grants/{id}/heartbeat` — position/state every ~30s
- `POST /grants/{id}/complete`

**Admin API** (session or API-key auth; consumed by SPA and by Primer):
- CRUD via `RegisterCRUD`: media items, availability windows, schedule
  entries (create/update hand-written for overlap rejection), devices
- `GET /schedule-grid?from=&days=` — resolved grid for the week editor;
  `POST /schedule-entries/copy-week` — re-air a week, reporting what collided
- `POST /jellyfin/sync` — refresh metadata cache; `GET /jellyfin/browse` —
  pick unimported library items
- `GET /metrics/*` — watch time by class/subject/day, completion rates
- Image proxy: `GET /images/{mediaItemId}/{type}` (covers via Jellyfin, so
  the client needs no Jellyfin access for artwork)

**Primer side** (new LMS endpoints):
- `POST /instruction-logs/ingest` — `{source: "tv", sourceRef, mediaTitle,
  subjectTags, standardCodes, watchedSeconds, occurredOn, class}`. Guarded by
  the LMS service token (`X-Service-Token`, env `SERVICE_TOKEN`); the guard is
  inert when unset so spec generation and a bare checkout still work.
  `class` is `educational|mixed` only — the enum refuses entertainment with 422.
  Idempotent on `(source, sourceRef)`: 201 on first sight, 200 with
  `created: false` on a replay. `occurredOn` is a bare `YYYY-MM-DD` because the
  household day is the producer's to decide, not the LMS's.
- `RegisterCRUD` at `/instruction-logs` — the parent's own list/create/edit/
  delete surface, on the LMS's usual unauthenticated footing. Ingest is
  deliberately a separate path: it is the only machine caller, so it can carry
  a credential in deployment without locking the parent out of the admin SPA.
- Later joins mastery evidence via standard codes.

### Primer → TV: managing what is on

The TV admin API is the whole interface; it needs no new endpoints. Primer's
overseer and tutor agents present the same admin key the SPA does
(`X-Admin-Key`, env `TV_ADMIN_API_KEY`), which keeps one credential per
direction rather than a third scheme:

| Intent | Call |
|--------|------|
| Read what is on today / this week | `GET /schedule-grid?from=YYYY-MM-DD&days=1` (or `days=7`) |
| Find taggable content for a unit | `GET /media-items?q=inertia` or `?filter=class:educational` |
| Place a curriculum-driven window | `POST /availability-windows {mediaItemId, startsAt, endsAt, note}` |
| Retire a window early | `PATCH /availability-windows/{id} {endsAt}` |
| Programme a slot on the channel | `POST /schedule-entries {mediaItemId, airsAt}` (409 on overlap) |
| Re-air a week | `POST /schedule-entries/copy-week` |
| See what was counted as instruction | `GET /primer-reports`, `POST /primer-reports/run` |

Worked example — force-laws week: the science tutor searches `/media-items` for
inertia material, then `POST /availability-windows` for each hit with the week's
`startsAt`/`endsAt` and a `note` naming the unit. The items appear in the
student's on-demand catalog for that week only; watching one comes back to
Primer as instructional time under the media item's `standardCodes`.

Note the deliberate asymmetry: `GET /now` and `GET /schedule?day=` carry
*device* tokens, not the admin key, because they are the client's tuning
interface. Primer reads the same grid through `/schedule-grid`, which is the
admin projection of it and needs no device identity.

## Android app

- **Stack**: Kotlin, Compose (Material3 + `tv-material` for leanback),
  Media3/ExoPlayer, Retrofit/Ktor client generated from the OpenAPI spec,
  DataStore for device token.
- **Form-factor switch**: `UiModeManager.currentModeType == TELEVISION` picks
  the leanback shell; shared domain/player modules underneath.
- **Screens**:
  - Pairing (enter/display code)
  - Home: "On now" hero (programmed) + on-demand rails by category; watched
    entertainment items shown greyed/"already watched"
  - EPG (today/tomorrow grid)
  - Player — programmed mode: no seek bar, no ff/rw, back = exit only,
    auto-join at server offset; on-demand educational: pause/seek allowed;
    on-demand entertainment: pause allowed, completion consumes the play
  - Settings (device info, server URL, update check)
- **Playback discipline**: heartbeats every 30s; on crash/network loss,
  resume from grant if unexpired; programmed mode re-syncs offset from `/now`
  on every resume.
- **Kiosk-ish TV box**: set as HOME/launcher intent handler on the box so the
  channel is the default experience (phase 5; Android 9 allows this without
  device-owner setup).

## Admin SPA (`tv-web/`)

Clone the `web/` scaffolding (Vite/React/shadcn/openapi-typescript, shared
column-builder patterns):
1. **Library** — Jellyfin browse/import, classify (class, subject tags,
   standard codes), direct-play validation status
2. **Availability** — rotation calendar for on-demand windows; "expire &
   rotate" bulk actions
3. **Schedule** — week grid editor for the programmed channel; conflict
   detection; copy-week
4. **Devices** — pairing codes, rename, revoke
5. **Metrics** — watch time by class/subject, entertainment plays used
6. **Primer Reports** — the instructional-hours export ledger, with a manual
   "report now" pass and a delete that re-queues a viewing

## Phases

### Phase 1 — TV server core (Go) ✅
- Scaffold `server/internal/tv/` (domain, repo configs, api) + `cmd/tv-server`
- Migrations (separate DB, separate goose table), config, health
- Jellyfin client (`internal/tv/jellyfin`): auth, item browse, metadata,
  direct-play stream URL construction, image fetch
- Device pairing + token auth middleware
- Media items, availability windows, ledger, grants, heartbeats, sessions
- Tests: testcontainer integration mirroring the LMS suite; fake Jellyfin
  (httptest) for the client
- **Exit criteria**: curl-driven flow — pair, list catalog, get grant, play
  URL against real Jellyfin, heartbeat, watch-once lockout works

### Phase 2 — Admin SPA ✅
- `tv-web/` scaffold, OpenAPI codegen, Library/Availability/Devices pages
- Jellyfin import flow with cover art
- Embed in tv-server binary like the existing `internal/spa`
- **Exit criteria**: parent can import, classify, and rotate content without SQL

### Phase 3 — Android app MVP (on-demand) ✅
- Project scaffold (`android/`), CI build (GitHub Actions, unsigned + signed APK)
- Pairing, catalog, detail, ExoPlayer playback via grants, heartbeats,
  watch-once UX, tablet + leanback shells
- Validate direct play of representative H.264/H.265 files on the T9 box
- **Exit criteria**: student can watch an available item on both devices;
  entertainment item locks after one viewing

### Phase 4 — Programmed channel ✅
- Schedule entries + `/now` resolution, `GET /schedule?day=` EPG, admin
  `GET /schedule-grid` and `POST /schedule-entries/copy-week`
- Overlap rejection on create/update (decision 9); `POST /media/{id}/grant?mode=programmed`
  refuses anything not airing now and stamps the join offset
- `tv-web` week-grid editor with conflict highlighting and copy-week
- Android channel + EPG screens; locked-down programmed player (no seek, no
  pause, back exits) re-syncing its offset from `/now` on resume
- **Exit criteria**: box tuned to the channel plays the day's grid unattended

### Phase 5 — Primer integration ✅
- Primer: `instruction_logs` resource (migration, domain type, repo resource,
  generic CRUD) + `POST /instruction-logs/ingest`, idempotent on
  `(source, source_ref)` and guarded by the LMS service token
- Primer admin SPA: **Instruction Logs** page — logged instructional time by
  day, with subject and standard chips and a duration column
- TV server: `internal/tv/primer` — LMS client plus a reporter job that walks
  finished educational/mixed `playback_sessions`, posts them, and records each
  in `primer_reports` (unique per session). Configured by `TV_PRIMER_BASE_URL`,
  `TV_PRIMER_SERVICE_TOKEN`, `TV_PRIMER_REPORT_INTERVAL`,
  `TV_PRIMER_REPORT_BATCH_SIZE`, `TV_PRIMER_TIMEOUT`; inert when unconfigured
- TV admin SPA: **Primer Reports** page — the export ledger plus a "report now"
  button; deleting a row re-queues its viewing
- Primer→TV: documented above; no new TV endpoints were needed
- **Exit criteria met**: watching a tagged documentary shows up in Primer as
  instructional time under the right subject

**Entertainment exclusion** is enforced twice, on purpose. The LMS is
authoritative — its `class` enum and a table `CHECK` refuse anything but
`educational`/`mixed`, because the definition of instructional time belongs to
the system of record and must not depend on a caller behaving. The TV reporter
*also* excludes entertainment inside the "what is unreported" query rather than
skipping rows after selecting them: a reporter that filtered afterwards would
rescan an ever-growing backlog of sessions it will never send.

**Reporting is at-least-once.** The ledger write follows the successful post, so
a crash in between costs one repeated request — which the LMS answers 200 /
`created: false` — instead of losing the hours. An LMS that is down or refusing
leaves each session queued and counts it as failed; nothing is dropped and
nothing wedges, because a pass never fails as a whole unless the TV database
itself is unreadable.

### Phase 6 — Polish ✅
- **APK self-update**: the TV server publishes an APK from `TV_RELEASE_DIR`
  (`GET /app/release` describes it, `GET /app/release/apk` serves it, both
  device-authenticated). The app compares version codes, downloads, verifies
  the SHA-256, and hands the file to the package installer through a
  `FileProvider`. The APK is supplied out of band and mounted as a volume, so
  no binary enters git or the image.
- **TV box kiosk mode**: a `KioskAlias` activity-alias carries the HOME intent
  filter and ships **disabled**. Enabling it per device
  (`adb shell pm enable com.aleksclark.primer.tv/.KioskAlias`) makes the
  channel the box's whole experience; the same APK on the tablet cannot take
  over its home screen.
- **Rotation automation**: `GET /rotation/suggestions` proposes direct-playable
  items with nothing on offer and no viewing spent, oldest first;
  `POST /rotation/rotate` closes what is out and opens a fresh set. Surfaced as
  a panel above the availability calendar.
- **Metrics dashboard**: `GET /metrics?days=` returns watch time by class,
  subject, and day plus completion rates and the entertainment ration, all for
  one window in one document. The tv-web **Viewing** page renders it.
- **Deployment**: `Dockerfile.tv` and `deploy/primer-tv.nomad.hcl.tmpl` give the
  TV server the same containerised story the LMS already had, and a `go` CI
  workflow runs vet, gofmt, the coverage gate, and a check that the committed
  OpenAPI specs are current.

**Weekly parent report — implemented as a dashboard, not email.** Sending mail
would mean introducing an SMTP dependency, credentials, and a delivery failure
mode for a household where the parent already opens the admin UI. The metrics
page answers the same questions on demand and over any window, so the report is
a page rather than an inbox item. If mail is wanted later, the `/metrics`
endpoint is already the whole payload.

**Offline grace — deliberately not implemented.** Every playback needs a
server-issued grant, so a client that cannot reach the server cannot lawfully
start anything; caching an EPG would only let the box display a schedule it
cannot act on. The related open question — whether educational content should
be replayable offline — is answered **no**: replay is already allowed while a
window is open (the ledger only rations entertainment), so the missing capability
is not replay but disconnected operation, and that would mean shipping media to
the device and reporting hours it cannot substantiate. The channel is a LAN
appliance; when the LAN is down, the television is off.

## Risks & mitigations

| Risk | Mitigation |
|------|------------|
| RK3318 codec/container quirks (EAC3 audio, 10-bit HEVC) | Direct-play validator flags items; test matrix in phase 3; keep transcoding disabled |
| Stream URL leakage (grant reuse outside app) | Single-use grants, short expiry, server-side session accounting; accept LAN threat model for a middle-schooler |
| Clock drift breaks "what's on now" | Server returns offset + server time; client trusts server exclusively |
| Android 9 box abandonment (no updates) | minSdk 28, avoid APIs >28 at runtime on TV path; keep app lean |
| Double-reporting hours to Primer | `primer_reports` ledger keyed by session, and the LMS ingest keyed by `(source, sourceRef)` — either alone suffices, so a partial failure on one side is still safe |
| Jellyfin library churn (renames/deletes) | Sync job marks orphaned media items; admin surfacing |

## Out of scope (per guidelines)

- Content acquisition/ripping/downloading pipelines (guideline 11)
- Multi-student profiles (single student for now; devices are the unit)
- DRM or hostile-user hardening beyond watch-once bookkeeping
- Live/streaming-service sources — local Jellyfin only
