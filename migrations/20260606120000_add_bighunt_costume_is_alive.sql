-- +goose Up
ALTER TABLE user_big_hunt_costume_battle_infos ADD COLUMN is_alive INTEGER NOT NULL DEFAULT 1;

-- +goose Down
ALTER TABLE user_big_hunt_costume_battle_infos DROP COLUMN is_alive;
