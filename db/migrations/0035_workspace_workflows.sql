-- +goose Up
-- Branch-only migration: create the final relational model directly from v34.
CREATE TABLE workspace_workflows (
    workspace_id text NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    version bigint NOT NULL DEFAULT 1,
    id text NOT NULL,
    name text NOT NULL DEFAULT 'Default',
    configured_by text NOT NULL DEFAULT '',
    PRIMARY KEY(workspace_id,id)
);
CREATE TABLE workflow_stages (
    workspace_id text NOT NULL,
    id text NOT NULL,
    responsible_agent_id text,
    workflow_id text NOT NULL,
    position integer NOT NULL,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    auto_start boolean NOT NULL DEFAULT false,
    auto_advance boolean NOT NULL DEFAULT false,
    PRIMARY KEY(workspace_id,workflow_id,id),
    CONSTRAINT workflow_stages_workflow_fkey FOREIGN KEY(workspace_id,workflow_id)
        REFERENCES workspace_workflows(workspace_id,id) ON DELETE CASCADE,
    CONSTRAINT workflow_stage_agent_fkey FOREIGN KEY(workspace_id,responsible_agent_id)
        REFERENCES agents(workspace_id,id) ON DELETE NO ACTION DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT workflow_stage_position_check CHECK(position>=0),
    CONSTRAINT workflow_stage_position_key UNIQUE(workspace_id,workflow_id,position) DEFERRABLE INITIALLY DEFERRED
);
INSERT INTO workspace_workflows(workspace_id,id) SELECT id,id FROM workspaces;
INSERT INTO workflow_stages(workspace_id,workflow_id,id,position,name,description,auto_start,auto_advance)
SELECT w.id,w.id,s.* FROM workspaces w CROSS JOIN (VALUES
    ('todo',0,'Todo','Ready to pick up',false,false),
    ('in_progress',1,'In progress','Work in progress',true,true),
    ('in_review',2,'In review','Ready for review',false,false),
    ('done',3,'Done','Completed work',false,false)
) AS s(id,position,name,description,auto_start,auto_advance);

ALTER TABLE tasks DROP CONSTRAINT tasks_status_check;
ALTER TABLE tasks ADD COLUMN workflow_id text;
UPDATE tasks SET workflow_id=workspace_id;
ALTER TABLE tasks ALTER COLUMN workflow_id SET NOT NULL;
ALTER TABLE tasks ADD COLUMN stage_version bigint NOT NULL DEFAULT 1;
ALTER TABLE tasks ADD COLUMN stage_work_state text NOT NULL DEFAULT 'idle'
    CHECK(stage_work_state IN ('idle','queued','running','completed','failed'));
ALTER TABLE tasks ADD COLUMN workflow_run_id text;
UPDATE tasks t SET workflow_run_id=(SELECT run_id FROM agent_launches l
    WHERE l.workspace_id=t.workspace_id AND l.task_id=t.id ORDER BY l.created_at DESC LIMIT 1);
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
ALTER TABLE task_integrations ADD CONSTRAINT task_integrations_identity_workflow_key UNIQUE(workspace_id,id,workflow_id);
ALTER TABLE tasks ADD CONSTRAINT tasks_source_workflow_integration_fkey
    FOREIGN KEY(workspace_id,source_integration_id,workflow_id) REFERENCES task_integrations(workspace_id,id,workflow_id)
    ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE task_stage_history (
    workspace_id text NOT NULL,
    task_id text NOT NULL,
    stage_version bigint NOT NULL,
    stage_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    workflow_id text NOT NULL,
    configuration_version bigint NOT NULL,
    PRIMARY KEY(workspace_id,task_id,stage_version),
    FOREIGN KEY(workspace_id,task_id) REFERENCES tasks(workspace_id,id) ON DELETE CASCADE
);
INSERT INTO task_stage_history(workspace_id,task_id,stage_version,stage_id,workflow_id,configuration_version)
SELECT workspace_id,id,stage_version,status,workflow_id,1 FROM tasks;
ALTER TABLE agent_launches ADD COLUMN stage_version bigint NOT NULL DEFAULT 0;
UPDATE agent_launches l SET stage_version=t.stage_version FROM tasks t
WHERE l.workspace_id=t.workspace_id AND l.task_id=t.id;
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

-- +goose StatementBegin
CREATE FUNCTION initialize_workspace_workflow() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO workspace_workflows(workspace_id,id,name) VALUES(NEW.id,NEW.id,'Default');
    INSERT INTO workflow_stages(workspace_id,workflow_id,id,position,name,description,auto_start,auto_advance) VALUES
      (NEW.id,NEW.id,'todo',0,'Todo','Ready to pick up',false,false),
      (NEW.id,NEW.id,'in_progress',1,'In progress','Work in progress',true,true),
      (NEW.id,NEW.id,'in_review',2,'In review','Ready for review',false,false),
      (NEW.id,NEW.id,'done',3,'Done','Completed work',false,false);
    RETURN NEW;
END $$;
CREATE FUNCTION record_task_stage_entry() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='UPDATE' AND NEW.workflow_id IS DISTINCT FROM OLD.workflow_id THEN
    RAISE EXCEPTION 'task workflow cannot change' USING ERRCODE='23514';
  END IF;
  IF TG_OP='INSERT' OR NEW.status IS DISTINCT FROM OLD.status THEN
    IF TG_OP='UPDATE' THEN NEW.stage_version=OLD.stage_version+1; END IF;
    NEW.stage_work_state='idle';
    NEW.workflow_run_id=NULL;
  END IF;
  RETURN NEW;
END $$;
CREATE FUNCTION record_task_stage_history() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='INSERT' OR NEW.stage_version IS DISTINCT FROM OLD.stage_version THEN
    INSERT INTO task_stage_history(workspace_id,task_id,stage_version,stage_id,workflow_id,configuration_version)
    SELECT NEW.workspace_id,NEW.id,NEW.stage_version,NEW.status,NEW.workflow_id,version
    FROM workspace_workflows WHERE workspace_id=NEW.workspace_id AND id=NEW.workflow_id;
  END IF;
  RETURN NEW;
END $$;
CREATE FUNCTION workflow_final_stage(workspace_key text, workflow_key text) RETURNS text LANGUAGE sql STABLE AS $$
SELECT id FROM workflow_stages WHERE workspace_id=workspace_key AND workflow_id=workflow_key ORDER BY position DESC LIMIT 1;
$$;
-- +goose StatementEnd
CREATE TRIGGER workspace_workflow_init AFTER INSERT ON workspaces FOR EACH ROW EXECUTE FUNCTION initialize_workspace_workflow();
CREATE TRIGGER task_stage_entry BEFORE INSERT OR UPDATE ON tasks FOR EACH ROW EXECUTE FUNCTION record_task_stage_entry();
CREATE TRIGGER task_stage_history AFTER INSERT OR UPDATE ON tasks FOR EACH ROW EXECUTE FUNCTION record_task_stage_history();
-- Permit workspace deletion to cascade in any order while retaining reference checks.
ALTER TABLE task_repositories DROP CONSTRAINT task_repositories_workspace_id_repository_id_fkey;
ALTER TABLE task_repositories ADD CONSTRAINT task_repositories_workspace_id_repository_id_fkey
    FOREIGN KEY(workspace_id,repository_id) REFERENCES code_repositories(workspace_id,id)
    ON DELETE NO ACTION DEFERRABLE INITIALLY DEFERRED;

-- +goose Down
-- Stop platform command delivery and reconcile active runs before rollback.
-- Deletes workflow configuration/history/commands; preserves tasks, integrations,
-- runtime sessions and working files. Custom stage keys become todo.
DROP TRIGGER task_stage_history ON tasks;
DROP TRIGGER task_stage_entry ON tasks;
DROP TRIGGER workspace_workflow_init ON workspaces;
DROP FUNCTION record_task_stage_history();
DROP FUNCTION record_task_stage_entry();
DROP FUNCTION initialize_workspace_workflow();
DROP FUNCTION workflow_final_stage(text,text);
DROP TABLE workflow_commands;
DROP TABLE task_stage_history;
ALTER TABLE tasks DROP CONSTRAINT tasks_source_workflow_integration_fkey;
ALTER TABLE tasks DROP CONSTRAINT tasks_workflow_stage_fkey;
ALTER TABLE tasks DROP CONSTRAINT tasks_workflow_fkey;
ALTER TABLE task_integrations DROP CONSTRAINT task_integrations_identity_workflow_key;
ALTER TABLE task_integrations DROP CONSTRAINT task_integrations_workflow_fkey;
ALTER TABLE task_integrations DROP COLUMN workflow_id;
ALTER TABLE tasks DROP COLUMN workflow_id,
    DROP COLUMN workflow_run_id,
    DROP COLUMN stage_work_state, DROP COLUMN stage_version;
ALTER TABLE agent_launches DROP COLUMN stage_version;
UPDATE tasks SET status='todo' WHERE status NOT IN ('todo','in_progress','in_review','done');
ALTER TABLE tasks ADD CONSTRAINT tasks_status_check CHECK(status IN ('todo','in_progress','in_review','done'));
DROP TABLE workflow_stages;
DROP TABLE workspace_workflows;
ALTER TABLE task_repositories DROP CONSTRAINT task_repositories_workspace_id_repository_id_fkey;
ALTER TABLE task_repositories ADD CONSTRAINT task_repositories_workspace_id_repository_id_fkey
    FOREIGN KEY(workspace_id,repository_id) REFERENCES code_repositories(workspace_id,id) ON DELETE RESTRICT;
