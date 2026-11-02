-- +goose Up
ALTER TABLE workspace_workflows ADD COLUMN initial_stage_id text NOT NULL DEFAULT 'todo',
 ADD COLUMN closed_stage_id text NOT NULL DEFAULT 'done';
UPDATE workspace_workflows w SET initial_stage_id=(SELECT id FROM workflow_stages s WHERE s.workspace_id=w.workspace_id AND s.workflow_id=w.id ORDER BY position LIMIT 1),
 closed_stage_id=(SELECT id FROM workflow_stages s WHERE s.workspace_id=w.workspace_id AND s.workflow_id=w.id ORDER BY position DESC LIMIT 1);
ALTER TABLE workspace_workflows ADD CONSTRAINT workflow_initial_stage_fkey FOREIGN KEY(workspace_id,id,initial_stage_id) REFERENCES workflow_stages(workspace_id,workflow_id,id) DEFERRABLE INITIALLY DEFERRED,
 ADD CONSTRAINT workflow_closed_stage_fkey FOREIGN KEY(workspace_id,id,closed_stage_id) REFERENCES workflow_stages(workspace_id,workflow_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE workflow_rules (
 workspace_id text NOT NULL,
 workflow_id text NOT NULL,
 id text NOT NULL,
 enabled boolean NOT NULL,
 event_type text NOT NULL CHECK(event_type IN ('stage_entered','run_succeeded','stage_completed')),
 source_stage_id text NOT NULL,
 action_type text NOT NULL CHECK(action_type IN ('move_task','start_work')),
 target_stage_id text,
 PRIMARY KEY(workspace_id,workflow_id,id),
 FOREIGN KEY(workspace_id,workflow_id) REFERENCES workspace_workflows(workspace_id,id) ON DELETE CASCADE,
 FOREIGN KEY(workspace_id,workflow_id,source_stage_id) REFERENCES workflow_stages(workspace_id,workflow_id,id) DEFERRABLE INITIALLY DEFERRED,
 FOREIGN KEY(workspace_id,workflow_id,target_stage_id) REFERENCES workflow_stages(workspace_id,workflow_id,id) DEFERRABLE INITIALLY DEFERRED,
 CHECK((event_type='stage_entered' AND action_type='start_work' AND target_stage_id IS NULL) OR
 (event_type IN ('run_succeeded','stage_completed') AND action_type='move_task' AND target_stage_id IS NOT NULL AND target_stage_id<>source_stage_id))
);
CREATE UNIQUE INDEX workflow_rules_match ON workflow_rules(workspace_id,workflow_id,event_type,source_stage_id) WHERE enabled;
INSERT INTO workflow_rules SELECT workspace_id,workflow_id,'start:'||md5(id),true,'stage_entered',id,'start_work',NULL FROM workflow_stages WHERE auto_start;
INSERT INTO workflow_rules SELECT s.workspace_id,s.workflow_id,e.event_type||':'||md5(s.id),true,e.event_type,s.id,'move_task',n.id
FROM workflow_stages s JOIN LATERAL (SELECT id FROM workflow_stages n WHERE n.workspace_id=s.workspace_id AND n.workflow_id=s.workflow_id AND n.position>s.position ORDER BY position LIMIT 1) n ON true
CROSS JOIN (VALUES ('run_succeeded'),('stage_completed')) e(event_type) WHERE s.auto_advance;
ALTER TABLE workflow_stages DROP COLUMN auto_start, DROP COLUMN auto_advance;
CREATE TABLE workflow_events (
 id text PRIMARY KEY,
 workspace_id text NOT NULL,
 task_id text NOT NULL,
 stage_version bigint NOT NULL,
 actor_id text NOT NULL,
 event_type text NOT NULL CHECK(event_type IN ('stage_entered','run_succeeded','run_failed','stage_completed')),
 run_id text NOT NULL DEFAULT '',
 causation_id text NOT NULL,
 correlation_id text NOT NULL,
 occurred_at timestamptz NOT NULL DEFAULT now(),
 processed_at timestamptz,
 configuration_version bigint,
 matched_rule_id text,
 FOREIGN KEY(workspace_id,task_id) REFERENCES tasks(workspace_id,id) ON DELETE CASCADE
);
CREATE INDEX workflow_events_pending ON workflow_events(occurred_at,id) WHERE processed_at IS NULL;
INSERT INTO workflow_events(id,workspace_id,task_id,stage_version,actor_id,event_type,run_id,causation_id,correlation_id,occurred_at,processed_at)
SELECT id,workspace_id,task_id,stage_version,actor_id,CASE WHEN (options->>'success')::boolean THEN 'run_succeeded' ELSE 'run_failed' END,options->>'runId',id,id,created_at,CASE WHEN state='pending' THEN NULL ELSE now() END
FROM workflow_commands WHERE options->>'action'='run_outcome';
DELETE FROM workflow_commands WHERE options->>'action'='run_outcome';
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION workflow_final_stage(workspace_key text, workflow_key text) RETURNS text LANGUAGE sql STABLE AS $$
 SELECT closed_stage_id FROM workspace_workflows WHERE workspace_id=workspace_key AND id=workflow_key;
$$;
CREATE OR REPLACE FUNCTION initialize_workspace_workflow() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO workspace_workflows(workspace_id,id,name) VALUES(NEW.id,NEW.id,'Default');
 INSERT INTO workflow_stages(workspace_id,workflow_id,id,position,name,description) VALUES
 (NEW.id,NEW.id,'todo',0,'Todo','Ready to pick up'),(NEW.id,NEW.id,'in_progress',1,'In progress','Work in progress'),
 (NEW.id,NEW.id,'in_review',2,'In review','Ready for review'),(NEW.id,NEW.id,'done',3,'Done','Completed work');
 INSERT INTO workflow_rules VALUES
 (NEW.id,NEW.id,'start-progress',true,'stage_entered','in_progress','start_work',NULL),
 (NEW.id,NEW.id,'review-success',true,'run_succeeded','in_progress','move_task','in_review'),
 (NEW.id,NEW.id,'review-complete',true,'stage_completed','in_progress','move_task','in_review');
 RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose Down
-- Explicit destination rules and event audit history cannot be represented by v35.
-- Stop workers and drain pending events before rollback. Back up configuration.
ALTER TABLE workflow_stages ADD COLUMN auto_start boolean NOT NULL DEFAULT false, ADD COLUMN auto_advance boolean NOT NULL DEFAULT false;
UPDATE workflow_stages s SET auto_start=EXISTS(SELECT 1 FROM workflow_rules r WHERE r.workspace_id=s.workspace_id AND r.workflow_id=s.workflow_id AND r.source_stage_id=s.id AND r.enabled AND r.event_type='stage_entered'),
 auto_advance=EXISTS(SELECT 1 FROM workflow_rules r WHERE r.workspace_id=s.workspace_id AND r.workflow_id=s.workflow_id AND r.source_stage_id=s.id AND r.enabled AND r.event_type='run_succeeded');
DROP TABLE workflow_events;
DROP TABLE workflow_rules;
ALTER TABLE workspace_workflows DROP CONSTRAINT workflow_initial_stage_fkey, DROP CONSTRAINT workflow_closed_stage_fkey,
 DROP COLUMN initial_stage_id, DROP COLUMN closed_stage_id;
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION workflow_final_stage(workspace_key text, workflow_key text) RETURNS text LANGUAGE sql STABLE AS $$
 SELECT id FROM workflow_stages WHERE workspace_id=workspace_key AND workflow_id=workflow_key ORDER BY position DESC LIMIT 1;
$$;
CREATE OR REPLACE FUNCTION initialize_workspace_workflow() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO workspace_workflows(workspace_id,id,name) VALUES(NEW.id,NEW.id,'Default');
 INSERT INTO workflow_stages(workspace_id,workflow_id,id,position,name,description,auto_start,auto_advance) VALUES
 (NEW.id,NEW.id,'todo',0,'Todo','Ready to pick up',false,false),
 (NEW.id,NEW.id,'in_progress',1,'In progress','Work in progress',true,true),
 (NEW.id,NEW.id,'in_review',2,'In review','Ready for review',false,false),
 (NEW.id,NEW.id,'done',3,'Done','Completed work',false,false);
 RETURN NEW;
END $$;
-- +goose StatementEnd
