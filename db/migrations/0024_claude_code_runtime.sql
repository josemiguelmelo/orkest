-- +goose Up
ALTER TABLE agents DROP CONSTRAINT agents_runtime_check;
ALTER TABLE agents ADD CONSTRAINT agents_runtime_check CHECK (runtime IN ('opencode', 'claude-code'));

-- +goose Down
-- Refuse rollback while Claude Code agents exist rather than deleting definitions.
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM agents WHERE runtime = 'claude-code') THEN
        RAISE EXCEPTION 'cannot remove claude-code runtime while Claude Code agents exist';
    END IF;
END $$;
ALTER TABLE agents DROP CONSTRAINT agents_runtime_check;
ALTER TABLE agents ADD CONSTRAINT agents_runtime_check CHECK (runtime IN ('opencode'));
