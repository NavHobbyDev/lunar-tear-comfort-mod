-- +goose Up
CREATE TABLE user_quest_auto_orbit (
    user_id                  INTEGER NOT NULL PRIMARY KEY REFERENCES users(user_id),
    quest_type               INTEGER NOT NULL DEFAULT 0,
    chapter_id               INTEGER NOT NULL DEFAULT 0,
    quest_id                 INTEGER NOT NULL DEFAULT 0,
    max_auto_orbit_count     INTEGER NOT NULL DEFAULT 0,
    cleared_auto_orbit_count INTEGER NOT NULL DEFAULT 0,
    last_clear_datetime      INTEGER NOT NULL DEFAULT 0,
    latest_version           INTEGER NOT NULL DEFAULT 0,
    accumulated_drops_json   TEXT    NOT NULL DEFAULT '[]'
);

INSERT INTO user_quest_auto_orbit (user_id) SELECT user_id FROM users;

-- +goose Down
DROP TABLE IF EXISTS user_quest_auto_orbit;
