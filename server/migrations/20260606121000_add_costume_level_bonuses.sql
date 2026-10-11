-- +goose Up
CREATE TABLE user_character_costume_level_bonuses (
    user_id                  INTEGER NOT NULL REFERENCES users(user_id),
    character_id             INTEGER NOT NULL,
    status_calculation_type  INTEGER NOT NULL,
    hp                       INTEGER NOT NULL DEFAULT 0,
    attack                   INTEGER NOT NULL DEFAULT 0,
    vitality                 INTEGER NOT NULL DEFAULT 0,
    agility                  INTEGER NOT NULL DEFAULT 0,
    critical_ratio           INTEGER NOT NULL DEFAULT 0,
    critical_attack          INTEGER NOT NULL DEFAULT 0,
    latest_version           INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, character_id, status_calculation_type)
);

CREATE TABLE user_costume_level_bonus_release_statuses (
    user_id                   INTEGER NOT NULL REFERENCES users(user_id),
    costume_id                INTEGER NOT NULL,
    last_released_bonus_level INTEGER NOT NULL DEFAULT 0,
    confirmed_bonus_level     INTEGER NOT NULL DEFAULT 0,
    latest_version            INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, costume_id)
);

-- +goose Down
DROP TABLE IF EXISTS user_costume_level_bonus_release_statuses;
DROP TABLE IF EXISTS user_character_costume_level_bonuses;
