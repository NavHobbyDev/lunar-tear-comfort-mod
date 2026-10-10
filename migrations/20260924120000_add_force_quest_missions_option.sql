-- +goose Up
-- Adds the per-user "Force quest missions" toggle to user_options.
-- force_quest_missions: 1 = the additional missions inside quests (the three
--   challenge missions plus their "Clear all missions" meta-mission) count as
--   achieved for the quest being cleared right now; 0 (default) = the player
--   must clear them legitimately. Applied in questflow's clearQuestMissions,
--   so the missions are written as real cleared rows in user_quest_missions and
--   their rewards (plus the BigWin bonus) are paid by the normal finish logic.
--   It never rewrites quests the player has already finished.
ALTER TABLE user_options ADD COLUMN force_quest_missions INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- SQLite cannot drop a column with DEFAULT without a table rebuild; the
-- column is harmless to leave in place, so Down is a no-op.
