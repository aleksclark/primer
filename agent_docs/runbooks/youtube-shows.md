# YouTube shows — on-disk contract and ops runbook

Household / operator contract for Primer YouTube ingest. Companion to
`agent_docs/plans/content-ingest.md` and `docs/plans/youtube-ingest-execution.md`.
No schedule or VOD mutations in this campaign.

## Canonical roots

| Side | Path |
|------|------|
| Host (MooseFS) | `/mnt/moosefs/media/tv/Primer` |
| Container | `/media/tv/Primer` (`INGEST_YTDLP_OUTPUT_DIR`) |

Volume mount remains host volume `moosefs-media` → container `/media`. The YouTube
output dir is the **Primer** subtree under that mount, not `/media` itself and not
legacy `/data/media`.

## On-disk layout

```
{OutputDir}/
  Shows/
    <slug>/
      tvshow.nfo
      .primer-index.json
      .ytdlp-archive.txt
      Season 01/
        {slug} - S01E{nnn} - {title} [{id}].mkv
        {slug} - S01E{nnn} - {title} [{id}].info.json
        {slug} - S01E{nnn} - {title} [{id}].nfo
        {slug} - S01E{nnn} - {title} [{id}].jpg   # thumbnail when present
```

Rules:

- **Identity** = YouTube video id (11-char in `[id]` basename) + manifest `slug` +
  Jellyfin item id after import. Episode keys are `S01E{nnn}` from the per-slug
  ledger (first-seen by `upload_date` then `id`), never bare `playlist_index`.
- **Present does not skip yt-dlp.** Catalog `present` still runs archive/ledger so
  new channel uploads land; only operator-gated `failed` stays skipped.
- **Path boundary.** Match `Shows/<slug>/` as a path segment boundary
  (`paul-sellers` must not match `paul-sellers-extra`).
- **Poison NFO.** Never write `{OutputDir}/tvshow.nfo`. If that file exists at the
  Primer root, delete **only** that path. Keep `Shows/<slug>/tvshow.nfo`.
- **Per-show archive.** `{OutputDir}/Shows/<slug>/.ytdlp-archive.txt`. Global
  `INGEST_YTDLP_ARCHIVE_PATH` is deprecated/unused — do not invent a new global
  archive under `/media/ytdlp-archive.txt`.
- **Index ledger.** `{OutputDir}/Shows/<slug>/.primer-index.json` owns ordinals.

## Deploy / env contract (W6)

Authoritative jobspec: `deploy/nomad/jobs/content-ingest.nomad.hcl`
Overlay: `deploy/nomad/env/home.nomadvars.hcl`
Legacy dual-source tmpl (not production submit): `deploy/content-ingest.nomad.hcl.tmpl`

| Knob | Value |
|------|--------|
| `content_ingest_ytdlp_output_dir` / `INGEST_YTDLP_OUTPUT_DIR` | `/media/tv/Primer` |
| Task `kill_timeout` | `4h` (longest allowed dump) |
| `INGEST_YTDLP_COOKIES_PATH` | filesystem **path** only (default empty) |
| `INGEST_YTDLP_ARCHIVE_PATH` | deprecated/unused (empty default) |
| Image | `Dockerfile.ingest` — `nodejs` installed for `--js-runtimes node` |

Cookies:

- Host jar **outside git**, mode `0600`.
- Env/var carries the **path string only**. Never commit, print, or template cookie
  jar contents. Never `cat` jars into chat or git.
- Jobspec default is empty string so missing overlay does not fail closed at parse.

yt-dlp pin:

- Image currently installs from GitHub `/releases/latest` (unpinned). Risk: a
  broken upstream release can break Gate A until the image is rebuilt. Prefer a
  dated release URL once a known-good tag is verified; until then rebuild/roll
  quickly if YouTube player changes land.

Node/js:

- `Dockerfile.ingest` `apk add nodejs` so `--js-runtimes node` works in the job.
- Gate A probes need cookies path + js runtime both available.

## Periodic scheduler

The jobspec keeps the existing **6h** cron (`0 */6 * * *`, `prohibit_overlap`).
**Do not register a second periodic.**

Until W9 one-slug success **and** `kill_timeout=4h` are deployed:

1. Pause or disable the content-ingest periodic (or wait for a quiet window).
2. Prove no child alloc is running before writer handoff.
3. Do not CI-dispatch or `nomad job run` from `deploy/deploy.sh` (script refuses
   production submit; fleet pull reconciler is the writer).

## Gate A — no-write probe

Run a single-video probe that must **not** write into the library. Use env for the
cookies **path** only; do not embed cookie values.

```bash
# PATH-only env; jar stays on host mode 0600 outside git.
# Example (operator fills path locally — never commit the value):
#   export INGEST_YTDLP_COOKIES_PATH=/path/to/cookies.txt
#   export INGEST_YTDLP_OUTPUT_DIR=/media/tv/Primer   # or host equivalent

OUT="${INGEST_YTDLP_OUTPUT_DIR:-/media/tv/Primer}"
COOKIE_ARGS=()
if [[ -n "${INGEST_YTDLP_COOKIES_PATH:-}" ]]; then
  COOKIE_ARGS=(--cookies "${INGEST_YTDLP_COOKIES_PATH}")
fi

# Probe video id from the execution plan (no-write).
yt-dlp \
  --skip-download \
  --no-write-playlist-metafiles \
  --js-runtimes node \
  "${COOKIE_ARGS[@]}" \
  -O '%(id)s %(title)s' \
  -- 'https://www.youtube.com/watch?v=h0EGCnBjTVk'
```

Success: prints id/title, exit 0, **no** new files under `${OUT}/Shows/`.
Failure (bot/challenge): fix cookies path + node runtime before any MR YAML or bulk.

## Live ops constraints (this campaign)

- **Hardlink** legacy seven-channel media into the Primer tree; do not copy bulk.
- **Quarantine** Justin Rhodes (wrong-channel risk) until URL/slug is verified.
- **Protect** the source 1779 archive; never delete `/mnt/moosefs/media/Shows`.
- **No schedule / VOD** mutations from ingest.
- Mark Rober later: three slugs only —
  `mark-rober` (`/videos`, 130, mixed, default filters),
  `mark-rober-science-class` (`/streams`, 8, educational, `exclude_live: false`),
  `mark-rober-shorts` (`/shorts`, 128, mixed, `exclude_shorts: false`, `min_duration_seconds: -1`). Official channel
  `UCY1kMZp36IQSyNx_9h4mpCg`. One-slug canary before bulk. Do not add live YAML before Gate A.

## Related paths

- Plan: `docs/plans/youtube-ingest-execution.md`
- Architecture proposal: `agent_docs/plans/content-ingest.md`
- Job: `deploy/nomad/jobs/content-ingest.nomad.hcl`
- Env overlay: `deploy/nomad/env/home.nomadvars.hcl`
- Image: `Dockerfile.ingest`
- Local contract only: `./deploy/deploy.sh contract`
