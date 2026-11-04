-- +goose Up
ALTER TABLE workflow_commands
 ADD COLUMN claim_token text NOT NULL DEFAULT '',
 ADD COLUMN claim_until timestamptz,
 ADD COLUMN claim_attempt integer NOT NULL DEFAULT 0 CHECK(claim_attempt >= 0),
 ADD COLUMN delivery_error text NOT NULL DEFAULT '';

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM workflow_commands WHERE state='pending' AND claim_token<>'') THEN
  RAISE EXCEPTION 'reconcile pending delivery claims before rollback';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE workflow_commands DROP COLUMN claim_token,DROP COLUMN claim_until,DROP COLUMN claim_attempt,DROP COLUMN delivery_error;
