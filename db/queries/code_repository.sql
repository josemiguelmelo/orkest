-- name: ListCodeRepositories :many
SELECT cr.id, cr.workspace_id, cr.provider_name, cr.url
FROM code_repositories cr JOIN workspaces w ON w.id = cr.workspace_id
WHERE w.organization_id = $1 AND cr.workspace_id = $2
ORDER BY cr.url, cr.id;

-- name: GetCodeRepository :one
SELECT cr.id, cr.workspace_id, cr.provider_name, cr.url
FROM code_repositories cr JOIN workspaces w ON w.id = cr.workspace_id
WHERE w.organization_id = $1 AND cr.workspace_id = $2 AND cr.id = $3;

-- name: InsertCodeRepository :one
INSERT INTO code_repositories (id, workspace_id, provider_name, url)
SELECT $1, w.id, $3, $4 FROM workspaces w WHERE w.id = $2
RETURNING id, workspace_id, provider_name, url;

-- name: DeleteCodeRepository :execrows
DELETE FROM code_repositories cr USING workspaces w
WHERE w.organization_id = $1 AND cr.workspace_id = w.id AND cr.workspace_id = $2 AND cr.id = $3;
