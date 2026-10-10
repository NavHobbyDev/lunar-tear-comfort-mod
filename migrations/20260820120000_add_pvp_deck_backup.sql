-- +goose Up
ALTER TABLE user_pvp_state ADD COLUMN pvp_deck_backup TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE user_pvp_state DROP COLUMN pvp_deck_backup;
