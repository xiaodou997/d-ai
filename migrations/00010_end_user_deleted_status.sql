-- +goose Up

ALTER TABLE iam_users DROP CONSTRAINT iam_users_status_check;
ALTER TABLE iam_users ADD CONSTRAINT iam_users_status_check
    CHECK (status IN ('active', 'disabled', 'locked', 'inherited_disabled', 'deleted'));

-- +goose Down

UPDATE iam_users SET status = 'disabled' WHERE status = 'deleted';
ALTER TABLE iam_users DROP CONSTRAINT iam_users_status_check;
ALTER TABLE iam_users ADD CONSTRAINT iam_users_status_check
    CHECK (status IN ('active', 'disabled', 'locked', 'inherited_disabled'));
