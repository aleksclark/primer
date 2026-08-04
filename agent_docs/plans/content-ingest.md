# Proposal: content-ingest — manifest-driven library population

Takes curated lists (like `agent_docs/content_library/`) and converges Radarr/Sonarr/yt-dlp,
Jellyfin, and the TV server toward them. **Zero LLM in the loop; human involvement is
one review file for ambiguous matches.** Scheduling stays out of scope — this service
stops at "classified media_items exist in the TV server."

## The core trick: provider IDs make the whole pipeline deterministic

The reason no LLM/fuzzy matching is needed end-to-end: resolve each title to a
**TMDB/TVDB ID once** at manifest-compile time, then every later join is exact:

```
manifest entry (tmdb:603)
  → Radarr add (tmdb:603)                    exact
  → file lands in library folder
  → Jellyfin scans, stores ProviderIds.Tmdb=603   exact
  → ingest matches Jellyfin item by provider ID   exact
  → POST /media-items {jellyfinItemId, class, subject_tags, standard_codes}  exact
```

Title-string matching happens exactly once (manifest compile), where it's cheap to
put a human in the loop.

## Architecture

```
content_library/*.md          (human/LLM-authored, prose)
        │  ① compile (one-time per list; the ONLY step where an LLM may help)
        ▼
manifest.yaml                 (desired state, committed to git)
        │
        ▼
┌─────────────────────────────────────────────────────────┐
│                CONTENT-INGEST (Go, reconciler)           │
│                                                          │
│  ② resolve   title/year → tmdb/tvdb id (Radarr/Sonarr   │
│              lookup APIs); 0-or-many hits → review.yaml  │
│  ③ acquire   movies→Radarr  series→Sonarr               │
│              youtube→yt-dlp (--download-archive)         │
│              dvd→manual-rip queue (report only)          │
│  ④ sync      trigger Jellyfin library scan;              │
│              tv-server POST /jellyfin/sync               │
│  ⑤ import    match Jellyfin items by provider id /       │
│              path convention → POST /media-items with    │
│              class + subject_tags + standard_codes       │
│  ⑥ report    one markdown status report per run          │
└─────────────────────────────────────────────────────────┘
```

Each pass is idempotent; the manifest is desired state, the run converges toward it
(Terraform-plan style: `ingest plan` shows the diff, `ingest apply` acts).

## Manifest schema

```yaml
# curriculum/content-manifest.yaml
items:
  - id: living-planet                # stable slug, referenced by review.yaml
    title: "The Living Planet"
    year: 1984
    kind: series                     # movie | series | youtube_channel | youtube_playlist | manual
    provider: {tvdb: 79165}          # filled by ② resolve; empty = unresolved
    class: educational               # educational | mixed | entertainment
    subject_tags: [science, life-science, ecosystems]
    standard_codes: [TN.SCI.6.LS2.1]
    priority: 1                      # acquisition ordering only
    exclude_episodes: []             # e.g. Blue Planet II: [S01E07]

  - id: paul-sellers
    title: "Paul Sellers"
    kind: youtube_channel
    url: https://www.youtube.com/@PaulSellersWoodwork
    filters: {playlists: ["Beginner Series"]}   # optional narrowing
    class: mixed
    subject_tags: [practical, woodworking, measurement]

  - id: bernstein-ypc
    title: "Leonard Bernstein Young People's Concerts"
    kind: manual                     # DVD rip — appears in the manual queue report
    class: educational
    subject_tags: [music, arts]
```

`exclude_episodes` handles the ideology/content skip-lists (Blue Planet II ep 7,
Legacy ep 6, MythBusters adult-myth eps) — the importer simply never creates
media_items for excluded episodes, so they can't be scheduled or granted.

## Stage details

**① Compile (list → manifest).** One-time per curated list. An LLM (or the parent)
converts prose tables to YAML. This is authoring, not operation — the service never
reads the markdown.

**② Resolve.** `GET /api/v3/movie/lookup?term=` (Radarr) / `series/lookup` (Sonarr).
Exactly one hit with matching year → write provider ID into the manifest (the tool
edits the YAML in place, committable diff). Zero or multiple hits → append to
`review.yaml` with the candidates; human picks by uncommenting a line. **This file
is the entire human workload** — everything downstream is exact-ID joins.

**③ Acquire.**
- *movies/series*: add via Radarr/Sonarr API with a `primer` tag and a dedicated
  quality profile capped at **1080p H.264/H.265** (RK3318 direct-play ceiling — this
  encodes the direct-play constraint at acquisition time instead of validating after).
  Sonarr monitors only non-excluded episodes.
- *youtube*: yt-dlp with `--download-archive` (idempotent), output template shaped for
  Jellyfin (`Shows/<Channel>/Season 01/...`), `--embed-metadata --write-thumbnail`,
  format capped `bv*[height<=1080][vcodec~='(avc|hevc)']+ba`. Runs as the Nomad job's
  periodic batch; new channel uploads are picked up on later passes automatically.
- *manual*: no action; emits the rip-queue section of the run report.

**④ Sync.** Jellyfin `POST /Library/Refresh`, wait for scan completion, then tv-server
`POST /jellyfin/sync` (existing endpoint) so its metadata cache sees the new items.

**⑤ Import.** For each manifest item with provider IDs: find the Jellyfin item(s) via
`/Items?hasTmdbId=...` / provider-id filter (yt-dlp content matches by path prefix
instead — the output template embeds the manifest slug). Create/update tv-server
media_items via the existing admin API with class/subject_tags/standard_codes from
the manifest. Series → one media_item per episode (matching current sync behavior),
excluded episodes skipped. Already-imported items (by `jellyfin_item_id` unique key)
are updated only if manifest classification changed.

**⑥ Report.** One markdown file per run (and optionally a Telegram post): resolved,
acquired, awaiting-download, imported, review-queue size, manual-rip queue. The
parent's steady-state involvement: read the report, occasionally answer `review.yaml`,
rip the DVD list at leisure.

## Implementation shape

- `server/cmd/content-ingest` in the existing Go module — reuses config conventions
  (`INGEST_` env prefix), the Jellyfin client (`internal/tv/jellyfin`), and testutil
  harness. New thin clients for Radarr/Sonarr (each is ~4 endpoints: lookup, add,
  tag, queue status). yt-dlp shelled out (it's already the fleet standard).
- **State**: desired-state YAML stays in git; the TV server owns runtime acquisition
  state in `content_manifest_entries` (status missing/present/failed/manual,
  attempt_count, first/last attempt timestamps). content-ingest upserts desired
  state on every run (`POST /content-manifest/sync`), increments attempts when it
  tries to obtain missing media, and marks present when Jellyfin has the title.
  TV flips missing → failed after `TV_MANIFEST_FAIL_MAX_ATTEMPTS` or
  `TV_MANIFEST_FAIL_MAX_DAYS` so a human can buy/rip. (`review.yaml` remains the
  ambiguous-lookup working file in git.)
- **Deploy**: Nomad periodic batch job (`deploy/content-ingest.nomad.hcl.tmpl`,
  every 6h) + on-demand `make ingest-apply`. Image is `Dockerfile.ingest`
  (bakes curriculum YAML + yt-dlp). Needs mounts: Jellyfin library volume (for
  yt-dlp output) — same host volume the media stack already shares.
- **Testing**: httptest fakes for Radarr/Sonarr (mirroring `jellyfin/fake.go`),
  golden-file tests for plan output, integration test for the resolve→import path
  against the existing testcontainer harness.

## Human/LLM involvement budget

| Step | Actor | Frequency |
|------|-------|-----------|
| Write/revise curated lists | human + LLM | rare (curriculum planning) |
| Compile list → manifest | LLM once, or human | once per list revision |
| Answer review.yaml | human | ~a few items per list, once |
| Rip DVDs from manual queue | human | at leisure |
| Everything else | reconciler | automatic, idempotent |

## Out of scope
- Scheduling / availability windows (Primer agents + parent, via existing TV admin API)
- Transcoding (acquisition profile prevents the need)
- Seeding/indexer management (Prowlarr/qBittorrent already handle it)

## Open questions
1. Manifest location: `curriculum/content-manifest.yaml` in this repo vs. its own repo. Suggest this repo — it versions with the content_library docs that source it.
2. Episode-level `media_items` for 400-episode series (How It's Made): import-on-arrival is fine, but consider a `max_episodes` manifest cap or Sonarr season filters to avoid drowning the library page.
3. Whether ⑥'s report should also post to Telegram (nice, trivial) or stay file-only.
