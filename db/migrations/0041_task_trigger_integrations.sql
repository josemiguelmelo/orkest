-- +goose Up
CREATE TABLE task_trigger_integrations (
 id text PRIMARY KEY,
 workspace_id text NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
 name text NOT NULL CHECK(length(trim(name)) BETWEEN 1 AND 200),
 provider text NOT NULL,
 config jsonb NOT NULL,
 enabled boolean NOT NULL,
 version bigint NOT NULL DEFAULT 1,
 configured_by text NOT NULL,
 encrypted_secret text NOT NULL,
 UNIQUE(workspace_id,id)
);
CREATE TABLE task_trigger_rules (
 integration_id text NOT NULL,
 workspace_id text NOT NULL,
 id text NOT NULL,
 event_type text NOT NULL,
 workflow_id text NOT NULL,
 source_stage_id text NOT NULL,
 target_stage_id text NOT NULL,
 action text NOT NULL DEFAULT 'move_task' CHECK(action IN ('move_task','start_work')),
 prompt_text text NOT NULL DEFAULT '' CHECK(length(prompt_text)<=32768),
 PRIMARY KEY(integration_id,id),
 UNIQUE(integration_id,event_type,workflow_id,source_stage_id),
 CHECK(action='start_work' OR source_stage_id<>target_stage_id),
 FOREIGN KEY(workspace_id,integration_id) REFERENCES task_trigger_integrations(workspace_id,id) ON DELETE CASCADE,
 FOREIGN KEY(workspace_id,workflow_id,source_stage_id) REFERENCES workflow_stages(workspace_id,workflow_id,id),
 FOREIGN KEY(workspace_id,workflow_id,target_stage_id) REFERENCES workflow_stages(workspace_id,workflow_id,id)
);
CREATE TABLE task_trigger_resource_links (
 integration_id text NOT NULL,
 workspace_id text NOT NULL,
 repository_id text NOT NULL,
 resource_id text NOT NULL,
 task_id text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(integration_id,repository_id,resource_id),
 FOREIGN KEY(workspace_id,integration_id) REFERENCES task_trigger_integrations(workspace_id,id) ON DELETE CASCADE,
 FOREIGN KEY(workspace_id,task_id) REFERENCES tasks(workspace_id,id) ON DELETE CASCADE
);
CREATE TABLE task_trigger_deliveries (
 id text PRIMARY KEY,
 integration_id text NOT NULL REFERENCES task_trigger_integrations(id) ON DELETE CASCADE,
 delivery_id text NOT NULL,
 event_key text NOT NULL,
 event jsonb NOT NULL,
 task_id text NOT NULL DEFAULT '',
 stage_version bigint NOT NULL DEFAULT 0,
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','processed','ignored','blocked','failed')),
 received_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(integration_id,delivery_id,event_key),
 UNIQUE(integration_id,event_key)
);
CREATE INDEX task_trigger_deliveries_pending ON task_trigger_deliveries(received_at,id) WHERE status='pending';

-- +goose Down
-- Destructive: export integration configuration, links, and delivery audit first.
DROP TABLE task_trigger_deliveries;
DROP TABLE task_trigger_resource_links;
DROP TABLE task_trigger_rules;
DROP TABLE task_trigger_integrations;
