-- +goose Up
ALTER TABLE agents DROP CONSTRAINT agents_runtime_check;
ALTER TABLE agents ADD CONSTRAINT agents_runtime_check CHECK (runtime IN ('opencode', 'claude-code', 'codex'));

-- +goose Down
-- Refuse rollback while Codex agents exist rather than deleting definitions.
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM agents WHERE runtime = 'codex') THEN
        RAISE EXCEPTION 'cannot remove codex runtime while Codex agents exist';
    END IF;
END $$;
ALTER TABLE agents DROP CONSTRAINT agents_runtime_check;
ALTER TABLE agents ADD CONSTRAINT agents_runtime_check CHECK (runtime IN ('opencode', 'claude-code'));
