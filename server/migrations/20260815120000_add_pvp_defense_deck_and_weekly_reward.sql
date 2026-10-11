-- +goose Up
ALTER TABLE user_pvp_state ADD COLUMN defense_deck_number INTEGER NOT NULL DEFAULT 0;
ALTER TABLE user_pvp_state ADD COLUMN weekly_reward_version INTEGER NOT NULL DEFAULT 0;
ALTER TABLE user_pvp_state ADD COLUMN pending_weekly_reward_group_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE user_pvp_state ADD COLUMN pending_weekly_reward_point INTEGER NOT NULL DEFAULT 0;
ALTER TABLE user_pvp_state ADD COLUMN pending_weekly_reward_season_id INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE user_pvp_state DROP COLUMN pending_weekly_reward_season_id;
ALTER TABLE user_pvp_state DROP COLUMN pending_weekly_reward_point;
ALTER TABLE user_pvp_state DROP COLUMN pending_weekly_reward_group_id;
ALTER TABLE user_pvp_state DROP COLUMN weekly_reward_version;
ALTER TABLE user_pvp_state DROP COLUMN defense_deck_number;
