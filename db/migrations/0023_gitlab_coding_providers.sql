-- +goose Up
ALTER TABLE code_repositories DROP CONSTRAINT code_repositories_provider_name_check;
ALTER TABLE code_repositories ADD CONSTRAINT code_repositories_provider_name_check
    CHECK (provider_name IN ('github', 'gitlab_cloud', 'gitlab_server'));

-- +goose Down
-- Rollback requires removing GitLab repository registrations first.
ALTER TABLE code_repositories DROP CONSTRAINT code_repositories_provider_name_check;
ALTER TABLE code_repositories ADD CONSTRAINT code_repositories_provider_name_check CHECK (provider_name IN ('github'));
