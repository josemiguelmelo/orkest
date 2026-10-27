-- +goose Up
CREATE TABLE task_agent_sessions (
    id text PRIMARY KEY,
    organization_id text NOT NULL,
    workspace_id text NOT NULL,
    task_id text NOT NULL,
    configuration jsonb NOT NULL,
    runtime_session_id text NOT NULL DEFAULT '',
    is_current boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (workspace_id, organization_id) REFERENCES workspaces(id, organization_id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, task_id) REFERENCES tasks(workspace_id, id) ON DELETE CASCADE,
    UNIQUE (workspace_id, task_id, id)
);
CREATE UNIQUE INDEX task_agent_sessions_current_idx ON task_agent_sessions(workspace_id, task_id) WHERE is_current;
ALTER TABLE agent_launches ADD COLUMN task_session_id text;
ALTER TABLE agent_launches ADD CONSTRAINT agent_launch_session_scope_fk
FOREIGN KEY (workspace_id, task_id, task_session_id)
REFERENCES task_agent_sessions(workspace_id, task_id, id);
CREATE UNIQUE INDEX agent_launches_active_task_idx ON agent_launches(workspace_id, task_id)
WHERE status IN ('starting', 'process_started', 'ready', 'completed_pending_close', 'failed_pending_close');
CREATE TABLE agent_prompt_outbox (
    run_id text NOT NULL REFERENCES agent_launches(run_id) ON DELETE CASCADE,
    command_id text NOT NULL,
    prompt text NOT NULL,
    delivered boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (run_id, command_id)
);

-- +goose Down
DROP TABLE agent_prompt_outbox;
DROP INDEX agent_launches_active_task_idx;
ALTER TABLE agent_launches DROP COLUMN task_session_id;
DROP TABLE task_agent_sessions;
