-- +goose Up
-- +goose StatementBegin

-- Evidence taxonomy: classify mastery evidence and attach immutable provenance.
-- Historic rows are labeled procedural_continuous; confidence/status aggregates
-- are intentionally left unchanged (they may have been computed under the
-- earlier unconditional-bump policy and need parent review, not silent rewrite).
ALTER TABLE mastery_evidence
    ADD COLUMN evidence_class TEXT NOT NULL DEFAULT 'procedural_continuous',
    ADD COLUMN provenance JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN policy_version INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN migration_note TEXT NOT NULL DEFAULT '';

ALTER TABLE mastery_evidence
    ADD CONSTRAINT mastery_evidence_class_check
    CHECK (evidence_class IN (
        'procedural_continuous',
        'conceptual_response',
        'parent_attestation',
        'formal_assessment',
        'portfolio'
    ));

UPDATE mastery_evidence
SET evidence_class = 'procedural_continuous',
    migration_note = 'phase-0: preserved pre-taxonomy evidence as procedural_continuous; historic mastery status/confidence not rewritten',
    provenance = jsonb_build_object(
        'migratedFrom', 'pre-taxonomy',
        'legacyKind', kind,
        'sourceRef', source_ref
    )
WHERE migration_note = '';

CREATE INDEX idx_mastery_evidence_class ON mastery_evidence(evidence_class);

-- Per revision-standard link: which evidence classes are required before a
-- status transition. Default v1 terminal policy: procedural_continuous alone
-- may advance not_introduced → in_progress and must not independently establish
-- approaching/mastered.
ALTER TABLE learning_activity_revision_standards
    ADD COLUMN evidence_policy JSONB NOT NULL DEFAULT '{
        "version": 1,
        "statusRequirements": {
            "in_progress": ["procedural_continuous"],
            "approaching": ["procedural_continuous", "conceptual_response"],
            "mastered": ["procedural_continuous", "conceptual_response", "formal_assessment"]
        }
    }'::jsonb;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE learning_activity_revision_standards
    DROP COLUMN IF EXISTS evidence_policy;

DROP INDEX IF EXISTS idx_mastery_evidence_class;

ALTER TABLE mastery_evidence
    DROP CONSTRAINT IF EXISTS mastery_evidence_class_check;

ALTER TABLE mastery_evidence
    DROP COLUMN IF EXISTS migration_note,
    DROP COLUMN IF EXISTS policy_version,
    DROP COLUMN IF EXISTS provenance,
    DROP COLUMN IF EXISTS evidence_class;

-- +goose StatementEnd
