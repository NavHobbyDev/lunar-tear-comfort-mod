-- +goose Up
CREATE TABLE user_event_quest_labyrinth_seasons (
    user_id                                   INTEGER NOT NULL REFERENCES users(user_id),
    event_quest_chapter_id                    INTEGER NOT NULL,
    last_join_season_number                   INTEGER NOT NULL DEFAULT 0,
    last_season_reward_received_season_number INTEGER NOT NULL DEFAULT 0,
    latest_version                            INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, event_quest_chapter_id)
);

CREATE TABLE user_event_quest_labyrinth_stages (
    user_id                                          INTEGER NOT NULL REFERENCES users(user_id),
    event_quest_chapter_id                           INTEGER NOT NULL,
    stage_order                                      INTEGER NOT NULL,
    is_received_stage_clear_reward                   INTEGER NOT NULL DEFAULT 0,
    accumulation_reward_received_quest_mission_count INTEGER NOT NULL DEFAULT 0,
    latest_version                                   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, event_quest_chapter_id, stage_order)
);

-- +goose Down
DROP TABLE IF EXISTS user_event_quest_labyrinth_stages;
DROP TABLE IF EXISTS user_event_quest_labyrinth_seasons;
