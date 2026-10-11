package store

import (
	"fmt"
	"testing"

	"lunar-tear/server/internal/model"
)

func giftWithId(id int32, uuid string) NotReceivedGiftState {
	return NotReceivedGiftState{
		GiftCommon: GiftCommonState{
			PossessionType: int32(model.PossessionTypeConsumableItem),
			PossessionId:   id,
			Count:          1,
		},
		UserGiftUuid: uuid,
	}
}

func fillMailbox(user *UserState, n int) {
	for i := range n {
		user.Gifts.NotReceived = append(user.Gifts.NotReceived, giftWithId(int32(i), fmt.Sprintf("mail-%d", i)))
	}
}

// TestAddGiftAppendsBelowCap verifies mails are simply appended (in order)
// while the mailbox has free slots, with the badge kept in sync.
func TestAddGiftAppendsBelowCap(t *testing.T) {
	user := &UserState{}
	user.AddGift(giftWithId(1, "a"))
	user.AddGift(giftWithId(2, "b"))

	if len(user.Gifts.NotReceived) != 2 {
		t.Fatalf("expected 2 mails, got %d", len(user.Gifts.NotReceived))
	}
	if user.Gifts.NotReceived[0].UserGiftUuid != "a" || user.Gifts.NotReceived[1].UserGiftUuid != "b" {
		t.Fatalf("mails must keep insertion order")
	}
	if user.Notifications.GiftNotReceiveCount != 2 {
		t.Fatalf("expected badge count 2, got %d", user.Notifications.GiftNotReceiveCount)
	}
}

// TestAddGiftEvictsOldestAtCap verifies that at a full mailbox (999 mails)
// the oldest mail is dropped to make room for the new one, and the mailbox
// never grows past the cap.
func TestAddGiftEvictsOldestAtCap(t *testing.T) {
	user := &UserState{}
	fillMailbox(user, int(model.GiftInventoryCap))

	user.AddGift(giftWithId(7777, "newest"))

	if len(user.Gifts.NotReceived) != int(model.GiftInventoryCap) {
		t.Fatalf("mailbox must not exceed the cap, got %d", len(user.Gifts.NotReceived))
	}
	first := user.Gifts.NotReceived[0]
	if first.UserGiftUuid != "mail-1" {
		t.Fatalf("the oldest mail (mail-0) must be evicted first, got %s", first.UserGiftUuid)
	}
	last := user.Gifts.NotReceived[len(user.Gifts.NotReceived)-1]
	if last.UserGiftUuid != "newest" || last.GiftCommon.PossessionId != 7777 {
		t.Fatalf("the new mail must land last, got uuid=%s id=%d", last.UserGiftUuid, last.GiftCommon.PossessionId)
	}
	if user.Notifications.GiftNotReceiveCount != model.GiftInventoryCap {
		t.Fatalf("expected badge count %d, got %d", model.GiftInventoryCap, user.Notifications.GiftNotReceiveCount)
	}

	// A burst of extra mails keeps evicting one oldest per arrival.
	for i := 0; i < 5; i++ {
		user.AddGift(giftWithId(int32(8000+i), fmt.Sprintf("burst-%d", i)))
	}
	if len(user.Gifts.NotReceived) != int(model.GiftInventoryCap) {
		t.Fatalf("mailbox must stay at the cap after a burst, got %d", len(user.Gifts.NotReceived))
	}
	if user.Gifts.NotReceived[0].UserGiftUuid != "mail-6" {
		t.Fatalf("expected mails 1-5 evicted by the burst, oldest is %s", user.Gifts.NotReceived[0].UserGiftUuid)
	}
}

// TestAddGiftsOrderedDecrementsTimestamps verifies that when multiple gifts
// are sent ordered, each subsequent gift has its timestamp decremented by 1ms
// so the mailbox displays them in the intended order (first gift appears first
// when sorted newest-first).
func TestAddGiftsOrderedDecrementsTimestamps(t *testing.T) {
	user := &UserState{}
	baseTime := int64(1000000)
	expiry := baseTime + 86400000

	gifts := []NotReceivedGiftState{
		{
			GiftCommon:         GiftCommonState{PossessionType: int32(model.PossessionTypePaidGem), PossessionId: 0, Count: 1000, GrantDatetime: baseTime},
			ExpirationDatetime: expiry,
			UserGiftUuid:       "gems",
		},
		{
			GiftCommon:         GiftCommonState{PossessionType: int32(model.PossessionTypeConsumableItem), PossessionId: 9001, Count: 100, GrantDatetime: baseTime},
			ExpirationDatetime: expiry,
			UserGiftUuid:       "mama",
		},
		{
			GiftCommon:         GiftCommonState{PossessionType: int32(model.PossessionTypeConsumableItem), PossessionId: 242, Count: 10, GrantDatetime: baseTime},
			ExpirationDatetime: expiry,
			UserGiftUuid:       "medal",
		},
	}

	user.AddGiftsOrdered(gifts)

	if len(user.Gifts.NotReceived) != 3 {
		t.Fatalf("expected 3 mails, got %d", len(user.Gifts.NotReceived))
	}

	// Verify timestamps are decremented: first gift has baseTime, second baseTime-1, third baseTime-2
	expected := []struct {
		uuid      string
		timestamp int64
	}{
		{"gems", baseTime},
		{"mama", baseTime - 1},
		{"medal", baseTime - 2},
	}

	for i, exp := range expected {
		got := user.Gifts.NotReceived[i]
		if got.UserGiftUuid != exp.uuid {
			t.Errorf("mail[%d] uuid = %q, want %q", i, got.UserGiftUuid, exp.uuid)
		}
		if got.GiftCommon.GrantDatetime != exp.timestamp {
			t.Errorf("mail[%d] timestamp = %d, want %d", i, got.GiftCommon.GrantDatetime, exp.timestamp)
		}
		if got.ExpirationDatetime != expiry-int64(i) {
			t.Errorf("mail[%d] expiry = %d, want %d", i, got.ExpirationDatetime, expiry-int64(i))
		}
	}
}

// TestAddGiftsOrderedEmpty verifies that an empty slice is a no-op.
func TestAddGiftsOrderedEmpty(t *testing.T) {
	user := &UserState{}
	user.AddGiftsOrdered(nil)
	user.AddGiftsOrdered([]NotReceivedGiftState{})
	if len(user.Gifts.NotReceived) != 0 {
		t.Fatalf("expected no mails after empty ordered add, got %d", len(user.Gifts.NotReceived))
	}
}

// TestConsolidateGifts verifies that unreceived mails matching the given keys
// are removed with their counts summed, while other mails survive untouched.
func TestConsolidateGifts(t *testing.T) {
	user := &UserState{}
	user.Gifts.NotReceived = []NotReceivedGiftState{
		// Arena coins (type 6, id 3001) — two separate mails
		{GiftCommon: GiftCommonState{PossessionType: 6, PossessionId: 3001, Count: 100}, UserGiftUuid: "arena-1"},
		// Unrelated weapon mail — should be kept
		{GiftCommon: GiftCommonState{PossessionType: 2, PossessionId: 501, Count: 1}, UserGiftUuid: "weapon"},
		// Arena coins again — should be summed with the first
		{GiftCommon: GiftCommonState{PossessionType: 6, PossessionId: 3001, Count: 200}, UserGiftUuid: "arena-2"},
		// Gems (type 11, id 0) — not in the key set, should be kept
		{GiftCommon: GiftCommonState{PossessionType: 11, PossessionId: 0, Count: 500}, UserGiftUuid: "gems"},
		// Material (type 5, id 1001) — in the key set
		{GiftCommon: GiftCommonState{PossessionType: 5, PossessionId: 1001, Count: 50}, UserGiftUuid: "mat"},
	}

	keys := map[[2]int32]bool{
		{6, 3001}: true, // arena coins
		{5, 1001}: true, // material
	}

	pending := user.ConsolidateGifts(keys)

	// Check pending sums
	if got := pending[[2]int32{6, 3001}]; got != 300 {
		t.Errorf("expected 300 arena coins pending, got %d", got)
	}
	if got := pending[[2]int32{5, 1001}]; got != 50 {
		t.Errorf("expected 50 materials pending, got %d", got)
	}
	// Gems should not be in pending (not in key set)
	if _, ok := pending[[2]int32{11, 0}]; ok {
		t.Error("gems should not be consolidated (not in key set)")
	}

	// Check surviving mails: weapon + gems = 2
	if len(user.Gifts.NotReceived) != 2 {
		t.Fatalf("expected 2 surviving mails, got %d", len(user.Gifts.NotReceived))
	}
	if user.Gifts.NotReceived[0].UserGiftUuid != "weapon" {
		t.Errorf("first surviving mail should be weapon, got %s", user.Gifts.NotReceived[0].UserGiftUuid)
	}
	if user.Gifts.NotReceived[1].UserGiftUuid != "gems" {
		t.Errorf("second surviving mail should be gems, got %s", user.Gifts.NotReceived[1].UserGiftUuid)
	}
}

// TestConsolidateGiftsEmptyKeys verifies that an empty key set is a no-op.
func TestConsolidateGiftsEmptyKeys(t *testing.T) {
	user := &UserState{}
	user.Gifts.NotReceived = []NotReceivedGiftState{
		{GiftCommon: GiftCommonState{PossessionType: 6, PossessionId: 3001, Count: 100}, UserGiftUuid: "a"},
	}

	pending := user.ConsolidateGifts(map[[2]int32]bool{})

	if len(pending) != 0 {
		t.Errorf("expected empty pending, got %v", pending)
	}
	if len(user.Gifts.NotReceived) != 1 {
		t.Errorf("expected mail to survive, got %d mails", len(user.Gifts.NotReceived))
	}
}
