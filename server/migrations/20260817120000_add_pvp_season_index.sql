-- +goose Up
ALTER TABLE user_pvp_state ADD COLUMN season_index INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE user_pvp_state DROP COLUMN season_index;
