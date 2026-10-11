package masterdata

import (
	"fmt"

	"lunar-tear/server/internal/utils"
)

type StepKey struct {
	GroupId            int32
	BeforeRebirthCount int32
}

type CharacterRebirthCatalog struct {
	StepGroupByCharacterId map[int32]int32
	StepByGroupAndCount    map[StepKey]EntityMCharacterRebirthStepGroup
	MaterialsByGroupId     map[int32][]EntityMCharacterRebirthMaterialGroup

	// MissionOptionGroupByCharacterId maps each rebirthable character to the
	// MissionClearConditionOptionGroupId used by type-71 "Exalt <character>
	// N times" missions. Master data does not carry this mapping as a field;
	// instead the client assigns 60000 + rank_of_character_in_m_character_rebirth
	// (ascending by CharacterId). All 33 known type-71 option groups (60001
	// through 60033) match this rule 1:1 with the m_character_rebirth rows.
	MissionOptionGroupByCharacterId map[int32]int32
}

func (c *CharacterRebirthCatalog) CostumeLevelLimitUp(characterId, rebirthCount int32) int32 {
	if c == nil || rebirthCount <= 0 {
		return 0
	}
	stepGroupId, ok := c.StepGroupByCharacterId[characterId]
	if !ok {
		return 0
	}
	var total int32
	for i := range rebirthCount {
		step, ok := c.StepByGroupAndCount[StepKey{GroupId: stepGroupId, BeforeRebirthCount: i}]
		if !ok {
			continue
		}
		total += step.CostumeLevelLimitUp
	}
	return total
}

func LoadCharacterRebirthCatalog() (*CharacterRebirthCatalog, error) {
	rebirthRows, err := utils.ReadTable[EntityMCharacterRebirth]("m_character_rebirth")
	if err != nil {
		return nil, fmt.Errorf("load character rebirth table: %w", err)
	}

	stepRows, err := utils.ReadTable[EntityMCharacterRebirthStepGroup]("m_character_rebirth_step_group")
	if err != nil {
		return nil, fmt.Errorf("load character rebirth step group table: %w", err)
	}

	materialRows, err := utils.ReadTable[EntityMCharacterRebirthMaterialGroup]("m_character_rebirth_material_group")
	if err != nil {
		return nil, fmt.Errorf("load character rebirth material group table: %w", err)
	}

	stepGroupByCharacterId := make(map[int32]int32, len(rebirthRows))
	for _, r := range rebirthRows {
		stepGroupByCharacterId[r.CharacterId] = r.CharacterRebirthStepGroupId
	}

	stepByGroupAndCount := make(map[StepKey]EntityMCharacterRebirthStepGroup, len(stepRows))
	for _, s := range stepRows {
		stepByGroupAndCount[StepKey{GroupId: s.CharacterRebirthStepGroupId, BeforeRebirthCount: s.BeforeRebirthCount}] = s
	}

	materialsByGroupId := make(map[int32][]EntityMCharacterRebirthMaterialGroup)
	for _, m := range materialRows {
		materialsByGroupId[m.CharacterRebirthMaterialGroupId] = append(materialsByGroupId[m.CharacterRebirthMaterialGroupId], m)
	}

	missionOptionGroupByCharacterId := make(map[int32]int32, len(rebirthRows))
	// Hardcoded mapping CharacterId → MissionClearConditionOptionGroupId (60001–60033).
	// Derived directly from exalt mission data (MCCOGI field). The order does NOT
	// follow CharacterId or SortOrder consistently across all characters.
	hardcodedMapping := map[int32]int32{
		1001:   60001, // 2B
		1002:   60002, // 9S
		1003:   60003, // A2
		1004:   60004, // Levania
		1006:   60005, // Argo
		1007:   60006, // Dimos
		1008:   60007, // Rion
		1009:   60008, // Gayle
		1010:   60009, // Noelle
		1011:   60010, // 063y
		1012:   60011, // F66x
		1013:   60012, // Lars
		1014:   60013, // Griff
		1015:   60014, // Akeha
		1016:   60015, // The World-Ender
		1018:   60016, // Kainé
		1017:   60017, // Emil
		1019:   60018, // Fio
		1022:   60019, // Saryu
		1023:   60020, // Priyet
		1024:   60021, // Marie
		1025:   60022, // Yurie
		1027:   60023, // Yudil
		1026:   60024, // Sarafa
		1020:   60025, // Hina
		1021:   60026, // Yuzuki
		1031:   60027, // Zero
		1044:   60028, // Joker
		1045:   60029, // Queen
		1046:   60030, // Fox
		1047:   60031, // Crow
		100001: 60032, // 2P
		1048:   60033, // 10H
	}
	for _, r := range rebirthRows {
		if optId, ok := hardcodedMapping[r.CharacterId]; ok {
			missionOptionGroupByCharacterId[r.CharacterId] = optId
		}
	}

	return &CharacterRebirthCatalog{
		StepGroupByCharacterId:          stepGroupByCharacterId,
		StepByGroupAndCount:             stepByGroupAndCount,
		MaterialsByGroupId:              materialsByGroupId,
		MissionOptionGroupByCharacterId: missionOptionGroupByCharacterId,
	}, nil
}
