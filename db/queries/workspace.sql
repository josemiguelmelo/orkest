-- name: ListWorkspaces :many
SELECT w.id, w.organization_id, w.name,
       CASE WHEN om.role IN ('owner', 'admin') THEN om.role ELSE wm.role END::text AS role
FROM workspaces w
LEFT JOIN organization_memberships om ON om.organization_id = w.organization_id AND om.user_id = $2
LEFT JOIN workspace_memberships wm ON wm.workspace_id = w.id AND wm.user_id = $2
WHERE w.organization_id = $1 AND (om.role IN ('owner', 'admin') OR wm.user_id IS NOT NULL)
ORDER BY w.name;

-- name: GetWorkspace :one
SELECT w.id, w.organization_id, w.name,
       CASE WHEN om.role IN ('owner', 'admin') THEN om.role ELSE wm.role END::text AS role
FROM workspaces w
LEFT JOIN organization_memberships om ON om.organization_id = w.organization_id AND om.user_id = $3
LEFT JOIN workspace_memberships wm ON wm.workspace_id = w.id AND wm.user_id = $3
WHERE w.organization_id = $1 AND w.id = $2 AND (om.role IN ('owner', 'admin') OR wm.user_id IS NOT NULL);

-- name: InsertWorkspace :one
INSERT INTO workspaces (id, organization_id, name) VALUES ($1, $2, $3)
RETURNING id, organization_id, name;

-- name: UpdateWorkspace :one
UPDATE workspaces
SET name = $3
WHERE organization_id = $1 AND id = $2
RETURNING id, organization_id, name;

-- name: DeleteWorkspace :execrows
DELETE FROM workspaces
WHERE organization_id = $1 AND id = $2;

-- name: ListWorkspaceMembers :many
SELECT u.id, u.username, u.email, m.role
FROM workspace_memberships m
JOIN users u ON u.id = m.user_id
WHERE m.organization_id = $1 AND m.workspace_id = $2
ORDER BY u.username;

-- name: UpsertWorkspaceMember :exec
INSERT INTO workspace_memberships (organization_id, workspace_id, user_id, role) VALUES ($1, $2, $3, $4) ON CONFLICT (workspace_id, user_id)
DO UPDATE SET role = EXCLUDED.role;

-- name: DeleteWorkspaceMember :execrows
DELETE FROM workspace_memberships
WHERE organization_id = $1 AND workspace_id = $2 AND user_id = $3;
