-- +goose Up
-- Adds the per-user "Show in-game news popups" toggle to user_options.
-- show_news: 1 = news popups are shown normally (default for new players);
--            0 = active dokan entries are reported as already-seen in the
--            data projection so the client never surfaces them as news.
--            The DB is never written for unseen news; only RegisterDokanConfirmed
--            records a genuine view.
ALTER TABLE user_options ADD COLUMN show_news INTEGER NOT NULL DEFAULT 1;

-- +goose Down
-- SQLite cannot drop a column with DEFAULT without a table rebuild; the
-- column is harmless to leave in place, so Down is a no-op.
