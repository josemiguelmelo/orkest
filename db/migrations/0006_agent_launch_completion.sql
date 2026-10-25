-- +goose Up
ALTER TABLE agent_launches DROP CONSTRAINT agent_launches_status_check;
ALTER TABLE agent_launches ADD CONSTRAINT agent_launches_status_check CHECK (
    status IN (
        'starting', 'process_started', 'ready', 'failed',
        'completed_pending_close', 'failed_pending_close', 'completed'
    )
);

-- +goose Down
-- Rollback collapses completed states to failed to satisfy the old status constraint.
UPDATE agent_launches
SET status = 'failed'
WHERE status IN ('completed_pending_close', 'failed_pending_close', 'completed');
ALTER TABLE agent_launches DROP CONSTRAINT agent_launches_status_check;
ALTER TABLE agent_launches ADD CONSTRAINT agent_launches_status_check CHECK (
    status IN ('starting', 'process_started', 'ready', 'failed')
);
