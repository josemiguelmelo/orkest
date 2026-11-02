-- name: InsertAgentLaunch :exec
INSERT INTO agent_launches (run_id, organization_id, workspace_id, task_id, status, task_session_id, execution_image, stage_version)
VALUES ($1, $2, $3, $4, $5, $6, $7, COALESCE((SELECT stage_version FROM tasks WHERE workspace_id=$3 AND id=$4),0));

-- name: UpdateAgentLaunch :exec
UPDATE agent_launches
SET status = $2, process_id = $3, endpoint = $4, failure_stage = $5,
    failure_reason = $6, updated_at = now()
WHERE run_id = $1;

-- name: ListReconnectableAgentLaunches :many
SELECT launch.run_id, launch.execution_image, launch.task_session_id, launch.organization_id, launch.workspace_id, launch.task_id,
       launch.status, launch.process_id, launch.endpoint, launch.failure_stage,
       launch.failure_reason, launch.last_sequence,
       COALESCE(latest.event->>'type', '')::text AS last_event_type, launch.updated_at
FROM agent_launches AS launch
LEFT JOIN LATERAL (
    SELECT event FROM agent_launch_events
    WHERE run_id = launch.run_id
    ORDER BY sequence DESC LIMIT 1
) AS latest ON true
WHERE launch.status IN (
    'starting', 'process_started', 'ready', 'failed',
    'completed_pending_close', 'failed_pending_close'
)
ORDER BY launch.created_at;

-- name: ListTaskAgentLaunches :many
SELECT run_id, execution_image, task_session_id, organization_id, workspace_id, task_id, status, endpoint, failure_stage,
       failure_reason, last_sequence, created_at, updated_at
FROM agent_launches
WHERE organization_id = $1 AND workspace_id = $2 AND task_id = $3
ORDER BY created_at DESC;

-- name: ListTaskAgentLaunchEvents :many
SELECT event, sequence FROM agent_launch_events
WHERE run_id = $1 AND sequence > $2
ORDER BY sequence;

-- name: SaveTerminalAgentLaunchEvent :exec
WITH inserted AS (
    INSERT INTO agent_launch_events (run_id, sequence, event)
    VALUES ($1, $2, $3)
    ON CONFLICT (run_id, sequence) DO NOTHING
    RETURNING run_id
)
UPDATE agent_launches AS launch
SET status = $4, last_sequence = GREATEST(launch.last_sequence, $2), updated_at = now()
WHERE launch.run_id = $1
  AND (EXISTS (SELECT 1 FROM inserted) OR EXISTS (
      SELECT 1 FROM agent_launch_events AS event
      WHERE event.run_id = $1 AND event.sequence = $2
  ));

-- name: SetAgentLaunchStatus :exec
UPDATE agent_launches
SET status = $2, updated_at = now()
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
