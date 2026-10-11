package sqlite

import (
	"testing"

	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
	"lunar-tear/server/migrations"
)

// Reproduces booting an old database (predating 20260919120000's inlining into
// the initial schema) whose goose ledger lets the neutralized migration no-op:
// migrations.Up must restore the missing user_battle/user_main_quest columns
// via repairSchema so LoadUser stops failing with "no such column".
func TestSchemaRepairRestoresDroppedBattleQuestColumns(t *testing.T) {
	s := newMigratedStore(t)
	uid, err := s.CreateUser("uuid-repair", model.ClientPlatform{})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := s.UpdateUser(uid, func(u *store.UserState) {
		u.Battle.StartCount = 7 // non-default value to prove repair keeps rows
	}); err != nil {
		t.Fatalf("seed battle state: %v", err)
	}

	// Simulate the old shape: columns absent (SQLite 3.35+ DROP COLUMN).
	drops := []string{
		`ALTER TABLE user_battle DROP COLUMN last_started_quest_id`,
		`ALTER TABLE user_battle DROP COLUMN last_started_quest_at`,
		`ALTER TABLE user_main_quest DROP COLUMN suppress_replay_flow`,
	}
	for _, stmt := range drops {
		if _, err := s.db.Exec(stmt); err != nil {
			t.Fatalf("setup %s: %v", stmt, err)
		}
	}

	// Boot-time repair.
	if err := migrations.Up(t.Context(), s.db); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}

	user, err := s.LoadUser(uid)
	if err != nil {
		t.Fatalf("LoadUser after repair: %v", err)
	}
	if user.Battle.StartCount != 7 {
		t.Fatalf("repair lost battle data: StartCount=%d, want 7", user.Battle.StartCount)
	}
	if user.Battle.LastStartedQuestId != 0 || user.Battle.LastStartedQuestAt != 0 {
		t.Fatalf("repaired columns should default to 0, got %d/%d", user.Battle.LastStartedQuestId, user.Battle.LastStartedQuestAt)
	}

	// The repaired columns must be writable again through the save path.
	if _, err := s.UpdateUser(uid, func(u *store.UserState) {
		u.Battle.LastStartedQuestId = 12345
	}); err != nil {
		t.Fatalf("update repaired columns: %v", err)
	}
	user, err = s.LoadUser(uid)
	if err != nil {
		t.Fatalf("LoadUser after write: %v", err)
	}
	if user.Battle.LastStartedQuestId != 12345 {
		t.Fatalf("LastStartedQuestId=%d, want 12345", user.Battle.LastStartedQuestId)
	}
}
