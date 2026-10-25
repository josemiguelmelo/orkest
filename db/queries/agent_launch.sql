-- name: InsertAgentLaunch :exec
INSERT INTO agent_launches (run_id, organization_id, workspace_id, task_id, status)
VALUES ($1, $2, $3, $4, $5);

-- name: UpdateAgentLaunch :exec
UPDATE agent_launches
SET status = $2, process_id = $3, endpoint = $4, failure_stage = $5,
    failure_reason = $6, updated_at = now()
WHERE run_id = $1;

-- name: SaveAgentLaunchEvent :execrows
WITH inserted AS (
    INSERT INTO agent_launch_events (run_id, sequence, event)
    VALUES ($1, $2, $3)
    ON CONFLICT (run_id, sequence) DO NOTHING
    RETURNING run_id, sequence
)
UPDATE agent_launches l SET last_sequence = GREATEST(l.last_sequence, $2), updated_at = now()
WHERE l.run_id = $1 AND EXISTS (SELECT 1 FROM inserted);
