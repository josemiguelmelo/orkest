-- +goose Up
-- Early databases applied migration 0041 before rule actions were included.
-- Existing rules retain their original move behavior and an empty prompt.
ALTER TABLE task_trigger_rules
 ADD COLUMN IF NOT EXISTS action text NOT NULL DEFAULT 'move_task' CHECK(action IN ('move_task','start_work')),
 ADD COLUMN IF NOT EXISTS prompt_text text NOT NULL DEFAULT '' CHECK(length(prompt_text)<=32768);

-- Replace the original unconditional stage-change guard so start_work may stay
-- in the same stage. The constraint has this name on both versions of 0041.
ALTER TABLE task_trigger_rules DROP CONSTRAINT IF EXISTS task_trigger_rules_check;
ALTER TABLE task_trigger_rules ADD CONSTRAINT task_trigger_rules_check
 CHECK(action='start_work' OR source_stage_id<>target_stage_id);

-- +goose Down
-- Retain rule actions and prompts: they belong to the current 0041 schema.
-- Removing them would discard configuration and break current query code.
SELECT 1;
