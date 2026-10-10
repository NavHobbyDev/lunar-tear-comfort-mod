-- +goose Up
ALTER TABLE user_pvp_state ADD COLUMN max_season_rank INTEGER NOT NULL DEFAULT 0;
ALTER TABLE user_pvp_state ADD COLUMN max_season_rank_season_id INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE user_pvp_state DROP COLUMN max_season_rank_season_id;
ALTER TABLE user_pvp_state DROP COLUMN max_season_rank;
