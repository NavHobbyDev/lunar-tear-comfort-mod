-- +goose Up
CREATE TABLE user_event_quest_tower_accumulation_rewards (
    user_id                                         INTEGER NOT NULL REFERENCES users(user_id),
    event_quest_chapter_id                          INTEGER NOT NULL,
    latest_reward_receive_quest_mission_clear_count INTEGER NOT NULL DEFAULT 0,
    latest_version                                  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, event_quest_chapter_id)
);

-- +goose Down
DROP TABLE IF EXISTS user_event_quest_tower_accumulation_rewards;
