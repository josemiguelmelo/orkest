-- name: RegisterUser :one
INSERT INTO users (id, keycloak_subject, username, email)
VALUES ($1, $2, $3, $4)
ON CONFLICT (keycloak_subject) DO UPDATE
SET keycloak_subject = EXCLUDED.keycloak_subject
RETURNING id, username, email;
