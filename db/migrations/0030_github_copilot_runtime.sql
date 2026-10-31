-- +goose Up
ALTER TABLE agents DROP CONSTRAINT agents_runtime_check;
ALTER TABLE agents ADD CONSTRAINT agents_runtime_check CHECK (runtime IN ('opencode', 'claude-code', 'codex', 'github-copilot'));

ALTER TABLE agent_runtime_profiles DROP CONSTRAINT agent_runtime_profiles_runtime_check;
ALTER TABLE agent_runtime_profiles ADD CONSTRAINT agent_runtime_profiles_runtime_check CHECK (runtime IN ('opencode', 'claude-code', 'codex', 'github-copilot'));

-- +goose Down
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM agents WHERE runtime = 'github-copilot') THEN
        RAISE EXCEPTION 'cannot remove GitHub Copilot runtime while Copilot agents exist';
    END IF;
    IF EXISTS (SELECT 1 FROM agent_runtime_profiles WHERE runtime = 'github-copilot') THEN
        RAISE EXCEPTION 'cannot remove GitHub Copilot runtime while Copilot profiles exist';
    END IF;
END $$;

ALTER TABLE agent_runtime_profiles DROP CONSTRAINT agent_runtime_profiles_runtime_check;
ALTER TABLE agent_runtime_profiles ADD CONSTRAINT agent_runtime_profiles_runtime_check CHECK (runtime IN ('opencode', 'claude-code', 'codex'));
ALTER TABLE agents DROP CONSTRAINT agents_runtime_check;
ALTER TABLE agents ADD CONSTRAINT agents_runtime_check CHECK (runtime IN ('opencode', 'claude-code', 'codex'));
