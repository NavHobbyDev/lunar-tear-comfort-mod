package service

import (
	"testing"

	"lunar-tear/server/internal/store"
)

func TestMaybeResetCheerDay_clearsStaleFlags(t *testing.T) {
	u := &store.UserState{Friends: map[int64]store.FriendEdge{
		1: {PlayerId: 1, CheerSentToday: true, StaminaReceivedToday: true, LastResetDay: 0},
	}}
	maybeResetCheerDay(u)
	e := u.Friends[1]
	if e.CheerSentToday || e.StaminaReceivedToday {
		t.Fatalf("stale daily flags not cleared: %+v", e)
	}
	if e.LastResetDay != dayBucket() {
		t.Fatalf("reset day not stamped")
	}
}

func TestGrantCheerReward_grantsOnceAndMarks(t *testing.T) {
	const testMaxMillis int32 = 120_000 // 120 stamina units, well above seed value
	u := &store.UserState{}
	u.EnsureMaps()
	u.Friends[5] = store.FriendEdge{PlayerId: 5, CheerReceivedPending: true, LastResetDay: dayBucket()}
	// Start below max so the grant is observable (seed is 0 here, cheerStaminaMillis = 10_000)
	before := u.Status.StaminaMilliValue
	grantCheerReward(u, 5, testMaxMillis)
	e := u.Friends[5]
	if e.CheerReceivedPending || !e.StaminaReceivedToday {
		t.Fatalf("reward flags not updated: %+v", e)
	}
	if u.Status.StaminaMilliValue <= before {
		t.Fatalf("stamina not granted: before=%d after=%d", before, u.Status.StaminaMilliValue)
	}
	// second collect is a no-op
	mid := u.Status.StaminaMilliValue
	grantCheerReward(u, 5, testMaxMillis)
	if u.Status.StaminaMilliValue != mid {
		t.Fatalf("second collect must not grant again: mid=%d after=%d", mid, u.Status.StaminaMilliValue)
	}
}
