-- +goose Up
CREATE TABLE workspace_workflows (
    workspace_id text PRIMARY KEY REFERENCES workspaces(id) ON DELETE CASCADE,
    version bigint NOT NULL DEFAULT 1,
    definition jsonb NOT NULL
);
CREATE TABLE workflow_stages (
    workspace_id text NOT NULL REFERENCES workspace_workflows(workspace_id) ON DELETE CASCADE,
    id text NOT NULL,
    PRIMARY KEY (workspace_id, id)
);
-- +goose StatementBegin
CREATE FUNCTION default_workspace_workflow(workspace_id text) RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$
SELECT jsonb_build_object('id', workspace_id, 'version', 1, 'initialStageId', 'todo', 'closedStageId', 'done', 'stages',
        '[{"id":"todo","name":"Todo","description":"Ready to pick up","responsibleAgentId":"","autoStart":false,"autoAdvance":false},
          {"id":"in_progress","name":"In progress","description":"Work in progress","responsibleAgentId":"","autoStart":true,"autoAdvance":true},
          {"id":"in_review","name":"In review","description":"Ready for review","responsibleAgentId":"","autoStart":false,"autoAdvance":false},
          {"id":"done","name":"Done","description":"Completed work","responsibleAgentId":"","autoStart":false,"autoAdvance":false}]'::jsonb);
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION initialize_workspace_workflow() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO workspace_workflows(workspace_id, definition) VALUES (NEW.id,
      default_workspace_workflow(NEW.id));
    INSERT INTO workflow_stages(workspace_id,id) SELECT NEW.id, value->>'id' FROM workspace_workflows, jsonb_array_elements(definition->'stages') WHERE workspace_id = NEW.id;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER workspace_workflow_init AFTER INSERT ON workspaces FOR EACH ROW EXECUTE FUNCTION initialize_workspace_workflow();
INSERT INTO workspace_workflows(workspace_id,definition) SELECT id,default_workspace_workflow(id) FROM workspaces;
INSERT INTO workflow_stages(workspace_id,id) SELECT workspace_id,value->>'id' FROM workspace_workflows,jsonb_array_elements(definition->'stages');
ALTER TABLE tasks DROP CONSTRAINT tasks_status_check;
ALTER TABLE tasks ADD CONSTRAINT tasks_workflow_stage_fkey FOREIGN KEY(workspace_id,status) REFERENCES workflow_stages(workspace_id,id);
ALTER TABLE tasks ADD COLUMN stage_version bigint NOT NULL DEFAULT 1;
ALTER TABLE tasks ADD COLUMN stage_work_state text NOT NULL DEFAULT 'idle' CHECK(stage_work_state IN ('idle','queued','running','completed','failed'));
ALTER TABLE tasks ADD COLUMN workflow_snapshot jsonb NOT NULL DEFAULT '{}';
UPDATE tasks SET workflow_snapshot = definition FROM workspace_workflows WHERE tasks.workspace_id=workspace_workflows.workspace_id;
CREATE TABLE task_stage_history (
    workspace_id text NOT NULL,
    task_id text NOT NULL,
    stage_version bigint NOT NULL,
    stage_id text NOT NULL,
    definition jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(workspace_id,task_id,stage_version),
    FOREIGN KEY(workspace_id,task_id) REFERENCES tasks(workspace_id,id) ON DELETE CASCADE
);
INSERT INTO task_stage_history SELECT workspace_id,id,stage_version,status,workflow_snapshot,now() FROM tasks;
-- +goose StatementBegin
CREATE FUNCTION record_task_stage_entry() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='INSERT' OR NEW.status IS DISTINCT FROM OLD.status THEN
    SELECT definition INTO NEW.workflow_snapshot FROM workspace_workflows WHERE workspace_id=NEW.workspace_id;
    IF TG_OP='UPDATE' THEN NEW.stage_version=OLD.stage_version+1; END IF;
    NEW.stage_work_state='idle';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER task_stage_entry BEFORE INSERT OR UPDATE ON tasks FOR EACH ROW EXECUTE FUNCTION record_task_stage_entry();
-- +goose StatementBegin
CREATE FUNCTION record_task_stage_history() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  INSERT INTO task_stage_history(workspace_id,task_id,stage_version,stage_id,definition)
  VALUES(NEW.workspace_id,NEW.id,NEW.stage_version,NEW.status,NEW.workflow_snapshot) ON CONFLICT DO NOTHING;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER task_stage_history AFTER INSERT OR UPDATE ON tasks FOR EACH ROW EXECUTE FUNCTION record_task_stage_history();
ALTER TABLE agent_launches ADD COLUMN stage_version bigint NOT NULL DEFAULT 0;
UPDATE agent_launches l SET stage_version=t.stage_version FROM tasks t WHERE t.workspace_id=l.workspace_id AND t.id=l.task_id;
CREATE TABLE workflow_commands (
    id text PRIMARY KEY,
    workspace_id text NOT NULL,
    task_id text NOT NULL,
    stage_version bigint NOT NULL,
    actor_id text NOT NULL,
    options jsonb NOT NULL,
    state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','delivered','failed')),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY(workspace_id,task_id) REFERENCES tasks(workspace_id,id) ON DELETE CASCADE
);
CREATE INDEX workflow_commands_pending ON workflow_commands(created_at) WHERE state='pending';

-- +goose Down
-- Custom stage identities cannot be represented by the old schema. Rollback
-- collapses custom stages to todo and deletes workflow configuration/history.
DROP TABLE workflow_commands;
ALTER TABLE agent_launches DROP COLUMN stage_version;
DROP TRIGGER task_stage_history ON tasks;
DROP TRIGGER task_stage_entry ON tasks;
DROP FUNCTION record_task_stage_history();
DROP FUNCTION record_task_stage_entry();
DROP TABLE task_stage_history;
ALTER TABLE tasks DROP CONSTRAINT tasks_workflow_stage_fkey;
ALTER TABLE tasks DROP COLUMN workflow_snapshot, DROP COLUMN stage_work_state, DROP COLUMN stage_version;
UPDATE tasks SET status='todo' WHERE status NOT IN ('todo','in_progress','in_review','done');
ALTER TABLE tasks ADD CONSTRAINT tasks_status_check CHECK(status IN ('todo','in_progress','in_review','done'));
DROP TRIGGER workspace_workflow_init ON workspaces;
DROP FUNCTION initialize_workspace_workflow();
DROP FUNCTION IF EXISTS default_workspace_workflow(text);
DROP TABLE workflow_stages;
DROP TABLE workspace_workflows;
