package masterdata

import (
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/utils"
)

func LoadDupExchange() (map[int32][]model.DupExchangeEntry, error) {
	result := make(map[int32][]model.DupExchangeEntry)

	costumeRows, err := utils.ReadTable[EntityMCostumeDuplicationExchangePossessionGroup]("m_costume_duplication_exchange_possession_group")
	if err != nil {
		return nil, err
	}
	for _, r := range costumeRows {
		result[r.CostumeId] = append(result[r.CostumeId], model.DupExchangeEntry{
			PossessionType: r.PossessionType,
			PossessionId:   r.PossessionId,
			Count:          r.Count,
		})
	}

	return result, nil
}

func LoadCompanionDupExchange() (map[int32][]model.DupExchangeEntry, error) {
	rows, err := utils.ReadTable[EntityMCompanionDuplicationExchangePossessionGroup]("m_companion_duplication_exchange_possession_group")
	if err != nil {
		return nil, err
	}
	result := make(map[int32][]model.DupExchangeEntry)
	for _, row := range rows {
		result[row.CompanionId] = append(result[row.CompanionId], model.DupExchangeEntry{
			PossessionType: row.PossessionType,
			PossessionId:   row.PossessionId,
			Count:          row.Count,
		})
	}
	return result, nil
}

const dupExchangeFallbackCount int32 = 10

// EnrichDupExchange backfills missing duplication-exchange entries for every
// costume in the gacha pool.
//
// For each costume it ensures both of the following are present when the
// underlying master data can supply them:
//  1. Limit-break material ("text", id 311xxx) × 10
//  2. Awakening stone ("stone", id 313xxx) with the count from the awaken material group
//
// Existing entries are never overwritten or duplicated.
func EnrichDupExchange(dupMap map[int32][]model.DupExchangeEntry, pool *GachaCatalog) (int, error) {
	lbRows, err := utils.ReadTable[EntityMCostumeLimitBreakMaterialGroup]("m_costume_limit_break_material_group")
	if err != nil {
		return 0, err
	}
	groupToMaterial := make(map[int32]int32, len(lbRows))
	for _, r := range lbRows {
		groupToMaterial[r.CostumeLimitBreakMaterialGroupId] = r.MaterialId
	}

	costumeRows, err := utils.ReadTable[EntityMCostume]("m_costume")
	if err != nil {
		return 0, err
	}
	costumeLBGroup := make(map[int32]int32, len(costumeRows))
	for _, r := range costumeRows {
		costumeLBGroup[r.CostumeId] = r.CostumeLimitBreakMaterialGroupId
	}

	awakenRows, err := utils.ReadTable[EntityMCostumeAwaken]("m_costume_awaken")
	if err != nil {
		return 0, err
	}
	awakenStepRows, err := utils.ReadTable[EntityMCostumeAwakenStepMaterialGroup]("m_costume_awaken_step_material_group")
	if err != nil {
		return 0, err
	}
	awakenMaterialRows, err := utils.ReadTable[EntityMCostumeAwakenMaterialGroup]("m_costume_awaken_material_group")
	if err != nil {
		return 0, err
	}

	stepMaterialGroup := make(map[int32]int32, len(awakenStepRows))
	for _, row := range awakenStepRows {
		stepMaterialGroup[row.CostumeAwakenStepMaterialGroupId] = row.CostumeAwakenMaterialGroupId
	}

	// Prefer the material with the lowest SortOrder inside each group
	// (the primary awakening stone).
	firstMaterialByGroup := make(map[int32]EntityMCostumeAwakenMaterialGroup, len(awakenMaterialRows))
	for _, row := range awakenMaterialRows {
		current, exists := firstMaterialByGroup[row.CostumeAwakenMaterialGroupId]
		if !exists || row.SortOrder < current.SortOrder {
			firstMaterialByGroup[row.CostumeAwakenMaterialGroupId] = row
		}
	}

	awakenMaterialByCostume := make(map[int32]EntityMCostumeAwakenMaterialGroup, len(awakenRows))
	for _, row := range awakenRows {
		materialGroupId := stepMaterialGroup[row.CostumeAwakenStepMaterialGroupId]
		if material := firstMaterialByGroup[materialGroupId]; material.MaterialId != 0 {
			awakenMaterialByCostume[row.CostumeId] = material
		}
	}

	added := 0
	for costumeId := range pool.CostumeById {
		entries := dupMap[costumeId]
		entryAdded := false

		// 1. Limit-break material (text / 311xxx)
		if matId := groupToMaterial[costumeLBGroup[costumeId]]; matId != 0 {
			var appended bool
			entries, appended = appendMissingDupExchangeEntry(entries, model.DupExchangeEntry{
				PossessionType: int32(model.PossessionTypeMaterial),
				PossessionId:   matId,
				Count:          dupExchangeFallbackCount,
			})
			entryAdded = entryAdded || appended
		}

		// 2. Awakening stone (stone / 313xxx)
		if material := awakenMaterialByCostume[costumeId]; material.MaterialId != 0 && material.Count > 0 {
			var appended bool
			entries, appended = appendMissingDupExchangeEntry(entries, model.DupExchangeEntry{
				PossessionType: int32(model.PossessionTypeMaterial),
				PossessionId:   material.MaterialId,
				Count:          material.Count,
			})
			entryAdded = entryAdded || appended
		}

		if !entryAdded {
			continue
		}
		dupMap[costumeId] = entries
		added++
	}
	return added, nil
}

func appendMissingDupExchangeEntry(entries []model.DupExchangeEntry, candidate model.DupExchangeEntry) ([]model.DupExchangeEntry, bool) {
	for _, entry := range entries {
		if entry.PossessionType == candidate.PossessionType && entry.PossessionId == candidate.PossessionId {
			return entries, false
		}
	}
	return append(entries, candidate), true
}

// EnrichCompanionDupExchange backfills duplication-exchange entries for any
// companion that has none configured, so a duplicate companion granted as a
// quest/mail reward is never silently swallowed. The fallback mirrors the
// costume backfill: 10x the companion's enhancement material (resolved via its
// category), i.e. the same shard the player spends to level that companion up.
func EnrichCompanionDupExchange(dupMap map[int32][]model.DupExchangeEntry) (int, error) {
	companionRows, err := utils.ReadTable[EntityMCompanion]("m_companion")
	if err != nil {
		return 0, err
	}
	matRows, err := utils.ReadTable[EntityMCompanionEnhancementMaterial]("m_companion_enhancement_material")
	if err != nil {
		return 0, err
	}

	// Resolve each category's representative enhancement material: the one used
	// at its lowest (base) level.
	catMaterial := make(map[int32]int32)
	catMinLevel := make(map[int32]int32)
	for _, r := range matRows {
		if r.MaterialId == 0 {
			continue
		}
		if cur, ok := catMinLevel[r.CompanionCategoryType]; !ok || r.Level < cur {
			catMinLevel[r.CompanionCategoryType] = r.Level
			catMaterial[r.CompanionCategoryType] = r.MaterialId
		}
	}

	added := 0
	for _, c := range companionRows {
		if _, exists := dupMap[c.CompanionId]; exists {
			continue
		}
		matId := catMaterial[c.CompanionCategoryType]
		if matId == 0 {
			continue
		}
		dupMap[c.CompanionId] = []model.DupExchangeEntry{{
			PossessionType: int32(model.PossessionTypeMaterial),
			PossessionId:   matId,
			Count:          dupExchangeFallbackCount,
		}}
		added++
	}
	return added, nil
}