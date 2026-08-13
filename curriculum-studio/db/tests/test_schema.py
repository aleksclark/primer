"""Apply and exercise the Curriculum Studio PostgreSQL schema.

The suite prefers a disposable Docker Postgres (postgres:17-alpine). Set
TEST_DATABASE_URL to use an existing throwaway database instead. When neither
Docker nor a URL is available, parser/static checks still run.
"""

from __future__ import annotations

import os
import re
import shutil
import socket
import subprocess
import time
import uuid
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[1]
MIGRATIONS = sorted((ROOT / "migrations").glob("*.sql"))
SCHEMA = "curriculum_studio"


def _split_goose(sql: str, section: str) -> str:
    marker = f"-- +goose {section}"
    if marker not in sql:
        raise AssertionError(f"missing {marker}")
    body = sql.split(marker, 1)[1]
    for other in ("-- +goose Up", "-- +goose Down"):
        if other == marker:
            continue
        nxt = body.find(other)
        if nxt != -1:
            body = body[:nxt]
    body = re.sub(r"-- \+goose StatementBegin\n?", "", body)
    body = re.sub(r"-- \+goose StatementEnd\n?", "", body)
    return body.strip()


def parse_up(path: Path) -> str:
    return _split_goose(path.read_text(), "Up")


def parse_down(path: Path) -> str:
    return _split_goose(path.read_text(), "Down")


def _docker_available() -> bool:
    return shutil.which("docker") is not None and subprocess.call(
        ["docker", "info"],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    ) == 0


def _free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def _wait_pg(dsn: dict, timeout: float = 30.0) -> None:
    import psycopg2

    deadline = time.time() + timeout
    last = None
    while time.time() < deadline:
        try:
            conn = psycopg2.connect(**dsn)
            conn.close()
            return
        except Exception as exc:  # noqa: BLE001 - retry until ready
            last = exc
            time.sleep(0.25)
    raise RuntimeError(f"postgres never became ready: {last}")


@pytest.fixture(scope="session")
def pg_url():
    env_url = os.environ.get("TEST_DATABASE_URL")
    if env_url:
        yield env_url
        return
    if not _docker_available():
        pytest.skip("neither TEST_DATABASE_URL nor Docker available")

    port = _free_port()
    name = f"curriculum-studio-schema-{uuid.uuid4().hex[:8]}"
    cmd = [
        "docker",
        "run",
        "-d",
        "--rm",
        "--name",
        name,
        "-e",
        "POSTGRES_USER=studio",
        "-e",
        "POSTGRES_PASSWORD=studio",
        "-e",
        "POSTGRES_DB=curriculum_studio_test",
        "-p",
        f"127.0.0.1:{port}:5432",
        "postgres:17-alpine",
    ]
    subprocess.check_call(cmd, stdout=subprocess.DEVNULL)
    dsn = {
        "host": "127.0.0.1",
        "port": port,
        "user": "studio",
        "password": "studio",
        "dbname": "curriculum_studio_test",
    }
    try:
        _wait_pg(dsn)
        yield f"postgres://studio:studio@127.0.0.1:{port}/curriculum_studio_test"
    finally:
        subprocess.call(["docker", "stop", "-t", "1", name], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


@pytest.fixture(scope="session")
def migrated(pg_url):
    import psycopg2

    conn = psycopg2.connect(pg_url)
    conn.autocommit = True
    with conn.cursor() as cur:
        for path in MIGRATIONS:
            cur.execute(parse_up(path))
    yield conn
    conn.close()


@pytest.fixture
def db(migrated):
    import psycopg2.extensions

    conn = migrated
    conn.autocommit = False
    with conn.cursor() as cur:
        cur.execute("SAVEPOINT schema_test")
    try:
        yield conn
    finally:
        conn.rollback()


def _adapt(params):
    if params is None:
        return None
    out = []
    for value in params:
        if isinstance(value, uuid.UUID):
            out.append(str(value))
        else:
            out.append(value)
    return tuple(out)


def execute(conn, sql, params=None):
    with conn.cursor() as cur:
        cur.execute(sql, _adapt(params))
        if cur.description:
            return list(cur.fetchall())
        return []


# ---------------------------------------------------------------------------
# Static / parser checks — always run, even without Postgres.
# ---------------------------------------------------------------------------

def test_migrations_are_goose_paired():
    assert MIGRATIONS, "expected numbered goose SQL files"
    names = [p.name for p in MIGRATIONS]
    assert names == [
        "00001_identity_and_catalogs.sql",
        "00002_plan_domain.sql",
        "00003_materialization_and_integration.sql",
        "00004_invariants.sql",
    ]
    for path in MIGRATIONS:
        text = path.read_text()
        assert "-- +goose Up" in text
        assert "-- +goose Down" in text
        assert parse_up(path)
        assert parse_down(path)


def _strip_sql_comments(sql: str) -> str:
    return re.sub(r"--[^\n]*", "", sql)


def test_schema_stays_inside_studio_namespace():
    forbidden = re.compile(
        r"\b(dblink|postgres_fdw|CREATE\s+EXTENSION|CREATE\s+SERVER|CREATE\s+FOREIGN)\b",
        re.I,
    )
    public_create = re.compile(r"CREATE\s+TABLE\s+(?!IF\s+NOT\s+EXISTS\s+)?(?!curriculum_studio\.)", re.I)
    for path in MIGRATIONS:
        up = _strip_sql_comments(parse_up(path))
        assert not forbidden.search(up), f"{path.name} must not open a cross-database path"
        for match in public_create.finditer(up):
            snippet = up[match.start() : match.start() + 80]
            if "CREATE TABLE curriculum_studio" in snippet:
                continue
            raise AssertionError(f"{path.name} creates a table outside curriculum_studio: {snippet}")


def test_no_credential_columns():
    banned = re.compile(r"\b(password|passwd|token_hash|password_hash|secret)\b", re.I)
    for path in MIGRATIONS:
        # secret_ref is an opaque pointer to an external secret store — allowed.
        text = re.sub(r"secret_ref", "", parse_up(path))
        assert not banned.search(text), f"{path.name} must not store credentials"


def test_docs_cover_inventory_and_invariants():
    schema = (ROOT / "SCHEMA.md").read_text()
    erd = (ROOT / "ERD.md").read_text()
    for name in (
        "tenants",
        "workspaces",
        "workspace_memberships",
        "plan_revisions",
        "materialization_runs",
        "materialized_items",
        "outbox_events",
        "webhook_deliveries",
        "audit_events",
    ):
        assert name in schema
        assert name in erd
    assert "Published plan immutability" in schema
    assert "Acyclic outcome prerequisites" in schema
    assert "Locked content protection" in schema
    assert "Assessment publication requires rubric/key" in schema


# ---------------------------------------------------------------------------
# Live schema tests
# ---------------------------------------------------------------------------

EXPECTED_TABLES = [
    "tenants",
    "workspaces",
    "workspace_memberships",
    "integration_identities",
    "standard_frameworks",
    "catalog_standards",
    "standard_crosswalks",
    "catalog_standard_prerequisites",
    "resources",
    "curricula",
    "plan_revisions",
    "objectives",
    "outcomes",
    "outcome_standard_mappings",
    "outcome_prerequisites",
    "learning_arcs",
    "units",
    "projects",
    "unit_outcomes",
    "project_outcomes",
    "evidence_requirements",
    "scheduling_constraints",
    "plan_resources",
    "validation_reports",
    "validation_findings",
    "learner_profiles",
    "materialization_runs",
    "workflow_stages",
    "workflow_attempts",
    "materialized_items",
    "materialized_item_edits",
    "assessment_supports",
    "exports",
    "outbox_events",
    "webhook_endpoints",
    "webhook_deliveries",
    "idempotency_keys",
    "audit_events",
]


def test_migrations_create_every_table(db):
    rows = execute(
        db,
        """
        SELECT table_name FROM information_schema.tables
        WHERE table_schema = %s
        ORDER BY table_name
        """,
        (SCHEMA,),
    )
    names = {r[0] for r in rows}
    missing = [t for t in EXPECTED_TABLES if t not in names]
    assert not missing, f"missing tables: {missing}"
    extra = names - set(EXPECTED_TABLES)
    assert not extra, f"unexpected tables: {sorted(extra)}"


def test_no_public_tables_created(db):
    rows = execute(
        db,
        """
        SELECT table_name FROM information_schema.tables
        WHERE table_schema = 'public'
          AND table_type = 'BASE TABLE'
        """,
    )
    assert rows == [] or rows is None or [r[0] for r in rows] == []


def seed_workspace(db):
    tenant_id = execute(
        db,
        "INSERT INTO curriculum_studio.tenants (slug, name) VALUES (%s, %s) RETURNING id",
        (f"t-{uuid.uuid4().hex[:8]}", "Tenant"),
    )[0][0]
    workspace_id = execute(
        db,
        """
        INSERT INTO curriculum_studio.workspaces (tenant_id, slug, name, kind)
        VALUES (%s, %s, %s, 'teacher') RETURNING id
        """,
        (tenant_id, f"ws-{uuid.uuid4().hex[:8]}", "Workspace"),
    )[0][0]
    return tenant_id, workspace_id


def seed_revision(db, workspace_id, status="draft"):
    curriculum_id = execute(
        db,
        """
        INSERT INTO curriculum_studio.curricula (workspace_id, slug, title)
        VALUES (%s, %s, %s) RETURNING id
        """,
        (workspace_id, f"alg-{uuid.uuid4().hex[:6]}", "Algebra I"),
    )[0][0]
    published_at = "now()" if status != "draft" else "NULL"
    revision_id = execute(
        db,
        f"""
        INSERT INTO curriculum_studio.plan_revisions
            (curriculum_id, revision, title, status, published_at)
        VALUES (%s, 1, 'Rev 1', %s, {published_at})
        RETURNING id
        """,
        (curriculum_id, status),
    )[0][0]
    return curriculum_id, revision_id


def test_membership_is_authorization_projection(db):
    _, workspace_id = seed_workspace(db)
    execute(
        db,
        """
        INSERT INTO curriculum_studio.workspace_memberships
            (workspace_id, subject_ref, role)
        VALUES (%s, %s, 'author')
        """,
        (workspace_id, "idp:user:abc"),
    )
    execute(
        db,
        """
        INSERT INTO curriculum_studio.integration_identities
            (workspace_id, system, external_kind, external_ref, snapshot)
        VALUES (%s, 'primer_lms', 'learner', 'learner_456', '{"grade": 6}'::jsonb)
        """,
        (workspace_id,),
    )


def test_published_revision_is_immutable(db):
    import psycopg2

    _, workspace_id = seed_workspace(db)
    _, revision_id = seed_revision(db, workspace_id, status="published")
    with pytest.raises(psycopg2.Error):
        execute(
            db,
            "UPDATE curriculum_studio.plan_revisions SET title = 'mutated' WHERE id = %s",
            (revision_id,),
        )
    db.rollback()
    # Need a fresh savepoint after rollback of the whole transaction? The fixture
    # rolls back at the end; after this exception the tx is aborted. Re-open.
    db.rollback()
    _, workspace_id = seed_workspace(db)
    _, revision_id = seed_revision(db, workspace_id, status="published")
    with pytest.raises(psycopg2.Error):
        execute(
            db,
            """
            INSERT INTO curriculum_studio.objectives (plan_revision_id, code, title)
            VALUES (%s, 'O1', 'Obj')
            """,
            (revision_id,),
        )


def test_draft_revision_accepts_plan_graph(db):
    tenant_id, workspace_id = seed_workspace(db)
    _, revision_id = seed_revision(db, workspace_id, status="draft")
    obj_id = execute(
        db,
        """
        INSERT INTO curriculum_studio.objectives (plan_revision_id, code, title)
        VALUES (%s, 'O1', 'Reason proportionally') RETURNING id
        """,
        (revision_id,),
    )[0][0]
    out_a = execute(
        db,
        """
        INSERT INTO curriculum_studio.outcomes
            (plan_revision_id, objective_id, code, title)
        VALUES (%s, %s, 'OUT.A', 'Solve ratios') RETURNING id
        """,
        (revision_id, obj_id),
    )[0][0]
    out_b = execute(
        db,
        """
        INSERT INTO curriculum_studio.outcomes
            (plan_revision_id, objective_id, code, title)
        VALUES (%s, %s, 'OUT.B', 'Apply unit rates') RETURNING id
        """,
        (revision_id, obj_id),
    )[0][0]
    execute(
        db,
        """
        INSERT INTO curriculum_studio.outcome_prerequisites
            (plan_revision_id, outcome_id, prerequisite_id)
        VALUES (%s, %s, %s)
        """,
        (revision_id, out_b, out_a),
    )
    arc_id = execute(
        db,
        """
        INSERT INTO curriculum_studio.learning_arcs (plan_revision_id, code, title)
        VALUES (%s, 'ARC1', 'Ratios') RETURNING id
        """,
        (revision_id,),
    )[0][0]
    unit_id = execute(
        db,
        """
        INSERT INTO curriculum_studio.units (plan_revision_id, learning_arc_id, code, title)
        VALUES (%s, %s, 'U1', 'Ratio unit') RETURNING id
        """,
        (revision_id, arc_id),
    )[0][0]
    execute(
        db,
        "INSERT INTO curriculum_studio.unit_outcomes (unit_id, outcome_id) VALUES (%s, %s)",
        (unit_id, out_a),
    )
    execute(
        db,
        """
        INSERT INTO curriculum_studio.evidence_requirements
            (plan_revision_id, outcome_id, kind, description)
        VALUES (%s, %s, 'formal', 'Quiz')
        """,
        (revision_id, out_a),
    )
    execute(
        db,
        """
        INSERT INTO curriculum_studio.scheduling_constraints
            (plan_revision_id, kind, payload)
        VALUES (%s, 'available_minutes', '{"minutes": 600}'::jsonb)
        """,
        (revision_id,),
    )
    res_id = execute(
        db,
        """
        INSERT INTO curriculum_studio.resources (tenant_id, workspace_id, kind, title)
        VALUES (%s, %s, 'book', 'Euclid') RETURNING id
        """,
        (tenant_id, workspace_id),
    )[0][0]
    execute(
        db,
        """
        INSERT INTO curriculum_studio.plan_resources (plan_revision_id, resource_id, role)
        VALUES (%s, %s, 'required')
        """,
        (revision_id, res_id),
    )


def test_outcome_prerequisites_reject_cycles(db):
    import psycopg2

    _, workspace_id = seed_workspace(db)
    _, revision_id = seed_revision(db, workspace_id)
    a = execute(
        db,
        """
        INSERT INTO curriculum_studio.outcomes (plan_revision_id, code, title)
        VALUES (%s, 'A', 'A') RETURNING id
        """,
        (revision_id,),
    )[0][0]
    b = execute(
        db,
        """
        INSERT INTO curriculum_studio.outcomes (plan_revision_id, code, title)
        VALUES (%s, 'B', 'B') RETURNING id
        """,
        (revision_id,),
    )[0][0]
    execute(
        db,
        """
        INSERT INTO curriculum_studio.outcome_prerequisites
            (plan_revision_id, outcome_id, prerequisite_id)
        VALUES (%s, %s, %s)
        """,
        (revision_id, b, a),
    )
    with pytest.raises(psycopg2.Error):
        execute(
            db,
            """
            INSERT INTO curriculum_studio.outcome_prerequisites
                (plan_revision_id, outcome_id, prerequisite_id)
            VALUES (%s, %s, %s)
            """,
            (revision_id, a, b),
        )


def test_catalog_prerequisites_reject_cycles(db):
    import psycopg2

    fw = execute(
        db,
        """
        INSERT INTO curriculum_studio.standard_frameworks (code, name, jurisdiction)
        VALUES ('TN', 'Tennessee', 'TN') RETURNING id
        """,
    )[0][0]
    s1 = execute(
        db,
        """
        INSERT INTO curriculum_studio.catalog_standards (framework_id, code, description)
        VALUES (%s, 'TN.A', 'A') RETURNING id
        """,
        (fw,),
    )[0][0]
    s2 = execute(
        db,
        """
        INSERT INTO curriculum_studio.catalog_standards (framework_id, code, description)
        VALUES (%s, 'TN.B', 'B') RETURNING id
        """,
        (fw,),
    )[0][0]
    execute(
        db,
        """
        INSERT INTO curriculum_studio.catalog_standard_prerequisites
            (standard_id, prerequisite_id) VALUES (%s, %s)
        """,
        (s2, s1),
    )
    with pytest.raises(psycopg2.Error):
        execute(
            db,
            """
            INSERT INTO curriculum_studio.catalog_standard_prerequisites
                (standard_id, prerequisite_id) VALUES (%s, %s)
            """,
            (s1, s2),
        )


def _seed_run(db):
    _, workspace_id = seed_workspace(db)
    _, revision_id = seed_revision(db, workspace_id, status="published")
    profile_id = execute(
        db,
        """
        INSERT INTO curriculum_studio.learner_profiles
            (workspace_id, kind, label, profile)
        VALUES (%s, 'class', 'Grade 6', '{"hours": 5}'::jsonb)
        RETURNING id
        """,
        (workspace_id,),
    )[0][0]
    run_id = execute(
        db,
        """
        INSERT INTO curriculum_studio.materialization_runs
            (workspace_id, plan_revision_id, learner_profile_id,
             status, input_snapshot, input_fingerprint)
        VALUES (%s, %s, %s, 'running',
                '{"plan_revision_id": "x", "window": {"start": "2026-09-14"}}'::jsonb,
                'sha256:deadbeef')
        RETURNING id
        """,
        (workspace_id, revision_id, profile_id),
    )[0][0]
    return workspace_id, revision_id, run_id


def test_materialization_records_snapshot_and_stages(db):
    workspace_id, revision_id, run_id = _seed_run(db)
    stage_id = execute(
        db,
        """
        INSERT INTO curriculum_studio.workflow_stages
            (run_id, stage_key, position, status)
        VALUES (%s, 'assessment_generation', 1, 'failed')
        RETURNING id
        """,
        (run_id,),
    )[0][0]
    execute(
        db,
        """
        INSERT INTO curriculum_studio.workflow_attempts
            (stage_id, attempt_number, status, agent, error)
        VALUES (%s, 1, 'failed', 'Assessment Designer', 'timeout')
        """,
        (stage_id,),
    )
    item_id = execute(
        db,
        """
        INSERT INTO curriculum_studio.materialized_items
            (run_id, plan_revision_id, kind, title, body, status, provenance)
        VALUES (%s, %s, 'lesson', 'Day 1', '{"md": "..."}'::jsonb, 'ready',
                '{"agent": "Materializer"}'::jsonb)
        RETURNING id
        """,
        (run_id, revision_id),
    )[0][0]
    execute(
        db,
        """
        INSERT INTO curriculum_studio.materialized_item_edits
            (item_id, editor_subject_ref, patch)
        VALUES (%s, 'idp:user:abc', '{"title": "Day 1 revised"}'::jsonb)
        """,
        (item_id,),
    )
    execute(
        db,
        """
        INSERT INTO curriculum_studio.exports
            (workspace_id, run_id, plan_revision_id, format, status)
        VALUES (%s, %s, %s, 'markdown', 'requested')
        """,
        (workspace_id, run_id, revision_id),
    )


def test_locked_item_cannot_be_overwritten(db):
    import psycopg2

    _, revision_id, run_id = _seed_run(db)
    item_id = execute(
        db,
        """
        INSERT INTO curriculum_studio.materialized_items
            (run_id, plan_revision_id, kind, title, body, locked, locked_at)
        VALUES (%s, %s, 'lesson', 'Locked', '{}'::jsonb, TRUE, now())
        RETURNING id
        """,
        (run_id, revision_id),
    )[0][0]
    with pytest.raises(psycopg2.Error):
        execute(
            db,
            "UPDATE curriculum_studio.materialized_items SET title = 'nope' WHERE id = %s",
            (item_id,),
        )


def test_assessment_publish_requires_rubric_or_key(db):
    import psycopg2

    _, revision_id, run_id = _seed_run(db)
    assess_id = execute(
        db,
        """
        INSERT INTO curriculum_studio.materialized_items
            (run_id, plan_revision_id, kind, title, status)
        VALUES (%s, %s, 'assessment', 'Quiz', 'draft')
        RETURNING id
        """,
        (run_id, revision_id),
    )[0][0]
    with pytest.raises(psycopg2.Error):
        execute(
            db,
            "UPDATE curriculum_studio.materialized_items SET status = 'published' WHERE id = %s",
            (assess_id,),
        )
    db.rollback()
    _, revision_id, run_id = _seed_run(db)
    assess_id = execute(
        db,
        """
        INSERT INTO curriculum_studio.materialized_items
            (run_id, plan_revision_id, kind, title, status)
        VALUES (%s, %s, 'assessment', 'Quiz', 'draft')
        RETURNING id
        """,
        (run_id, revision_id),
    )[0][0]
    rubric_id = execute(
        db,
        """
        INSERT INTO curriculum_studio.materialized_items
            (run_id, plan_revision_id, kind, title, status)
        VALUES (%s, %s, 'rubric', 'Rubric', 'ready')
        RETURNING id
        """,
        (run_id, revision_id),
    )[0][0]
    execute(
        db,
        """
        INSERT INTO curriculum_studio.assessment_supports
            (assessment_item_id, support_item_id)
        VALUES (%s, %s)
        """,
        (assess_id, rubric_id),
    )
    execute(
        db,
        "UPDATE curriculum_studio.materialized_items SET status = 'published' WHERE id = %s",
        (assess_id,),
    )


def test_webhook_delivery_idempotency(db):
    import psycopg2

    _, workspace_id = seed_workspace(db)
    event_id = execute(
        db,
        """
        INSERT INTO curriculum_studio.outbox_events
            (workspace_id, event_type, aggregate_kind, aggregate_id, payload)
        VALUES (%s, 'plan_revision.published', 'plan_revision', %s, '{}'::jsonb)
        RETURNING id
        """,
        (workspace_id, uuid.uuid4()),
    )[0][0]
    endpoint_id = execute(
        db,
        """
        INSERT INTO curriculum_studio.webhook_endpoints (workspace_id, url)
        VALUES (%s, 'https://primer.example/hooks') RETURNING id
        """,
        (workspace_id,),
    )[0][0]
    key = f"wh-{endpoint_id}-{event_id}"
    execute(
        db,
        """
        INSERT INTO curriculum_studio.webhook_deliveries
            (endpoint_id, event_id, idempotency_key)
        VALUES (%s, %s, %s)
        """,
        (endpoint_id, event_id, key),
    )
    with pytest.raises(psycopg2.Error):
        execute(
            db,
            """
            INSERT INTO curriculum_studio.webhook_deliveries
                (endpoint_id, event_id, idempotency_key)
            VALUES (%s, %s, %s)
            """,
            (endpoint_id, event_id, key + "-dup"),
        )


def test_audit_and_inbound_idempotency(db):
    import psycopg2

    _, workspace_id = seed_workspace(db)
    execute(
        db,
        """
        INSERT INTO curriculum_studio.audit_events
            (workspace_id, actor_subject_ref, action, entity_kind, entity_id)
        VALUES (%s, 'idp:user:abc', 'publish', 'plan_revision', %s)
        """,
        (workspace_id, uuid.uuid4()),
    )
    execute(
        db,
        """
        INSERT INTO curriculum_studio.idempotency_keys
            (workspace_id, scope, key)
        VALUES (%s, 'materialize', 'req-1')
        """,
        (workspace_id,),
    )
    with pytest.raises(psycopg2.Error):
        execute(
            db,
            """
            INSERT INTO curriculum_studio.idempotency_keys
                (workspace_id, scope, key)
            VALUES (%s, 'materialize', 'req-1')
            """,
            (workspace_id,),
        )
