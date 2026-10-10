package masterdata

import (
	"fmt"

	"lunar-tear/server/internal/utils"
)

type CompanionLevelKey struct {
	CategoryType int32
	Level        int32
}

type CompanionMaterialCost struct {
	MaterialId int32
	Count      int32
}

type CompanionCatalog struct {
	CompanionById      map[int32]EntityMCompanion
	GoldCostByCategory map[int32]NumericalFunc
	MaterialsByKey     map[CompanionLevelKey]CompanionMaterialCost
}

func LoadCompanionCatalog() (*CompanionCatalog, error) {
	companions, err := utils.ReadTable[EntityMCompanion]("m_companion")
	if err != nil {
		return nil, fmt.Errorf("load companion table: %w", err)
	}

	categories, err := utils.ReadTable[EntityMCompanionCategory]("m_companion_category")
	if err != nil {
		return nil, fmt.Errorf("load companion category table: %w", err)
	}

	materials, err := utils.ReadTable[EntityMCompanionEnhancementMaterial]("m_companion_enhancement_material")
	if err != nil {
		return nil, fmt.Errorf("load companion enhancement material table: %w", err)
	}

	funcResolver, err := LoadFunctionResolver()
	if err != nil {
		return nil, fmt.Errorf("load function resolver: %w", err)
	}

	companionById := make(map[int32]EntityMCompanion, len(companions))
	for _, c := range companions {
		companionById[c.CompanionId] = c
	}

	goldCostByCategory := make(map[int32]NumericalFunc, len(categories))
	for _, cat := range categories {
		if f, ok := funcResolver.Resolve(cat.EnhancementCostNumericalFunctionId); ok {
			goldCostByCategory[cat.CompanionCategoryType] = f
		}
	}

	materialsByKey := make(map[CompanionLevelKey]CompanionMaterialCost, len(materials))
	for _, m := range materials {
		key := CompanionLevelKey{CategoryType: m.CompanionCategoryType, Level: m.Level}
		materialsByKey[key] = CompanionMaterialCost{MaterialId: m.MaterialId, Count: m.Count}
	}

	return &CompanionCatalog{
		CompanionById:      companionById,
		GoldCostByCategory: goldCostByCategory,
		MaterialsByKey:     materialsByKey,
	}, nil
}
