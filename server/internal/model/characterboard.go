package model

type CharacterBoardEffectType int32

const (
	CharacterBoardEffectTypeUnknown  CharacterBoardEffectType = 0
	CharacterBoardEffectTypeAbility  CharacterBoardEffectType = 1
	CharacterBoardEffectTypeStatusUp CharacterBoardEffectType = 2
)

type CharacterBoardStatusUpType int32

const (
	CharacterBoardStatusUpTypeUnknown          CharacterBoardStatusUpType = 0
	CharacterBoardStatusUpTypeAgilityAdd       CharacterBoardStatusUpType = 1
	CharacterBoardStatusUpTypeAgilityMultiply  CharacterBoardStatusUpType = 2
	CharacterBoardStatusUpTypeAttackAdd        CharacterBoardStatusUpType = 3
	CharacterBoardStatusUpTypeAttackMultiply   CharacterBoardStatusUpType = 4
	CharacterBoardStatusUpTypeCritAttackAdd    CharacterBoardStatusUpType = 5
	CharacterBoardStatusUpTypeCritRatioAdd     CharacterBoardStatusUpType = 6
	CharacterBoardStatusUpTypeHpAdd            CharacterBoardStatusUpType = 7
	CharacterBoardStatusUpTypeHpMultiply       CharacterBoardStatusUpType = 8
	CharacterBoardStatusUpTypeVitalityAdd      CharacterBoardStatusUpType = 9
	CharacterBoardStatusUpTypeVitalityMultiply CharacterBoardStatusUpType = 10
)

type StatusCalculationType int32

const (
	StatusCalculationTypeUnknown  StatusCalculationType = 0
	StatusCalculationTypeAdd      StatusCalculationType = 1
	StatusCalculationTypeMultiply StatusCalculationType = 2
)

func StatusUpTypeToCalcType(t CharacterBoardStatusUpType) StatusCalculationType {
	switch t {
	case CharacterBoardStatusUpTypeAgilityMultiply,
		CharacterBoardStatusUpTypeAttackMultiply,
		CharacterBoardStatusUpTypeHpMultiply,
		CharacterBoardStatusUpTypeVitalityMultiply:
		return StatusCalculationTypeMultiply
	default:
		return StatusCalculationTypeAdd
	}
}
