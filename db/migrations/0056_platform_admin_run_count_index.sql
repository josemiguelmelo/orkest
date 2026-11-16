-- +goose NO TRANSACTION
-- +goose Up
-- Supports the platform dashboard's rolling 30-day run count without scanning
-- task history or run events.
CREATE INDEX CONCURRENTLY IF NOT EXISTS agent_launches_created_at_idx
    ON agent_launches (created_at);

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS agent_launches_created_at_idx;
