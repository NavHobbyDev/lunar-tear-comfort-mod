package store

import "testing"

// TestRollPartsVariantGrantsExact verifies a parts drop always grants the exact
// part id it names, never a random sibling from the same group+rarity (the bug
// where farming "Shard (Automata Crossover)" handed out other shards).
func TestRollPartsVariantGrantsExact(t *testing.T) {
	g := &PossessionGranter{
		PartsById: map[int32]PartsRef{
			8003: {PartsGroupId: 401, RarityType: 10, PartsInitialLotteryId: 3},
		},
		// A full 5-variant set is present; the old code would pick one at random.
		PartsVariantsByGroupRarity: map[int32]map[int32][]int32{
			401: {10: {8001, 8002, 8003, 8004, 8005}},
		},
	}

	for i := 0; i < 100; i++ {
		id, ref, ok := g.rollPartsVariant(8003)
		if !ok {
			t.Fatalf("expected ok for known part")
		}
		if id != 8003 {
			t.Fatalf("got wrong part id %d, want exactly 8003", id)
		}
		if ref.PartsGroupId != 401 || ref.PartsInitialLotteryId != 3 {
			t.Fatalf("ref mismatch for granted part: %+v", ref)
		}
	}

	// Unknown part: returns the requested id and ok=false (granted bare).
	if id, _, ok := g.rollPartsVariant(99999); ok || id != 99999 {
		t.Errorf("unknown part should return (99999, false), got (%d, %v)", id, ok)
	}
}

// TestRollPartsDropPieceRollsRarity verifies a battle-drop memoir piece rolls
// its rarity across every tier up to the quest's cap instead of always cloning
// the wired rarity, and never exceeds the cap.
func TestRollPartsDropPieceRollsRarity(t *testing.T) {
	g := &PossessionGranter{
		PartsById: map[int32]PartsRef{
			1:  {PartsGroupId: 1, RarityType: 10, PartsInitialLotteryId: 1},
			6:  {PartsGroupId: 1, RarityType: 20, PartsInitialLotteryId: 1},
			11: {PartsGroupId: 1, RarityType: 30, PartsInitialLotteryId: 1},
			16: {PartsGroupId: 1, RarityType: 40, PartsInitialLotteryId: 1},
		},
		PartsVariantsByGroupRarity: map[int32]map[int32][]int32{
			1: {10: {1}, 20: {6}, 30: {11}, 40: {16}},
		},
	}

	// Cap 30 stamped by the quest: tiers 10/20/30 must all occur, rarity 40
	// must never drop even though the wired part is only rarity 10.
	seen := map[int32]bool{}
	for i := 0; i < 2000; i++ {
		_, ref, ok := g.rollPartsDropPiece(1, 1, 30)
		if !ok {
			t.Fatalf("expected ok for known part")
		}
		if ref.RarityType > 30 {
			t.Fatalf("rolled rarity %d above the quest cap 30", ref.RarityType)
		}
		seen[ref.RarityType] = true
	}
	for _, r := range []int32{10, 20, 30} {
		if !seen[r] {
			t.Errorf("rarity %d never rolled under cap 30", r)
		}
	}

	// Cap 0 falls back to the wired part's own rarity.
	for i := 0; i < 100; i++ {
		if _, ref, _ := g.rollPartsDropPiece(1, 1, 0); ref.RarityType != 10 {
			t.Fatalf("cap 0 must fall back to wired rarity 10, got %d", ref.RarityType)
		}
	}

	// Unknown part: returns the requested id and ok=false (granted bare).
	if id, _, ok := g.rollPartsDropPiece(99999, 1, 40); ok || id != 99999 {
		t.Errorf("unknown part should return (99999, false), got (%d, %v)", id, ok)
	}
}

// TestGrantOrSellPartsPoolDropDrawsWithoutReplacement verifies the pool drop
// draws deck-style from the union of all set pieces: within one run a piece
// can only repeat after every piece in the pool has dropped, so 5 draws from
// a 3-piece pool yield each piece at least once and none more than twice.
func TestGrantOrSellPartsPoolDropDrawsWithoutReplacement(t *testing.T) {
	g := &PossessionGranter{
		PartsById: map[int32]PartsRef{
			1: {PartsGroupId: 1, RarityType: 10, PartsInitialLotteryId: 1},
			2: {PartsGroupId: 2, RarityType: 10, PartsInitialLotteryId: 1},
			3: {PartsGroupId: 3, RarityType: 10, PartsInitialLotteryId: 1},
		},
		PartsVariantsByGroupRarity: map[int32]map[int32][]int32{
			1: {10: {1}},
			2: {10: {2}},
			3: {10: {3}},
		},
		PartsSetGroupIdsByGroupId: map[int32][]int32{
			1: {1, 2, 3},
			2: {1, 2, 3},
			3: {1, 2, 3},
		},
	}
	user := &UserState{
		Parts:           map[string]PartsState{},
		PartsGroupNotes: map[int32]PartsGroupNoteState{},
		PartsStatusSubs: map[PartsStatusSubKey]PartsStatusSubState{},
		ConsumableItems: map[int32]int32{},
	}

	for i := 0; i < 50; i++ {
		rolls := g.GrantOrSellPartsPoolDrop(user, []int32{1}, 5, 10, nil, nil, 1000)
		if len(rolls) != 5 {
			t.Fatalf("want 5 rolls, got %d", len(rolls))
		}
		seenGroups := map[int32]int{}
		for _, r := range rolls {
			if r.Sold {
				t.Fatalf("no auto-sale rules set, but roll %d was sold", r.PartsId)
			}
			ref, ok := g.PartsById[r.PartsId]
			if !ok {
				t.Fatalf("rolled unknown part id %d", r.PartsId)
			}
			seenGroups[ref.PartsGroupId]++
		}
		for _, gr := range []int32{1, 2, 3} {
			if seenGroups[gr] < 1 || seenGroups[gr] > 2 {
				t.Fatalf("group %d drawn %d times from a 3-piece pool over 5 draws, want 1..2 (rolls: %+v)", gr, seenGroups[gr], rolls)
			}
		}
	}
	if len(user.Parts) != 250 {
		t.Errorf("want 250 inventory rows after 50 pool drops of 5, got %d", len(user.Parts))
	}

	// A 6-piece pool from two wired sets: 5 draws must all be distinct.
	two := &PossessionGranter{
		PartsById: map[int32]PartsRef{
			1: {PartsGroupId: 1, RarityType: 10, PartsInitialLotteryId: 1},
			2: {PartsGroupId: 2, RarityType: 10, PartsInitialLotteryId: 1},
			3: {PartsGroupId: 3, RarityType: 10, PartsInitialLotteryId: 1},
			4: {PartsGroupId: 4, RarityType: 10, PartsInitialLotteryId: 1},
			5: {PartsGroupId: 5, RarityType: 10, PartsInitialLotteryId: 1},
			6: {PartsGroupId: 6, RarityType: 10, PartsInitialLotteryId: 1},
		},
		PartsVariantsByGroupRarity: map[int32]map[int32][]int32{
			1: {10: {1}}, 2: {10: {2}}, 3: {10: {3}},
			4: {10: {4}}, 5: {10: {5}}, 6: {10: {6}},
		},
		PartsSetGroupIdsByGroupId: map[int32][]int32{
			1: {1, 2, 3}, 2: {1, 2, 3}, 3: {1, 2, 3},
			4: {4, 5, 6}, 5: {4, 5, 6}, 6: {4, 5, 6},
		},
	}
	for i := 0; i < 50; i++ {
		rolls := two.GrantOrSellPartsPoolDrop(user, []int32{1, 4}, 5, 10, nil, nil, 1000)
		if len(rolls) != 5 {
			t.Fatalf("want 5 rolls, got %d", len(rolls))
		}
		seen := map[int32]bool{}
		for _, r := range rolls {
			if seen[r.PartsId] {
				t.Fatalf("duplicate piece %d in 5 draws from a 6-piece pool (rolls: %+v)", r.PartsId, rolls)
			}
			seen[r.PartsId] = true
		}
	}

	// Unknown wired parts: granted bare, one per slot.
	if rolls := two.GrantOrSellPartsPoolDrop(user, []int32{99999}, 2, 10, nil, nil, 1000); len(rolls) != 2 || rolls[0].PartsId != 99999 || rolls[0].Sold {
		t.Errorf("unknown pool should yield bare unsold rolls, got %+v", rolls)
	}
}

// TestRollPartsDropPieceRankFollowsRarity verifies the rolled drop's rank
// (PartsInitialLotteryId) is fixed by its rolled rarity per
// partsDropLotteryByRarity: max rarity comes with all sub-status slots
// pre-unlocked, lower tiers with progressively fewer.
func TestRollPartsDropPieceRankFollowsRarity(t *testing.T) {
	g := &PossessionGranter{PartsById: map[int32]PartsRef{}, PartsVariantsByGroupRarity: map[int32]map[int32][]int32{1: {}}}
	// Group 1 with the full 5-rank ladder at every rarity: id = rarity + lottery.
	for _, rarity := range []int32{10, 20, 30, 40} {
		for lottery := int32(1); lottery <= 5; lottery++ {
			id := rarity*10 + lottery
			g.PartsById[id] = PartsRef{PartsGroupId: 1, RarityType: rarity, PartsInitialLotteryId: lottery}
			g.PartsVariantsByGroupRarity[1][rarity] = append(g.PartsVariantsByGroupRarity[1][rarity], id)
		}
	}

	wantLottery := map[int32]int32{10: 2, 20: 3, 30: 4, 40: 5}
	for i := 0; i < 2000; i++ {
		_, ref, ok := g.rollPartsDropPiece(101, 1, 40)
		if !ok {
			t.Fatalf("expected ok for known part")
		}
		if ref.PartsInitialLotteryId != wantLottery[ref.RarityType] {
			t.Fatalf("rarity %d rolled rank %d, want %d", ref.RarityType, ref.PartsInitialLotteryId, wantLottery[ref.RarityType])
		}
	}

	// Missing exact rank: falls back to the closest lower rank.
	if got := g.pickVariantByLottery([]int32{101, 103}, 5); got != 103 {
		t.Errorf("want closest lower rank variant 103, got %d", got)
	}
	// No lower rank at all: any variant is acceptable, never a panic.
	if got := g.pickVariantByLottery([]int32{105}, 2); got != 105 {
		t.Errorf("want the only variant 105, got %d", got)
	}
}
