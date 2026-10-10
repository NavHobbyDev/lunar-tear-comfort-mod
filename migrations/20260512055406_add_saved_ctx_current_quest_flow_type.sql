-- +goose Up
ALTER TABLE user_main_quest ADD COLUMN saved_ctx_current_quest_flow_type INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE user_main_quest DROP COLUMN saved_ctx_current_quest_flow_type;
