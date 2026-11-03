-- name: LockArtifactTask :one
SELECT t.status,t.stage_version FROM tasks t JOIN workspaces w ON w.id=t.workspace_id
WHERE w.organization_id=$1 AND t.workspace_id=$2 AND t.id=$3 FOR UPDATE OF t;

-- name: ArtifactPublishingRun :one
SELECT stage_version FROM agent_launches WHERE run_id=$1 AND organization_id=$2 AND workspace_id=$3 AND task_id=$4 AND status IN ('starting','process_started','ready') FOR SHARE;

-- name: ArtifactPublication :one
SELECT r.*,a.kind,a.output_name FROM task_artifact_revisions r JOIN task_artifacts a ON a.id=r.artifact_id
WHERE r.workspace_id=$1 AND r.task_id=$2 AND r.idempotency_key=$3;

-- name: CreateArtifact :exec
INSERT INTO task_artifacts(id,workspace_id,task_id,kind,output_name) VALUES($1,$2,$3,$4,$5);

-- name: LockArtifact :one
SELECT * FROM task_artifacts WHERE workspace_id=$1 AND task_id=$2 AND id=$3 FOR UPDATE;

-- name: InsertArtifactRevision :one
INSERT INTO task_artifact_revisions(id,workspace_id,task_id,artifact_id,number,title,content,created_by,producing_run_id,stage_id,stage_version,idempotency_key,request)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,NULLIF(sqlc.arg(producing_run_id)::text,''),$9,$10,$11,$12) RETURNING *;

-- name: AdvanceArtifactRevision :exec
UPDATE task_artifacts SET revision=revision+1 WHERE id=$1;

-- name: ListTaskArtifacts :many
SELECT r.*,a.kind,a.output_name FROM task_artifacts a JOIN task_artifact_revisions r ON r.artifact_id=a.id AND r.number=a.revision
WHERE a.workspace_id=$1 AND a.task_id=$2 ORDER BY r.created_at,a.id;

-- name: ListArtifactRevisions :many
SELECT r.*,a.kind,a.output_name FROM task_artifact_revisions r JOIN task_artifacts a ON a.id=r.artifact_id
WHERE r.workspace_id=$1 AND r.task_id=$2 AND r.artifact_id=$3 ORDER BY r.number;

-- name: PinRunArtifactInput :execrows
INSERT INTO run_artifact_inputs(run_id,revision_id)
SELECT l.run_id,r.id FROM agent_launches l JOIN task_artifact_revisions r ON r.workspace_id=l.workspace_id AND r.task_id=l.task_id
WHERE l.run_id=$1 AND r.id=$2 ON CONFLICT DO NOTHING;
