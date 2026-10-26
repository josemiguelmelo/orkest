-- +goose Up
ALTER TABLE task_integrations DROP CONSTRAINT task_integrations_provider_check;
UPDATE task_integrations SET provider = 'github_cloud' WHERE provider = 'github';
UPDATE task_integrations SET provider = 'jira_server' WHERE provider = 'jira';
ALTER TABLE task_integrations ADD CONSTRAINT task_integrations_provider_check
    CHECK (provider IN ('github_cloud', 'jira_server'));

-- +goose Down
ALTER TABLE task_integrations DROP CONSTRAINT task_integrations_provider_check;
UPDATE task_integrations SET provider = 'github' WHERE provider = 'github_cloud';
UPDATE task_integrations SET provider = 'jira' WHERE provider = 'jira_server';
ALTER TABLE task_integrations ADD CONSTRAINT task_integrations_provider_check
    CHECK (provider IN ('github', 'jira'));
