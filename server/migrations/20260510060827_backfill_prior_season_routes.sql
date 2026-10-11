-- +goose Up
-- For each user past season 2 with no season-2 row yet, infer which sun/moon
-- route they actually played from their cleared quests in user_quests, and
-- insert the matching (season=2, route=X) row. Heals players who progressed
-- past season 2 before per-scene RecordSeasonRoute existed (commit 9a2cc92).
--
-- Quest-ID ranges per route (from master_data/EntityMMainQuestSequenceTable.json):
--   Route 2 (sun/moon variant A): quests 301..370
--   Route 3 (sun/moon variant B): quests 401..470
--
-- The NOT EXISTS clause matches on (user, season) regardless of route, so any
-- existing real RecordSeasonRoute write is preserved verbatim.

-- Rule 1: user has cleared a route-3 quest -> they picked variant B.
INSERT INTO user_main_quest_season_routes (user_id, main_quest_season_id, main_quest_route_id, latest_version)
SELECT umq.user_id, 2, 3, umq.latest_version
FROM user_main_quest umq
WHERE umq.main_quest_season_id > 2
  AND NOT EXISTS (
    SELECT 1 FROM user_main_quest_season_routes r
    WHERE r.user_id = umq.user_id AND r.main_quest_season_id = 2
  )
  AND EXISTS (
    SELECT 1 FROM user_quests q
    WHERE q.user_id = umq.user_id AND q.clear_count > 0
      AND q.quest_id BETWEEN 401 AND 470
  );

-- Rule 2: otherwise insert route 2 (covers users who picked variant A and
-- users with no observable route-2/route-3 history -- last-resort default).
INSERT INTO user_main_quest_season_routes (user_id, main_quest_season_id, main_quest_route_id, latest_version)
SELECT umq.user_id, 2, 2, umq.latest_version
FROM user_main_quest umq
WHERE umq.main_quest_season_id > 2
  AND NOT EXISTS (
    SELECT 1 FROM user_main_quest_season_routes r
    WHERE r.user_id = umq.user_id AND r.main_quest_season_id = 2
  );

-- +goose Down
SELECT 1;
