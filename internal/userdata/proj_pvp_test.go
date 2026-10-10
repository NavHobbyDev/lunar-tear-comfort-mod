package userdata

import (
	"encoding/json"
	"testing"

	"lunar-tear/server/internal/store"
)

// TestPvpStatusProjection guards the IUserPvpStatus record shape: the client
// reads arena BP (staminaMilliValue) from this table before allowing a battle,
// and silently ignores unknown JSON keys — so the key names/casing must match
// the client's EntityIUserPvpStatus exactly, and BP must be non-zero.
func TestPvpStatusProjection(t *testing.T) {
	const uid int64 = 12345
	out := ProjectTables(store.UserState{UserId: uid}, []string{
		"IUserPvpStatus", "IUserPvpDefenseDeck", "IUserPvpWeeklyResult",
	})

	var recs []map[string]any
	if err := json.Unmarshal([]byte(out["IUserPvpStatus"]), &recs); err != nil {
		t.Fatalf("IUserPvpStatus is not valid JSON: %v (%q)", err, out["IUserPvpStatus"])
	}
	if len(recs) != 1 {
		t.Fatalf("want exactly 1 IUserPvpStatus record, got %d", len(recs))
	}
	rec := recs[0]

	wantKeys := []string{
		"userId", "staminaMilliValue", "staminaUpdateDatetime",
		"latestRewardReceivePvpSeasonId", "latestRewardReceivePvpWeeklyVersion",
		"winStreakCount", "winStreakCountUpdateDatetime", "latestVersion",
	}
	for _, k := range wantKeys {
		if _, ok := rec[k]; !ok {
			t.Errorf("IUserPvpStatus missing key %q (have %v)", k, rec)
		}
	}
	if len(rec) != len(wantKeys) {
		t.Errorf("IUserPvpStatus has %d keys, want %d: %v", len(rec), len(wantKeys), rec)
	}

	if got := rec["userId"].(float64); int64(got) != uid {
		t.Errorf("userId = %v, want %d", got, uid)
	}
	// BP must be at the cap (100 units * 1000 milli) so the client lets you battle.
	if got := rec["staminaMilliValue"].(float64); got != pvpMaxBattlePointMilli {
		t.Errorf("staminaMilliValue = %v, want %d", got, pvpMaxBattlePointMilli)
	}

	// The two stateless tables must still be valid (empty) record sets.
	for _, tbl := range []string{"IUserPvpDefenseDeck", "IUserPvpWeeklyResult"} {
		if out[tbl] != "[]" {
			t.Errorf("%s = %q, want %q", tbl, out[tbl], "[]")
		}
	}
}
