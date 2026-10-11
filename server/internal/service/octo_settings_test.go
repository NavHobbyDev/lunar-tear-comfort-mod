package service

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"

	"lunar-tear/server/internal/database"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store/sqlite"
	"lunar-tear/server/migrations"
)

func newOctoSettingsServer(t *testing.T) (*OctoHTTPServer, *sqlite.SQLiteStore, int64) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "game.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := migrations.Up(context.Background(), db); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	st := sqlite.New(db, nil)
	uid, err := st.CreateUser("uuid-web", model.ClientPlatform{})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return &OctoHTTPServer{users: st}, st, uid
}

func TestOctoSettingsSaveByUUIDAndUserID(t *testing.T) {
	s, st, uid := newOctoSettingsServer(t)

	// Never saved: the page shows the checkbox unchecked (missions must be played).
	if got := s.getUserSettings("uuid-web"); got.AutoComplete {
		t.Fatalf("default checkbox should be unchecked (must play), got %+v", got)
	}

	if err := s.saveUserSettings("uuid-web", UserSettings{MasterdataVariant: "Clear", AutoComplete: true}); err != nil {
		t.Fatalf("save by uuid: %v", err)
	}
	opts, err := st.GetUserOptions(uid)
	if err != nil || opts.MasterDataVariant != "Clear" || !opts.ForceHiddenStoryMissions {
		t.Fatalf("after uuid save: %+v err=%v", opts, err)
	}

	// The page may also carry the numeric user_id.
	if err := s.saveUserSettings(strconv.FormatInt(uid, 10), UserSettings{MasterdataVariant: "Bonus", AutoComplete: false}); err != nil {
		t.Fatalf("save by numeric id: %v", err)
	}
	opts, _ = st.GetUserOptions(uid)
	if opts.MasterDataVariant != "Bonus" || opts.ForceHiddenStoryMissions {
		t.Fatalf("after numeric save (unchecked = must play): %+v", opts)
	}

	got := s.getUserSettings("uuid-web")
	if got.MasterdataVariant != "Bonus" || got.AutoComplete {
		t.Fatalf("load by uuid: %+v", got)
	}
}

func TestOctoSettingsAllVariantNormalizesToEmpty(t *testing.T) {
	s, st, uid := newOctoSettingsServer(t)

	// "All" is the page label for the base master-data file; it must persist
	// as the empty token and read back as "All".
	if err := s.saveUserSettings("uuid-web", UserSettings{MasterdataVariant: displayAllVariant, AutoComplete: false}); err != nil {
		t.Fatalf("save: %v", err)
	}
	opts, _ := st.GetUserOptions(uid)
	if opts.MasterDataVariant != "" {
		t.Fatalf("expected empty DB token, got %q", opts.MasterDataVariant)
	}
	if got := s.getUserSettings("uuid-web"); got.MasterdataVariant != displayAllVariant {
		t.Fatalf("expected %q on the wire, got %q", displayAllVariant, got.MasterdataVariant)
	}
}

func TestOctoSettingsUnidentifiedFallsBackToDefaultUser(t *testing.T) {
	s, _, uid := newOctoSettingsServer(t)

	// With no recognizable identifier the oldest account is used, so a locally
	// opened page still round-trips.
	if err := s.saveUserSettings("default_user", UserSettings{MasterdataVariant: "Gacha", AutoComplete: true}); err != nil {
		t.Fatalf("save via fallback: %v", err)
	}
	got := s.getUserSettings("no-such-id")
	if got.MasterdataVariant != "Gacha" || !got.AutoComplete {
		t.Fatalf("fallback user %d settings: %+v", uid, got)
	}
}
