-- PRD v1.1: task dueAt required; stage remindAt optional
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS due_at TIMESTAMPTZ;
UPDATE tasks SET due_at = created_at + INTERVAL '7 days' WHERE due_at IS NULL;
ALTER TABLE tasks ALTER COLUMN due_at SET NOT NULL;

ALTER TABLE stages ADD COLUMN IF NOT EXISTS remind_at TIMESTAMPTZ;
