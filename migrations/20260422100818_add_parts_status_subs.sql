-- +goose Up
CREATE TABLE user_parts_status_subs (
    user_id                      INTEGER NOT NULL REFERENCES users(user_id),
    user_parts_uuid              TEXT    NOT NULL,
    status_index                 INTEGER NOT NULL,
    parts_status_sub_lottery_id  INTEGER NOT NULL DEFAULT 0,
    level                        INTEGER NOT NULL DEFAULT 0,
    status_kind_type             INTEGER NOT NULL DEFAULT 0,
    status_calculation_type      INTEGER NOT NULL DEFAULT 0,
    status_change_value          INTEGER NOT NULL DEFAULT 0,
    latest_version               INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, user_parts_uuid, status_index)
);

UPDATE user_parts SET level = 1;

-- +goose Down
DROP TABLE IF EXISTS user_parts_status_subs;
