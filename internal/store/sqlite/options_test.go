package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"lunar-tear/server/internal/database"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
	"lunar-tear/server/migrations"
)

// newMigratedStore opens a temp game.db with the full migration set applied,
// mirroring how the servers boot.
func newMigratedStore(t *testing.T) *SQLiteStore {
	t.Helper()
	return newStoreWithMigrations(t, nil)
}

// newStoreWithMigrations opens a temp game.db, optionally running preMigrate
// (on a fresh empty DB, before migrations) so tests can simulate legacy shapes.
func newStoreWithMigrations(t *testing.T, preMigrate func(db *sql.DB) error) *SQLiteStore {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "game.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if preMigrate != nil {
		if err := preMigrate(db); err != nil {
			t.Fatalf("pre-migrate: %v", err)
		}
	}
	if err := migrations.Up(context.Background(), db); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	return New(db, nil)
}

// A database that already has an old hand-made user_options table (different
// column names) must be repaired by migrations.Up: CREATE TABLE IF NOT EXISTS
// skips it, and without the column repair LoadUser fails on "no such column".
func TestUserOptionsLegacyTableRepaired(t *testing.T) {
	legacy := func(db *sql.DB) error {
		_, err := db.Exec(`CREATE TABLE user_options (
			user_id INTEGER PRIMARY KEY REFERENCES users(user_id),
			masterdata_variant TEXT NOT NULL DEFAULT '',
			hidden_story_missions_enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`)
		return err
	}
	s := newStoreWithMigrations(t, legacy)

	uid, err := s.CreateUser("uuid-legacy", model.ClientPlatform{})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	// Repaired column reads back the defaults: hidden-story unlock missions
	// must be played (0), news popups are shown (1, per the column DEFAULT).
	if got, err := s.GetUserOptions(uid); err != nil || got != (store.UserOptionsState{ShowNews: true}) {
		t.Fatalf("after repair: got %+v err=%v", got, err)
	}
	// ...and the upsert path works against the repaired table.
	want := store.UserOptionsState{MasterDataVariant: "Rec", ForceHiddenStoryMissions: true}
	if err := s.SaveUserOptions(uid, want); err != nil {
		t.Fatalf("save on repaired table: %v", err)
	}
	if got, _ := s.GetUserOptions(uid); got != want {
		t.Fatalf("repaired table round-trip: got %+v, want %+v", got, want)
	}
}

func TestUserOptionsRoundTrip(t *testing.T) {
	s := newMigratedStore(t)
	uid, err := s.CreateUser("uuid-options", model.ClientPlatform{})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Missing row yields defaults, not ErrNotFound: base variant, no
	// auto-complete, news shown (every new player sees news by default).
	got, err := s.GetUserOptions(uid)
	if err != nil {
		t.Fatalf("get default options: %v", err)
	}
	if got != (store.UserOptionsState{ShowNews: true}) {
		t.Fatalf("expected default options (news shown), got %+v", got)
	}

	want := store.UserOptionsState{MasterDataVariant: "Clear", ForceHiddenStoryMissions: true}
	if err := s.SaveUserOptions(uid, want); err != nil {
		t.Fatalf("save options: %v", err)
	}
	if got, err = s.GetUserOptions(uid); err != nil {
		t.Fatalf("get options: %v", err)
	}
	if got != want {
		t.Fatalf("round-trip mismatch: got %+v, want %+v", got, want)
	}

	// Upsert (second save) overwrites instead of failing.
	want2 := store.UserOptionsState{MasterDataVariant: "Bonus"}
	if err := s.SaveUserOptions(uid, want2); err != nil {
		t.Fatalf("re-save options: %v", err)
	}
	if got, err = s.GetUserOptions(uid); err != nil {
		t.Fatalf("get options: %v", err)
	}
	if got != want2 {
		t.Fatalf("upsert mismatch: got %+v, want %+v", got, want2)
	}

	// LoadUser surfaces the row in UserState.Options.
	user, err := s.LoadUser(uid)
	if err != nil {
		t.Fatalf("load user: %v", err)
	}
	if user.Options != want2 {
		t.Fatalf("LoadUser.Options = %+v, want %+v", user.Options, want2)
	}

	// UpdateUser must never rewrite options (octo is the only writer).
	_, err = s.UpdateUser(uid, func(u *store.UserState) {
		u.Options = store.UserOptionsState{MasterDataVariant: "SHOULD_NOT_PERSIST"}
	})
	if err != nil {
		t.Fatalf("update user: %v", err)
	}
	if got, _ := s.GetUserOptions(uid); got != want2 {
		t.Fatalf("UpdateUser clobbered user_options: got %+v, want %+v", got, want2)
	}

	// FK guards writes for unknown users.
	if err := s.SaveUserOptions(999999, want); err == nil {
		t.Fatal("expected FK failure for unknown user")
	}
}
