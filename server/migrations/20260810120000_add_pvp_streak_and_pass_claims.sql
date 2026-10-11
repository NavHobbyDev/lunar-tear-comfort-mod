-- +goose Up
ALTER TABLE user_pvp_state ADD COLUMN attack_win_streak INTEGER NOT NULL DEFAULT 0;
ALTER TABLE user_mission_pass_points ADD COLUMN rewards_claimed INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE user_mission_pass_points DROP COLUMN rewards_claimed;
ALTER TABLE user_pvp_state DROP COLUMN attack_win_streak;
