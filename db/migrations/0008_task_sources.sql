-- +goose Up
ALTER TABLE tasks
    ADD COLUMN source_provider text NOT NULL DEFAULT 'manual',
    ADD COLUMN source_id text,
    ADD COLUMN source_url text,
    ADD CONSTRAINT tasks_source_identity_unique UNIQUE (workspace_id, source_provider, source_id),
    ADD CONSTRAINT tasks_source_identity_check CHECK (
        (source_provider = 'manual' AND source_id IS NULL)
        OR (source_provider <> 'manual' AND source_id IS NOT NULL)
    );

-- +goose Down
ALTER TABLE tasks
    DROP CONSTRAINT tasks_source_identity_check,
    DROP CONSTRAINT tasks_source_identity_unique,
    DROP COLUMN source_url,
    DROP COLUMN source_id,
    DROP COLUMN source_provider;
