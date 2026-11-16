-- +goose Up
ALTER TABLE users ALTER COLUMN keycloak_subject DROP NOT NULL;

CREATE TABLE auth_bootstrap_state (
    id smallint PRIMARY KEY CHECK (id = 1),
    initialized boolean NOT NULL DEFAULT false
);
INSERT INTO auth_bootstrap_state(id) VALUES (1);

CREATE TABLE user_auth_identities (
    provider text NOT NULL CHECK (provider IN ('keycloak')),
    subject text NOT NULL,
    user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, subject),
    UNIQUE (provider, user_id)
);

INSERT INTO user_auth_identities(provider, subject, user_id)
SELECT 'keycloak', keycloak_subject, id FROM users WHERE keycloak_subject IS NOT NULL;

CREATE TABLE local_credentials (
    user_id text PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    normalized_username text NOT NULL UNIQUE,
    normalized_email text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE local_sessions (
    token_hash bytea PRIMARY KEY,
    user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz
);
CREATE INDEX local_sessions_user_idx ON local_sessions(user_id, expires_at);

-- +goose Down
DROP TABLE local_sessions;
DROP TABLE local_credentials;
DROP TABLE user_auth_identities;
DROP TABLE IF EXISTS auth_bootstrap_state;
ALTER TABLE users ALTER COLUMN keycloak_subject SET NOT NULL;
