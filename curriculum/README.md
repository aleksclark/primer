# Curriculum data

## Content manifest

`content-manifest.yaml` is the desired-state library for the Primer TV channel.
It is authored from the curated lists in `agent_docs/content_library/` and
converged by the `content-ingest` tool (see `agent_docs/plans/content-ingest.md`).
Runtime acquisition status (missing/present/failed/manual and attempt counts)
lives on the TV server in `content_manifest_entries`, updated by every
plan/apply run.

```bash
# From repo root, with INGEST_* env set (Radarr/Sonarr/Jellyfin/TV):
make ingest-plan    # diff + review.yaml candidates + TV catalog sync
make ingest-apply   # resolve → acquire → sync → import → report
# Fleet periodic job (every 6h): SERVICE=ingest ./deploy/deploy.sh
```

Human workload:

1. Edit the manifest (or recompile from content_library prose).
2. Answer `content-review.yaml` when resolve finds 0 or many catalog hits.
3. Rip DVDs listed under the manual queue / failed queue in each run report
   (or in the TV admin content-manifest list filtered by `status:failed`).
4. Everything else is automatic and idempotent.
