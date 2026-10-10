-- +goose Up
CREATE TABLE user_parts_preset_tags (
    user_id                       INTEGER NOT NULL REFERENCES users(user_id),
    user_parts_preset_tag_number  INTEGER NOT NULL,
    name                          TEXT    NOT NULL DEFAULT '',
    latest_version                INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, user_parts_preset_tag_number)
);

-- +goose Down
DROP TABLE IF EXISTS user_parts_preset_tags;
