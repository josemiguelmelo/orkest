-- +goose Up
ALTER TABLE workflow_rules ADD COLUMN integration jsonb,
 ADD COLUMN prompt_text text NOT NULL DEFAULT '',
 ADD COLUMN session_policy text NOT NULL DEFAULT '',
 ADD COLUMN connection_id text GENERATED ALWAYS AS (integration->>'connectionId') STORED;
ALTER TABLE workflow_rules ADD CONSTRAINT workflow_rule_connection_fkey
 FOREIGN KEY(workspace_id,connection_id) REFERENCES workflow_stage_integrations(workspace_id,id) ON DELETE NO ACTION DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE workflow_rules DROP CONSTRAINT workflow_rules_event_type_check,
 DROP CONSTRAINT workflow_rules_action_type_check, DROP CONSTRAINT workflow_rules_check;
ALTER TABLE workflow_rules ADD CONSTRAINT workflow_rules_event_type_check
 CHECK(event_type IN ('stage_entered','run_succeeded','stage_completed','integration_action')),
 ADD CONSTRAINT workflow_rules_action_type_check CHECK(action_type IN ('move_task','start_work','call_agent')),
 ADD CONSTRAINT workflow_rules_check CHECK(
 (event_type='stage_entered' AND action_type='start_work' AND target_stage_id IS NULL AND integration IS NULL AND prompt_text='' AND session_policy='') OR
 (event_type IN ('run_succeeded','stage_completed') AND action_type='move_task' AND target_stage_id IS NOT NULL AND target_stage_id<>source_stage_id AND integration IS NULL AND prompt_text='' AND session_policy='') OR
 (event_type='integration_action' AND jsonb_typeof(integration)='object' AND COALESCE(integration->>'connectionId','')<>'' AND COALESCE(integration->>'actionId','')<>'' AND
 ((action_type='move_task' AND target_stage_id IS NOT NULL AND target_stage_id<>source_stage_id AND prompt_text='' AND session_policy='') OR
 (action_type='call_agent' AND target_stage_id IS NULL AND length(trim(prompt_text))>0 AND session_policy IN ('continue_or_create_if_absent','new')))));
DROP INDEX workflow_rules_match;
CREATE UNIQUE INDEX workflow_rules_match ON workflow_rules(workspace_id,workflow_id,event_type,source_stage_id) WHERE enabled AND event_type<>'integration_action';
CREATE UNIQUE INDEX workflow_rules_integration_match ON workflow_rules(workspace_id,workflow_id,source_stage_id,connection_id,(integration->>'actionId')) WHERE enabled AND event_type='integration_action';
ALTER TABLE workflow_events ADD COLUMN integration jsonb, ADD COLUMN processing_error text NOT NULL DEFAULT '';
ALTER TABLE workflow_events DROP CONSTRAINT workflow_events_event_type_check;
ALTER TABLE workflow_events ADD CONSTRAINT workflow_events_event_type_check
 CHECK(event_type IN ('stage_entered','run_succeeded','run_failed','stage_completed','integration_action'));
-- Explicit column lists preserve seeded rules as the table gains fields.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION initialize_workspace_workflow() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO workspace_workflows(workspace_id,id,name) VALUES(NEW.id,NEW.id,'Default');
 INSERT INTO workflow_stages(workspace_id,workflow_id,id,position,name,description) VALUES
 (NEW.id,NEW.id,'todo',0,'Todo','Ready to pick up'),(NEW.id,NEW.id,'in_progress',1,'In progress','Work in progress'),
 (NEW.id,NEW.id,'in_review',2,'In review','Ready for review'),(NEW.id,NEW.id,'done',3,'Done','Completed work');
 INSERT INTO workflow_rules(workspace_id,workflow_id,id,enabled,event_type,source_stage_id,action_type,target_stage_id) VALUES
 (NEW.id,NEW.id,'start-progress',true,'stage_entered','in_progress','start_work',NULL),
 (NEW.id,NEW.id,'review-success',true,'run_succeeded','in_progress','move_task','in_review'),
 (NEW.id,NEW.id,'review-complete',true,'stage_completed','in_progress','move_task','in_review');
 RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose Down
-- Stop platform ingestion/workflow workers before rollback. Never discard new events or commands.
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM workflow_events WHERE event_type='integration_action') OR
 EXISTS(SELECT 1 FROM workflow_rules WHERE event_type='integration_action') OR
 EXISTS(SELECT 1 FROM workflow_commands WHERE state='pending' AND options->'integration' IS NOT NULL AND options->'integration'<>'null'::jsonb) THEN
 RAISE EXCEPTION 'Remove integration rules and reconcile/archive integration event history before rollback';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE workflow_rules DROP CONSTRAINT workflow_rule_connection_fkey;
DROP INDEX workflow_rules_integration_match;
DROP INDEX workflow_rules_match;
CREATE UNIQUE INDEX workflow_rules_match ON workflow_rules(workspace_id,workflow_id,event_type,source_stage_id) WHERE enabled;
ALTER TABLE workflow_rules DROP CONSTRAINT workflow_rules_event_type_check,
 DROP CONSTRAINT workflow_rules_action_type_check, DROP CONSTRAINT workflow_rules_check;
ALTER TABLE workflow_rules ADD CONSTRAINT workflow_rules_event_type_check CHECK(event_type IN ('stage_entered','run_succeeded','stage_completed')),
 ADD CONSTRAINT workflow_rules_action_type_check CHECK(action_type IN ('move_task','start_work')),
 ADD CONSTRAINT workflow_rules_check CHECK((event_type='stage_entered' AND action_type='start_work' AND target_stage_id IS NULL) OR
 (event_type IN ('run_succeeded','stage_completed') AND action_type='move_task' AND target_stage_id IS NOT NULL AND target_stage_id<>source_stage_id));
ALTER TABLE workflow_rules DROP COLUMN connection_id, DROP COLUMN integration, DROP COLUMN prompt_text, DROP COLUMN session_policy;
ALTER TABLE workflow_events DROP CONSTRAINT workflow_events_event_type_check;
ALTER TABLE workflow_events ADD CONSTRAINT workflow_events_event_type_check CHECK(event_type IN ('stage_entered','run_succeeded','run_failed','stage_completed'));
ALTER TABLE workflow_events DROP COLUMN integration, DROP COLUMN processing_error;
