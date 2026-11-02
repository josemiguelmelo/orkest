-- +goose Up
-- Stage entries and repeated executions have distinct identities. A late run
-- completion may only affect the run currently bound to the task.
ALTER TABLE tasks ADD COLUMN workflow_run_id text;
UPDATE tasks t SET workflow_run_id=(SELECT run_id FROM agent_launches l WHERE l.workspace_id=t.workspace_id AND l.task_id=t.id ORDER BY l.created_at DESC LIMIT 1);
ALTER TABLE workflow_stages ADD COLUMN responsible_agent_id text;
ALTER TABLE workflow_stages ADD CONSTRAINT workflow_stage_agent_fkey FOREIGN KEY(workspace_id,responsible_agent_id) REFERENCES agents(workspace_id,id);
-- +goose StatementBegin
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

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION record_task_stage_entry() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='INSERT' OR NEW.status IS DISTINCT FROM OLD.status THEN
    SELECT definition INTO NEW.workflow_snapshot FROM workspace_workflows WHERE workspace_id=NEW.workspace_id;
    IF TG_OP='UPDATE' THEN NEW.stage_version=OLD.stage_version+1; END IF;
    NEW.stage_work_state='idle';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
ALTER TABLE workflow_stages DROP COLUMN responsible_agent_id;
ALTER TABLE tasks DROP COLUMN workflow_run_id;
