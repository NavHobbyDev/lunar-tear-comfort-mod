-- +goose Up
ALTER TABLE user_main_quest ADD COLUMN saved_ctx_active INTEGER NOT NULL DEFAULT 0;
ALTER TABLE user_main_quest ADD COLUMN saved_ctx_current_quest_scene_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE user_main_quest ADD COLUMN saved_ctx_head_quest_scene_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE user_main_quest ADD COLUMN saved_ctx_current_main_quest_route_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE user_main_quest ADD COLUMN saved_ctx_main_quest_season_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE user_main_quest ADD COLUMN saved_ctx_is_reached_last_quest_scene INTEGER NOT NULL DEFAULT 0;
ALTER TABLE user_main_quest ADD COLUMN saved_ctx_portal_cage_in_progress INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE user_main_quest DROP COLUMN saved_ctx_active;
ALTER TABLE user_main_quest DROP COLUMN saved_ctx_current_quest_scene_id;
ALTER TABLE user_main_quest DROP COLUMN saved_ctx_head_quest_scene_id;
ALTER TABLE user_main_quest DROP COLUMN saved_ctx_current_main_quest_route_id;
ALTER TABLE user_main_quest DROP COLUMN saved_ctx_main_quest_season_id;
ALTER TABLE user_main_quest DROP COLUMN saved_ctx_is_reached_last_quest_scene;
ALTER TABLE user_main_quest DROP COLUMN saved_ctx_portal_cage_in_progress;
