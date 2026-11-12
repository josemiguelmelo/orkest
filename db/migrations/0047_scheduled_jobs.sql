-- +goose Up
CREATE TABLE scheduled_jobs (
 id text PRIMARY KEY,
 workspace_id text NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
 revision bigint NOT NULL DEFAULT 1,
 authorized_by text NOT NULL REFERENCES users(id),
 definition jsonb NOT NULL,
 enabled boolean NOT NULL,
 next_due_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(workspace_id,id)
);
CREATE INDEX scheduled_jobs_due ON scheduled_jobs(next_due_at,id) WHERE enabled;
CREATE TABLE scheduled_job_revisions (
 workspace_id text NOT NULL, job_id text NOT NULL, revision bigint NOT NULL,
 authorized_by text NOT NULL REFERENCES users(id), definition jsonb NOT NULL,
 PRIMARY KEY(workspace_id,job_id,revision),
 FOREIGN KEY(workspace_id,job_id) REFERENCES scheduled_jobs(workspace_id,id) ON DELETE CASCADE
);
CREATE TABLE scheduled_job_runs (
 id text PRIMARY KEY, workspace_id text NOT NULL, job_id text NOT NULL, revision bigint NOT NULL,
 actor_id text NOT NULL REFERENCES users(id), definition jsonb NOT NULL,
 repository_ids text[] NOT NULL DEFAULT '{}', scheduled_for timestamptz NOT NULL,
 trigger_key text NOT NULL,
 state text NOT NULL CHECK(state IN ('pending','starting','running','succeeded','failed','cancelled','skipped','missed','needs_attention','delivery_uncertain')),
 reason text NOT NULL DEFAULT '', run_id text NOT NULL UNIQUE, session_id text NOT NULL UNIQUE,
 command_id text NOT NULL UNIQUE, claim_token text NOT NULL DEFAULT '', claim_attempt integer NOT NULL DEFAULT 0,
 claim_until timestamptz, cancel_requested boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(workspace_id,job_id,revision) REFERENCES scheduled_job_revisions(workspace_id,job_id,revision) ON DELETE CASCADE,
 UNIQUE(workspace_id,job_id,trigger_key), UNIQUE(workspace_id,id)
);
CREATE INDEX scheduled_job_runs_pending ON scheduled_job_runs(created_at,id) WHERE state IN ('pending','starting');
CREATE INDEX scheduled_job_runs_history ON scheduled_job_runs(workspace_id,job_id,created_at DESC);
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM scheduled_job_runs WHERE state IN ('pending','starting','running','delivery_uncertain')) THEN
  RAISE EXCEPTION 'Reconcile scheduled job ownership before rollback';
 END IF;
END $$;
-- +goose StatementEnd
-- Removes history and session snapshots; does not remove durable runtime files.
DELETE FROM agent_launch_events WHERE run_id IN (SELECT run_id FROM scheduled_job_runs);
DELETE FROM agent_prompt_outbox WHERE run_id IN (SELECT run_id FROM scheduled_job_runs);
DELETE FROM agent_launches WHERE run_id IN (SELECT run_id FROM scheduled_job_runs);
DELETE FROM task_agent_sessions WHERE task_id IN (SELECT id FROM scheduled_job_runs);
DROP TABLE scheduled_job_runs;
DROP TABLE scheduled_job_revisions;
DROP TABLE scheduled_jobs;
