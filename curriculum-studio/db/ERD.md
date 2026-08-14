# Curriculum Studio ERD

All tables live in the `curriculum_studio` PostgreSQL schema. There are no
edges to Primer LMS or TV tables.

## Authorization and catalogs

```mermaid
erDiagram
    tenants ||--o{ workspaces : contains
    workspaces ||--o{ workspace_memberships : authorizes
    workspaces ||--o{ integration_identities : snapshots
    tenants ||--o{ resources : owns
    workspaces ||--o{ resources : optional
    workspaces ||--o{ standard_frameworks : custom
    standard_frameworks ||--o{ catalog_standards : contains
    catalog_standards ||--o{ catalog_standards : parent
    catalog_standards ||--o{ catalog_standard_prerequisites : requires
    catalog_standards ||--o{ standard_crosswalks : from
    catalog_standards ||--o{ standard_crosswalks : to

    tenants {
        uuid id PK
        text slug UK
        text status
    }
    workspaces {
        uuid id PK
        uuid tenant_id FK
        text slug
        text kind
    }
    workspace_memberships {
        uuid id PK
        uuid workspace_id FK
        text subject_ref
        text role
    }
    integration_identities {
        uuid id PK
        uuid workspace_id FK
        text system
        text external_kind
        text external_ref
        jsonb snapshot
    }
    standard_frameworks {
        uuid id PK
        uuid workspace_id FK
        text code
    }
    catalog_standards {
        uuid id PK
        uuid framework_id FK
        text code
    }
    resources {
        uuid id PK
        uuid tenant_id FK
        text kind
        text title
    }
```

## Plan graph

```mermaid
erDiagram
    workspaces ||--o{ curricula : authors
    curricula ||--o{ plan_revisions : versions
    curricula }o--o| plan_revisions : current_draft
    curricula }o--o| plan_revisions : published
    plan_revisions ||--o{ objectives : has
    plan_revisions ||--o{ outcomes : has
    objectives ||--o{ outcomes : decomposes
    outcomes ||--o{ outcome_standard_mappings : maps
    catalog_standards ||--o{ outcome_standard_mappings : referenced
    outcomes ||--o{ outcome_prerequisites : requires
    plan_revisions ||--o{ learning_arcs : sequences
    plan_revisions ||--o{ units : contains
    learning_arcs ||--o{ units : groups
    plan_revisions ||--o{ projects : contains
    units ||--o{ projects : hosts
    units ||--o{ unit_outcomes : targets
    projects ||--o{ project_outcomes : targets
    outcomes ||--o{ evidence_requirements : evidence
    plan_revisions ||--o{ scheduling_constraints : constrains
    plan_revisions ||--o{ plan_resources : cites
    resources ||--o{ plan_resources : used
    plan_revisions ||--o{ validation_reports : validates
    validation_reports ||--o{ validation_findings : lists

    plan_revisions {
        uuid id PK
        uuid curriculum_id FK
        int revision
        text status
        timestamptz published_at
        jsonb brief
    }
    outcomes {
        uuid id PK
        uuid plan_revision_id FK
        text code
        text mastery_criteria
    }
```

## Materialization, exports, events

```mermaid
erDiagram
    workspaces ||--o{ learner_profiles : profiles
    integration_identities ||--o{ learner_profiles : optional_link
    workspaces ||--o{ materialization_runs : requests
    plan_revisions ||--o{ materialization_runs : materializes
    learner_profiles ||--o{ materialization_runs : optional
    materialization_runs ||--o{ workflow_stages : stages
    workflow_stages ||--o{ workflow_attempts : attempts
    materialization_runs ||--o{ materialized_items : produces
    plan_revisions ||--o{ materialized_items : traces
    materialized_items ||--o{ materialized_items : supersedes
    materialized_items ||--o{ materialized_item_edits : edits
    materialized_items ||--o{ assessment_supports : assessment
    materialized_items ||--o{ assessment_supports : support
    workspaces ||--o{ exports : ships
    materialization_runs ||--o{ exports : optional
    workspaces ||--o{ outbox_events : emits
    workspaces ||--o{ webhook_endpoints : registers
    webhook_endpoints ||--o{ webhook_deliveries : delivers
    outbox_events ||--o{ webhook_deliveries : payload
    workspaces ||--o{ idempotency_keys : inbound
    workspaces ||--o{ audit_events : records

    materialization_runs {
        uuid id PK
        uuid plan_revision_id FK
        jsonb input_snapshot
        text input_fingerprint
        text status
    }
    materialized_items {
        uuid id PK
        uuid run_id FK
        text kind
        boolean locked
        text status
        jsonb provenance
    }
    webhook_deliveries {
        uuid id PK
        text idempotency_key UK
        text status
    }
```

## Isolation sketch

```text
┌─────────────────────────────┐     HTTP / events only      ┌──────────────────┐
│ Curriculum Studio database  │  ←  opaque subject_ref /    │ Primer LMS DB    │
│ schema curriculum_studio    │     external_ref snapshots  │ (separate)       │
└─────────────────────────────┘                             └──────────────────┘
              ▲
              │ no FK / view / FDW
              ▼
        ┌───────────┐
        │ TV DB     │
        └───────────┘
```

## Identity refs

`subject_ref` uses `identity:<uuid>` / `identity:svc:<id>`. `integration_identities.system` ∈ primer_lms, primer_identity, oidc, other. No edges to Identity or LMS databases.
