-- +goose Up
CREATE TABLE user_mission_pass_points (
    user_id         INTEGER NOT NULL REFERENCES users(user_id),
    mission_pass_id INTEGER NOT NULL,
    point_count     INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, mission_pass_id)
);

-- +goose Down
DROP TABLE IF EXISTS user_mission_pass_points;
