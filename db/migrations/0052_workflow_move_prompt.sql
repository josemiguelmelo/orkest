-- +goose Up
ALTER TABLE workflow_rules DROP CONSTRAINT workflow_rules_check;
ALTER TABLE workflow_rules ADD CONSTRAINT workflow_rules_check CHECK(
 (event_type='stage_entered' AND action_type='start_work' AND target_stage_id IS NULL AND integration IS NULL AND prompt_text='' AND session_policy='') OR
 (event_type IN ('run_succeeded','stage_completed') AND action_type='move_task' AND target_stage_id IS NOT NULL AND target_stage_id<>source_stage_id AND integration IS NULL AND prompt_text='' AND session_policy='') OR
 (event_type='integration_action' AND jsonb_typeof(integration)='object' AND COALESCE(integration->>'connectionId','')<>'' AND COALESCE(integration->>'actionId','')<>'' AND
 ((action_type='move_task' AND target_stage_id IS NOT NULL AND octet_length(prompt_text)<=32768 AND session_policy='') OR
 (action_type='call_agent' AND target_stage_id IS NULL AND length(trim(prompt_text))>0 AND session_policy IN ('continue_or_create_if_absent','new')))));

-- +goose Down
-- Clear move-action prompts before rollback; validation preserves configured rules.
ALTER TABLE workflow_rules DROP CONSTRAINT workflow_rules_check;
ALTER TABLE workflow_rules ADD CONSTRAINT workflow_rules_check CHECK(
 (event_type='stage_entered' AND action_type='start_work' AND target_stage_id IS NULL AND integration IS NULL AND prompt_text='' AND session_policy='') OR
 (event_type IN ('run_succeeded','stage_completed') AND action_type='move_task' AND target_stage_id IS NOT NULL AND target_stage_id<>source_stage_id AND integration IS NULL AND prompt_text='' AND session_policy='') OR
 (event_type='integration_action' AND jsonb_typeof(integration)='object' AND COALESCE(integration->>'connectionId','')<>'' AND COALESCE(integration->>'actionId','')<>'' AND
 ((action_type='move_task' AND target_stage_id IS NOT NULL AND prompt_text='' AND session_policy='') OR
 (action_type='call_agent' AND target_stage_id IS NULL AND length(trim(prompt_text))>0 AND session_policy IN ('continue_or_create_if_absent','new')))));
