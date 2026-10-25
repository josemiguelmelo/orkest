-- +goose Up
CREATE TABLE agent_launches (
    run_id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    workspace_id text NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    task_id text NOT NULL,
    status text NOT NULL CHECK (status IN ('starting', 'process_started', 'ready', 'failed')),
    process_id text NOT NULL DEFAULT '',
    endpoint text NOT NULL DEFAULT '',
    failure_stage text NOT NULL DEFAULT '',
    failure_reason text NOT NULL DEFAULT '',
    last_sequence bigint NOT NULL DEFAULT 0 CHECK (last_sequence >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, run_id),
    FOREIGN KEY (workspace_id, organization_id) REFERENCES workspaces(id, organization_id) ON DELETE CASCADE
);
CREATE INDEX agent_launches_workspace_status_idx ON agent_launches(workspace_id, status);

CREATE TABLE agent_launch_events (
    run_id text NOT NULL REFERENCES agent_launches(run_id) ON DELETE CASCADE,
    sequence bigint NOT NULL CHECK (sequence > 0),
    event jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (run_id, sequence)
);

-- +goose Down
DROP TABLE agent_launch_events;
DROP TABLE agent_launches;
