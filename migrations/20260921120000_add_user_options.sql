-- +goose Up
-- Per-user options editable from the octo /information web page.
-- masterdata_variant: token appended to the master-data version by the game
--   server (see userdata.Options); empty means the base variant.
-- hidden_story_missions: the "Auto-complete hidden story unlock missions"
-- checkbox. 1 = present the hidden-story unlock missions (category 7) to the
-- client as already completed; 0 (default) = the player must clear them.
CREATE TABLE IF NOT EXISTS user_options (
    user_id               INTEGER NOT NULL PRIMARY KEY REFERENCES users(user_id),
    masterdata_variant    TEXT    NOT NULL DEFAULT '',
    hidden_story_missions INTEGER NOT NULL DEFAULT 0,
    updated_at            INTEGER NOT NULL DEFAULT 0
);

-- +goose Down
DROP TABLE IF EXISTS user_options;
