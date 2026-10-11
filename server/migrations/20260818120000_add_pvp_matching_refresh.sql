-- +goose Up
ALTER TABLE user_pvp_state ADD COLUMN matching_refresh_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE user_pvp_state ADD COLUMN matching_refresh_day INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE user_pvp_state DROP COLUMN matching_refresh_day;
ALTER TABLE user_pvp_state DROP COLUMN matching_refresh_count;
