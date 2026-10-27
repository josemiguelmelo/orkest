-- name: ListWorkspaceEnvironment :many
SELECT name, (CASE WHEN is_secret THEN '' ELSE value END)::text AS value, is_secret AS encrypted
FROM workspace_environment WHERE workspace_id=$1 ORDER BY name;

-- name: ResolveWorkspaceEnvironment :many
SELECT name, value, is_secret AS encrypted
FROM workspace_environment WHERE workspace_id=$1 ORDER BY name;

-- name: PutWorkspaceEnvironment :exec
INSERT INTO workspace_environment(workspace_id,name,value,is_secret) VALUES($1,$2,$3,$4)
ON CONFLICT(workspace_id,name) DO UPDATE SET value=EXCLUDED.value,is_secret=EXCLUDED.is_secret,updated_at=now();

-- name: DeleteWorkspaceEnvironment :execrows
DELETE FROM workspace_environment WHERE workspace_id=$1 AND name=$2;
