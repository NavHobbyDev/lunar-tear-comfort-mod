-- +goose Up
CREATE TABLE user_costume_lottery_effect_ability (
    user_id           INTEGER NOT NULL REFERENCES users(user_id),
    user_costume_uuid TEXT    NOT NULL,
    slot_number       INTEGER NOT NULL,
    ability_id        INTEGER NOT NULL,
    ability_level     INTEGER NOT NULL,
    latest_version    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, user_costume_uuid, slot_number)
);

CREATE TABLE user_costume_lottery_effect_status_up (
    user_id                 INTEGER NOT NULL REFERENCES users(user_id),
    user_costume_uuid       TEXT    NOT NULL,
    status_calculation_type INTEGER NOT NULL,
    hp                      INTEGER NOT NULL DEFAULT 0,
    attack                  INTEGER NOT NULL DEFAULT 0,
    vitality                INTEGER NOT NULL DEFAULT 0,
    agility                 INTEGER NOT NULL DEFAULT 0,
    critical_ratio          INTEGER NOT NULL DEFAULT 0,
    critical_attack         INTEGER NOT NULL DEFAULT 0,
    latest_version          INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, user_costume_uuid, status_calculation_type)
);

-- +goose Down
DROP TABLE IF EXISTS user_costume_lottery_effect_status_up;
DROP TABLE IF EXISTS user_costume_lottery_effect_ability;
