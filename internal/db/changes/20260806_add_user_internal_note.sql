BEGIN;

ALTER TABLE iam_users
ADD COLUMN internal_note TEXT NOT NULL DEFAULT '';

UPDATE dai_schema_metadata
SET version = 7
WHERE singleton = TRUE AND version = 6;

COMMIT;
