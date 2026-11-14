-- +goose Up
-- Observation identities were already deterministic; enforce and index the scan join.
CREATE UNIQUE INDEX workflow_stage_observation_identity ON workflow_stage_integration_observations
(connection_id,connection_version,task_id,stage_version,configuration_version,rule_id);
CREATE INDEX workflow_stage_observation_activity ON workflow_stage_integration_observations(workspace_id,connection_id,created_at DESC,id DESC);
CREATE INDEX workflow_stage_event_subject ON workflow_stage_integration_events
(connection_id,action_id,(subject->>'kind'),(subject->>'id'),(subject->>'repositoryId'),occurred_at,semantic_id);
CREATE INDEX workflow_stage_effect_activity ON workflow_events(workspace_id,(integration->>'connectionId'),occurred_at DESC,id DESC)
WHERE event_type='integration_action';

CREATE INDEX workflow_stage_admission_event ON workflow_stage_integration_admissions(event_id);

-- +goose Down
DROP INDEX workflow_stage_admission_event;
DROP INDEX workflow_stage_effect_activity;
DROP INDEX workflow_stage_event_subject;
DROP INDEX workflow_stage_observation_activity;
DROP INDEX workflow_stage_observation_identity;
