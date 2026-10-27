-- name: ListCodingIntegrations :many
SELECT id, workspace_id, name, provider_name, provider_config, updated_at
FROM coding_integrations WHERE workspace_id = $1 ORDER BY name, id;

-- name: GetCodingIntegration :one
SELECT id, workspace_id, name, provider_name, provider_config, encrypted_credential, updated_at
FROM coding_integrations WHERE workspace_id = $1 AND id = $2;

-- name: InsertCodingIntegration :one
INSERT INTO coding_integrations (id, workspace_id, name, provider_name, provider_config, encrypted_credential)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, workspace_id, name, provider_name, provider_config, updated_at;

-- name: UpdateCodingIntegration :execrows
UPDATE coding_integrations SET name=$3, provider_config=$4, encrypted_credential=$5, updated_at=now()
WHERE workspace_id=$1 AND id=$2;

-- name: DeleteCodingIntegration :execrows
DELETE FROM coding_integrations WHERE workspace_id=$1 AND id=$2;
