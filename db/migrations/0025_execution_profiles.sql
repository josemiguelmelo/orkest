-- +goose Up
CREATE TABLE execution_profiles (
 id text PRIMARY KEY,
 workspace_id text NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
 name text NOT NULL, description text NOT NULL DEFAULT '', runtime text NOT NULL,
 toolchain_image text NOT NULL, generation bigint NOT NULL DEFAULT 1,
 status text NOT NULL DEFAULT 'queued', execution_image text NOT NULL DEFAULT '', build_error text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(workspace_id,id)
);
CREATE TABLE execution_profile_builds (
 id text PRIMARY KEY, workspace_id text NOT NULL, profile_id text NOT NULL,
 generation bigint NOT NULL, runtime text NOT NULL, toolchain_image text NOT NULL,
 base_image text NOT NULL, destination text NOT NULL, runtime_version text NOT NULL,
 resolved_toolchain text NOT NULL DEFAULT '', resolved_base text NOT NULL DEFAULT '',
 status text NOT NULL DEFAULT 'queued', execution_image text NOT NULL DEFAULT '', build_error text NOT NULL DEFAULT '',
 owner text NOT NULL DEFAULT '', lease_until timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(workspace_id,profile_id) REFERENCES execution_profiles(workspace_id,id) ON DELETE CASCADE,
 UNIQUE(profile_id,generation)
);
CREATE INDEX execution_build_queue_idx ON execution_profile_builds(status,created_at);
ALTER TABLE agents ADD COLUMN execution_profile_id text;
ALTER TABLE agents ADD CONSTRAINT agents_execution_profile_fk FOREIGN KEY(workspace_id,execution_profile_id) REFERENCES execution_profiles(workspace_id,id);

ALTER TABLE agent_launches ADD COLUMN execution_image text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE agent_launches DROP COLUMN execution_image;
ALTER TABLE agents DROP CONSTRAINT agents_execution_profile_fk;
ALTER TABLE agents DROP COLUMN execution_profile_id;
DROP TABLE execution_profile_builds;
DROP TABLE execution_profiles;
