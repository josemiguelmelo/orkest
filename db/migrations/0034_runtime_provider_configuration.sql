-- +goose Up
ALTER TABLE agents ADD COLUMN runtime_configuration bytea;
ALTER TABLE agent_runtime_profiles ADD COLUMN runtime_configuration bytea;

-- +goose Down
ALTER TABLE agents DROP COLUMN runtime_configuration;
ALTER TABLE agent_runtime_profiles DROP COLUMN runtime_configuration;
