-- +goose Up
ALTER TABLE user_big_hunt_statuses ADD COLUMN last_daily_reward_received_day_version INTEGER NOT NULL DEFAULT 0;

CREATE TABLE user_big_hunt_costume_battle_infos (
    user_id                    INTEGER NOT NULL REFERENCES users(user_id),
    wave_index                 INTEGER NOT NULL DEFAULT 0,
    sort_order                 INTEGER NOT NULL,
    costume_id                 INTEGER NOT NULL DEFAULT 0,
    total_damage               INTEGER NOT NULL DEFAULT 0,
    hit_count                  INTEGER NOT NULL DEFAULT 0,
    random_display_value_type  INTEGER NOT NULL DEFAULT 0,
    random_display_value       INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, wave_index, sort_order)
);

-- +goose Down
DROP TABLE IF EXISTS user_big_hunt_costume_battle_infos;
ALTER TABLE user_big_hunt_statuses DROP COLUMN last_daily_reward_received_day_version;
