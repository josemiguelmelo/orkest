-- name: CreateMCPServer :one
INSERT INTO mcp_servers (id, workspace_id, name, url, transport, command, args, encrypted_headers, encrypted_environment)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING *;

-- name: UpdateMCPServer :one
UPDATE mcp_servers SET name=$3, url=$4, command=$5, args=$6,
    encrypted_headers=$7, encrypted_environment=$8, updated_at=now()
WHERE workspace_id=$1 AND id=$2 AND transport=$9 RETURNING *;

-- name: GetMCPServer :one
SELECT * FROM mcp_servers WHERE workspace_id=$1 AND id=$2;

-- name: ListMCPServers :many
SELECT * FROM mcp_servers WHERE workspace_id=$1 ORDER BY name,id;

-- name: DeleteMCPServer :execrows
DELETE FROM mcp_servers WHERE workspace_id=$1 AND id=$2;
