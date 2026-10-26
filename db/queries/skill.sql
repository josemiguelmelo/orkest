-- name: ListVisibleSkills :many
SELECT s.id, s.scope, s.organization_id, s.workspace_id, s.skill_md, s.created_at, s.updated_at
FROM skills s
WHERE s.scope = 'global'
   OR (s.scope = 'organization' AND s.organization_id = $1)
   OR (s.scope = 'workspace' AND s.organization_id = $1 AND s.workspace_id = $2)
ORDER BY s.scope, s.id;

-- name: GetVisibleSkill :one
SELECT s.id, s.scope, s.organization_id, s.workspace_id, s.skill_md, s.created_at, s.updated_at
FROM skills s
WHERE s.id = $3 AND (
    s.scope = 'global'
    OR (s.scope = 'organization' AND s.organization_id = $1)
    OR (s.scope = 'workspace' AND s.organization_id = $1 AND s.workspace_id = $2)
);

-- name: GetSkill :one
SELECT id, scope, organization_id, workspace_id, skill_md, created_at, updated_at
FROM skills WHERE id = $1;

-- name: InsertSkill :one
INSERT INTO skills(id, scope, organization_id, workspace_id, skill_md)
VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), $5)
RETURNING id, scope, organization_id, workspace_id, skill_md, created_at, updated_at;

-- name: UpdateSkill :one
UPDATE skills SET skill_md = $2, updated_at = now()
WHERE id = $1
RETURNING id, scope, organization_id, workspace_id, skill_md, created_at, updated_at;

-- name: DeleteSkill :execrows
DELETE FROM skills WHERE id = $1;
