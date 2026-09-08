# Primer — agent guide

Primer is a family-directed, mastery-based homeschooling system. The parent is
the primary educator; AI supports instruction, practice, and evidence collection.
Precision, disciplined work, and craftsmanship matter across subjects. The
workstation is terminal-first; parent administration, curriculum authoring, Tasks,
and TV also have browser/native interfaces.

## Start here

- **Repository layout, build/test commands, and service ownership:** [README.md](README.md).
  This is a Go workspace, not a root Go module. `make test` covers `server/` only.
- **Authentication or service integration:** [authentication boundaries](agent_docs/authentication.md).
  Tasks uses Clerk/authstack; other services still consume Primer Identity.
  Do not infer a completed ecosystem migration from the Tasks cutover.
- **Studio:** [component guide](curriculum-studio/README.md),
  [contract ownership](curriculum-studio/contracts/OWNERS.md), and
  [migration policy](curriculum-studio/db/MIGRATION_POLICY.md).
- **Identity:** [component guide](primer-identity/README.md).
- **Android Student, Control, or TV:** [Android guide](android/README.md).
- **Workstation:** [packaging](workstation/README.md),
  [operations](agent_docs/runbooks/student-client-ops.md), and
  [terminal evidence limits](server/internal/studentclient/terminal/TRUST.md).
- **Agent runtime:** [standalone service operations](agent_docs/runbooks/agents-operations.md)
  and [LMS cutover](agent_docs/runbooks/phase7-cutover.md).
  The [local MAF preview](agent_docs/runbooks/maf-go-runtime.md) is a separate path.
- **Content ingest:** [current YouTube storage/recovery contract](agent_docs/runbooks/youtube-shows.md).
  Do not restore the historical nested `/media/tv/Primer` layout.
- **Local development:** host Make targets remain the default;
  [Compose](docs/dev-compose.md) is opt-in through `scripts/compose-dev.sh` / `paseo.json`.
- **Browser UI changes:** load the installed frontend house-system skill and
  consult [design-system/README.md](design-system/README.md).

## Engineering constraints

1. Services own separate PostgreSQL databases. No cross-database reads, FKs, or
   shared database credentials. Integrations use the owning service's public boundary.
2. Generate API clients from their owning contracts; do not hand-edit generated
   output or duplicate transport DTOs. Studio generated clients remain untracked.
3. Real persistence claims require real PostgreSQL tests. Coverage floors remain
   server/Studio/Agents/Tasks **85%**, Identity **80%**. Never lower gates or hide
   failed checks. Use the component's tests plus affected integration gates.
4. Student/device tokens remain product-local. AI/client evidence must not set
   mastery directly. Entertainment viewing is never LMS instructional time.
5. Never inspect, print, commit, or upload ignored secret files or credential
   values. Live-provider, device, deployment, and destructive operations require
   explicit authorization. No automatic PR merge or direct push to `master`.
6. Historical phase names, checklists, and branch-local handoffs are not current
   completion evidence. Check reachable source and exact-tip test receipts;
   distinguish implemented, reviewed, deployed, and physically/live accepted.
   Retain unresolved acceptance and rollback obligations when revising docs.

## Educational design references

Read only the relevant topic: [pedagogy](agent_docs/pedagogy.md),
[target tutoring model](agent_docs/architecture.md),
[curriculum](agent_docs/curriculum.md), [injectors](agent_docs/injectors.md),
[assessment](agent_docs/assessment.md), [projects](agent_docs/projects.md),
[TV concept](agent_docs/tv-channel.md), or [external tools](agent_docs/tools.md).
These describe educational intent; they are not a claim that every proposed
capability is implemented or accepted on a student's device.
