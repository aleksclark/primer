# YouTube shows — storage, Jellyfin Collection, and recovery

Updated for the September 2026 live recovery. The August execution plan is
historical: its nested `/media/tv/Primer` layout was proven incorrect against
Jellyfin 10.11.11. Do not restore that layout.

## Two different Jellyfin objects

- **Primer Sources** is a `tvshows` **Library**, scanning only
  `/media/primer/Shows`. It provides the per-library metadata policy needed for
  YouTube, whose ledger episode numbers are not TVDB episode numbers.
- **Primer** is an actual **Collection / BoxSet**, populated by content-ingest
  after import. It groups eligible playable items across Primer Sources and
  existing movie/TV libraries. It is not a replacement broadcast schedule.

Collection membership is additive and idempotent. Only currently eligible
**Jellyfin IDs with a durable TV media-item row** may be added. Exclusions and import caps
apply; Series/Season parents are never added because that would expose excluded
children. Existing operator-added Collection members are preserved.

## Canonical paths

| Purpose | Host | Container |
|---|---|---|
| Ingest output root | `/mnt/moosefs/media/primer` | `/media/primer` |
| Jellyfin library scan root | `/mnt/moosefs/media/primer/Shows` | `/media/primer/Shows` |
| Persistent reports | `/mnt/moosefs/media/primer/.ingest-reports` | `/media/primer/.ingest-reports` |
| Protected legacy archive | `/mnt/moosefs/media/Shows` | **not scanned** |

`INGEST_YTDLP_OUTPUT_DIR` must be the output root, **not** its `Shows` child.
The shared volume remains `moosefs-media` → `/media`.

Do not add the output root itself beneath a TV library: Jellyfin treats the first
unconfigured directory as a Series. This caused the old `Primer/Shows/<slug>`
tree to become the false show **Primer Plano**, even beyond its poisoned NFO.
The old empty/incomplete root was preserved outside scan roots under
`/mnt/moosefs/media/.primer-quarantine/legacy-tv-Primer-20260907`.

## Primer Sources library policy

Keep this policy separate from the existing Shows library:

- `CollectionType=tvshows`; sole location `/media/primer/Shows`.
- `EnableInternetProviders=false` and empty `MetadataFetchers` / `ImageFetchers`
  in TypeOptions for Series, Season, Episode, and Video. Local NFOs/thumbnails
  remain the source of truth; local metadata readers must not be disabled.
- `SubtitleDownloadLanguages=[]` **explicitly empty, not null**. NFO metadata
  locks alone do not stop Jellyfin's subtitle downloader. TVDB episode-number
  matching downloaded incorrect subtitles during the recovery canary.
- `MetadataSavers=[]`, `SaveLocalMetadata=false`: Primer owns its NFOs.
- `EnableRealtimeMonitor=false`: ingest scans after finalization, not while
  partial staging bundles are being written.
- Chapter/trickplay/LUFS extraction is disabled for this source library.

Never change ordinary Shows/Movies metadata or subtitle preferences to fix
YouTube. When moving a configured path between libraries, do not scan in the
gap or expose it through both. Preserve exact file paths and item types, and
read back Jellyfin IDs before/after; do not assume identity stability or rekey
TV records blindly. The live recovery verified all 63 then-existing Series /
Episode IDs were unchanged. Ensure the configured TV Jellyfin user can access
the new library (existing all-library access needs no policy change).

## On-disk contract

```text
{OutputDir}/Shows/<slug>/
  tvshow.nfo
  .primer-index.json
  .ytdlp-archive.txt
  Season 01/
    <slug> - S01E<nnn> - <title> [<youtube-id>].mp4   # or .mkv/.webm
    <same basename>.info.json
    <same basename>.nfo
    <same basename>.jpg
    _staging/                                     # never import these items
```

- Identity is **YouTube ID + manifest slug + Jellyfin item ID**.
- `.primer-index.json` assigns ordinals once, initially by upload date then ID;
  later discoveries append. It is not a curriculum sequence or `playlist_index`.
- Show and episode NFOs set `<lockdata>false</lockdata>` so first import can
  read the authored title/overview instead of freezing a filename-derived name.
  Remote provider isolation is a library policy, not an initial NFO lock.
  Jellyfin 10.11 can still surface a fresh YouTube item with the filename as its
  title on the first scan; `content-ingest` now detects that state, unlocks the
  item, queues a local metadata refresh, waits for the authored NFO title /
  provider IDs, and only then imports or re-syncs the TV row. TV curator locks
  are separate and remain respected. Per-video identity stays embedded in the
  filename and episode NFO.
- MP4 progressive fallbacks and existing 240p/360p files remain valid; do not
  transcode or copy them merely to change extensions. New downloads prefer
  H.264/H.265 up to 1080p; the TV direct-play compatibility gate still applies.
- Catalog `present` still runs yt-dlp for new uploads. `failed` is operator-gated.
- `INGEST_YTDLP_ARCHIVE_PATH` is deprecated. Never replace or delete the original
  global archive; new operation uses a per-show archive.
- Archive IDs commit only after successful finalization. A partial yt-dlp exit 1
  finalizes good bundles but remains an error. Old files never hide new errors.
- A configured download-budget exit 101 is a successful checkpoint; finalized
  IDs are archived so the next pass progresses through the backlog.

## Runtime and scheduler

Authoritative source: `deploy/nomad/jobs/content-ingest.nomad.hcl` with
`deploy/nomad/env/home.nomadvars.hcl` and digest-only `images.lock.hcl`.

| Setting | Purpose |
|---|---|
| `INGEST_JELLYFIN_COLLECTION_NAME=Primer` | Actual Collection; explicitly empty disables grouping |
| `INGEST_YTDLP_MAX_DOWNLOADS=25` | New videos **per source per pass**; `0` is an unlimited manual dump |
| `INGEST_HTTP_TIMEOUT=5m` | Bounded upstream calls for larger catalogs |
| `INGEST_SYNC_WAIT=30m` | Initial library-scan budget; timeout/status failure is an error, not success |
| `INGEST_YTDLP_JS_RUNTIME=node` | YouTube JavaScript challenges |
| `INGEST_YTDLP_COOKIES_PATH` | Optional cookie-jar **path only**, outside Git, mode 0600 |

The image pins yt-dlp **2026.06.09**, with Python 3.12 / Node 22. Fresh downloads
were proven without cookies during this recovery; that is not a promise that
YouTube authentication will never change.

The sole periodic parent is **content-ingest**, every six hours (`0 */6 * * *`,
America/Chicago), `prohibit_overlap=true`. It requests a four-hour kill timeout;
that is not a wall-clock runtime limit or a substitute for single-writer control.
Fleet API hostnames require internal DNS, not public-only Docker resolvers.

Before a manual writer or deployment, prove no other ingest writer is active.
Never register a second periodic parent. `deploy/deploy.sh` refuses production
submission; use the approved fleet writer or a reviewed, explicitly authorized
operator CAS recovery. Do not merge or flip repository authority implicitly.

## Commands

With the appropriate `INGEST_*` credentials supplied privately:

```sh
content-ingest plan
content-ingest apply
content-ingest apply --skip-acquire  # real scan/import/Collection/sync; no downloads
```

Plan writes a report and review candidates, but does not mutate upstream services.
Use a separate `INGEST_MANIFEST_PATH` for a bounded canary; restore the canonical
manifest before unattended operation. Unknown CLI flags are rejected.

### Recover old downloads without copying media

```sh
cd server
go run ./cmd/content-recover \
  --source-root /mnt/moosefs/media/Shows \
  --output-dir /mnt/moosefs/media/primer \
  --manifest ../curriculum/content-manifest.yaml \
  --slugs mechanical-universe,primitive-technology,secret-life-of-components,townsends,essential-craftsman
# Review dry_run:true JSON, then repeat with --apply.
```

Recovery reads embedded ffprobe metadata, validates exact YouTube URLs/IDs and
upload dates, hardlinks into staging, and uses the production finalizer. No
source/output overlap, guessing, bulk copies, or source-tree deletion. Replay
reports existing identities rather than allocating new episode numbers.

**Never recover legacy `justin-rhodes`: it is the wrong musician channel.** The
correct homesteading channel is now pinned to `UCOSGEokQQcdAVFuL_Aq8dlg`; Paul
Sellers is pinned to `UCc3EpWncNq5QL0QhwUNQb7w` (the old handle returned 404).

The recovery found 1,779 completed originals: 1,688 eligible for hardlink recovery,
13 below the configured duration floor, and 78 wrong-channel Justin Rhodes files.
All original media inode/size/mtime values and the global archive checksum were
verified unchanged. Auto-generated SRTs created by the incorrect TV policy were
quarantined separately; they were not original downloaded assets.

Failed acquisition rows preserve their counters. Setting only `status=missing`
does not reset an exhausted/aged retry window. Any reviewed one-time reset must
save prior state and target only repaired sources, not clear the whole catalog.

## Real acceptance, not just a successful exit code

```sh
python3 scripts/verify-ingest-media.py \
  --output-root /mnt/moosefs/media/primer \
  --collection Primer \
  --youtube-id <11-char-id> \
  --evidence /private/path/ingest-media-proof.json
```

The verifier requires the existing `INGEST_JELLYFIN_*` credentials including user
ID and `INGEST_TV_*` credentials. It checks disk/NFO/ledger/archive/TV identities,
user-visible Collection membership, HTTP 206 ranges, a full-stream SHA-256 match,
and actual audio/video decoding at a mid-stream seek. Credentials are headers
only and never passed to ffmpeg or printed.

Also compare **all** expected recovered IDs against Jellyfin, TV, and Collection
sets, verify exclusions/wrong-source IDs are absent, and repeat the production
apply. A scan timeout can leave only part of a source visible; `present` alone
is not proof of complete recovery. Retain a container/allocation exit receipt
rather than losing it to an outer shell timeout plus `--rm`.

No schedule, VOD, device, or instructional-time mutations belong in this workflow.
