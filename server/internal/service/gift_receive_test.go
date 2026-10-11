package service

import (
	"fmt"
	"testing"

	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
)

func fillGiftWeapons(user *store.UserState, n int) {
	user.EnsureMaps()
	for i := range n {
		user.Weapons[fmt.Sprintf("w%d", i)] = store.WeaponState{WeaponId: int32(i)}
	}
}

func weaponGift(uuid string, weaponId int32) store.NotReceivedGiftState {
	return store.NotReceivedGiftState{
		GiftCommon: store.GiftCommonState{
			PossessionType: int32(model.PossessionTypeWeapon),
			PossessionId:   weaponId,
			Count:          1,
		},
		UserGiftUuid: uuid,
	}
}

func TestClaimGiftWeaponKeptInMailboxAtCap(t *testing.T) {
	user := &store.UserState{}
	fillGiftWeapons(user, int(model.WeaponInventoryCap))

	granter := &store.PossessionGranter{}
	gold := store.NotReceivedGiftState{
		GiftCommon: store.GiftCommonState{
			PossessionType: int32(model.PossessionTypeConsumableItem),
			PossessionId:   1,
			Count:          500,
		},
		UserGiftUuid: "gold-1",
	}

	// A "receive all" batch: the consumable is claimable, the weapon must not be.
	if claimGift(user, granter, weaponGift("weapon-1", 303), 123) {
		t.Fatalf("weapon gift claimed despite full weapon inventory")
	}
	if !claimGift(user, granter, gold, 123) {
		t.Fatalf("non-weapon gift must stay claimable at weapon cap")
	}

	if len(user.Weapons) != int(model.WeaponInventoryCap) {
		t.Fatalf("weapon inventory grew past the cap: %d", len(user.Weapons))
	}
	if len(user.Gifts.Received) != 1 {
		t.Fatalf("expected only the gold gift recorded as received, got %d", len(user.Gifts.Received))
	}
	if got := user.ConsumableItems[1]; got != 500 {
		t.Fatalf("expected gold granted, got %d", got)
	}
}

func partsGift(uuid string, partsId int32) store.NotReceivedGiftState {
	return store.NotReceivedGiftState{
		GiftCommon: store.GiftCommonState{
			PossessionType: int32(model.PossessionTypeParts),
			PossessionId:   partsId,
			Count:          1,
		},
		UserGiftUuid: uuid,
	}
}

func fillGiftParts(user *store.UserState, n int) {
	user.EnsureMaps()
	for i := range n {
		key := fmt.Sprintf("p%d", i)
		user.Parts[key] = store.PartsState{UserPartsUuid: key}
	}
}

func TestClaimGiftPartsKeptInMailboxAtCap(t *testing.T) {
	user := &store.UserState{}
	fillGiftParts(user, int(model.PartsInventoryCap))

	granter := &store.PossessionGranter{}
	if claimGift(user, granter, partsGift("parts-1", 501), 123) {
		t.Fatalf("parts gift claimed despite full parts inventory")
	}
	if len(user.Parts) != int(model.PartsInventoryCap) {
		t.Fatalf("parts inventory grew past the cap: %d", len(user.Parts))
	}
	if len(user.Gifts.Received) != 0 {
		t.Fatalf("no gift must be recorded as received, got %d", len(user.Gifts.Received))
	}
}

func TestClaimGiftPartsGrantsExactVariant(t *testing.T) {
	user := &store.UserState{}
	user.EnsureMaps()

	// Sibling 502 shares group+rarity: a re-rolling claim could grant it
	// instead of the mailed SSR variant 501.
	granter := &store.PossessionGranter{
		PartsById: map[int32]store.PartsRef{
			501: {PartsGroupId: 7, RarityType: 40, PartsInitialLotteryId: 5},
			502: {PartsGroupId: 7, RarityType: 40, PartsInitialLotteryId: 2},
		},
		PartsVariantsByGroupRarity: map[int32]map[int32][]int32{
			7: {40: {501, 502}},
		},
	}

	if !claimGift(user, granter, partsGift("parts-1", 501), 123) {
		t.Fatalf("parts gift must be claimable below the cap")
	}
	if len(user.Parts) != 1 {
		t.Fatalf("expected exactly one granted part, got %d", len(user.Parts))
	}
	for _, p := range user.Parts {
		if p.PartsId != 501 {
			t.Fatalf("claim re-rolled the mailed memoir: got partsId=%d, want 501", p.PartsId)
		}
	}
}

func giftOfKind(possessionType model.PossessionType, possessionId int32) store.GiftCommonState {
	return store.GiftCommonState{PossessionType: int32(possessionType), PossessionId: possessionId, Count: 1}
}

func TestGiftMatchesKinds(t *testing.T) {
	const goldId int32 = 3001
	weapon := giftOfKind(model.PossessionTypeWeapon, 303)
	parts := giftOfKind(model.PossessionTypeParts, 501)
	gold := giftOfKind(model.PossessionTypeConsumableItem, goldId)
	ticket := giftOfKind(model.PossessionTypeConsumableItem, 2003)
	gems := giftOfKind(model.PossessionTypeFreeGem, 0)

	// No filter: everything passes.
	for _, g := range []store.GiftCommonState{weapon, parts, gold, ticket, gems} {
		if !giftMatchesKinds(g, nil, goldId) {
			t.Fatalf("empty filter must match everything, type=%d rejected", g.PossessionType)
		}
	}

	// Weapon checkbox only.
	weaponOnly := map[int32]bool{giftRewardKindWeapon: true}
	if !giftMatchesKinds(weapon, weaponOnly, goldId) {
		t.Fatalf("weapon filter must keep weapon mails")
	}
	for _, g := range []store.GiftCommonState{parts, gold, ticket, gems} {
		if giftMatchesKinds(g, weaponOnly, goldId) {
			t.Fatalf("weapon filter must reject type=%d id=%d", g.PossessionType, g.PossessionId)
		}
	}

	// Gold vs OTHER split inside consumables.
	if !giftMatchesKinds(gold, map[int32]bool{giftRewardKindGold: true}, goldId) {
		t.Fatalf("gold filter must keep the gold consumable")
	}
	if giftMatchesKinds(ticket, map[int32]bool{giftRewardKindGold: true}, goldId) {
		t.Fatalf("gold filter must reject tickets")
	}
	if !giftMatchesKinds(ticket, map[int32]bool{giftRewardKindOther: true}, goldId) {
		t.Fatalf("other filter must keep tickets")
	}
	if giftMatchesKinds(gold, map[int32]bool{giftRewardKindOther: true}, goldId) {
		t.Fatalf("other filter must reject gold")
	}

	// Multiple checkboxes OR together.
	mixed := map[int32]bool{giftRewardKindParts: true, giftRewardKindGem: true}
	if !giftMatchesKinds(parts, mixed, goldId) || !giftMatchesKinds(gems, mixed, goldId) {
		t.Fatalf("multi-kind filter must keep every checked kind")
	}
	if giftMatchesKinds(weapon, mixed, goldId) {
		t.Fatalf("multi-kind filter must reject unchecked kinds")
	}
}

func TestGiftMatchesExpiration(t *testing.T) {
	const now int64 = 100000
	valid := int64(200000)
	expired := int64(50000)

	cases := []struct {
		expiration int64
		filter     int32
		want       bool
	}{
		{valid, giftExpirationNone, true},
		{expired, giftExpirationNone, true},
		{0, giftExpirationNone, true},
		{valid, giftExpirationOnlyValid, true},
		{0, giftExpirationOnlyValid, true},
		{expired, giftExpirationOnlyValid, false},
		{expired, giftExpirationOnlyExpired, true},
		{valid, giftExpirationOnlyExpired, false},
		{0, giftExpirationOnlyExpired, false},
	}
	for _, tc := range cases {
		if got := giftMatchesExpiration(tc.expiration, tc.filter, now); got != tc.want {
			t.Fatalf("expiration=%d filter=%d: got %v, want %v", tc.expiration, tc.filter, got, tc.want)
		}
	}
}

func TestGiftPageBounds(t *testing.T) {
	cases := []struct {
		name                  string
		total, pageSize       int
		nextCursor, prevCursor int64
		wantStart, wantNext, wantPrev int64
	}{
		{"first page of three", 50, 20, 0, 0, 0, 20, 0},
		{"forward to page two", 50, 20, 20, 0, 20, 40, 0},
		{"forward to last page", 50, 20, 40, 0, 40, 0, 20},
		{"back from page two is first page", 50, 20, 0, 20, 20, 40, 0},
		{"back from last page", 50, 20, 0, 40, 40, 0, 20},
		{"stale cursor past the end falls back to first page", 50, 20, 80, 0, 0, 20, 0},
		{"single page has no neighbours", 10, 20, 0, 0, 0, 0, 0},
		{"empty mailbox", 0, 20, 0, 0, 0, 0, 0},
		{"partial middle page back", 100, 20, 0, 45, 45, 65, 25},
	}
	for _, tc := range cases {
		start, next, prev := giftPageBounds(tc.total, tc.pageSize, tc.nextCursor, tc.prevCursor)
		if start != tc.wantStart || next != tc.wantNext || prev != tc.wantPrev {
			t.Fatalf("%s: got start=%d next=%d prev=%d, want start=%d next=%d prev=%d",
				tc.name, start, next, prev, tc.wantStart, tc.wantNext, tc.wantPrev)
		}
	}
}

func TestClaimGiftWeaponFillsLastSlotThenBlocks(t *testing.T) {
	user := &store.UserState{}
	fillGiftWeapons(user, int(model.WeaponInventoryCap)-1)

	granter := &store.PossessionGranter{}
	if !claimGift(user, granter, weaponGift("weapon-1", 301), 123) {
		t.Fatalf("expected weapon claimed into the last free slot")
	}
	if len(user.Weapons) != int(model.WeaponInventoryCap) {
		t.Fatalf("expected inventory filled to cap, got %d", len(user.Weapons))
	}
	if claimGift(user, granter, weaponGift("weapon-2", 302), 123) {
		t.Fatalf("second weapon gift claimed with inventory at cap")
	}
}
