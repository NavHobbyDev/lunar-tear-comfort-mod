package model

import "testing"

func TestCostumeLevelBonusTypeToStat(t *testing.T) {
	cases := map[int32]CostumeLevelBonusStat{
		3: CostumeLevelBonusStatAttack,   // confirmed
		7: CostumeLevelBonusStatHp,       // confirmed
		9: CostumeLevelBonusStatVitality, // confirmed
		1: CostumeLevelBonusStatAgility,
		2: CostumeLevelBonusStatAttack,
		4: CostumeLevelBonusStatCriticalRatio,
		6: CostumeLevelBonusStatHp,
		0: CostumeLevelBonusStatNone,
		5: CostumeLevelBonusStatNone,
	}
	for typ, want := range cases {
		if got := CostumeLevelBonusTypeToStat(typ); got != want {
			t.Errorf("CostumeLevelBonusTypeToStat(%d) = %d, want %d", typ, got, want)
		}
	}
}

// bonusRow is a minimal stand-in for an m_costume_level_bonus row so the
// accumulation logic can be exercised without importing the masterdata package.
type bonusRow struct {
	level       int32
	bonusType   int32
	effectValue int32
}

// accumulate sums EffectValue per stat for rows in (from, to], mirroring the
// idempotent accumulation done by the costume service.
func accumulate(rows []bonusRow, from, to int32) map[CostumeLevelBonusStat]int32 {
	out := make(map[CostumeLevelBonusStat]int32)
	if to <= from {
		return out
	}
	for _, r := range rows {
		if r.level <= from || r.level > to {
			continue
		}
		out[CostumeLevelBonusTypeToStat(r.bonusType)] += r.effectValue
	}
	return out
}

func TestCostumeLevelBonusAccumulationIdempotent(t *testing.T) {
	rows := []bonusRow{
		{level: 1, bonusType: 3, effectValue: 10}, // Attack
		{level: 2, bonusType: 7, effectValue: 20}, // Hp
		{level: 3, bonusType: 9, effectValue: 30}, // Vitality
		{level: 4, bonusType: 3, effectValue: 5},  // Attack
	}

	// First confirm up to level 3.
	got := accumulate(rows, 0, 3)
	if got[CostumeLevelBonusStatAttack] != 10 || got[CostumeLevelBonusStatHp] != 20 || got[CostumeLevelBonusStatVitality] != 30 {
		t.Fatalf("confirm 0->3 = %v, want attack=10 hp=20 vitality=30", got)
	}

	// Re-confirming the same level adds nothing.
	if again := accumulate(rows, 3, 3); len(again) != 0 {
		t.Fatalf("re-confirm 3->3 = %v, want empty", again)
	}

	// Confirming a lower level adds nothing.
	if lower := accumulate(rows, 3, 2); len(lower) != 0 {
		t.Fatalf("confirm 3->2 = %v, want empty", lower)
	}

	// Only the newly-confirmed level 4 contributes when raising 3->4.
	if next := accumulate(rows, 3, 4); next[CostumeLevelBonusStatAttack] != 5 {
		t.Fatalf("confirm 3->4 = %v, want attack=5", next)
	}
}
