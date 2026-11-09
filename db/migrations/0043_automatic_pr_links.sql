-- +goose Up
ALTER TABLE task_trigger_deliveries DROP CONSTRAINT task_trigger_deliveries_status_check;
ALTER TABLE task_trigger_deliveries ADD CONSTRAINT task_trigger_deliveries_status_check CHECK(status IN ('awaiting_link','pending','processed','ignored','blocked','failed'));
ALTER TABLE task_trigger_deliveries ADD COLUMN resolution_after timestamptz NOT NULL DEFAULT now();
ALTER TABLE task_trigger_deliveries ADD COLUMN diagnostic text NOT NULL DEFAULT '';
CREATE INDEX task_trigger_deliveries_resolution ON task_trigger_deliveries(resolution_after,id) WHERE status='awaiting_link';
CREATE INDEX agent_launch_events_repository_observation ON agent_launch_events(run_id,sequence) WHERE event->>'type'='repository_observed';
CREATE TABLE task_pull_request_links (
 workspace_id text NOT NULL,
 provider text NOT NULL,
 repository_id text NOT NULL,
 resource_id text NOT NULL,
 task_id text NOT NULL,
 head_repository_id text NOT NULL,
 head_branch text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(workspace_id,provider,repository_id,resource_id),
 FOREIGN KEY(workspace_id,task_id) REFERENCES tasks(workspace_id,id) ON DELETE CASCADE
);
CREATE TABLE task_pull_request_suppressions (
 integration_id text NOT NULL REFERENCES task_trigger_integrations(id) ON DELETE CASCADE,
 repository_id text NOT NULL,
 resource_id text NOT NULL,
 PRIMARY KEY(integration_id,repository_id,resource_id)
);

CREATE TABLE task_pull_request_reconciliation (
 integration_id text PRIMARY KEY REFERENCES task_trigger_integrations(id) ON DELETE CASCADE,
 next_attempt timestamptz NOT NULL DEFAULT now(),
 activated_at timestamptz NOT NULL DEFAULT now(),
 configuration_version bigint NOT NULL DEFAULT 0,
 diagnostic text NOT NULL DEFAULT ''
);

-- +goose Down
-- Export automatic association provenance first. Waiting events are not replayed on downgrade.
UPDATE task_trigger_deliveries SET status='ignored' WHERE status='awaiting_link';
DROP TABLE task_pull_request_reconciliation;
DROP TABLE task_pull_request_suppressions;
DROP TABLE task_pull_request_links;
DROP INDEX agent_launch_events_repository_observation;
DROP INDEX task_trigger_deliveries_resolution;
ALTER TABLE task_trigger_deliveries DROP COLUMN diagnostic;
ALTER TABLE task_trigger_deliveries DROP COLUMN resolution_after;
ALTER TABLE task_trigger_deliveries DROP CONSTRAINT task_trigger_deliveries_status_check;
ALTER TABLE task_trigger_deliveries ADD CONSTRAINT task_trigger_deliveries_status_check CHECK(status IN ('pending','processed','ignored','blocked','failed'));
