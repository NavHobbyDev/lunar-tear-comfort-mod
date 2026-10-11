-- +goose Up

ALTER TABLE user_main_quest ADD COLUMN suppress_replay_flow INTEGER NOT NULL DEFAULT 0;

-- +goose Down

ALTER TABLE user_main_quest DROP COLUMN suppress_replay_flow;
