# Primer ingest live recovery — 2026-09-07

Concise sanitized operator summary for the September 2026 live YouTube recovery.
Private payloads and credentials remain under `/tmp/primer-ingest-e2e/` and are
not copied here.

## Live Jellyfin objects

- Collection / BoxSet: **Primer** (`e572627521fb504b4d1af05824032c2b`)
- Dedicated TV library: **Primer Sources** (`8afdd8e25c7ebe4c452f1e63819e68a8`)
- Canonical ingest root: `/media/primer` in containers
- Canonical host root: `/mnt/moosefs/media/primer`
- Legacy poisoned root quarantined outside scan paths:
  `/mnt/moosefs/media/.primer-quarantine/legacy-tv-Primer-20260907`

## Final live images

- `primer-tv`: `registry.fleet.clark.team/primer-tv@sha256:ba7965af990019a0193a07f9aa4a296d00dd69d7d0576e254d5ca98d7b77dae0`
- `content-ingest`: `registry.fleet.clark.team/content-ingest@sha256:f5e5b89be2c757dc122c7059e5c948a01ed6a4c0d1d74c1010ce0fb9d504ec6a`

The restored periodic parent is `content-ingest`, cron `0 */6 * * *`, timezone
`America/Chicago`, `prohibit_overlap=true`, canonical manifest, no temporary
`--skip-acquire` args, budget `25`, output `/media/primer`, persistent reports
`/media/primer/.ingest-reports`, internal DNS `192.168.0.23/24/89`.

## What changed

- Recovery/import path is the dedicated **Primer Sources** library, not the old
  nested `/media/tv/Primer` tree.
- Collection membership is actual Jellyfin BoxSet membership, restricted to
  durable TV-backed playable items.
- YouTube initial metadata handling was finished in two layers:
  - newly written show/episode NFOs now emit `<lockdata>false</lockdata>`
  - `content-ingest` now detects filename-derived Jellyfin YouTube metadata,
    unlocks the item through Jellyfin, queues a local metadata refresh, waits
    for the corrected title/provider IDs, and only then imports/syncs TV rows
- Final replay fix: YouTube Jellyfin browse now explicitly requests
  `ProviderIds`, preventing healthy entries from being unnecessarily unlocked
  and refreshed on every replay.

## Live proof checkpoints

### Existing library and recovery

- Original archive preserved: `1779` legacy MP4s unchanged; global archive SHA
  unchanged.
- Hardlink recovery preserved originals and surfaced `1688` eligible recovered
  videos from the legacy archive.
- Existing retained proofs still apply:
  - `/tmp/primer-ingest-e2e/ten-source-stream-proof.json`
  - `/tmp/primer-ingest-e2e/nomad-budget-stream-proof.json`

### Metadata bug closure

The filename-derived metadata bug was reproduced on a real Nomad canary, then
closed and re-proved live.

- Manual repair proof for the already-imported Moon short:
  `/tmp/primer-ingest-e2e/moon-metadata-proof.json`
- Manual repair proof for the first Slime canary imported before the final code
  fix: `/tmp/primer-ingest-e2e/mark-shorts-metadata-stream-proof.json`
- Fresh Nomad proof with the final image: `0BAkKJ_3Tic` (**Lowest Heart Rate
  Wins!**) imported with the correct Jellyfin title, provider IDs, overview,
  TV title, Collection membership, and streamed decode.
- Isolated one-item replay proof with the replay-fix image:
  `/mnt/moosefs/media/primer/.ingest-reports/ingest-20260907-063549.md`
  completed with imported `0` / updated `0`, and a live lock-state canary on
  `0BAkKJ_3Tic` remained locked during the replay, proving healthy metadata was
  not re-unlocked/refreshed. The item was then restored to its normal unlocked
  state.

### Final replay / no-churn proof

Final canonical replay with the restored logic ran as a real Nomad child with
`--skip-acquire` and reported:

- imported `0`
- updated `0`
- collection added `0`
- errors `0`

Report: `/mnt/moosefs/media/primer/.ingest-reports/ingest-20260907-060414.md`

## Final verified counts

From `/tmp/primer-ingest-e2e/bulk-identity-proof-final.json`:

- expected YouTube playables on disk: `1696`
- TV rows: `1696`
- Jellyfin playables: `1696`
- Primer Collection members for those YouTube items: `1696`
- Total actual Primer Collection members: `1862` (`1696` YouTube + `166`
  curated conventional entries)
- direct-play compatible: `1696`
- duplicate YouTube IDs: `0`
- short-duration legacy exclusions still absent: `13`
- wrong-channel legacy Justin Rhodes music uploads still absent: `78`

Per-source YouTube totals now visible in Primer Sources:

- essential-craftsman `947`
- townsends `484`
- secret-life-of-components `97`
- primitive-technology `104`
- mechanical-universe `56`
- mark-rober-shorts `4`
- mark-rober-science-class `1`
- mark-rober `1`
- paul-sellers `1`
- justin-rhodes `1`

## Conventional-manifest and catalog status

Current actual `content_manifest_entries` status counts are:

- `present`: `22`
- `failed`: `12`
- `manual`: `1` (`bernstein-ypc`)

The final canonical replay still showed these manifest items as **not yet in
Jellyfin**:

- `living-planet`
- `engineering-an-empire`
- `woodwrights-shop`
- `victorian-farm`
- `cosmos-sagan`
- `secret-life-of-machines`
- `hornblower`
- `life-on-earth`
- `trials-of-life`
- `private-life-of-plants`
- `life-of-birds`
- `earth-power-of-the-planet`

That is an acquisition/catalog availability gap, not a YouTube import identity
problem. The final replay reported manual rip queue `0` and failed queue `0`
because it was a `--skip-acquire` verification pass; those replay counters do
not mean the catalog has no pending failed/manual titles.

## TV schedule / availability guardrail

Read-only DB verification after all ingest work:

- `schedule_entries`: count `474`, SHA-256
  `7666e7ba3c1e60e02ba813828efbdf61fb5d5633b11024cb268bcf7b2116f94e`
- `availability_windows`: count `378`, SHA-256
  `11aa689bc2e4c36ea46c32d96c1b3c8a47c1ce7ba77c6b4b90f24c6be0845369`

These hashes match the pre-recovery values exactly.

## Honest remaining work

- The periodic ingest is restored to normal bounded backfill mode, but bounded
  backfill is still ongoing; this recovery did **not** claim every upstream
  YouTube upload has already been acquired.
- One local flat-playlist check against `mark-rober-shorts` still showed `126`
  remaining upstream shorts after the verified checkpoints, which is expected
  under the `25`-per-source periodic budget.
- The 12 conventional manifest items listed above still need ordinary upstream
  acquisition/presence before they can join the Collection.
