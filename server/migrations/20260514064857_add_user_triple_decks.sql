-- +goose Up
CREATE TABLE user_triple_decks (
    user_id          INTEGER NOT NULL REFERENCES users(user_id),
    deck_type        INTEGER NOT NULL,
    user_deck_number INTEGER NOT NULL,
    name             TEXT    NOT NULL DEFAULT '',
    deck_number01    INTEGER NOT NULL DEFAULT 0,
    deck_number02    INTEGER NOT NULL DEFAULT 0,
    deck_number03    INTEGER NOT NULL DEFAULT 0,
    latest_version   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, deck_type, user_deck_number)
);

-- Legacy BigHunt wave-decks have no preset wrapper rows and aren't reachable
-- via the new IUserTripleDeck projection. Drop them so users start clean and
-- repopulate via ReplaceTripleDeck from the client.
DELETE FROM user_decks WHERE deck_type = 5;

-- +goose Down
DROP TABLE IF EXISTS user_triple_decks;
