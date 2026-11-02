-- +goose Up
-- Promote the existing workspace workflow to a named workflow, then allow
-- every task and source integration to select a workflow in that workspace.
ALTER TABLE workspace_workflows ADD COLUMN id text;
ALTER TABLE workspace_workflows ADD COLUMN name text NOT NULL DEFAULT 'Default';
UPDATE workspace_workflows SET id=workspace_id;
ALTER TABLE workspace_workflows ALTER COLUMN id SET NOT NULL;
UPDATE workspace_workflows SET definition=definition || jsonb_build_object('name',name);
-- Replace the stage table's old single-column reference before replacing the
-- workflow primary key it targets.
ALTER TABLE workflow_stages DROP CONSTRAINT workflow_stages_workspace_id_fkey;
ALTER TABLE workspace_workflows DROP CONSTRAINT workspace_workflows_pkey;
ALTER TABLE workspace_workflows ADD PRIMARY KEY(workspace_id,id);

ALTER TABLE workflow_stages ADD COLUMN workflow_id text;
UPDATE workflow_stages SET workflow_id=workspace_id;
ALTER TABLE workflow_stages ALTER COLUMN workflow_id SET NOT NULL;
-- Tasks still reference the old (workspace_id, id) stage key at this point.
ALTER TABLE tasks DROP CONSTRAINT tasks_workflow_stage_fkey;
ALTER TABLE workflow_stages DROP CONSTRAINT workflow_stages_pkey;
ALTER TABLE workflow_stages ADD PRIMARY KEY(workspace_id,workflow_id,id);
ALTER TABLE workflow_stages ADD CONSTRAINT workflow_stages_workflow_fkey
    FOREIGN KEY(workspace_id,workflow_id) REFERENCES workspace_workflows(workspace_id,id) ON DELETE CASCADE;

ALTER TABLE tasks ADD COLUMN workflow_id text;
UPDATE tasks SET workflow_id=workspace_id;
ALTER TABLE tasks ALTER COLUMN workflow_id SET NOT NULL;
ALTER TABLE tasks ADD CONSTRAINT tasks_workflow_stage_fkey
    FOREIGN KEY(workspace_id,workflow_id,status) REFERENCES workflow_stages(workspace_id,workflow_id,id)
    ON DELETE NO ACTION DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE tasks ADD CONSTRAINT tasks_workflow_fkey
    FOREIGN KEY(workspace_id,workflow_id) REFERENCES workspace_workflows(workspace_id,id)
    ON DELETE NO ACTION DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE task_integrations ADD COLUMN workflow_id text;
UPDATE task_integrations SET workflow_id=workspace_id;
ALTER TABLE task_integrations ALTER COLUMN workflow_id SET NOT NULL;
ALTER TABLE task_integrations ADD CONSTRAINT task_integrations_workflow_fkey
    FOREIGN KEY(workspace_id,workflow_id) REFERENCES workspace_workflows(workspace_id,id)
    ON DELETE NO ACTION DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE task_integrations ADD CONSTRAINT task_integrations_identity_workflow_key
    UNIQUE(workspace_id,id,workflow_id);
ALTER TABLE tasks ADD CONSTRAINT tasks_source_workflow_integration_fkey
    FOREIGN KEY(workspace_id,source_integration_id,workflow_id)
    REFERENCES task_integrations(workspace_id,id,workflow_id)
    ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED;

-- Keep auto-created workspaces compatible with the expanded workflow schema.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION default_workspace_workflow(workspace_id text) RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$
SELECT jsonb_build_object('id', workspace_id, 'name', 'Default', 'version', 1, 'initialStageId', 'todo', 'closedStageId', 'done', 'stages',
        '[{"id":"todo","name":"Todo","description":"Ready to pick up","responsibleAgentId":"","autoStart":false,"autoAdvance":false},
          {"id":"in_progress","name":"In progress","description":"Work in progress","responsibleAgentId":"","autoStart":true,"autoAdvance":true},
          {"id":"in_review","name":"In review","description":"Ready for review","responsibleAgentId":"","autoStart":false,"autoAdvance":false},
          {"id":"done","name":"Done","description":"Completed work","responsibleAgentId":"","autoStart":false,"autoAdvance":false}]'::jsonb);
$$;
CREATE OR REPLACE FUNCTION initialize_workspace_workflow() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO workspace_workflows(workspace_id,id,name,definition) VALUES (NEW.id,NEW.id,'Default',default_workspace_workflow(NEW.id));
    INSERT INTO workflow_stages(workspace_id,workflow_id,id)
      SELECT NEW.id,NEW.id,value->>'id' FROM jsonb_array_elements(default_workspace_workflow(NEW.id)->'stages');
    RETURN NEW;
END $$;
-- +goose StatementEnd
-- Workflow selection is immutable for a task after creation. A different
-- workflow can be selected only by creating a new task or source integration.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION record_task_stage_entry() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='INSERT' OR NEW.status IS DISTINCT FROM OLD.status OR NEW.workflow_id IS DISTINCT FROM OLD.workflow_id THEN
    SELECT definition INTO NEW.workflow_snapshot FROM workspace_workflows
      WHERE workspace_id=NEW.workspace_id AND id=NEW.workflow_id;
    IF NEW.workflow_snapshot IS NULL THEN
      RAISE EXCEPTION 'workflow not found in workspace' USING ERRCODE='23503';
    END IF;
    IF TG_OP='UPDATE' THEN NEW.stage_version=OLD.stage_version+1; END IF;
    NEW.stage_work_state='idle';
    NEW.workflow_run_id=NULL;
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose Down
-- Refuse rollback rather than deleting tasks or integrations that use the new
-- workflows. Remove those resources explicitly before reverting this migration.
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS(SELECT 1 FROM workspace_workflows WHERE id <> workspace_id)
     OR EXISTS(SELECT 1 FROM tasks WHERE workflow_id <> workspace_id)
     OR EXISTS(SELECT 1 FROM task_integrations WHERE workflow_id <> workspace_id) THEN
    RAISE EXCEPTION 'cannot revert multiple workflows while non-default workflows are in use';
  END IF;
END $$;
-- +goose StatementEnd
-- Restore the pre-multi-workflow workspace initializer before dropping IDs.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION default_workspace_workflow(workspace_id text) RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$
SELECT jsonb_build_object('id', workspace_id, 'version', 1, 'initialStageId', 'todo', 'closedStageId', 'done', 'stages',
        '[{"id":"todo","name":"Todo","description":"Ready to pick up","responsibleAgentId":"","autoStart":false,"autoAdvance":false},
          {"id":"in_progress","name":"In progress","description":"Work in progress","responsibleAgentId":"","autoStart":true,"autoAdvance":true},
          {"id":"in_review","name":"In review","description":"Ready for review","responsibleAgentId":"","autoStart":false,"autoAdvance":false},
          {"id":"done","name":"Done","description":"Completed work","responsibleAgentId":"","autoStart":false,"autoAdvance":false}]'::jsonb);
$$;
CREATE OR REPLACE FUNCTION initialize_workspace_workflow() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO workspace_workflows(workspace_id,definition) VALUES (NEW.id,default_workspace_workflow(NEW.id));
    INSERT INTO workflow_stages(workspace_id,id)
      SELECT NEW.id,value->>'id' FROM jsonb_array_elements(default_workspace_workflow(NEW.id)->'stages');
    RETURN NEW;
END $$;
CREATE OR REPLACE FUNCTION record_task_stage_entry() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='INSERT' OR NEW.status IS DISTINCT FROM OLD.status THEN
    SELECT definition INTO NEW.workflow_snapshot FROM workspace_workflows WHERE workspace_id=NEW.workspace_id;
    IF TG_OP='UPDATE' THEN NEW.stage_version=OLD.stage_version+1; END IF;
    NEW.stage_work_state='idle';
    NEW.workflow_run_id=NULL;
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
ALTER TABLE tasks DROP CONSTRAINT tasks_source_workflow_integration_fkey;
ALTER TABLE task_integrations DROP CONSTRAINT task_integrations_identity_workflow_key;
ALTER TABLE task_integrations DROP CONSTRAINT task_integrations_workflow_fkey;
ALTER TABLE task_integrations DROP COLUMN workflow_id;
ALTER TABLE tasks DROP CONSTRAINT tasks_workflow_fkey;
ALTER TABLE tasks DROP CONSTRAINT tasks_workflow_stage_fkey;
ALTER TABLE tasks DROP COLUMN workflow_id;
ALTER TABLE tasks ADD CONSTRAINT tasks_workflow_stage_fkey
    FOREIGN KEY(workspace_id,status) REFERENCES workflow_stages(workspace_id,id)
    ON DELETE NO ACTION DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE workflow_stages DROP CONSTRAINT workflow_stages_workflow_fkey;
ALTER TABLE workflow_stages DROP CONSTRAINT workflow_stages_pkey;
ALTER TABLE workflow_stages ADD PRIMARY KEY(workspace_id,id);
ALTER TABLE workflow_stages DROP COLUMN workflow_id;
ALTER TABLE workspace_workflows DROP CONSTRAINT workspace_workflows_pkey;
ALTER TABLE workspace_workflows ADD PRIMARY KEY(workspace_id);
ALTER TABLE workflow_stages ADD CONSTRAINT workflow_stages_workspace_id_fkey
    FOREIGN KEY(workspace_id) REFERENCES workspace_workflows(workspace_id) ON DELETE CASCADE;
ALTER TABLE workspace_workflows DROP COLUMN id, DROP COLUMN name;
