-- +goose Up
CREATE TABLE user_costume_lottery_effect_pending (
    user_id           INTEGER NOT NULL REFERENCES users(user_id),
    user_costume_uuid TEXT    NOT NULL,
    slot_number       INTEGER NOT NULL,
    odds_number       INTEGER NOT NULL DEFAULT 0,
    latest_version    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, user_costume_uuid)
);

-- +goose Down
DROP TABLE IF EXISTS user_costume_lottery_effect_pending;
