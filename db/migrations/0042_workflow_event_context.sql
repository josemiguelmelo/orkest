-- +goose Up
-- Existing pending events keep an empty startup context.
ALTER TABLE workflow_events ADD COLUMN prompt_text text NOT NULL DEFAULT '';

-- +goose Down
-- Destructive: event startup context is lost. Stop workflow workers and drain
-- pending events before rollback; never drop context for queued external work.
ALTER TABLE workflow_events DROP COLUMN prompt_text;
