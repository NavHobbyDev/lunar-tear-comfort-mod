package model

type PossessionType int32

const (
	PossessionTypeUnknown           PossessionType = 0
	PossessionTypeCostume           PossessionType = 1
	PossessionTypeWeapon            PossessionType = 2
	PossessionTypeCompanion         PossessionType = 3
	PossessionTypeParts             PossessionType = 4
	PossessionTypeMaterial          PossessionType = 5
	PossessionTypeConsumableItem    PossessionType = 6
	PossessionTypeCostumeEnhanced   PossessionType = 7
	PossessionTypeWeaponEnhanced    PossessionType = 8
	PossessionTypeCompanionEnhanced PossessionType = 9
	PossessionTypePartsEnhanced     PossessionType = 10
	PossessionTypePaidGem           PossessionType = 11
	PossessionTypeFreeGem           PossessionType = 12
	PossessionTypeImportantItem     PossessionType = 13
	PossessionTypeThought           PossessionType = 14
	PossessionTypeMissionPassPoint  PossessionType = 15
	PossessionTypePremiumItem       PossessionType = 16
)
