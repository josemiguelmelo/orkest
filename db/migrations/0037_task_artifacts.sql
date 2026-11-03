-- +goose Up
ALTER TABLE workflow_stages ADD COLUMN artifact_inputs jsonb NOT NULL DEFAULT '[]';
ALTER TABLE workflow_stages ADD COLUMN artifact_outputs jsonb NOT NULL DEFAULT '[]';
CREATE TABLE task_artifacts (
 id text PRIMARY KEY,
 workspace_id text NOT NULL,
 task_id text NOT NULL,
 kind text NOT NULL,
 output_name text NOT NULL DEFAULT '',
 revision bigint NOT NULL DEFAULT 0,
 UNIQUE(workspace_id,task_id,id),
 FOREIGN KEY(workspace_id,task_id) REFERENCES tasks(workspace_id,id) ON DELETE CASCADE
);
CREATE TABLE task_artifact_revisions (
 id text PRIMARY KEY,
 workspace_id text NOT NULL,
 task_id text NOT NULL,
 artifact_id text NOT NULL,
 number bigint NOT NULL CHECK(number>0),
 title text NOT NULL,
 content jsonb NOT NULL,
 created_by text NOT NULL REFERENCES users(id),
 producing_run_id text,
 stage_id text NOT NULL,
 stage_version bigint NOT NULL,
 idempotency_key text NOT NULL,
 request jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(artifact_id,number),
 UNIQUE(workspace_id,task_id,idempotency_key),
 FOREIGN KEY(workspace_id,task_id,artifact_id) REFERENCES task_artifacts(workspace_id,task_id,id) ON DELETE CASCADE
);
CREATE TABLE run_artifact_inputs (
 run_id text NOT NULL REFERENCES agent_launches(run_id) ON DELETE CASCADE,
 revision_id text NOT NULL REFERENCES task_artifact_revisions(id) ON DELETE CASCADE,
 PRIMARY KEY(run_id,revision_id)
);
-- +goose Down
-- Destructive: rollback permanently removes artifact content, history and run input links.
DROP TABLE run_artifact_inputs;
DROP TABLE task_artifact_revisions;
DROP TABLE task_artifacts;
ALTER TABLE workflow_stages DROP COLUMN artifact_inputs;
ALTER TABLE workflow_stages DROP COLUMN artifact_outputs;
