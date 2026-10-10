-- +goose Up
CREATE TABLE user_costume_lottery_effects (
    user_id               INTEGER NOT NULL REFERENCES users(user_id),
    user_costume_uuid     TEXT    NOT NULL,
    slot_number           INTEGER NOT NULL,
    odds_number           INTEGER NOT NULL DEFAULT 0,
    latest_version        INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, user_costume_uuid, slot_number)
);

ALTER TABLE user_costumes ADD COLUMN costume_lottery_effect_unlocked_slot_count INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE user_costumes DROP COLUMN costume_lottery_effect_unlocked_slot_count;
DROP TABLE IF EXISTS user_costume_lottery_effects;
