package model

// CostumeLevelBonusStat identifies which character stat a costume level-bonus
// row contributes to.
type CostumeLevelBonusStat int32

const (
	CostumeLevelBonusStatNone CostumeLevelBonusStat = iota
	CostumeLevelBonusStatHp
	CostumeLevelBonusStatAttack
	CostumeLevelBonusStatVitality
	CostumeLevelBonusStatAgility
	CostumeLevelBonusStatCriticalRatio
)

// CostumeLevelBonusTypeToStat maps an m_costume_level_bonus.CostumeLevelBonusType
// value to the stat it raises. The data only ever uses {3, 7, 9} (Attack, Hp,
// Vitality); the remaining values are handled for robustness should they ever
// appear.
func CostumeLevelBonusTypeToStat(t int32) CostumeLevelBonusStat {
	switch t {
	case 3: // Attack (additive) — confirmed in data
		return CostumeLevelBonusStatAttack
	case 7: // Hp (additive) — confirmed in data
		return CostumeLevelBonusStatHp
	case 9: // Vitality (additive) — confirmed in data
		return CostumeLevelBonusStatVitality
	case 1:
		return CostumeLevelBonusStatAgility
	case 2:
		return CostumeLevelBonusStatAttack
	case 4:
		return CostumeLevelBonusStatCriticalRatio
	case 6:
		return CostumeLevelBonusStatHp
	default:
		return CostumeLevelBonusStatNone
	}
}
