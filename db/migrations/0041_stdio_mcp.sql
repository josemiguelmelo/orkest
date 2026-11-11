-- +goose Up
ALTER TABLE mcp_servers
    ADD COLUMN transport text NOT NULL DEFAULT 'http',
    ADD COLUMN command text NOT NULL DEFAULT '',
    ADD COLUMN args text[] NOT NULL DEFAULT '{}',
    ADD COLUMN encrypted_environment text NOT NULL DEFAULT '',
    DROP CONSTRAINT mcp_servers_url_check,
    ADD CONSTRAINT mcp_servers_transport_check CHECK (
        (transport = 'http' AND length(trim(url)) > 0 AND command = '' AND cardinality(args) = 0 AND encrypted_environment = '') OR
        (transport = 'stdio' AND url = '' AND command LIKE '/%' AND encrypted_headers = '')
    );

-- +goose Down
-- Refuse rollback while stdio servers exist; remove them explicitly before rollback.
ALTER TABLE mcp_servers ADD CONSTRAINT mcp_servers_url_check CHECK (length(trim(url)) > 0);
ALTER TABLE mcp_servers
    DROP CONSTRAINT mcp_servers_transport_check,
    DROP COLUMN encrypted_environment,
    DROP COLUMN args,
    DROP COLUMN command,
    DROP COLUMN transport;
