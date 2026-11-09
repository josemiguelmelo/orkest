-- name: ListTaskTriggerIntegrations :many
SELECT i.*,w.organization_id FROM task_trigger_integrations i JOIN workspaces w ON w.id=i.workspace_id WHERE i.workspace_id=$1 ORDER BY i.name,i.id;
-- name: GetTaskTriggerIntegration :one
SELECT i.*,w.organization_id FROM task_trigger_integrations i JOIN workspaces w ON w.id=i.workspace_id WHERE i.id=$1;
-- name: LockTaskTriggerIntegration :one
SELECT i.*,w.organization_id FROM task_trigger_integrations i JOIN workspaces w ON w.id=i.workspace_id WHERE i.id=$1 FOR UPDATE OF i;
-- name: CreateTaskTriggerIntegration :exec
INSERT INTO task_trigger_integrations(id,workspace_id,name,provider,config,enabled,configured_by,encrypted_secret) VALUES($1,$2,$3,$4,$5,$6,$7,$8);
-- name: UpdateTaskTriggerIntegration :execrows
UPDATE task_trigger_integrations SET name=$3,config=$4,enabled=$5,configured_by=$6,encrypted_secret=$7,version=version+1 WHERE id=$1 AND version=$2;
-- name: DeleteTaskTriggerIntegration :execrows
DELETE FROM task_trigger_integrations WHERE workspace_id=$1 AND id=$2;
-- name: ListTaskTriggerRules :many
SELECT * FROM task_trigger_rules WHERE integration_id=$1 ORDER BY id;
-- name: DeleteTaskTriggerRules :exec
DELETE FROM task_trigger_rules WHERE integration_id=$1;
-- name: InsertTaskTriggerRule :exec
INSERT INTO task_trigger_rules(integration_id,workspace_id,id,event_type,workflow_id,source_stage_id,target_stage_id,action,prompt_text) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9);
-- name: PutTaskTriggerResourceLink :execrows
INSERT INTO task_trigger_resource_links(integration_id,workspace_id,repository_id,resource_id,task_id)
SELECT sqlc.arg(integration_id),sqlc.arg(workspace_id),sqlc.arg(repository_id),sqlc.arg(resource_id),t.id FROM tasks t
WHERE t.workspace_id=sqlc.arg(workspace_id) AND t.id=sqlc.arg(task_id)
AND EXISTS(SELECT 1 FROM task_repositories tr JOIN code_repositories r ON r.workspace_id=tr.workspace_id AND r.id=tr.repository_id WHERE tr.workspace_id=t.workspace_id AND tr.task_id=t.id AND r.provider_name=sqlc.arg(provider) AND r.provider_repository_id=sqlc.arg(repository_id))
ON CONFLICT(integration_id,repository_id,resource_id) DO NOTHING;
-- name: ListTaskTriggerResourceLinks :many
SELECT * FROM task_trigger_resource_links WHERE integration_id=$1 ORDER BY repository_id,resource_id;
-- name: DeleteTaskTriggerResourceLink :exec
DELETE FROM task_trigger_resource_links WHERE integration_id=$1 AND repository_id=$2 AND resource_id=$3;
-- name: InsertTaskTriggerDelivery :exec
INSERT INTO task_trigger_deliveries(id,integration_id,delivery_id,event_key,event,task_id,stage_version,status)
VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING;
-- name: NextTaskTriggerDelivery :one
SELECT * FROM task_trigger_deliveries WHERE status='pending' ORDER BY received_at,id LIMIT 1 FOR UPDATE SKIP LOCKED;
-- name: FinishTaskTriggerDelivery :exec
UPDATE task_trigger_deliveries SET status=$2 WHERE id=$1;
-- name: ListTaskTriggerDeliveries :many
SELECT * FROM task_trigger_deliveries WHERE integration_id=$1 ORDER BY received_at DESC,id DESC LIMIT 100;
-- name: TaskTriggerLinkActive :one
SELECT EXISTS(
 SELECT 1 FROM task_trigger_resource_links l
 JOIN task_repositories tr ON tr.workspace_id=l.workspace_id AND tr.task_id=l.task_id
 JOIN code_repositories r ON r.workspace_id=tr.workspace_id AND r.id=tr.repository_id
 WHERE l.integration_id=$1 AND l.task_id=$2 AND l.repository_id=$3 AND l.resource_id=$4
 AND r.provider_name=$5 AND r.provider_repository_id=l.repository_id
);

-- name: SuppressTaskTriggerLink :exec
INSERT INTO task_pull_request_suppressions(integration_id,repository_id,resource_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING;
-- name: UnsuppressTaskTriggerLink :exec
DELETE FROM task_pull_request_suppressions WHERE integration_id=$1 AND repository_id=$2 AND resource_id=$3;

-- name: LockTaskTriggerDelivery :one
SELECT * FROM task_trigger_deliveries WHERE id=$1 FOR UPDATE;

-- name: TaskTriggerIngestionLink :one
SELECT l.task_id,l.created_at,t.stage_version,
 (SELECT max(e.occurred_at) FROM workflow_events e WHERE e.workspace_id=t.workspace_id AND e.task_id=t.id AND e.event_type='stage_entered' AND e.stage_version=t.stage_version)::timestamptz AS stage_entered_at
FROM task_trigger_resource_links l JOIN tasks t ON t.workspace_id=l.workspace_id AND t.id=l.task_id
WHERE l.integration_id=$1 AND l.repository_id=$2 AND l.resource_id=$3;
