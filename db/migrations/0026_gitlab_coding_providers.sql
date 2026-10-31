-- +goose Up
ALTER TABLE code_repositories DROP CONSTRAINT IF EXISTS code_repositories_provider_name_check;
ALTER TABLE code_repositories ADD CONSTRAINT code_repositories_provider_name_check
    CHECK (provider_name IN ('github', 'gitlab_cloud', 'gitlab_server'));

-- +goose Down
-- Migration 0020 already removed this whitelist. Restore that unrestricted schema.
ALTER TABLE code_repositories DROP CONSTRAINT IF EXISTS code_repositories_provider_name_check;
