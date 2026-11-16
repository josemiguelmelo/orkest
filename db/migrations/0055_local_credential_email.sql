-- +goose Up
ALTER TABLE local_credentials
    ADD COLUMN IF NOT EXISTS normalized_email text;

UPDATE local_credentials AS credential
SET normalized_email = lower(trim(target.email))
FROM users AS target
WHERE target.id = credential.user_id
  AND credential.normalized_email IS NULL;

ALTER TABLE local_credentials
    ALTER COLUMN normalized_email SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS local_credentials_normalized_email_key
    ON local_credentials(normalized_email);

-- +goose Down
SELECT 1;
