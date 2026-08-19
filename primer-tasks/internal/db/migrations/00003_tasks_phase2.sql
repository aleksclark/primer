CREATE TABLE IF NOT EXISTS task_templates (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL REFERENCES tenants(id), title text NOT NULL,
 status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','published','retired')),
 current_revision integer NOT NULL DEFAULT 0, created_at timestamptz NOT NULL DEFAULT now(), retired_at timestamptz,
 UNIQUE(tenant_id,id)
);
CREATE INDEX IF NOT EXISTS task_templates_tenant_status ON task_templates(tenant_id,status,created_at DESC);
CREATE TABLE IF NOT EXISTS task_revisions (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL, template_id uuid NOT NULL REFERENCES task_templates(id),
 version integer NOT NULL, title text NOT NULL, instructions text NOT NULL DEFAULT '', status text NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','published','retired')),
 created_at timestamptz NOT NULL DEFAULT now(), published_at timestamptz, UNIQUE(tenant_id,template_id,version), UNIQUE(tenant_id,id),
 FOREIGN KEY(tenant_id,template_id) REFERENCES task_templates(tenant_id,id)
);
CREATE TABLE IF NOT EXISTS verification_requirements (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL, revision_id uuid NOT NULL REFERENCES task_revisions(id), ordinal integer NOT NULL,
 kind text NOT NULL, config_version integer NOT NULL, config jsonb NOT NULL DEFAULT '{}', interaction text NOT NULL, executor text NOT NULL,
 UNIQUE(tenant_id,revision_id,ordinal), UNIQUE(tenant_id,id), FOREIGN KEY(tenant_id,revision_id) REFERENCES task_revisions(tenant_id,id)
);
CREATE TABLE IF NOT EXISTS task_schedules (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL REFERENCES tenants(id), student_id uuid NOT NULL, template_id uuid NOT NULL,
 revision_id uuid NOT NULL, kind text NOT NULL CHECK(kind IN ('one_off','recurrence')), timezone text NOT NULL,
 start_local timestamptz NOT NULL, end_local timestamptz, rrule text NOT NULL DEFAULT '', due_offset_minutes integer NOT NULL DEFAULT 0,
 enabled boolean NOT NULL DEFAULT true, version integer NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(), retired_at timestamptz,
 UNIQUE(tenant_id,id),
 FOREIGN KEY(tenant_id,student_id) REFERENCES students(tenant_id,id), FOREIGN KEY(tenant_id,template_id) REFERENCES task_templates(tenant_id,id),
 FOREIGN KEY(tenant_id,revision_id) REFERENCES task_revisions(tenant_id,id)
);
CREATE INDEX IF NOT EXISTS task_schedules_materialize ON task_schedules(tenant_id,enabled,start_local);
CREATE TABLE IF NOT EXISTS task_occurrences (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL REFERENCES tenants(id), schedule_id uuid NOT NULL, student_id uuid NOT NULL,
 revision_id uuid NOT NULL, nominal_at timestamptz NOT NULL, due_at timestamptz NOT NULL, status text NOT NULL DEFAULT 'pending'
 CHECK(status IN ('pending','in_progress','awaiting_verification','completed','excused','canceled')),
 revision_snapshot jsonb NOT NULL DEFAULT '{}', lease_until timestamptz, lease_owner text, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(tenant_id,id),
 UNIQUE(tenant_id,schedule_id,nominal_at), FOREIGN KEY(tenant_id,schedule_id) REFERENCES task_schedules(tenant_id,id),
 FOREIGN KEY(tenant_id,student_id) REFERENCES students(tenant_id,id), FOREIGN KEY(tenant_id,revision_id) REFERENCES task_revisions(tenant_id,id)
);
CREATE INDEX IF NOT EXISTS task_occurrences_student_due ON task_occurrences(tenant_id,student_id,nominal_at);
CREATE TABLE IF NOT EXISTS verification_attempts (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL REFERENCES tenants(id), occurrence_id uuid NOT NULL, requirement_id uuid NOT NULL,
 number integer NOT NULL, status text NOT NULL DEFAULT 'open' CHECK(status IN ('open','accepted','rejected','exhausted')),
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(tenant_id,occurrence_id,requirement_id,number), UNIQUE(tenant_id,id),
 FOREIGN KEY(tenant_id,occurrence_id) REFERENCES task_occurrences(tenant_id,id), FOREIGN KEY(tenant_id,requirement_id) REFERENCES verification_requirements(tenant_id,id)
);
CREATE TABLE IF NOT EXISTS verification_submissions (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL REFERENCES tenants(id), attempt_id uuid NOT NULL, submitted_by text NOT NULL,
 payload jsonb NOT NULL DEFAULT '{}', created_at timestamptz NOT NULL DEFAULT now(), FOREIGN KEY(tenant_id,attempt_id) REFERENCES verification_attempts(tenant_id,id)
);
CREATE TABLE IF NOT EXISTS verification_decisions (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL REFERENCES tenants(id), attempt_id uuid NOT NULL, accepted boolean NOT NULL,
 reason text NOT NULL DEFAULT '', decided_by text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(tenant_id,attempt_id), FOREIGN KEY(tenant_id,attempt_id) REFERENCES verification_attempts(tenant_id,id)
);
CREATE TABLE IF NOT EXISTS task_materializer_leases (
 tenant_id uuid PRIMARY KEY REFERENCES tenants(id), owner text NOT NULL, lease_until timestamptz NOT NULL, updated_at timestamptz NOT NULL DEFAULT now()
);
