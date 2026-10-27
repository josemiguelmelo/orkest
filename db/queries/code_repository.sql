-- name: ListCodeRepositories :many
SELECT cr.id, cr.workspace_id, cr.provider_name, cr.url, cr.integration_id, cr.provider_repository_id, cr.default_branch, cr.display_name, cr.is_private
FROM code_repositories cr JOIN workspaces w ON w.id = cr.workspace_id
WHERE w.organization_id = $1 AND cr.workspace_id = $2
ORDER BY cr.url, cr.id;

-- name: GetCodeRepository :one
SELECT cr.id, cr.workspace_id, cr.provider_name, cr.url, cr.integration_id, cr.provider_repository_id, cr.default_branch, cr.display_name, cr.is_private
FROM code_repositories cr JOIN workspaces w ON w.id = cr.workspace_id
WHERE w.organization_id = $1 AND cr.workspace_id = $2 AND cr.id = $3;

-- name: InsertIntegratedCodeRepository :one
INSERT INTO code_repositories (id, workspace_id, provider_name, url, integration_id, provider_repository_id, default_branch, display_name, is_private)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (workspace_id, provider_name, url) DO UPDATE
SET integration_id=EXCLUDED.integration_id,
    provider_repository_id=EXCLUDED.provider_repository_id,
    default_branch=EXCLUDED.default_branch,
    display_name=EXCLUDED.display_name,
    is_private=EXCLUDED.is_private
WHERE code_repositories.integration_id IS NULL
RETURNING id, workspace_id, provider_name, url, integration_id, provider_repository_id, default_branch, display_name, is_private;

-- name: DeleteCodeRepository :execrows
DELETE FROM code_repositories cr USING workspaces w
WHERE w.organization_id = $1 AND cr.workspace_id = w.id AND cr.workspace_id = $2 AND cr.id = $3;
