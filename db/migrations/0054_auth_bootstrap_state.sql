-- +goose Up
CREATE TABLE IF NOT EXISTS auth_bootstrap_state (
    id smallint PRIMARY KEY CHECK (id = 1),
    initialized boolean NOT NULL DEFAULT false
);
INSERT INTO auth_bootstrap_state(id) VALUES (1) ON CONFLICT (id) DO NOTHING;

-- +goose Down
DROP TABLE auth_bootstrap_state;
