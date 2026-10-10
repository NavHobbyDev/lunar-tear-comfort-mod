-- +goose Up
CREATE TABLE IF NOT EXISTS user_cage_ornament_accesses (
    user_id                 INTEGER NOT NULL REFERENCES users(user_id),
    cage_ornament_id        INTEGER NOT NULL,
    first_access_datetime   INTEGER NOT NULL DEFAULT 0,
    latest_access_datetime  INTEGER NOT NULL DEFAULT 0,
    latest_version          INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, cage_ornament_id)
);

-- +goose Down
DROP TABLE IF EXISTS user_cage_ornament_accesses;
