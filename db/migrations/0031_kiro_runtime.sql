-- +goose Up
ALTER TABLE agents DROP CONSTRAINT agents_runtime_check;
ALTER TABLE agents ADD CONSTRAINT agents_runtime_check CHECK (runtime IN ('opencode', 'claude-code', 'codex', 'github-copilot', 'kiro'));

ALTER TABLE agent_runtime_profiles DROP CONSTRAINT agent_runtime_profiles_runtime_check;
ALTER TABLE agent_runtime_profiles ADD CONSTRAINT agent_runtime_profiles_runtime_check CHECK (runtime IN ('opencode', 'claude-code', 'codex', 'github-copilot', 'kiro'));

-- +goose Down
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM agents WHERE runtime = 'kiro') THEN
        RAISE EXCEPTION 'cannot remove Kiro runtime while Kiro agents exist';
    END IF;
    IF EXISTS (SELECT 1 FROM agent_runtime_profiles WHERE runtime = 'kiro') THEN
        RAISE EXCEPTION 'cannot remove Kiro runtime while Kiro profiles exist';
    END IF;
END $$;

ALTER TABLE agent_runtime_profiles DROP CONSTRAINT agent_runtime_profiles_runtime_check;
ALTER TABLE agent_runtime_profiles ADD CONSTRAINT agent_runtime_profiles_runtime_check CHECK (runtime IN ('opencode', 'claude-code', 'codex', 'github-copilot'));
ALTER TABLE agents DROP CONSTRAINT agents_runtime_check;
ALTER TABLE agents ADD CONSTRAINT agents_runtime_check CHECK (runtime IN ('opencode', 'claude-code', 'codex', 'github-copilot'));
