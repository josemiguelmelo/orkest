-- name: ListOrganizations :many
SELECT o.id, o.name, m.role
FROM organizations o
JOIN organization_memberships m ON m.organization_id = o.id
WHERE m.user_id = $1
ORDER BY o.name;

-- name: GetOrganization :one
SELECT o.id, o.name, m.role
FROM organizations o
JOIN organization_memberships m ON m.organization_id = o.id
WHERE o.id = $1 AND m.user_id = $2;

-- name: InsertOrganization :one
INSERT INTO organizations (id, name) VALUES ($1, $2)
RETURNING id, name;

-- name: InsertOrganizationOwner :exec
INSERT INTO organization_memberships (organization_id, user_id, role) VALUES ($1, $2, 'owner');

-- name: UpdateOrganization :one
UPDATE organizations o
SET name = $3
FROM organization_memberships m
WHERE o.id = $1 AND m.organization_id = o.id AND m.user_id = $2 AND m.role IN ('owner', 'admin')
RETURNING o.id, o.name, m.role;

-- name: DeleteOrganization :execrows
DELETE FROM organizations
WHERE id = $1;

-- name: GetOrganizationRole :one
SELECT role
FROM organization_memberships
WHERE user_id = $1 AND organization_id = $2;

-- name: ListOrganizationMembers :many
SELECT u.id, u.username, u.email, m.role
FROM organization_memberships m
JOIN users u ON u.id = m.user_id
WHERE m.organization_id = $1
ORDER BY u.username;

-- name: LockOrganization :one
SELECT id
FROM organizations
WHERE id = $1
FOR UPDATE;

-- name: GetOrganizationMemberRole :one
SELECT role
FROM organization_memberships
WHERE organization_id = $1 AND user_id = $2;

-- name: LockOrganizationMemberRole :one
SELECT role
FROM organization_memberships
WHERE organization_id = $1 AND user_id = $2
FOR UPDATE;

-- name: CountOrganizationOwners :one
SELECT count(*)
FROM organization_memberships
WHERE organization_id = $1 AND role = 'owner';

-- name: UpsertOrganizationMember :exec
INSERT INTO organization_memberships (organization_id, user_id, role) VALUES ($1, $2, $3) ON CONFLICT (organization_id, user_id)
DO UPDATE SET role = EXCLUDED.role;

-- name: DeleteOrganizationMember :execrows
DELETE FROM organization_memberships
WHERE organization_id = $1 AND user_id = $2;
