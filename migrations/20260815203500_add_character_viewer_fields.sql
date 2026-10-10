-- +goose Up
CREATE TABLE IF NOT EXISTS user_character_viewer_fields (
    user_id                     INTEGER NOT NULL REFERENCES users(user_id),
    character_viewer_field_id   INTEGER NOT NULL,
    is_notified                 INTEGER NOT NULL DEFAULT 0,
    notified_datetime           INTEGER NOT NULL DEFAULT 0,
    latest_version              INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, character_viewer_field_id)
);

-- +goose Down
DROP TABLE IF EXISTS user_character_viewer_fields;
