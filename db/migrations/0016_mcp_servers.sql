-- +goose Up
CREATE TABLE mcp_servers (
    id text PRIMARY KEY,
    workspace_id text NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(trim(name)) > 0),
    url text NOT NULL CHECK (length(trim(url)) > 0),
    encrypted_headers text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, name)
);
CREATE TABLE agent_mcp_servers (
    workspace_id text NOT NULL,
    agent_id text NOT NULL,
    mcp_server_id text NOT NULL,
    PRIMARY KEY (agent_id, mcp_server_id),
    FOREIGN KEY (workspace_id, agent_id) REFERENCES agents(workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, mcp_server_id) REFERENCES mcp_servers(workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX agent_mcp_servers_server_idx ON agent_mcp_servers(workspace_id, mcp_server_id);

-- +goose Down
DROP TABLE agent_mcp_servers;
DROP TABLE mcp_servers;
