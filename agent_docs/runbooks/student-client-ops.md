# Student client operations

Household runbook for the Primer student workstation (`primer-student`) and the
parent-facing LMS controls that replace the standalone `command_teacher`
prototype.

## Pairing flow

1. Parent signs in to the LMS admin SPA (Student client → Overview) or calls
   `POST /api/v1/auth/login`.
2. Issue a one-time code: `POST /api/v1/pairing-codes` with `{ "studentId": "…" }`.
   The plaintext code is returned **once** and expires in 15 minutes.
3. On the workstation, pair:

   ```bash
   primer-student pair --code <CODE>
   # or during first boot via the broker/TUI pairing prompt
   ```

4. Confirm the device appears under Student client → Devices with a recent
   `lastSeenAt` after the client calls `/student/profile` or syncs work.

## Revoke / re-pair

| Goal | Action |
|---|---|
| Stop a lost/stolen workstation immediately | Devices → **Revoke**, or `POST /student-devices/{id}/revoke` |
| Rotate credentials without keeping the old token | Devices → **Rotate / re-pair**, or `POST /student-devices/{id}/rotate-token` |
| Fresh machine for the same student | Rotate (or revoke) then pair with the new code |

Rotate **revokes** the existing device row and returns a new pairing code. The
old Bearer device token fails with 401 on the next request. The client must
clear local credentials and pair again; do not try to re-inject the old token.

## Assign / cancel / retry work

- Assign a specific activity: Overview → **Assign basic-navigation**, or
  `POST /students/{id}/assign-next` with `{ "slug": "basic-navigation" }`.
- Cancel open work: Assignments → **Cancel**, or
  `POST /assignments/{id}/cancel`.
- Retry the same activity revision: Assignments → **Retry**, or
  `POST /assignments/{id}/retry`. Retry creates a **new** assignment row; cancel
  the old one when replacing open work.
- Parent mastery correction: `POST /mastery-evidence/{id}/supersede` with an
  optional note. This marks the original evidence and inserts replacement
  evidence; device event history is not rewritten.

## Tutor controls

- Per-student off switch: Overview → Tutor **Disable**, or
  `POST /students/{id}/tutor` with `{ "enabled": false }` (writes `tutor:off`
  into `student.notes`).
- Diagnostics: `GET /students/{id}/tutor-status` and the Overview tutor panel
  (provider name + process-local recent failure count).
- Deployment-wide gate remains server config (`TutorEnabled` / Bedrock); the
  notes marker only disables one student.

## Metrics

`GET /api/v1/ops/student-metrics` (parent session) returns:

- `devicesActive`, `devicesRevoked`
- `assignmentsOpen`, `sessionsActive`
- `completionsLast24h`
- `tutorFailuresLast24h` (process-local tutor policy counters when available)

These are simple SQL counts for household ops, not a Prometheus scrape target.

## Deploy / rollback (workstation)

Preferred path (Nix flake package):

```bash
cd workstation
./deploy.sh root@primer.local
```

The deploy wrapper builds on the management machine, activates with `test`,
schedules a five-minute automatic rollback, then cancels rollback only after
SSH health succeeds. If connectivity is lost mid-deploy, **wait five minutes**
before retrying so the previous generation can restore.

Manual recovery at the console: pick an older NixOS generation from the boot
menu.

Deprecated: `make student-deploy` (scp prebuilt binary). Keep
`make student-build` for local developer binaries only.

## Backup

### Server (LMS academic state)

- PostgreSQL is authoritative for students, devices, assignments, sessions,
  events, completions, mastery records, and evidence.
- Take logical dumps on a schedule the household can restore:

  ```bash
  pg_dump "$DATABASE_URL" -Fc -f "primer-lms-$(date -u +%Y%m%dT%H%M%SZ).dump"
  ```

- Restore into a clean database and re-run app migrations only if the dump is
  pre-migration; prefer dumps taken from a migrated schema.

### Workstation (broker / local state)

- Broker state and durable outbox live under the Primer student data dir on the
  impermanent host (see `workstation/hosts/workstation/primer-student.nix`).
- Persistent student projects survive reboot; system root does not.
- Before replacing a machine: revoke or rotate the old device, then pair the
  new workstation. Do **not** copy device tokens between hosts.

## Incident: duplicate completion

Completions are idempotent on `completionId` + request digest.

1. Confirm the session already has a completion row
   (`learning_session_completions`).
2. Client retries with the **same** completion UUID and digest must return the
   original result (HTTP 200), not a second mastery write.
3. A different digest for the same completion UUID is a conflict (HTTP 409) —
   inspect client outbox corruption; do not delete mastery rows by hand.
4. Parent corrections use supersede evidence, never silent DELETE of completion
   history.

## Acceptance matrix (physical workstation)

Record date, image/version, and pass/fail:

| # | Scenario | Pass? |
|---|---|---|
| 1 | Fresh pair with parent-issued code | |
| 2 | Online completion of `basic-navigation` | |
| 3 | Offline completion then sync | |
| 4 | Reboot mid-session; resume terminal | |
| 5 | Reboot mid-session; resume typing | |
| 6 | Server outage during event upload; recover | |
| 7 | Device revoke → client 401; re-pair works | |
| 8 | Deploy rollback restores previous generation | |
| 9 | Parent Overview shows session + mastery | |
| 10 | Duplicate completion retry does not double mastery | |

Automated smoke (API + headless engine) lives in
`scripts/student-acceptance.sh`. Physical rows above still require the Lenovo
workstation.

## Retire `command_teacher` checklist

Only after the acceptance matrix is recorded for the packaged broker/TUI:

- [ ] Stop launching `command_teacher` on student machines
- [ ] Remove desktop/autostart entries pointing at the prototype
- [ ] Archive prototype progress markdown as **notes only** (no auto-mastery import)
- [ ] Confirm Primer SPA Devices/Assignments/Sessions are the operational UI
- [ ] Keep `primer-student-stub` and `primer-student-harness` as **test-only**
      tools (see Makefile comments); do not ship them to the workstation image
- [ ] Update household docs/bookmarks to Primer LMS admin + `primer-student`

There is no runtime data migration from prototype markdown into mastery.
