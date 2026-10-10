-- +goose Up

-- The columns were folded into the initial schema; the original ALTER statements
-- would fail with "duplicate column name" on databases created from the current
-- initial file. Databases that already applied this version are tracked in
-- goose_db_version and never re-run it. Databases predating the inlining that
-- never applied this version get the columns through migrations.repairSchema
-- (requiredColumns lists them), which runs after goose.Up on every boot.
SELECT 1;

-- +goose Down

ALTER TABLE user_battle DROP COLUMN last_started_quest_id;
ALTER TABLE user_battle DROP COLUMN last_started_quest_at;

