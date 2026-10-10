package store

import (
	"testing"

	"lunar-tear/server/internal/model"
)

func TestCanonicalConsumableMedalId(t *testing.T) {
	// Tier variants remap to their spendable base medal.
	remaps := map[int32]int32{53: 22, 54: 22, 63: 29, 64: 29, 65: 29}
	for from, want := range remaps {
		if got := CanonicalConsumableMedalId(from); got != want {
			t.Errorf("%d -> %d, want %d", from, got, want)
		}
	}
	// Base medals and unrelated consumables are untouched.
	for _, id := range []int32{22, 29, 1, 99, 8002} {
		if got := CanonicalConsumableMedalId(id); got != id {
			t.Errorf("%d remapped to %d, want unchanged", id, got)
		}
	}
}

func TestGrantPossessionRemapsTierMedal(t *testing.T) {
	u := &UserState{ConsumableItems: map[int32]int32{}}

	// Copper + Silver Coffin of Repose Medals both land on the base id 22.
	GrantPossession(u, model.PossessionTypeConsumableItem, 53, 5)
	GrantPossession(u, model.PossessionTypeConsumableItem, 54, 3)
	if u.ConsumableItems[53] != 0 || u.ConsumableItems[54] != 0 {
		t.Errorf("tier variants should not be granted directly (53=%d 54=%d)", u.ConsumableItems[53], u.ConsumableItems[54])
	}
	if u.ConsumableItems[22] != 8 {
		t.Errorf("Coffin of Repose tiers should accumulate on base id 22, got %d (want 8)", u.ConsumableItems[22])
	}

	// Gold Rhythm's Citadel Medal lands on base id 29.
	GrantPossession(u, model.PossessionTypeConsumableItem, 65, 2)
	if u.ConsumableItems[29] != 2 {
		t.Errorf("id-65 grant should land on base id 29, got %d", u.ConsumableItems[29])
	}

	// A non-remapped consumable is unaffected.
	GrantPossession(u, model.PossessionTypeConsumableItem, 99, 3)
	if u.ConsumableItems[99] != 3 {
		t.Errorf("id 99 grant = %d, want 3", u.ConsumableItems[99])
	}
}

func TestGrantPossessionConvertsStaminaToSkips(t *testing.T) {
	u := &UserState{ConsumableItems: map[int32]int32{}}

	// Stamina 3001 -> 120 gold (1)
	GrantPossession(u, model.PossessionTypeConsumableItem, 3001, 5)
	if u.ConsumableItems[3001] != 0 {
		t.Errorf("stamina 3001 should not remain, got %d", u.ConsumableItems[3001])
	}
	if u.ConsumableItems[1] != 600 {
		t.Errorf("5x stamina 3001 should give 600 gold, got %d", u.ConsumableItems[1])
	}

	// Stamina 3002 -> 240 gold (1)
	GrantPossession(u, model.PossessionTypeConsumableItem, 3002, 3)
	if u.ConsumableItems[3002] != 0 {
		t.Errorf("stamina 3002 should not remain, got %d", u.ConsumableItems[3002])
	}
	if u.ConsumableItems[1] != 1320 {
		t.Errorf("after 3x stamina 3002 (720 gold), total should be 1320, got %d", u.ConsumableItems[1])
	}

	// Stamina 3003 -> 360 gold (1)
	GrantPossession(u, model.PossessionTypeConsumableItem, 3003, 2)
	if u.ConsumableItems[3003] != 0 {
		t.Errorf("stamina 3003 should not remain, got %d", u.ConsumableItems[3003])
	}
	if u.ConsumableItems[1] != 2040 {
		t.Errorf("after 2x stamina 3003 (720 gold), total should be 2040, got %d", u.ConsumableItems[1])
	}

	// Other consumables are not affected
	GrantPossession(u, model.PossessionTypeConsumableItem, 8001, 10)
	if u.ConsumableItems[8001] != 10 {
		t.Errorf("non-stamina consumable 8001 should not be converted, got %d", u.ConsumableItems[8001])
	}
}

func TestConvertStaminaToGoldOnLoad(t *testing.T) {
	u := &UserState{ConsumableItems: map[int32]int32{}}

	// Setup: add stamina to inventory as if loaded from database
	u.ConsumableItems[3001] = 5
	u.ConsumableItems[3002] = 3
	u.ConsumableItems[3003] = 2
	u.ConsumableItems[8001] = 10 // Other consumable should remain

	// Run conversion
	ConvertStaminaToGold(u)

	// Verify: stamina should be gone
	if u.ConsumableItems[3001] != 0 {
		t.Errorf("stamina 3001 should be removed, got %d", u.ConsumableItems[3001])
	}
	if u.ConsumableItems[3002] != 0 {
		t.Errorf("stamina 3002 should be removed, got %d", u.ConsumableItems[3002])
	}
	if u.ConsumableItems[3003] != 0 {
		t.Errorf("stamina 3003 should be removed, got %d", u.ConsumableItems[3003])
	}

	// Verify: gold should be correct
	// 5x 3001 (600 gold) + 3x 3002 (720 gold) + 2x 3003 (720 gold) = 2040 gold
	if u.ConsumableItems[1] != 2040 {
		t.Errorf("should have 2040 gold, got %d", u.ConsumableItems[1])
	}

	// Verify: other consumables are not affected
	if u.ConsumableItems[8001] != 10 {
		t.Errorf("other consumables should not be converted, got %d", u.ConsumableItems[8001])
	}
}

func TestGrantPossessionConvertsSummonTicketsToGems(t *testing.T) {
	u := &UserState{ConsumableItems: map[int32]int32{}}

	// Premium Summon Ticket 1002 -> 300 free gems each
	GrantPossession(u, model.PossessionTypeConsumableItem, 1002, 2)
	if u.ConsumableItems[1002] != 0 {
		t.Errorf("ticket 1002 should not remain, got %d", u.ConsumableItems[1002])
	}
	if u.Gem.FreeGem != 600 {
		t.Errorf("2x ticket 1002 should give 600 free gems, got %d", u.Gem.FreeGem)
	}

	// Premium Summon Ticket x10 1003 -> 3000 each
	GrantPossession(u, model.PossessionTypeConsumableItem, 1003, 1)
	if u.ConsumableItems[1003] != 0 {
		t.Errorf("ticket 1003 should not remain, got %d", u.ConsumableItems[1003])
	}
	if u.Gem.FreeGem != 3600 {
		t.Errorf("after ticket 1003 (3000), total should be 3600, got %d", u.Gem.FreeGem)
	}

	// All Guaranteed Summon Ticket 1210 -> 9000 each
	GrantPossession(u, model.PossessionTypeConsumableItem, 1210, 1)
	if u.ConsumableItems[1210] != 0 {
		t.Errorf("ticket 1210 should not remain, got %d", u.ConsumableItems[1210])
	}
	if u.Gem.FreeGem != 12600 {
		t.Errorf("after ticket 1210 (9000), total should be 12600, got %d", u.Gem.FreeGem)
	}

	// Chapter gacha ticket (id 2) must NOT be converted: consumed directly.
	GrantPossession(u, model.PossessionTypeConsumableItem, 2, 5)
	if u.ConsumableItems[2] != 5 {
		t.Errorf("chapter ticket id 2 should remain, got %d", u.ConsumableItems[2])
	}
	if u.Gem.FreeGem != 12600 {
		t.Errorf("free gems should be unchanged by ticket 2, got %d", u.Gem.FreeGem)
	}
}

func TestConvertSummonTicketsToGemsOnLoad(t *testing.T) {
	u := &UserState{ConsumableItems: map[int32]int32{}}

	// Setup: leftover tickets as if loaded from database
	u.ConsumableItems[1002] = 2  // 600
	u.ConsumableItems[1004] = 3  // 4500
	u.ConsumableItems[1210] = 1  // 9000 (All Guaranteed Summon Ticket)
	u.ConsumableItems[2] = 5     // chapter ticket, must remain
	u.ConsumableItems[8001] = 10 // other consumable, must remain

	ConvertSummonTicketsToGems(u)

	if u.ConsumableItems[1002] != 0 || u.ConsumableItems[1004] != 0 || u.ConsumableItems[1210] != 0 {
		t.Errorf("tickets 1002/1004/1210 should be removed")
	}
	// 600 + 4500 + 9000 = 14100
	if u.Gem.FreeGem != 14100 {
		t.Errorf("expected 14100 free gems, got %d", u.Gem.FreeGem)
	}
	if u.ConsumableItems[2] != 5 {
		t.Errorf("chapter ticket id 2 should remain, got %d", u.ConsumableItems[2])
	}
	if u.ConsumableItems[8001] != 10 {
		t.Errorf("other consumables should remain, got %d", u.ConsumableItems[8001])
	}
}
