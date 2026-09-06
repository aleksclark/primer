-- Occurrences are historical records. Keep all display and scheduling inputs in
-- the occurrence snapshot so later task revisions and schedule edits cannot
-- rewrite what was issued to a student.
UPDATE task_occurrences o
SET revision_snapshot = revision_snapshot || jsonb_build_object(
  'title', r.title,
  'instructions', r.instructions,
  'taskRevisionVersion', r.version,
  'timezone', COALESCE(revision_snapshot->>'timezone', s.timezone),
  'dueOffsetMinutes', COALESCE((revision_snapshot->>'dueOffsetMinutes')::integer, s.due_offset_minutes),
  'dueSemantics', 'offset_from_nominal',
  'scheduleVersion', COALESCE((revision_snapshot->>'scheduleVersion')::integer, s.version)
)
FROM task_revisions r, task_schedules s
WHERE o.tenant_id = r.tenant_id
  AND o.tenant_id = s.tenant_id
  AND o.revision_id = r.id
  AND o.schedule_id = s.id
  AND (o.revision_snapshot->>'title' IS NULL
    OR o.revision_snapshot->>'instructions' IS NULL
    OR o.revision_snapshot->>'taskRevisionVersion' IS NULL
    OR o.revision_snapshot->>'dueSemantics' IS NULL);

ALTER TABLE task_occurrences
  ALTER COLUMN revision_snapshot SET DEFAULT '{"dueSemantics":"offset_from_nominal"}'::jsonb;
