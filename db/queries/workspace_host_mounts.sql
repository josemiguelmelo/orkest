-- name: ListWorkspaceHostMounts :one
SELECT mounts FROM workspace_host_mounts WHERE workspace_id = $1;

-- name: PutWorkspaceHostMounts :exec
INSERT INTO workspace_host_mounts (workspace_id, mounts)
VALUES ($1, $2)
ON CONFLICT (workspace_id) DO UPDATE SET mounts = EXCLUDED.mounts;
