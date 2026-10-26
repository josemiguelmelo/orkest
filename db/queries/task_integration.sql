-- name: CreateTaskIntegration :one
INSERT INTO task_integrations (id, workspace_id, provider, name, enabled, provider_config, labels, interval_seconds, encrypted_credential)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
RETURNING *;

-- name: UpdateTaskIntegration :one
UPDATE task_integrations SET provider=$3, name=$4, enabled=$5, provider_config=$6, labels=$7, interval_seconds=$8, encrypted_credential=$9, updated_at=now()
WHERE workspace_id=$1 AND id=$2 RETURNING *;

-- name: GetTaskIntegration :one
SELECT * FROM task_integrations WHERE workspace_id=$1 AND id=$2;

-- name: ListTaskIntegrations :many
SELECT * FROM task_integrations WHERE workspace_id=$1 ORDER BY created_at, id;

-- name: DeleteTaskIntegration :execrows
DELETE FROM task_integrations WHERE workspace_id=$1 AND id=$2;

-- name: DueTaskIntegrations :many
SELECT * FROM task_integrations WHERE enabled AND (last_synced_at IS NULL OR last_synced_at <= sqlc.arg(now)::timestamptz - interval_seconds * interval '1 second') ORDER BY last_synced_at NULLS FIRST;

-- name: RecordTaskIntegrationSync :exec
UPDATE task_integrations SET last_synced_at=$2, last_error=$3, updated_at=now() WHERE id=$1;

-- name: UpsertImportedTask :exec
WITH legacy AS (
    UPDATE tasks SET source_integration_id = sqlc.arg(source_integration_id), title = sqlc.arg(title),
        description = sqlc.arg(description), source_url = sqlc.narg(source_url), labels = sqlc.arg(labels),
        status = CASE WHEN sqlc.arg(closed)::boolean THEN 'done' ELSE status END, updated_at = now()
    WHERE workspace_id = sqlc.arg(workspace_id) AND source_provider = sqlc.arg(source_provider)
      AND source_id = sqlc.arg(source_id) AND source_integration_id IS NULL
    RETURNING id
)
INSERT INTO tasks (id, workspace_id, title, description, source_provider, source_integration_id, source_id, source_url, labels, status)
SELECT sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(title), sqlc.arg(description), sqlc.arg(source_provider),
    sqlc.arg(source_integration_id), sqlc.arg(source_id), sqlc.narg(source_url), sqlc.arg(labels),
    CASE WHEN sqlc.arg(closed)::boolean THEN 'done' ELSE 'todo' END
WHERE NOT EXISTS (SELECT 1 FROM legacy)
ON CONFLICT (workspace_id, source_provider, source_integration_id, source_id)
DO UPDATE SET title=EXCLUDED.title, description=EXCLUDED.description, source_url=EXCLUDED.source_url, labels=EXCLUDED.labels,
    status=CASE WHEN sqlc.arg(closed)::boolean THEN 'done' ELSE tasks.status END, updated_at=now();

-- name: RemoveMissingImportedTasks :exec
DELETE FROM tasks
WHERE workspace_id = sqlc.arg(workspace_id) AND source_integration_id = sqlc.arg(source_integration_id)
  AND source_id <> ALL(sqlc.arg(source_ids)::text[]);
