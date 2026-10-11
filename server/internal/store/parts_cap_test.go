package store

import (
	"fmt"
	"testing"

	"lunar-tear/server/internal/model"
)

func capTestUser(partsCount int) *UserState {
	user := &UserState{
		Parts:           map[string]PartsState{},
		PartsGroupNotes: map[int32]PartsGroupNoteState{},
		PartsStatusSubs: map[PartsStatusSubKey]PartsStatusSubState{},
		ConsumableItems: map[int32]int32{},
	}
	for i := 0; i < partsCount; i++ {
		key := fmt.Sprintf("p%d", i)
		user.Parts[key] = PartsState{UserPartsUuid: key}
	}
	return user
}

func capTestGranter() *PossessionGranter {
	return &PossessionGranter{
		PartsById: map[int32]PartsRef{
			1: {PartsGroupId: 1, RarityType: 10, PartsInitialLotteryId: 1},
		},
		PartsVariantsByGroupRarity: map[int32]map[int32][]int32{
			1: {10: {1}},
		},
		PartsSetGroupIdsByGroupId: map[int32][]int32{
			1: {1},
		},
		PartsSellPriceL1ByRarity: map[int32]int32{10: 1234},
		GoldConsumableItemId:     9,
	}
}

// TestPartsGrantIgnoresCap verifies a guaranteed grant (first clear,
// missions, shop, gifts) is granted even past the inventory cap: these
// rewards can carry rare one-time memoirs that must never be auto-sold.
func TestPartsGrantIgnoresCap(t *testing.T) {
	g := capTestGranter()
	user := capTestUser(int(model.PartsInventoryCap))

	g.GrantParts(user, 1, 1000)
	if len(user.Parts) != int(model.PartsInventoryCap)+1 {
		t.Fatalf("guaranteed grant must overflow the cap, got %d parts", len(user.Parts))
	}
	if got := user.ConsumableItems[9]; got != 0 {
		t.Fatalf("guaranteed grants must never be sold, got %d gold", got)
	}

	// Below the cap the same grant adds the part as usual.
	below := capTestUser(int(model.PartsInventoryCap) - 1)
	g.GrantParts(below, 1, 1000)
	if len(below.Parts) != int(model.PartsInventoryCap) {
		t.Fatalf("below the cap the part must be granted, got %d parts", len(below.Parts))
	}
	if below.ConsumableItems[9] != 0 {
		t.Fatalf("nothing must be sold below the cap, got %d gold", below.ConsumableItems[9])
	}
}

// TestPoolDropSoldAtCap verifies re-farmable battle drops at the inventory
// cap are all sold for gold — including parts unknown to master data — so a
// quest finish never overflows the inventory.
func TestPoolDropSoldAtCap(t *testing.T) {
	g := capTestGranter()
	user := capTestUser(int(model.PartsInventoryCap))

	rolls := g.GrantOrSellPartsPoolDrop(user, []int32{1}, 3, 10, nil, nil, 1000)
	if len(rolls) != 3 {
		t.Fatalf("want 3 rolls, got %d", len(rolls))
	}
	for _, r := range rolls {
		if !r.Sold {
			t.Fatalf("roll %d must be sold at the cap", r.PartsId)
		}
	}
	if len(user.Parts) != int(model.PartsInventoryCap) {
		t.Fatalf("parts count must stay at the cap, got %d", len(user.Parts))
	}
	if got := user.ConsumableItems[9]; got != 3*1234 {
		t.Fatalf("want %d gold for 3 sold parts, got %d", 3*1234, got)
	}

	// Unknown wired parts have no rarity: still sold at the cheapest table
	// price, and the roll reports Sold.
	bare := capTestUser(int(model.PartsInventoryCap))
	bareRolls := g.GrantOrSellPartsPoolDrop(bare, []int32{99999}, 2, 10, nil, nil, 1000)
	if len(bareRolls) != 2 {
		t.Fatalf("want 2 bare rolls, got %d", len(bareRolls))
	}
	for _, r := range bareRolls {
		if !r.Sold {
			t.Fatalf("bare roll %d must be sold at the cap", r.PartsId)
		}
	}
	if len(bare.Parts) != int(model.PartsInventoryCap) {
		t.Fatalf("bare parts must not overflow the cap, got %d", len(bare.Parts))
	}
	if got := bare.ConsumableItems[9]; got != 2*1234 {
		t.Fatalf("want %d gold for 2 bare parts sold, got %d", 2*1234, got)
	}
}

func ssrCapTestGranter() *PossessionGranter {
	return &PossessionGranter{
		PartsById: map[int32]PartsRef{
			1: {PartsGroupId: 1, RarityType: 10, PartsInitialLotteryId: 1},
			2: {PartsGroupId: 2, RarityType: 40, PartsInitialLotteryId: 5},
		},
		PartsSellPriceL1ByRarity: map[int32]int32{10: 1234, 40: 9999},
		GoldConsumableItemId:     9,
	}
}

// TestSSRDropMailedAtCap verifies a rarity-40 drop at the inventory cap is
// mailed to the gift box instead of being sold, while a sub-SSR drop from
// the same run is still sold for gold.
func TestSSRDropMailedAtCap(t *testing.T) {
	g := ssrCapTestGranter()
	user := capTestUser(int(model.PartsInventoryCap))

	if sold := g.grantOrSellRolledPart(user, 2, g.PartsById[2], map[int32]bool{40: true}, map[int32]bool{5: true}, 1000); sold {
		t.Fatalf("SSR drop must never be sold")
	}
	if len(user.Parts) != int(model.PartsInventoryCap) {
		t.Fatalf("SSR drop must not overflow the cap, got %d parts", len(user.Parts))
	}
	if len(user.Gifts.NotReceived) != 1 {
		t.Fatalf("expected SSR drop mailed to gift box, got %d gifts", len(user.Gifts.NotReceived))
	}
	gift := user.Gifts.NotReceived[0]
	if gift.GiftCommon.PossessionType != int32(model.PossessionTypeParts) || gift.GiftCommon.PossessionId != 2 || gift.GiftCommon.Count != 1 {
		t.Fatalf("expected mailed gift to be parts 2, got type=%d id=%d count=%d",
			gift.GiftCommon.PossessionType, gift.GiftCommon.PossessionId, gift.GiftCommon.Count)
	}
	if user.Notifications.GiftNotReceiveCount != 1 {
		t.Fatalf("expected gift badge count 1, got %d", user.Notifications.GiftNotReceiveCount)
	}
	if got := user.ConsumableItems[9]; got != 0 {
		t.Fatalf("mailed SSR must not credit gold, got %d", got)
	}

	if sold := g.grantOrSellRolledPart(user, 1, g.PartsById[1], nil, nil, 1000); !sold {
		t.Fatalf("sub-SSR drop must be sold at the cap")
	}
	if got := user.ConsumableItems[9]; got != 1234 {
		t.Fatalf("want 1234 gold for the sold sub-SSR drop, got %d", got)
	}
}

// TestSSRDropGrantedBelowCap verifies an SSR drop below the cap is granted
// normally — never sold, even when the player's auto-sale settings list
// rarity 40.
func TestSSRDropGrantedBelowCap(t *testing.T) {
	g := ssrCapTestGranter()
	user := capTestUser(int(model.PartsInventoryCap) - 1)

	if sold := g.grantOrSellRolledPart(user, 2, g.PartsById[2], map[int32]bool{40: true}, map[int32]bool{5: true}, 1000); sold {
		t.Fatalf("SSR drop must not be auto-sold below the cap")
	}
	if len(user.Parts) != int(model.PartsInventoryCap) {
		t.Fatalf("SSR drop must fill the last free slot, got %d parts", len(user.Parts))
	}
	if len(user.Gifts.NotReceived) != 0 {
		t.Fatalf("nothing must be mailed below the cap, got %d gifts", len(user.Gifts.NotReceived))
	}
	if got := user.ConsumableItems[9]; got != 0 {
		t.Fatalf("granted SSR must not credit gold, got %d", got)
	}
}

// TestGrantPartsExactKeepsVariant verifies the claim path for mailed
// overflow memoirs grants the exact rolled variant instead of re-rolling a
// sibling of the same group+rarity.
func TestGrantPartsExactKeepsVariant(t *testing.T) {
	g := &PossessionGranter{
		PartsById: map[int32]PartsRef{
			501: {PartsGroupId: 7, RarityType: 40, PartsInitialLotteryId: 5},
			502: {PartsGroupId: 7, RarityType: 40, PartsInitialLotteryId: 2},
		},
		PartsVariantsByGroupRarity: map[int32]map[int32][]int32{7: {40: {501, 502}}},
	}
	user := capTestUser(0)

	for i := 0; i < 50; i++ {
		before := len(user.Parts)
		g.GrantPartsExact(user, 501, 1000)
		if len(user.Parts) != before+1 {
			t.Fatalf("expected one part granted, have %d want %d", len(user.Parts), before+1)
		}
		for _, p := range user.Parts {
			if p.PartsId != 501 {
				t.Fatalf("GrantPartsExact must keep the exact variant, got %d", p.PartsId)
			}
		}
		user.Parts = map[string]PartsState{}
	}

	// Unknown part: granted bare.
	g.GrantPartsExact(user, 99999, 1000)
	if len(user.Parts) != 1 || user.Parts[onlyKey(user.Parts)].PartsId != 99999 {
		t.Fatalf("unknown part must be granted bare")
	}
}

func onlyKey(m map[string]PartsState) string {
	for k := range m {
		return k
	}
	return ""
}
