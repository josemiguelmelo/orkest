-- +goose Up
CREATE TABLE direct_agent_conversations (
    id text PRIMARY KEY,
    organization_id text NOT NULL,
    workspace_id text NOT NULL,
    agent_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (workspace_id, organization_id) REFERENCES workspaces(id, organization_id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, agent_id) REFERENCES agents(workspace_id, id) ON DELETE CASCADE,
    UNIQUE (workspace_id, id)
);
CREATE INDEX direct_agent_conversations_recent_idx ON direct_agent_conversations(workspace_id, created_at DESC);

-- Session identity and launch task_id are also used as the stable conversation
-- identity for direct chats. Those IDs need not reference a workflow task.
ALTER TABLE task_agent_sessions DROP CONSTRAINT task_agent_sessions_workspace_id_task_id_fkey;

-- +goose Down
DELETE FROM agent_launches WHERE task_id IN (SELECT id FROM direct_agent_conversations);
DELETE FROM task_agent_sessions WHERE task_id IN (SELECT id FROM direct_agent_conversations);
-- Rolling back removes direct conversations and their persisted run history;
-- it does not stop agent processes already running in execution environments.
ALTER TABLE task_agent_sessions
    ADD CONSTRAINT task_agent_sessions_workspace_id_task_id_fkey
    FOREIGN KEY (workspace_id, task_id) REFERENCES tasks(workspace_id, id) ON DELETE CASCADE;
DROP TABLE direct_agent_conversations;
