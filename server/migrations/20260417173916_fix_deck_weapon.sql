-- +goose Up

-- Delete deck characters with empty weapons (always a bug).
DELETE FROM user_deck_characters
WHERE main_user_weapon_uuid = '';

-- Delete decks that reference deleted deck characters.
DELETE FROM user_decks
WHERE user_deck_character_uuid01 NOT IN (SELECT user_deck_character_uuid FROM user_deck_characters)
   AND user_deck_character_uuid01 != '';

-- +goose Down
-- No rollback needed: EnsureDefaultDeck recreates decks on next SetTutorialProgress call.
