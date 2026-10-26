-- +goose Up
ALTER TABLE task_integrations DROP CONSTRAINT task_integrations_provider_check;
ALTER TABLE task_integrations ADD COLUMN provider_config jsonb NOT NULL DEFAULT '{}'::jsonb;

UPDATE tasks SET source_provider = 'github_cloud' WHERE source_provider = 'github';
UPDATE tasks SET source_provider = 'jira_server' WHERE source_provider = 'jira';

UPDATE task_integrations
SET provider_config = jsonb_build_object('projectUrl', project_url)
WHERE provider = 'github_cloud';

UPDATE task_integrations
SET provider_config = jsonb_build_object('baseUrl', base_url, 'projects', to_jsonb(projects))
WHERE provider = 'jira_server';

ALTER TABLE task_integrations
    DROP COLUMN base_url,
    DROP COLUMN project_url,
    DROP COLUMN projects;

-- +goose Down
UPDATE tasks SET source_provider = 'github' WHERE source_provider = 'github_cloud';
UPDATE tasks SET source_provider = 'jira' WHERE source_provider = 'jira_server';

ALTER TABLE task_integrations
    ADD COLUMN base_url text NOT NULL DEFAULT '',
    ADD COLUMN project_url text NOT NULL DEFAULT '',
    ADD COLUMN projects text[] NOT NULL DEFAULT '{}';

UPDATE task_integrations
SET project_url = COALESCE(provider_config->>'projectUrl', '')
WHERE provider = 'github_cloud';

UPDATE task_integrations
SET base_url = COALESCE(provider_config->>'baseUrl', ''),
    projects = ARRAY(SELECT jsonb_array_elements_text(provider_config->'projects'))
WHERE provider = 'jira_server';

ALTER TABLE task_integrations DROP COLUMN provider_config;
ALTER TABLE task_integrations ADD CONSTRAINT task_integrations_provider_check
    CHECK (provider IN ('github_cloud', 'jira_server'));
