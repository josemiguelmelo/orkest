-- name: RegisterUser :one
WITH registered AS (
    INSERT INTO users (id, keycloak_subject, username, email)
    VALUES ($1, $2, $3, $4)
    ON CONFLICT (keycloak_subject) DO UPDATE SET keycloak_subject = EXCLUDED.keycloak_subject
    RETURNING id, username, email
), identity AS (
    INSERT INTO user_auth_identities(provider, subject, user_id)
    SELECT 'keycloak', $2, id FROM registered ON CONFLICT(provider, subject) DO NOTHING
)
SELECT id, username, email FROM registered;

-- name: IsPlatformAdmin :one
SELECT platform_admin FROM users WHERE id = $1;

-- name: PromotePlatformAdmin :execrows
UPDATE users SET platform_admin = true WHERE id = $1;

-- name: ProvisionLocalAccount :one
WITH new_user AS (
    INSERT INTO users (id, keycloak_subject, username, email)
    VALUES (sqlc.arg(id), NULL, sqlc.arg(username), sqlc.arg(email))
    RETURNING id, username, email
), credential AS (
    INSERT INTO local_credentials(user_id, normalized_username, normalized_email, password_hash)
    SELECT id, sqlc.arg(normalized_username), sqlc.arg(normalized_email), sqlc.arg(password_hash) FROM new_user
), membership AS (
    INSERT INTO organization_memberships(organization_id, user_id, role)
    SELECT sqlc.arg(organization_id), id, sqlc.arg(role) FROM new_user
)
SELECT id, username, email FROM new_user;

-- name: BootstrapLocalAdminRequired :one
SELECT NOT EXISTS (
    SELECT 1 FROM local_credentials AS credential
    JOIN users AS target ON target.id = credential.user_id
    WHERE credential.normalized_username = $1 AND target.platform_admin
);

-- name: BootstrapLocalAdmin :one
WITH claimed AS (
    UPDATE auth_bootstrap_state SET initialized = true
    WHERE id = 1 AND (initialized = false OR NOT EXISTS (
        SELECT 1 FROM local_credentials AS credential
        JOIN users AS target ON target.id = credential.user_id
        WHERE credential.normalized_username = sqlc.arg(normalized_username) AND target.platform_admin
    ))
    RETURNING id
), existing AS (
    UPDATE users AS target SET platform_admin = true
    FROM local_credentials AS credential, claimed
    WHERE credential.user_id = target.id
      AND credential.normalized_username = sqlc.arg(normalized_username)
    RETURNING target.id, target.username, target.email
), account AS (
    INSERT INTO users (id, keycloak_subject, username, email, platform_admin)
    SELECT sqlc.arg(id), NULL, sqlc.arg(username), sqlc.arg(email), true FROM claimed
    WHERE NOT EXISTS (SELECT 1 FROM existing)
    RETURNING id, username, email
), credential AS (
    INSERT INTO local_credentials(user_id, normalized_username, normalized_email, password_hash)
    SELECT id, sqlc.arg(normalized_username), sqlc.arg(normalized_email), sqlc.arg(password_hash) FROM account
)
SELECT id, username, email FROM existing
UNION ALL
SELECT id, username, email FROM account;

-- name: LocalCredentialByUsername :one
SELECT u.id, u.username, u.email, c.password_hash
FROM local_credentials c JOIN users u ON u.id = c.user_id
WHERE c.normalized_username = $1;

-- name: LocalCredentialByUserID :one
SELECT u.id, u.username, u.email, c.password_hash
FROM local_credentials c JOIN users u ON u.id = c.user_id
WHERE c.user_id = $1;

-- name: HasLocalPassword :one
SELECT EXISTS(SELECT 1 FROM local_credentials WHERE user_id = $1);

-- name: UpdateLocalPassword :execrows
UPDATE local_credentials
SET password_hash = sqlc.arg(new_password), updated_at = now()
WHERE user_id = sqlc.arg(user_id) AND password_hash = sqlc.arg(current_hash);

-- name: CreateLocalSession :exec
INSERT INTO local_sessions(token_hash, user_id, expires_at) VALUES($1, $2, $3);

-- name: UserForLocalSession :one
SELECT u.id, u.username, u.email
FROM local_sessions s JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1 AND s.revoked_at IS NULL AND s.expires_at > now();

-- name: RevokeLocalSession :exec
UPDATE local_sessions SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL;

-- name: LinkKeycloakIdentityByEmail :one
WITH candidate AS (
    SELECT id FROM users WHERE lower(trim(email)) = lower(trim(sqlc.arg(email)))
      AND id IN (SELECT user_id FROM local_credentials)
), linked AS (
    UPDATE users AS target SET keycloak_subject = sqlc.arg(subject)
    WHERE target.id = (SELECT id FROM candidate) AND (SELECT count(*) FROM candidate) = 1
      AND (target.keycloak_subject IS NULL OR target.keycloak_subject = sqlc.arg(subject))
      AND NOT EXISTS (SELECT 1 FROM users WHERE users.keycloak_subject = sqlc.arg(subject) AND users.id <> target.id)
    RETURNING id, username, email
), identity AS (
    INSERT INTO user_auth_identities(provider, subject, user_id)
    SELECT 'keycloak', sqlc.arg(subject), id FROM linked
    ON CONFLICT(provider, subject) DO NOTHING
)
SELECT linked.id, linked.username, linked.email,
       (SELECT count(*) FROM candidate)::bigint AS candidate_count
FROM (SELECT 1) seed LEFT JOIN linked ON true;

-- name: HasLocalAccountEmail :one
SELECT EXISTS(
    SELECT 1 FROM users WHERE lower(trim(email)) = lower(trim($1))
      AND id IN (SELECT user_id FROM local_credentials)
);
