-- +goose Up
CREATE TABLE user_main_quest_season_routes (
    user_id              INTEGER NOT NULL REFERENCES users(user_id),
    main_quest_season_id INTEGER NOT NULL,
    main_quest_route_id  INTEGER NOT NULL,
    latest_version       INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, main_quest_season_id, main_quest_route_id)
);

-- Backfill: seed each user's current (season, route) so existing saves immediately
-- have at least one entry. Players who progressed through prior seasons in our
-- server won't get historical entries — accept that for the immediate fix.
INSERT INTO user_main_quest_season_routes (user_id, main_quest_season_id, main_quest_route_id, latest_version)
SELECT user_id, main_quest_season_id, current_main_quest_route_id, latest_version
FROM user_main_quest
WHERE main_quest_season_id > 0 AND current_main_quest_route_id > 0;

-- +goose Down
DROP TABLE IF EXISTS user_main_quest_season_routes;
