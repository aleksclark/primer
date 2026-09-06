-- +goose Up
-- +goose StatementBegin
ALTER TABLE curriculum_studio.plan_comments ADD COLUMN item_id UUID REFERENCES curriculum_studio.materialized_items(id) ON DELETE CASCADE;
CREATE INDEX idx_studio_item_comments ON curriculum_studio.plan_comments(item_id,created_at,id) WHERE item_id IS NOT NULL;
CREATE FUNCTION curriculum_studio.check_item_comment_owner() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.item_id IS NOT NULL AND NOT EXISTS (
  SELECT 1 FROM curriculum_studio.materialized_items i JOIN curriculum_studio.materialization_runs m ON m.id=i.run_id
  JOIN curriculum_studio.plan_revisions r ON r.id=m.plan_revision_id JOIN curriculum_studio.curricula c ON c.id=r.curriculum_id
  WHERE i.id=NEW.item_id AND i.plan_revision_id=r.id AND r.id=NEW.plan_revision_id AND m.workspace_id=c.workspace_id AND c.workspace_id=NEW.workspace_id
 ) THEN RAISE EXCEPTION 'item comment ownership mismatch' USING ERRCODE='check_violation'; END IF;
 RETURN NEW;
END; $$;
CREATE TRIGGER item_comment_owner BEFORE INSERT OR UPDATE ON curriculum_studio.plan_comments FOR EACH ROW EXECUTE FUNCTION curriculum_studio.check_item_comment_owner();
-- Closed policy contract. Empty/missing keys retain the pre-S17 behavior.
ALTER TABLE curriculum_studio.workspace_policies ADD CONSTRAINT collaboration_policy_keys CHECK (
 policies - 'requireApprovalForPublish' - 'sharingEnabled' = '{}'::jsonb
 AND (NOT policies ? 'requireApprovalForPublish' OR jsonb_typeof(policies->'requireApprovalForPublish')='boolean')
 AND (NOT policies ? 'sharingEnabled' OR jsonb_typeof(policies->'sharingEnabled')='boolean')
);
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
ALTER TABLE curriculum_studio.workspace_policies DROP CONSTRAINT collaboration_policy_keys;
DROP TRIGGER item_comment_owner ON curriculum_studio.plan_comments;
DROP FUNCTION curriculum_studio.check_item_comment_owner();
ALTER TABLE curriculum_studio.plan_comments DROP COLUMN item_id;
-- +goose StatementEnd
