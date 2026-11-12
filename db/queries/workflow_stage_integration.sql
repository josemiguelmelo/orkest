-- name: ListWorkflowStageIntegrations :many
SELECT * FROM workflow_stage_integrations WHERE workspace_id=$1 ORDER BY name,id;

-- name: GetWorkflowStageIntegration :one
SELECT * FROM workflow_stage_integrations WHERE workspace_id=$1 AND id=$2;

-- name: GetWorkflowStageCallbackIntegration :one
SELECT i.*,w.organization_id FROM workflow_stage_integrations i JOIN workspaces w ON w.id=i.workspace_id WHERE i.id=$1;

-- name: LockWorkflowStageIntegration :one
SELECT * FROM workflow_stage_integrations WHERE id=$1 FOR UPDATE;

-- name: CreateWorkflowStageIntegration :one
INSERT INTO workflow_stage_integrations(id,workspace_id,provider,name,enabled,coding_integration_id,repository_ids,config,encrypted_secret,configured_by)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING *;

-- name: UpdateWorkflowStageIntegration :one
UPDATE workflow_stage_integrations SET name=$4,enabled=$5,repository_ids=$6,config=$7,encrypted_secret=$8,configured_by=$9,version=version+1,updated_at=now()
WHERE workspace_id=$1 AND id=$2 AND version=$3 RETURNING *;

-- name: DeleteWorkflowStageIntegration :execrows
DELETE FROM workflow_stage_integrations WHERE workspace_id=$1 AND id=$2;

-- name: WorkflowStageIntegrationRepositories :many
SELECT r.id,r.provider_name,r.provider_repository_id,r.url FROM code_repositories r
WHERE r.workspace_id=$1 AND r.integration_id=$2 AND r.id=ANY(sqlc.arg(repository_ids)::text[]) ORDER BY r.id FOR KEY SHARE;

-- name: WorkflowStageIntegrationCodingProvider :one
SELECT provider_name FROM coding_integrations WHERE workspace_id=$1 AND id=$2 FOR KEY SHARE;


-- name: WorkflowStageIntegrationReferenced :one
SELECT EXISTS(SELECT 1 FROM workflow_rules WHERE workspace_id=$1 AND integration->>'connectionId'=sqlc.arg(connection_id)::text);

-- name: InsertWorkflowStageReceipt :execrows
INSERT INTO workflow_stage_integration_receipts(workspace_id,connection_id,delivery_id,connection_version)
VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING;

-- name: InsertWorkflowStageProviderEvent :exec
INSERT INTO workflow_stage_integration_events(workspace_id,connection_id,semantic_id,action_id,subject,occurred_at,facts)
VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING;

-- name: ListWorkflowStageCandidates :many
SELECT i.*,w.organization_id,t.id AS task_id,t.workflow_id,t.status AS stage_id,t.stage_version,
 f.version AS configuration_version,r.id AS rule_id,r.integration,r.source_stage_id
FROM workflow_stage_integrations i
JOIN workspaces w ON w.id=i.workspace_id
JOIN workflow_rules r ON r.workspace_id=i.workspace_id AND r.integration->>'connectionId'=i.id
JOIN workspace_workflows f ON f.workspace_id=r.workspace_id AND f.id=r.workflow_id
JOIN tasks t ON t.workspace_id=r.workspace_id AND t.workflow_id=r.workflow_id AND t.status=r.source_stage_id
WHERE i.enabled AND r.enabled AND r.event_type='integration_action'
ORDER BY i.id,t.id,r.id;

-- name: GetWorkflowStageObservation :one
SELECT * FROM workflow_stage_integration_observations WHERE id=$1;

-- name: InsertWorkflowStageObservation :execrows
INSERT INTO workflow_stage_integration_observations(id,workspace_id,connection_id,connection_version,task_id,workflow_id,rule_id,stage_id,stage_version,configuration_version,action_id,activated_at,reconciled_at,revision_ids,subjects,last_error,attempted_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
ON CONFLICT DO NOTHING;

-- name: UpdateWorkflowStageObservation :exec
UPDATE workflow_stage_integration_observations SET reconciled_at=$2,last_error=$3,activated_at=$4,revision_ids=$5,subjects=$6,attempted_at=$7 WHERE id=$1;

-- name: ListWorkflowStageProviderEvents :many
SELECT e.* FROM workflow_stage_integration_events e
WHERE e.connection_id=$1 AND e.action_id=$2 AND e.occurred_at>sqlc.arg(activated_at)
AND NOT EXISTS(SELECT 1 FROM workflow_stage_integration_admissions a WHERE a.observation_id=$3 AND a.semantic_id=e.semantic_id)
AND NOT EXISTS(SELECT 1 FROM workflow_stage_integration_admissions a WHERE a.observation_id=$3 AND a.semantic_id='aggregate_satisfied')
ORDER BY e.occurred_at,e.semantic_id;

-- name: InsertWorkflowStageAdmission :exec
INSERT INTO workflow_stage_integration_admissions(observation_id,semantic_id,event_id) VALUES($1,$2,$3);

-- name: WorkflowStageAdmissionExists :one
SELECT EXISTS(SELECT 1 FROM workflow_stage_integration_admissions WHERE observation_id=$1 AND semantic_id=$2);

-- name: WorkflowStageAdmissionObservation :one
SELECT * FROM workflow_stage_integration_observations WHERE workspace_id=$1 AND id=$2 FOR UPDATE;

-- name: ListWorkflowStageObservations :many
SELECT * FROM workflow_stage_integration_observations WHERE workspace_id=$1 AND connection_id=$2 ORDER BY created_at DESC,id;

-- name: ListWorkflowStageEffectStatus :many
SELECT e.id,e.task_id,e.stage_version,e.integration,e.processed_at,e.processing_error,e.occurred_at,
 COALESCE(c.state,'')::text AS command_state,COALESCE(c.delivery_error,'')::text AS delivery_error,COALESCE(o.action_id,'')::text AS action_id
FROM workflow_events e
LEFT JOIN workflow_commands c ON c.id=e.id||':start'
LEFT JOIN workflow_stage_integration_admissions a ON a.event_id=e.id
LEFT JOIN workflow_stage_integration_observations o ON o.id=a.observation_id
WHERE e.workspace_id=$1 AND e.event_type='integration_action' AND e.integration->>'connectionId'=sqlc.arg(connection_id)::text
ORDER BY e.occurred_at DESC,e.id;

-- name: LockWorkflowStageRetryEvent :one
SELECT * FROM workflow_events WHERE workspace_id=$1 AND id=$2 AND event_type='integration_action' FOR UPDATE;

-- name: RetryWorkflowStageEvent :execrows
UPDATE workflow_events e SET processed_at=NULL,processing_error=''
WHERE e.workspace_id=$1 AND e.id=$2 AND e.event_type='integration_action' AND e.processed_at IS NOT NULL
AND e.processing_error IN ('busy','execution_busy','authorization_revoked','callback_prerequisites')
AND NOT EXISTS(SELECT 1 FROM agent_prompt_outbox p WHERE p.command_id=e.id||':start')
AND NOT EXISTS(SELECT 1 FROM workflow_commands c WHERE c.id=e.id||':start')
AND EXISTS(SELECT 1 FROM tasks t
 JOIN workspace_workflows w ON w.workspace_id=t.workspace_id AND w.id=t.workflow_id
 JOIN workflow_stage_integrations i ON i.workspace_id=t.workspace_id AND i.id=e.integration->>'connectionId'
 WHERE t.workspace_id=e.workspace_id AND t.id=e.task_id AND t.stage_version=e.stage_version
 AND w.version=(e.integration->>'configurationVersion')::bigint
 AND i.enabled AND i.version=(e.integration->>'connectionVersion')::bigint);

-- name: LocklessWorkflowStageCallbackIntegration :one
SELECT * FROM workflow_stage_integrations WHERE id=$1;

-- name: EnqueueWorkflowIntegrationEvent :exec
INSERT INTO workflow_events(id,workspace_id,task_id,stage_version,actor_id,event_type,run_id,causation_id,correlation_id,integration,occurred_at)
VALUES($1,$2,$3,$4,$5,'integration_action','',$6,$7,$8,$9) ON CONFLICT(id) DO NOTHING;

-- name: ListWorkflowStageTaskRepositories :many
SELECT r.* FROM code_repositories r JOIN task_repositories t ON t.workspace_id=r.workspace_id AND t.repository_id=r.id
WHERE t.workspace_id=$1 AND t.task_id=$2 ORDER BY r.id;
