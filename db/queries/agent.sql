-- name: ListAgents :many
SELECT a.id, a.workspace_id, a.name, a.runtime, a.provider, a.model, a.description, a.system_prompt
FROM agents a JOIN workspaces w ON w.id = a.workspace_id
WHERE w.organization_id = $1 AND a.workspace_id = $2
ORDER BY a.name, a.id;

-- name: GetAgent :one
SELECT a.id, a.workspace_id, a.name, a.runtime, a.provider, a.model, a.description, a.system_prompt
FROM agents a JOIN workspaces w ON w.id = a.workspace_id
WHERE w.organization_id = $1 AND a.workspace_id = $2 AND a.id = $3;

-- name: InsertAgent :one
INSERT INTO agents (id, workspace_id, name, runtime, provider, model, description, system_prompt)
SELECT $1, w.id, $3, $4, $5, $6, $7, $8 FROM workspaces w WHERE w.id = $2
RETURNING id, workspace_id, name, runtime, provider, model, description, system_prompt;

-- name: UpdateAgent :one
UPDATE agents a SET name = $4, runtime = $5, provider = $6, model = $7, description = $8, system_prompt = $9, updated_at = now()
FROM workspaces w
WHERE w.organization_id = $1 AND a.workspace_id = w.id AND a.workspace_id = $2 AND a.id = $3
RETURNING a.id, a.workspace_id, a.name, a.runtime, a.provider, a.model, a.description, a.system_prompt;

-- name: DeleteAgent :execrows
DELETE FROM agents a USING workspaces w
WHERE w.organization_id = $1 AND a.workspace_id = w.id AND a.workspace_id = $2 AND a.id = $3;

-- name: ListAgentSkillIDs :many
SELECT skill_id FROM agent_skills WHERE workspace_id = $1 AND agent_id = $2 ORDER BY skill_id;

-- name: DeleteAgentSkills :exec
DELETE FROM agent_skills WHERE workspace_id = $1 AND agent_id = $2;

-- name: InsertAgentSkill :exec
INSERT INTO agent_skills(workspace_id, agent_id, skill_id) VALUES($1, $2, $3);
