package service

import (
	"encoding/json"
	"strings"
	"testing"

	"lunar-tear/server/internal/store"
)

// A sell that hits stale uuids must still carry the genuine changes of the
// request (sold rows disappearing, gold updating) merged with forced delete
// keys for the uuids the server never had.
func TestPartsStaleDeleteDiffMergesRealAndStale(t *testing.T) {
	before := store.UserState{
		UserId: 7,
		Parts: map[string]store.PartsState{
			"real-sold": {UserPartsUuid: "real-sold", PartsId: 5, Level: 1},
		},
		ConsumableItems: map[int32]int32{1: 100},
	}
	after := store.UserState{
		UserId:          7,
		Parts:           map[string]store.PartsState{},
		ConsumableItems: map[int32]int32{1: 150},
	}

	diff := partsStaleDeleteDiff(&before, &after, []string{"ghost-1", "ghost-2"})

	parts := diff["IUserParts"]
	if parts == nil {
		t.Fatal("expected IUserParts diff entry")
	}
	var deleteKeys []map[string]any
	if err := json.Unmarshal([]byte(parts.DeleteKeysJson), &deleteKeys); err != nil {
		t.Fatalf("bad DeleteKeysJson: %v", err)
	}
	uuids := make(map[string]bool)
	for _, rec := range deleteKeys {
		uuid, _ := rec["userPartsUuid"].(string)
		uuids[uuid] = true
	}
	for _, want := range []string{"real-sold", "ghost-1", "ghost-2"} {
		if !uuids[want] {
			t.Errorf("delete keys missing uuid %q: %s", want, parts.DeleteKeysJson)
		}
	}

	if gold := diff["IUserConsumableItem"]; gold == nil || !strings.Contains(gold.UpdateRecordsJson, "150") {
		t.Errorf("expected gold update in diff, got %+v", gold)
	}

	subs := diff["IUserPartsStatusSub"]
	if subs == nil || !strings.Contains(subs.DeleteKeysJson, "ghost-1") {
		t.Errorf("expected sub-status delete keys for ghosts, got %+v", subs)
	}
}

// All-stale sells change nothing server-side; the diff must still surface the
// delete keys so the client cache converges.
func TestPartsStaleDeleteDiffAllStale(t *testing.T) {
	state := store.UserState{UserId: 7, Parts: map[string]store.PartsState{}}
	other := store.UserState{UserId: 7, Parts: map[string]store.PartsState{}}

	diff := partsStaleDeleteDiff(&state, &other, []string{"ghost-only"})

	parts := diff["IUserParts"]
	if parts == nil || !strings.Contains(parts.DeleteKeysJson, "ghost-only") {
		t.Fatalf("expected stale delete key, got %+v", parts)
	}
	if parts.UpdateRecordsJson != "[]" {
		t.Errorf("expected empty updates, got %s", parts.UpdateRecordsJson)
	}
}
