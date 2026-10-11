-- +goose Up
ALTER TABLE user_pvp_state ADD COLUMN last_auto_progress_day INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE user_pvp_state DROP COLUMN last_auto_progress_day;
