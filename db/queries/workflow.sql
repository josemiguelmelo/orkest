-- name: GetWorkflow :one
SELECT * FROM workspace_workflows WHERE workspace_id=$1 AND id=$2;

-- name: ListWorkflows :many
SELECT * FROM workspace_workflows WHERE workspace_id=$1 ORDER BY name,id;

-- name: CreateWorkflow :exec
INSERT INTO workspace_workflows(workspace_id,id,name,version,configured_by,initial_stage_id,closed_stage_id) VALUES($1,$2,$3,$4,$5,$6,$7);

-- name: DeleteWorkflow :execrows
DELETE FROM workspace_workflows WHERE workspace_id=$1 AND id=$2;

-- name: LockWorkflows :many
SELECT id FROM workspace_workflows WHERE workspace_id=$1 ORDER BY id FOR UPDATE;

-- name: LockWorkflow :one
SELECT * FROM workspace_workflows WHERE workspace_id=$1 AND id=$2 FOR UPDATE;

-- name: SaveWorkflow :execrows
UPDATE workspace_workflows SET name=$4,configured_by=$5,initial_stage_id=$6,closed_stage_id=$7,version=version+1 WHERE workspace_id=$1 AND id=$2 AND version=$3;

-- name: ListWorkflowStages :many
SELECT * FROM workflow_stages WHERE workspace_id=$1 AND workflow_id=$2 ORDER BY position;

-- name: PutWorkflowStage :exec
INSERT INTO workflow_stages(workspace_id,workflow_id,id,responsible_agent_id,position,name,description,custom_prompt,artifact_inputs,artifact_outputs)
VALUES(sqlc.arg(workspace_id),sqlc.arg(workflow_id),sqlc.arg(id),NULLIF(sqlc.arg(agent_id)::text,''),sqlc.arg(position),sqlc.arg(name),sqlc.arg(description),sqlc.arg(custom_prompt),sqlc.arg(artifact_inputs),sqlc.arg(artifact_outputs))
ON CONFLICT(workspace_id,workflow_id,id) DO UPDATE SET responsible_agent_id=EXCLUDED.responsible_agent_id,
position=EXCLUDED.position,name=EXCLUDED.name,description=EXCLUDED.description,custom_prompt=EXCLUDED.custom_prompt,artifact_inputs=EXCLUDED.artifact_inputs,artifact_outputs=EXCLUDED.artifact_outputs;

-- name: RemoveWorkflowStages :exec
DELETE FROM workflow_stages WHERE workspace_id=$1 AND workflow_id=$2 AND NOT (id=ANY(sqlc.arg(ids)::text[]));

-- name: ValidateWorkflowAgent :one
SELECT EXISTS(SELECT 1 FROM agents WHERE workspace_id=$1 AND id=$2);

-- name: TaskWorkflowState :one
SELECT t.stage_version,t.stage_work_state,t.workflow_id FROM tasks t WHERE workspace_id=$1 AND id=$2;

-- name: LockWorkflowTask :one
SELECT t.stage_version,t.stage_work_state,t.workflow_id,COALESCE(t.workflow_run_id,'')::text AS workflow_run_id,t.status,COALESCE(t.assignee_agent_id,'')::text AS assignee
FROM tasks t WHERE workspace_id=$1 AND id=$2 FOR UPDATE;

-- name: SetStageWorkState :exec
UPDATE tasks SET stage_work_state=$4 WHERE workspace_id=$1 AND id=$2 AND stage_version=$3;

-- name: EnqueueWorkflowCommand :exec
INSERT INTO workflow_commands(id,workspace_id,task_id,stage_version,actor_id,options) VALUES($1,$2,$3,$4,$5,$6)
;



-- name: RunWorkflowEntry :one
SELECT l.workspace_id,l.task_id,l.stage_version,c.actor_id,t.workflow_id
FROM agent_launches l JOIN tasks t ON t.workspace_id=l.workspace_id AND t.id=l.task_id
LEFT JOIN workflow_commands c ON c.workspace_id=l.workspace_id AND c.task_id=l.task_id AND c.stage_version=l.stage_version
WHERE l.run_id=$1 ORDER BY c.created_at DESC LIMIT 1;

-- name: MoveWorkflowTask :exec
UPDATE tasks SET status=$4,assignee_agent_id=NULLIF($5,''),updated_at=now()
WHERE workspace_id=$1 AND id=$2 AND stage_version=$3;

-- name: WorkflowCommandAccepted :one
SELECT EXISTS(SELECT 1 FROM agent_prompt_outbox WHERE command_id=$1);

-- name: SetQueuedStageState :exec
UPDATE tasks SET stage_work_state=$4 WHERE workspace_id=$1 AND id=$2 AND stage_version=$3 AND stage_work_state='queued';

-- name: TaskExecutionFailed :one
SELECT (l.status='failed')::boolean AS failed FROM tasks t JOIN agent_launches l ON l.run_id=t.workflow_run_id
WHERE t.workspace_id=$1 AND t.id=$2 FOR SHARE OF t,l;

-- name: BindWorkflowRun :exec
UPDATE tasks SET workflow_run_id=$3 WHERE workspace_id=$1 AND id=$2;

-- name: GetWorkflowCommand :one
SELECT options,stage_version FROM workflow_commands WHERE workspace_id=$1 AND task_id=$2 AND id=$3;

-- name: RecordWorkflowTransition :exec
INSERT INTO workflow_commands(id,workspace_id,task_id,stage_version,actor_id,options,state) VALUES($1,$2,$3,$4,$5,$6,'delivered');

-- name: ImportedWorkflowTask :one
SELECT id,status,workflow_id FROM tasks WHERE workspace_id=$1 AND source_provider=$2 AND source_id=$3
AND (source_integration_id=$4 OR source_integration_id IS NULL) LIMIT 1 FOR UPDATE;

-- name: ListWorkflowRules :many
SELECT * FROM workflow_rules WHERE workspace_id=$1 AND workflow_id=$2 ORDER BY id;

-- name: DeleteWorkflowRules :exec
DELETE FROM workflow_rules WHERE workspace_id=$1 AND workflow_id=$2;

-- name: InsertWorkflowRule :exec
INSERT INTO workflow_rules(workspace_id,workflow_id,id,enabled,event_type,source_stage_id,action_type,target_stage_id)
VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF(sqlc.arg(target_stage_id)::text,''));

-- name: EnqueueWorkflowEvent :exec
INSERT INTO workflow_events(id,workspace_id,task_id,stage_version,actor_id,event_type,run_id,causation_id,correlation_id)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(id) DO NOTHING;

-- name: PendingWorkflowEvent :one
SELECT * FROM workflow_events WHERE processed_at IS NULL ORDER BY occurred_at,id LIMIT 1 FOR UPDATE SKIP LOCKED;

-- name: AcknowledgeWorkflowEvent :exec
UPDATE workflow_events SET processed_at=now(),configuration_version=$2,matched_rule_id=$3 WHERE id=$1;

-- name: WorkflowConfigurationAgents :many
SELECT id FROM agents WHERE workspace_id=$1 ORDER BY id FOR KEY SHARE;

-- name: OccupiedWorkflowStages :many
SELECT DISTINCT status FROM tasks WHERE workspace_id=$1 AND workflow_id=$2;

-- name: WorkflowHasReferences :one
SELECT EXISTS(SELECT 1 FROM tasks t WHERE t.workspace_id=sqlc.arg(workspace_id)::text AND t.workflow_id=sqlc.arg(workflow_id)::text)
 OR EXISTS(SELECT 1 FROM task_integrations i WHERE i.workspace_id=sqlc.arg(workspace_id)::text AND i.workflow_id=sqlc.arg(workflow_id)::text);

-- name: ClaimWorkflowCommand :one
WITH candidate AS (
 SELECT id FROM workflow_commands
 WHERE state='pending' AND (claim_until IS NULL OR claim_until<=now())
 ORDER BY created_at,id LIMIT 1 FOR UPDATE SKIP LOCKED
), claimed AS (
 UPDATE workflow_commands c SET claim_token=sqlc.arg(claim_token),
 claim_until=now()+sqlc.arg(lease_seconds)::double precision*interval '1 second',claim_attempt=c.claim_attempt+1
 FROM candidate WHERE c.id=candidate.id RETURNING c.*
)
SELECT claimed.*,w.organization_id FROM claimed JOIN workspaces w ON w.id=claimed.workspace_id;

-- name: LockWorkflowDeliveryClaim :one
SELECT id FROM workflow_commands
WHERE id=sqlc.arg(id) AND claim_token=sqlc.arg(claim_token) AND state='pending'
FOR UPDATE;

-- name: AcknowledgeWorkflowDeliveryClaim :execrows
UPDATE workflow_commands SET state=sqlc.arg(state),delivery_error=sqlc.arg(delivery_error),claim_until=NULL
WHERE id=sqlc.arg(id) AND claim_token=sqlc.arg(claim_token) AND state='pending';
