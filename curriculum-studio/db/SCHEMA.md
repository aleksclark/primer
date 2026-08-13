# Curriculum Studio database schema

Standalone PostgreSQL schema for Curriculum Studio. This is **not** a Primer
LMS or TV migration. The service owns a completely separate database.

Suggested database name: `curriculum_studio`.

If this schema ever shares a PostgreSQL *instance* with another Primer service,
use a dedicated goose version table (`studio_goose_db_version`) and keep the
`curriculum_studio` Postgres schema so public-table names cannot collide.

## Isolation rules

- No foreign keys, views, materialized views, or FDW/dblink reads into LMS/TV.
- Primer learner, class, educator, and auth-subject identifiers are stored only
  as opaque `TEXT` (`subject_ref`, `external_ref`) plus optional JSON snapshots.
- Memberships are authorization projections. Credentials are never stored here.

## Subject and external identity conventions

| Field | Canonical form | Owner |
| --- | --- | --- |
| `workspace_memberships.subject_ref` (human) | `identity:<uuid>` | UUID is Primer Identity account `sub` |
| `workspace_memberships.subject_ref` (service) | `identity:svc:<id>` | Identity service principal id |
| `*_subject_ref` columns generally | same opaque text convention | Never FK to Identity/LMS DBs |
| `integration_identities.system` | `primer_lms` \| `primer_identity` \| `oidc` \| `other` | Snapshot source system |
| `integration_identities.external_ref` | opaque foreign id | LMS learner id, OIDC sub, etc. |

`primer_identity` is preferred when the snapshot is an Identity-issued
subject. `oidc` remains for generic/external OIDC provider snapshots that
are not routed through Primer Identity. Memberships store **authorization
projections only** — no passwords, token hashes, or secrets
(`secret_ref` pointer only if ever needed).

See `agent_docs/plans/curriculum-studio-foundation-crosswalk.md`.

## Item lifecycle vs lock

| Dimension | Storage | API/proto |
| --- | --- | --- |
| Lifecycle | `materialized_items.status` ∈ draft/ready/published/superseded | `MaterializedItemStatus` |
| Lock | `locked` boolean + `locked_at` + `locked_by_subject_ref` | `ItemLockState` editable\|locked |

Locked items cannot be overwritten by rematerialize; unlock is explicit.

## Enumerations (CHECK constraints)

PostgreSQL enums are avoided so values can evolve with CHECK + migration. The
closed sets below are the contract:

| Column / table | Values |
| --- | --- |
| `tenants.status` | `active`, `suspended`, `retired` |
| `workspaces.kind` | `school`, `family`, `coop`, `teacher`, `organization` |
| `workspaces.status` | `active`, `archived` |
| `workspace_memberships.subject_kind` | `human`, `service` |
| `workspace_memberships.role` | `owner`, `admin`, `author`, `reviewer`, `viewer` |
| `workspace_memberships.status` | `active`, `invited`, `revoked` |
| `integration_identities.system` | `primer_lms`, `primer_identity`, `oidc`, `other` |
| `integration_identities.external_kind` | `learner`, `educator`, `class`, `auth_subject`, `service` |
| `standard_crosswalks.relationship` | `equivalent`, `broader`, `narrower`, `related` |
| `resources.kind` | `book`, `document`, `video`, `tool`, `project_supply`, `url`, `other` |
| `curricula.approach` | `mastery_based`, `spiral`, `classical`, `unit_study`, `project_based`, `custom` |
| `curricula.status` | `draft`, `active`, `retired` (API `archived` ⇔ `retired`; see crosswalk) |
| `plan_revisions.status` | `draft`, `published`, `superseded` |
| `outcome_standard_mappings.alignment` | `addresses`, `assesses`, `introduces`, `reinforces` |
| `outcome_prerequisites.requirement` | `introduced`, `completed`, `mastered` |
| `unit_outcomes.role` / `project_outcomes.role` | `target`, `prior`, `stretch` |
| `evidence_requirements.kind` | `continuous`, `formal`, `project`, `portfolio`, `discussion`, `performance` |
| `scheduling_constraints.kind` | `available_minutes`, `calendar_window`, `blackout`, `testing_window`, `philosophy`, `non_negotiable`, `workload_cap` |
| `plan_resources.role` | `required`, `optional`, `teacher`, `extension` |
| `validation_reports.status` | `pending`, `passed`, `failed`, `warning` |
| `validation_findings.severity` | `error`, `warning`, `info` |
| `learner_profiles.kind` | `learner`, `class` |
| `materialization_runs.status` | `requested`, `running`, `ready`, `failed`, `cancelled` |
| `workflow_stages.status` | `pending`, `running`, `succeeded`, `failed`, `skipped` |
| `workflow_attempts.status` | `running`, `succeeded`, `failed` |
| `materialized_items.kind` | `lesson`, `teacher_guide`, `student_instructions`, `practice`, `assignment`, `discussion_guide`, `worksheet`, `assessment`, `rubric`, `project_task`, `answer_key`, `media_prompt`, `printable_packet`, `session_spec` |
| `materialized_items.status` | `draft`, `ready`, `published`, `superseded` |
| `exports.format` | `pdf`, `markdown`, `docx`, `csv`, `json`, `ical` |
| `exports.status` | `requested`, `ready`, `failed` |
| `webhook_endpoints.status` | `active`, `paused`, `retired` |
| `webhook_deliveries.status` | `pending`, `delivered`, `failed` |

## Table inventory

### Authorization and isolation

| Table | Purpose |
| --- | --- |
| `tenants` | Billing / org root |
| `workspaces` | School, family, co-op, teacher, or authoring org |
| `workspace_memberships` | Role projections keyed by opaque `subject_ref` |
| `integration_identities` | Bounded snapshots of external Primer / IdP refs |

### Catalogs

| Table | Purpose |
| --- | --- |
| `standard_frameworks` | Shared or workspace-owned standards catalogs |
| `catalog_standards` | Hierarchical standards inside a framework |
| `standard_crosswalks` | Cross-framework mappings |
| `catalog_standard_prerequisites` | Catalog-level prerequisite DAG |
| `resources` | Book / media / tool / URL metadata (no file blobs) |

### Plan domain

| Table | Purpose |
| --- | --- |
| `curricula` | Durable curriculum identity |
| `plan_revisions` | Versioned plan; immutable once published |
| `objectives` | High-level objectives on a revision |
| `outcomes` | Observable outcomes + mastery criteria |
| `outcome_standard_mappings` | Outcome ↔ catalog standard |
| `outcome_prerequisites` | Prerequisite DAG among outcomes |
| `learning_arcs` | Scope-and-sequence groupings |
| `units` | Unit blueprints |
| `projects` | Project blueprints and phases |
| `unit_outcomes` / `project_outcomes` | Outcome membership |
| `evidence_requirements` | Required evidence per outcome |
| `scheduling_constraints` | Hours, windows, philosophy, caps |
| `plan_resources` | Resource attachments on a revision |
| `validation_reports` / `validation_findings` | Coverage / graph / evidence reports |

### Materialization and integration

| Table | Purpose |
| --- | --- |
| `learner_profiles` | Generic learner or class profile |
| `materialization_runs` | Production run + complete input snapshot + fingerprint |
| `workflow_stages` | Resumable generation stages |
| `workflow_attempts` | Per-stage attempts |
| `materialized_items` | Generated lessons / assessments / guides |
| `materialized_item_edits` | Author edit history |
| `assessment_supports` | Assessment ↔ rubric / answer key |
| `exports` | PDF / Markdown / DOCX / CSV / JSON / iCal |
| `outbox_events` | Durable domain events |
| `webhook_endpoints` / `webhook_deliveries` | Delivery + idempotency |
| `idempotency_keys` | Inbound API / callback idempotency |
| `audit_events` | Authoring and integration audit trail |

## Invariant enforcement map

| Invariant | Enforcement |
| --- | --- |
| Separate database / no LMS FKs | Dedicated `curriculum_studio` schema; no objects outside it; opaque text refs only |
| Published plan immutability | `plan_revisions` trigger blocks UPDATE/DELETE except `published → superseded`; child-table triggers block INSERT/UPDATE/DELETE |
| Acyclic outcome prerequisites | BEFORE INSERT/UPDATE recursive walk on `outcome_prerequisites` |
| Acyclic catalog prerequisites | BEFORE INSERT/UPDATE recursive walk on `catalog_standard_prerequisites` |
| Prerequisites stay in-revision | Trigger compares both outcomes' `plan_revision_id` |
| Locked content protection | Trigger on `materialized_items` + insert block on `materialized_item_edits` |
| Referential integrity | Foreign keys throughout; `ON DELETE RESTRICT` on published-adjacent catalog/resource refs |
| Idempotent webhook delivery | `UNIQUE (endpoint_id, event_id)` and `UNIQUE (idempotency_key)` |
| Inbound idempotency | `UNIQUE (workspace_id, scope, key)` on `idempotency_keys` |
| Complete materialization input | `input_snapshot` NOT NULL object + `input_fingerprint` NOT NULL; index on `(plan_revision_id, input_fingerprint)` |
| Assessment publication requires rubric/key | Trigger on `materialized_items` when `kind=assessment` and `status=published`; `assessment_supports` kind check |
| Memberships are not credentials | No password/token/hash columns; only `subject_ref` + role |
| Generated items trace to a plan node | `plan_revision_id` NOT NULL + optional `unit_id` / `project_id` / `outcome_id`; run/revision match trigger |

Application-layer validation still owns richer coverage rules (every outcome
has evidence, superficial standards, workload). Those produce
`validation_reports` / `validation_findings` rather than hard CHECKs, because
they are multi-row and policy-tunable.

## Domain events (outbox `event_type`)

Suggested values matching the product plan:

- `curriculum.created`
- `plan_revision.published`
- `materialization.requested`
- `materialization.ready`
- `materialization.failed`
- `materialized_item.superseded`
- `plan_change.proposed`

## Migrations

Goose SQL, numbered:

1. `00001_identity_and_catalogs.sql`
2. `00002_plan_domain.sql`
3. `00003_materialization_and_integration.sql`
4. `00004_invariants.sql`
