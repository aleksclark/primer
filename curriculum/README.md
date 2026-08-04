# Curriculum data

## Learning activities

`activities/` holds versioned source definitions for student-client activities
(Phase 0+ of the command-teacher reimplementation). Each subdirectory is an
activity slug containing `activity.yaml` that matches the Go contracts in
`server/internal/studentclient/contracts`.

```bash
# Offline validate (schema, safe paths, fixture materialization, baseline checks):
make activity-validate

# Interactive summary list:
cd server && go run ./cmd/activity-validate --tui -dir ../curriculum/activities
```

Digital-literacy standard seeds live in `standards/digital-literacy.yaml`.
Database publish/reconcile of activities and standards is Phase 1.

## Content manifest

`content-manifest.yaml` is the desired-state library for the Primer TV channel.
It is authored from the curated lists in `agent_docs/content_library/` and
converged by the `content-ingest` tool (see `agent_docs/plans/content-ingest.md`).

```bash
# From repo root, with INGEST_* env set (Radarr/Sonarr/Jellyfin/TV):
make ingest-plan    # diff + review.yaml candidates
make ingest-apply   # resolve → acquire → sync → import → report
```

Human workload:

1. Edit the manifest (or recompile from content_library prose).
2. Answer `content-review.yaml` when resolve finds 0 or many catalog hits.
3. Rip DVDs listed under the manual queue in each run report.
4. Everything else is automatic and idempotent.
