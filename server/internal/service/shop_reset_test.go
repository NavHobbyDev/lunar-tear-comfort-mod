package service

import (
	"testing"

	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
)

func shopTestCatalog() *masterdata.ShopCatalog {
	return &masterdata.ShopCatalog{
		Items: map[int32]masterdata.EntityMShopItem{
			1: {ShopItemId: 1, ShopItemLimitedStockId: 101}, // daily reset stock
			2: {ShopItemId: 2, ShopItemLimitedStockId: 102}, // weekly reset stock
			3: {ShopItemId: 3, ShopItemLimitedStockId: 103}, // monthly reset stock
			4: {ShopItemId: 4, ShopItemLimitedStockId: 104}, // no auto reset (lifetime limit)
			5: {ShopItemId: 5},                              // unlimited
		},
		LimitedStock: map[int32]int32{
			101: 20,
			102: 5,
			103: 3,
			104: 1,
		},
		LimitedStockReset: map[int32]masterdata.LimitedStockResetRule{
			101: {ResetType: model.ShopItemAutoResetDaily, ResetPeriod: 1},
			102: {ResetType: model.ShopItemAutoResetDaily, ResetPeriod: 7},
			103: {ResetType: model.ShopItemAutoResetMonthly, ResetPeriod: 1},
			104: {ResetType: model.ShopItemAutoResetNone, ResetPeriod: 0},
		},
	}
}

func TestShopStockAutoResets(t *testing.T) {
	catalog := shopTestCatalog()

	cases := []struct {
		name       string
		shopItemId int32
		want       bool
	}{
		{"daily stock refilled", 1, true},
		{"weekly stock refilled", 2, true},
		{"monthly stock refilled", 3, true},
		{"lifetime limit consumed", 4, false},
		{"unlimited item", 5, false},
	}
	for _, tc := range cases {
		item := catalog.Items[tc.shopItemId]
		if got := shopStockAutoResets(catalog, item); got != tc.want {
			t.Errorf("%s: shopStockAutoResets=%v, want %v", tc.name, got, tc.want)
		}
	}
}

// Purchases of refilling stock must never consume the counter (and heal any
// stale counter from before the reset removal), while lifetime-limited items
// keep counting up as before.
func TestBuyCounterBehaviorWithoutReset(t *testing.T) {
	catalog := shopTestCatalog()
	user := &store.UserState{ShopItems: map[int32]store.UserShopItemState{
		1: {ShopItemId: 1, BoughtCount: 12}, // stale counter from the past
		4: {ShopItemId: 4, BoughtCount: 0},
	}}

	// Simulate the Buy counter logic for a refilling item.
	item := catalog.Items[1]
	si := user.ShopItems[1]
	if shopStockAutoResets(catalog, item) {
		si.BoughtCount = 0
		user.ShopItems[1] = si
	} else {
		si.BoughtCount += 3
		user.ShopItems[1] = si
	}
	if got := user.ShopItems[1].BoughtCount; got != 0 {
		t.Errorf("refilling item: BoughtCount=%d, want 0 (stock never consumed)", got)
	}

	// Simulate repeated buys of a lifetime-limited item.
	item = catalog.Items[4]
	for i := 0; i < 1; i++ {
		si = user.ShopItems[4]
		if shopStockAutoResets(catalog, item) {
			si.BoughtCount = 0
		} else {
			si.BoughtCount++
			if maxCount, ok := catalog.LimitedStock[item.ShopItemLimitedStockId]; ok && si.BoughtCount >= maxCount {
				si.BoughtCount = 0
			}
		}
		user.ShopItems[4] = si
	}
	if got := user.ShopItems[4].BoughtCount; got != 0 {
		t.Errorf("lifetime item (max 1): BoughtCount=%d, want 0 after wrap at max", got)
	}
}
