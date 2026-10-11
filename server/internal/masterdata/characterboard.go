package masterdata

import (
	"fmt"

	"lunar-tear/server/internal/store"
	"lunar-tear/server/internal/utils"
)

type CharacterBoardAssignmentRow struct {
	CharacterId                  int32 `json:"CharacterId"`
	CharacterBoardCategoryId     int32 `json:"CharacterBoardCategoryId"`
	SortOrder                    int32 `json:"SortOrder"`
	CharacterBoardAssignmentType int32 `json:"CharacterBoardAssignmentType"`
}

type CharacterBoardGroupRow struct {
	CharacterBoardGroupId    int32 `json:"CharacterBoardGroupId"`
	CharacterBoardCategoryId int32 `json:"CharacterBoardCategoryId"`
	SortOrder                int32 `json:"SortOrder"`
	CharacterBoardGroupType  int32 `json:"CharacterBoardGroupType"`
	TextAssetId              int32 `json:"TextAssetId"`
}

type CharacterBoardCatalog struct {
	PanelById               map[int32]EntityMCharacterBoardPanel
	PanelsByBoardId         map[int32][]EntityMCharacterBoardPanel
	ReleaseCostsByGroupId   map[int32][]EntityMCharacterBoardPanelReleasePossessionGroup
	ReleaseEffectsByGroupId map[int32][]EntityMCharacterBoardPanelReleaseEffectGroup
	StatusUpById            map[int32]EntityMCharacterBoardStatusUp
	AbilityById             map[int32]EntityMCharacterBoardAbility
	AbilityMaxLevel         map[store.CharacterBoardAbilityKey]int32
	EffectTargetsByGroupId  map[int32][]EntityMCharacterBoardEffectTargetGroup
	BoardById               map[int32]EntityMCharacterBoard
	// MissionOptionGroupByBoardId maps a CharacterBoardId to the type-54
	// MissionClearConditionOptionGroupId used by "Unlock N ... Monument panels
	// for <character>" missions (480001-480126). BoardIdsByOptionGroup is the
	// reverse index used to recount released panels for one option group.
	MissionOptionGroupByBoardId map[int32]int32
	BoardIdsByOptionGroup       map[int32][]int32
}

func LoadCharacterBoardCatalog() (*CharacterBoardCatalog, error) {
	panels, err := utils.ReadTable[EntityMCharacterBoardPanel]("m_character_board_panel")
	if err != nil {
		return nil, fmt.Errorf("load character board panel table: %w", err)
	}

	costs, err := utils.ReadTable[EntityMCharacterBoardPanelReleasePossessionGroup]("m_character_board_panel_release_possession_group")
	if err != nil {
		return nil, fmt.Errorf("load character board release possession table: %w", err)
	}

	effects, err := utils.ReadTable[EntityMCharacterBoardPanelReleaseEffectGroup]("m_character_board_panel_release_effect_group")
	if err != nil {
		return nil, fmt.Errorf("load character board release effect table: %w", err)
	}

	boards, err := utils.ReadTable[EntityMCharacterBoard]("m_character_board")
	if err != nil {
		return nil, fmt.Errorf("load character board table: %w", err)
	}

	statusUps, err := utils.ReadTable[EntityMCharacterBoardStatusUp]("m_character_board_status_up")
	if err != nil {
		return nil, fmt.Errorf("load character board status up table: %w", err)
	}

	abilities, err := utils.ReadTable[EntityMCharacterBoardAbility]("m_character_board_ability")
	if err != nil {
		return nil, fmt.Errorf("load character board ability table: %w", err)
	}

	abilityMaxLevels, err := utils.ReadTable[EntityMCharacterBoardAbilityMaxLevel]("m_character_board_ability_max_level")
	if err != nil {
		return nil, fmt.Errorf("load character board ability max level table: %w", err)
	}

	targets, err := utils.ReadTable[EntityMCharacterBoardEffectTargetGroup]("m_character_board_effect_target_group")
	if err != nil {
		return nil, fmt.Errorf("load character board effect target table: %w", err)
	}

	groups, err := utils.ReadTable[EntityMCharacterBoardGroup]("m_character_board_group")
	if err != nil {
		return nil, fmt.Errorf("load character board group table: %w", err)
	}

	assignments, err := utils.ReadTable[EntityMCharacterBoardAssignment]("m_character_board_assignment")
	if err != nil {
		return nil, fmt.Errorf("load character board assignment table: %w", err)
	}

	catalog := &CharacterBoardCatalog{
		PanelById:                   make(map[int32]EntityMCharacterBoardPanel, len(panels)),
		PanelsByBoardId:             make(map[int32][]EntityMCharacterBoardPanel),
		ReleaseCostsByGroupId:       make(map[int32][]EntityMCharacterBoardPanelReleasePossessionGroup),
		ReleaseEffectsByGroupId:     make(map[int32][]EntityMCharacterBoardPanelReleaseEffectGroup),
		StatusUpById:                make(map[int32]EntityMCharacterBoardStatusUp, len(statusUps)),
		AbilityById:                 make(map[int32]EntityMCharacterBoardAbility, len(abilities)),
		AbilityMaxLevel:             make(map[store.CharacterBoardAbilityKey]int32, len(abilityMaxLevels)),
		EffectTargetsByGroupId:      make(map[int32][]EntityMCharacterBoardEffectTargetGroup),
		BoardById:                   make(map[int32]EntityMCharacterBoard, len(boards)),
		MissionOptionGroupByBoardId: make(map[int32]int32, len(boards)),
		BoardIdsByOptionGroup:       make(map[int32][]int32),
	}

	for _, p := range panels {
		catalog.PanelById[p.CharacterBoardPanelId] = p
		catalog.PanelsByBoardId[p.CharacterBoardId] = append(catalog.PanelsByBoardId[p.CharacterBoardId], p)
	}
	for _, c := range costs {
		catalog.ReleaseCostsByGroupId[c.CharacterBoardPanelReleasePossessionGroupId] = append(
			catalog.ReleaseCostsByGroupId[c.CharacterBoardPanelReleasePossessionGroupId], c)
	}
	for _, e := range effects {
		catalog.ReleaseEffectsByGroupId[e.CharacterBoardPanelReleaseEffectGroupId] = append(
			catalog.ReleaseEffectsByGroupId[e.CharacterBoardPanelReleaseEffectGroupId], e)
	}
	for _, b := range boards {
		catalog.BoardById[b.CharacterBoardId] = b
	}
	for _, s := range statusUps {
		catalog.StatusUpById[s.CharacterBoardStatusUpId] = s
	}
	for _, a := range abilities {
		catalog.AbilityById[a.CharacterBoardAbilityId] = a
	}
	for _, m := range abilityMaxLevels {
		catalog.AbilityMaxLevel[store.CharacterBoardAbilityKey{
			CharacterId: m.CharacterId,
			AbilityId:   m.AbilityId,
		}] = m.MaxLevel
	}
	for _, t := range targets {
		catalog.EffectTargetsByGroupId[t.CharacterBoardEffectTargetGroupId] = append(
			catalog.EffectTargetsByGroupId[t.CharacterBoardEffectTargetGroupId], t)
	}

	// Resolve each board to its type-54 mission option group:
	// board -> group (GroupType 1 = Stone Tower Monument, 2 = Cursed God
	// Monument) -> category -> assigned character -> hardcoded option group.
	characterByCategoryId := make(map[int32]int32, len(assignments))
	for _, a := range assignments {
		characterByCategoryId[a.CharacterBoardCategoryId] = a.CharacterId
	}
	groupById := make(map[int32]EntityMCharacterBoardGroup, len(groups))
	for _, g := range groups {
		groupById[g.CharacterBoardGroupId] = g
	}
	for _, b := range boards {
		group, ok := groupById[b.CharacterBoardGroupId]
		if !ok {
			continue
		}
		characterId := characterByCategoryId[group.CharacterBoardCategoryId]
		optionGroup, ok := boardMissionOptionGroups[boardMissionKey{
			CharacterId: characterId,
			GroupType:   group.CharacterBoardGroupType,
		}]
		if !ok {
			continue
		}
		catalog.MissionOptionGroupByBoardId[b.CharacterBoardId] = optionGroup
		catalog.BoardIdsByOptionGroup[optionGroup] = append(catalog.BoardIdsByOptionGroup[optionGroup], b.CharacterBoardId)
	}

	return catalog, nil
}

type boardMissionKey struct {
	CharacterId int32
	GroupType   int32 // 1 = Stone Tower Monument, 2 = Cursed God Monument
}

// boardMissionOptionGroups is the hardcoded CharacterId+GroupType →
// MissionClearConditionOptionGroupId mapping (310001-310042) taken from the
// type-54 mission data (480001-480126). Like the exalt missions (60001-60033),
// the order is fixed by mission id and cannot be derived from master data.
var boardMissionOptionGroups = map[boardMissionKey]int32{
	{1008, 1}: 310001, {1008, 2}: 310002, // Rion
	{1009, 1}: 310003, {1009, 2}: 310004, // Gayle
	{1007, 1}: 310005, {1007, 2}: 310006, // Dimos
	{1015, 1}: 310007, {1015, 2}: 310008, // Akeha
	{1006, 1}: 310009, {1006, 2}: 310010, // Argo
	{1011, 1}: 310011, {1011, 2}: 310012, // 063y
	{1012, 1}: 310013, {1012, 2}: 310014, // F66x
	{1013, 1}: 310015, {1013, 2}: 310016, // Lars
	{1014, 1}: 310017, {1014, 2}: 310018, // Griff
	{1010, 1}: 310019, {1010, 2}: 310020, // Noelle
	{1004, 1}: 310021, {1004, 2}: 310022, // Levania
	{1019, 1}: 310023, {1019, 2}: 310024, // Fio
	{1022, 1}: 310025, {1022, 2}: 310026, // Saryu
	{1023, 1}: 310027, {1023, 2}: 310028, // Priyet
	{1024, 1}: 310029, {1024, 2}: 310030, // Marie
	{1025, 1}: 310031, {1025, 2}: 310032, // Yurie
	{1027, 1}: 310033, {1027, 2}: 310034, // Yudil
	{1026, 1}: 310035, {1026, 2}: 310036, // Sarafa
	{1020, 1}: 310037, {1020, 2}: 310038, // Hina
	{1021, 1}: 310039, {1021, 2}: 310040, // Yuzuki
	{1048, 1}: 310041, {1048, 2}: 310042, // 10H
}
