# YouTube Ingest Repair + Mark Rober E2E — Implementation Plan

> Plan-only W0 commit. Execute from this file + `/tmp/primer-youtube-orchestration/04-execution-index.md`.
> Sources (do not re-audit): `01-current-setup-audit.md`, `02-metadata-contract-plan.md`, `03-mark-rober-acquisition-plan.md`.

**Goal:** One JF-visible YouTube root, stable per-video identity, seven-channel repair, then all 266 official Mark Rober videos as schedulable Primer items. No schedule/VOD mutations.

**Base:** `origin/master` `bd0f8879104f4a6f4a5363db11726ab08a327e08` (PIN). Local main checkout `master` stays at `fd5b1fb` and must remain clean.

**Worktree:** `/home/aleks/work/projects/primer/worktrees/impl-youtube-ingest` on `impl/youtube-ingest`.

**Never:** edit/push/merge `master`; `git add -A`; print cookies/PO tokens/`.env` values; schedule or VOD new content; delete `/mnt/moosefs/media/Shows` source tree.

---

## Base branch choice

| Branch / tree | SHA | Why |
|---|---|---|
| `origin/master` (PIN) | `bd0f8879104f4a6f4a5363db11726ab08a327e08` | Published tip; implement from this, not stale local master |
| Local `master` checkout | `fd5b1fb1eda8c20ea368259afb69492be8b91e2c` | Read-only; do not checkout or dirty |
| `content-ingest` | `0300f41` | Not ancestor of PIN; do not use |

---

## Authority (index wins)

- Canonical root: host `/mnt/moosefs/media/tv/Primer` = container `/media/tv/Primer`. Layout `{OutputDir}/Shows/<slug>/Season 01/`.
- Episode ordinals: per-slug `.primer-index.json`. First-seen by `upload_date` then `id`, `S01E{nnn}`. Never `playlist_index`.
- Mark Rober: three slugs. `mark-rober` `/videos` 130 `mixed`; `mark-rober-science-class` `/streams` 8 `educational`; `mark-rober-shorts` `/shorts` 128 `mixed`. Official `UCY1kMZp36IQSyNx_9h4mpCg`.
- Explicit `/shorts` or `/streams` allowed only with `exclude_shorts=false` or `exclude_live=false`.
- W10 three-slug filters (do **not** add live YAML before Gate A): `mark-rober` defaults; `mark-rober-science-class` `exclude_live: false` (completed past streams only); `mark-rober-shorts` `exclude_shorts: false` + `min_duration_seconds: -1`. `-1` is the only allowed negative duration.
- YouTube `present` must still run yt-dlp. `failed` stays operator-gated.
- Schema `00003_youtube_identity.sql`: `youtube_video_id` UNIQUE+format, `manifest_slug`, `episode_key`, `upload_date`, `title_locked`, `overview_locked`, `classification_locked`.
- Delete poison `{OutputDir}/tvshow.nfo` only. Never write show NFO at Primer root.
- Cookies jar outside git, mode 0600, path-only. `INGEST_YTDLP_COOKIES_PATH` + `--js-runtimes node`.
- New downloads ≤1080p mkv. Legacy seven-channel keep 360p hardlinks.
- No T9 interaction. No secrets in git/chat.

---

## File ownership (serialize shared files)

| Lane | Owns | No-touch |
|---|---|---|
| A ytdlp | `server/internal/ingest/ytdlp/**` (add `identity.go`, `ledger.go`, `nfo.go`, `cleanup.go`, `poison.go` + tests); `server/internal/ingest/config/config.go` + `config_test.go` for cookies/archive env only | TV schema, manifest YAML, reconcile.go |
| B reconcile | `server/internal/ingest/reconcile/**`, `server/internal/tv/jellyfin/jellyfin.go` + tests | ytdlp internals after A lands |
| C manifest | `server/internal/ingest/manifest/**` | live `curriculum/content-manifest.yaml` until W8 |
| D TV schema/API | `server/internal/tv/db/migrations/00003_youtube_identity.sql`, `domain.go`, `api/resources.go`, `api/admin.go`, `repo/resources.go`, `tvclient/**`, `tv-web/openapi.yaml` via `make openapi-tv` | ingest acquire |
| E deploy/docs | `Dockerfile.ingest`, `deploy/content-ingest.nomad.hcl.tmpl`, `deploy/nomad/jobs/content-ingest.nomad.hcl`, `deploy/nomad/env/home.nomadvars.hcl`, `deploy/deploy.sh` (no `/data/media` default), `agent_docs/plans/content-ingest.md`, `agent_docs/runbooks/youtube-shows.md` | media trees, Go ingest/TV code |
| Integrator | `reconcile.go` wiring after A/C, Makefile if new targets, `curriculum/content-manifest.yaml` (MR + JR URL when known) | live apply |

Parallel only A∥C∥D∥E after W0. B after A/C/D integrated. Integrator last for W5.

Children **must** use isolated worktrees/branches. Never concurrently edit one shared worktree.

---

## Frozen create-body fields (Lane D)

Add to `MediaItem` / create / update / tvclient (optional omitempty; do not break existing clients):

| Field | JSON | DB | Notes |
|---|---|---|---|
| `youtube_video_id` | `youtubeVideoId` | `youtube_video_id` | UNIQUE where not null; `^[A-Za-z0-9_-]{11}$` |
| `manifest_slug` | `manifestSlug` | `manifest_slug` | default `''` |
| `episode_key` | `episodeKey` | `episode_key` | `S01E007` |
| `upload_date` | `uploadDate` | `upload_date` | DATE NULL |
| `title_locked` | `titleLocked` | `title_locked` | bool default false; PATCH title sets true |
| `overview_locked` | `overviewLocked` | `overview_locked` | PATCH overview sets true |
| `classification_locked` | `classificationLocked` | `classification_locked` | PATCH class/tags/codes sets true |

`jellyfinItemId` remains immutable (405). Unique youtube id → 409.

`metadataDiff`: skip title if locked; skip overview if locked; never write class/tags/codes/quality_notes except existing directplay stale-allowlist path; if unlocked and `manifest_slug != ""`, constructed title `{Show} {episode_key} — {JF.Name}`.

---

## Waves (TDD RED→GREEN per wave)

### W0 — this commit

Gate: worktree on `impl/youtube-ingest`, parent = PIN, master checkout unchanged, clean porcelain.

### W1 — Lane A: identity, argv, cookies/js

RED tests then impl:

- `PathMatches` boundary (`paul-sellers` ≠ `paul-sellers-extra`)
- `ParseYouTubeID` from `[id]` basename; reject short/long/invalid
- Ledger reorder-stable + new id → next; first ingest sort `upload_date` then `id`
- NFO goldens (show + episode youtube uniqueid). Read bytes from disk.
- Argv stub: `--write-info-json --continue --no-write-playlist-metafiles --cookies --js-runtimes`, staging `%(id)s`, per-show archive, match-filter default `!is_live` + `duration>59`; explicit `/shorts`/`/streams` allowed only with filter opt-out
- Cleanup EC fixture (jpg + `.f137.mp4.part` + `S01E000`); keep complete `[id].mkv`
- Poison: delete `{OutputDir}/tvshow.nfo` only; keep `Shows/<slug>/tvshow.nfo`
- Sanitize `last_error` (no full argv)

Config: `INGEST_YTDLP_COOKIES_PATH`, `INGEST_YTDLP_ARCHIVE_DIR` (default per-show); `INGEST_YTDLP_ARCHIVE_PATH` deprecated unused; `INGEST_YTDLP_JS_RUNTIME` default `node`.

Two-phase: stage `{OutputDir}/Shows/{slug}/Season 01/_staging/{slug} - %(title).80B [%(id)s].%(ext)s` then ledger rename to `{slug} - S01E{nnn} - {title:.80} [{id}].%(ext)s`.

Local: `cd server && go test ./internal/ingest/ytdlp/ ./internal/ingest/config/ -count=1`

### W2 — Lane B (after A/C/D integrated)

RED:

- YouTube `present` still calls FakeRunner; movies still skip
- `browseAll` pages on **unfiltered** page size / `TotalRecordCount`; empty first filtered page still finds a later hit; raise/eliminate 5000 ceiling for path-scoped queries
- Folder/Series not imported; Folder does not `markPresent`
- Constructed title `Paul Sellers S01E001 — Dovetails`
- Failed YouTube skip remains until operator PATCH; leftover-exit-1 after complete archive is not a new fail
- Import before `POST /jellyfin/sync`
- Path match uses `ytdlp.PathMatches`

Local: `go test ./internal/ingest/reconcile/ ./internal/tv/jellyfin/ -count=1`

### W3 — Lane C: manifest filters + exclude

`VideoOverride`, `filters.min_duration_seconds` / `exclude_shorts` / `exclude_live` / wired `playlists`, `exclude_episodes` accepts youtube id or `S01E007`, `max_episodes` applies to Video+Episode. Empty `playlists` on a channel → `/videos` only. Unresolved playlist name → fail item, do not dump whole channel.

Default filters: `min_duration_seconds=60`, `exclude_shorts=true`, `exclude_live=true`. Pointers allow opt-out.

Local: `go test ./internal/ingest/manifest/ -count=1`

### W4 — Lane D: schema + locks

`00003_youtube_identity.sql` as above. Domain/API/tvclient populate fields. `metadataDiff` locks + constructed-title. Unique youtube id 409. `make openapi-tv` committed.

Local: `go test ./internal/tv/... ./internal/ingest/tvclient/ -count=1` then `make openapi-tv` + clean spec diff. **Do not** `make migrate-tv` against prod.

### W5 — Integrator: fake E2E + integration

`youtube_e2e_test.go` + `youtube_integration_test.go`. Stub binary writes staging files; production rename assigns S01E001/002 by upload_date; second run stable; no Folder import. Do not pre-create `S01E00x` names.

Local: `go test ./internal/ingest/reconcile/ -run 'YouTube|Youtube' -count=1 -v`

### W6 — Lane E: deploy/env lock (no apply)

Align output to `/media/tv/Primer` (container). Nomad `kill_timeout` ≥ longest allowed dump (hours). Stop any `/data/media` default. Canonical HCL + `home.nomadvars.hcl` `content_ingest_ytdlp_output_dir = "/media/tv/Primer"`. Deprecate global archive var (leave unused). `deploy/.env` 0600 mode only — no value edits in git. Pin yt-dlp in image if cheap; else document unpinned risk. **Do not register a new periodic apply.** Add `INGEST_YTDLP_COOKIES_PATH` env **name** (value from Nomad Variable, never committed).

Docs: update `agent_docs/plans/content-ingest.md`; create `agent_docs/runbooks/youtube-shows.md`.

Gate: tmpl/HCL/env prefixes agree; no secret values in git.

### W6b — Local CI equivalent

```bash
cd /home/aleks/work/projects/primer/worktrees/impl-youtube-ingest
make lint
( cd server && test -z "$(gofmt -l .)" )
make cover          # 85%
make test
make openapi-tv && git diff --quiet -- tv-web/openapi.yaml
git diff --check origin/master...HEAD
```

Feature: `cd server && go test ./internal/ingest/... ./internal/tv/jellyfin/ ./internal/tv/api/ -count=1`

### W7–W10 live ops

Only after code gates. First disable/pause overlapping periodic ingest. Protect source 1779 archive. Quarantine Justin Rhodes. Remove poison Primer Plano NFO. Hardlink not copy. Recover identities or `_unmapped`. Never delete source tree. Fix seven failed rows only after code/root gates. One-slug canary before bulk. Gate A no-write probe `h0EGCnBjTVk` before any MR YAML. Then canary `a1D-fZP8qJk`, then 130, 8, 128. No schedule/VOD.

---

## Child worktree recipe

From PIN + W0 commit on `impl/youtube-ingest`:

```bash
unset GIT_DIR GIT_WORK_TREE GIT_COMMON_DIR GIT_INDEX_FILE
export -n GIT_DIR GIT_WORK_TREE GIT_COMMON_DIR GIT_INDEX_FILE 2>/dev/null || true
BASE=$(git -C /home/aleks/work/projects/primer/worktrees/impl-youtube-ingest rev-parse HEAD)
git -C /home/aleks/work/projects/primer/repo worktree add -b impl/youtube-ingest-lane-a \
  /home/aleks/work/projects/primer/worktrees/impl-youtube-ingest-lane-a "$BASE"
# same for lane-c, lane-d, lane-e
```

Stage allowlisted paths only. Commit on the lane branch. Integrator cherry-picks onto `impl/youtube-ingest`.

---

## Acceptance

Exactly W0–W10 in `/tmp/primer-youtube-orchestration/04-execution-index.md`. Do not lower gates. Open one PR `impl/youtube-ingest`→`master` after code/local gates; do not merge in this campaign.

## Completion rule

This plan is complete when it lives at `docs/plans/youtube-ingest-execution.md` on `impl/youtube-ingest`, parent is PIN `bd0f8879104f4a6f4a5363db11726ab08a327e08`, and implementers can execute W1–W10 without rediscovering split-brain, failed-catalog halt, browseAll, present-skip, poison NFO, JR wrong channel, or the bot gate.
