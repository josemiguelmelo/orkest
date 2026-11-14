-- +goose Up
CREATE TABLE workflow_stage_integrations (
 id text PRIMARY KEY,
 workspace_id text NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
 provider text NOT NULL,
 name text NOT NULL CHECK(length(trim(name))>0),
 enabled boolean NOT NULL DEFAULT false,
 coding_integration_id text NOT NULL,
 repository_ids text[] NOT NULL DEFAULT '{}',
 config jsonb NOT NULL DEFAULT '{}',
 encrypted_secret text NOT NULL CHECK(length(encrypted_secret)>0),
 configured_by text NOT NULL,
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(workspace_id,id),
 FOREIGN KEY(workspace_id,coding_integration_id) REFERENCES coding_integrations(workspace_id,id) ON DELETE NO ACTION DEFERRABLE INITIALLY DEFERRED
);

CREATE TABLE workflow_stage_integration_receipts (
 workspace_id text NOT NULL,
 connection_id text NOT NULL,
 delivery_id text NOT NULL,
 connection_version bigint NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(connection_id,delivery_id),
 FOREIGN KEY(workspace_id,connection_id) REFERENCES workflow_stage_integrations(workspace_id,id) ON DELETE CASCADE
);
CREATE TABLE workflow_stage_integration_events (
 workspace_id text NOT NULL,
 connection_id text NOT NULL,
 semantic_id text NOT NULL,
 action_id text NOT NULL,
 subject jsonb NOT NULL,
 occurred_at timestamptz NOT NULL,
 facts jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(connection_id,semantic_id),
 FOREIGN KEY(workspace_id,connection_id) REFERENCES workflow_stage_integrations(workspace_id,id) ON DELETE CASCADE
);
CREATE INDEX workflow_stage_integration_event_time ON workflow_stage_integration_events(connection_id,occurred_at,semantic_id);
CREATE TABLE workflow_stage_integration_observations (
 id text PRIMARY KEY,
 workspace_id text NOT NULL,
 connection_id text NOT NULL,
 connection_version bigint NOT NULL,
 task_id text NOT NULL,
 workflow_id text NOT NULL,
 rule_id text NOT NULL,
 stage_id text NOT NULL,
 stage_version bigint NOT NULL,
 configuration_version bigint NOT NULL,
 action_id text NOT NULL,
 activated_at timestamptz NOT NULL,
 reconciled_at timestamptz NOT NULL,
 attempted_at timestamptz NOT NULL,
 revision_ids text[] NOT NULL DEFAULT '{}',
 subjects jsonb NOT NULL DEFAULT '[]',
 last_error text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(workspace_id,id),
 FOREIGN KEY(workspace_id,connection_id) REFERENCES workflow_stage_integrations(workspace_id,id) ON DELETE CASCADE,
 FOREIGN KEY(workspace_id,task_id) REFERENCES tasks(workspace_id,id) ON DELETE CASCADE
);
CREATE TABLE workflow_stage_integration_admissions (
 observation_id text NOT NULL REFERENCES workflow_stage_integration_observations(id) ON DELETE CASCADE,
 semantic_id text NOT NULL,
 event_id text NOT NULL REFERENCES workflow_events(id) ON DELETE CASCADE,
 PRIMARY KEY(observation_id,semantic_id)
);

-- +goose Down
-- Destructive: connection configuration and encrypted webhook secrets are removed.
-- Stop ingestion and reconcile pending workflow events/commands before rollback.
-- Refuse loss of deduplication/ownership state while external effects are pending.
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM workflow_stage_integration_admissions a JOIN workflow_events e ON e.id=a.event_id WHERE e.processed_at IS NULL)
 OR EXISTS(SELECT 1 FROM workflow_stage_integration_admissions a JOIN workflow_commands c ON c.id=a.event_id||':start' WHERE c.state='pending') THEN
  RAISE EXCEPTION 'reconcile integration events and commands before rollback';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE workflow_stage_integration_admissions;
DROP TABLE workflow_stage_integration_observations;
DROP TABLE workflow_stage_integration_events;
DROP TABLE workflow_stage_integration_receipts;
DROP TABLE workflow_stage_integrations;
