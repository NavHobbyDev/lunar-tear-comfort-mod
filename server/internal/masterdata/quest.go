package masterdata

import (
	"fmt"
	"log"
	"sort"

	"lunar-tear/server/internal/utils"
)

type BattleDropInfo struct {
	QuestSceneId         int32
	BattleDropCategoryId int32
}

// NpcDeckKey is a composite key for looking up battle NPC decks by the combination
// of BattleNpcId, DeckType, and BattleNpcDeckNumber. This is needed because the same
// BattleNpcDeckNumber can be used by different BattleNpcIds with different DeckTypes.
type NpcDeckKey struct {
	BattleNpcId         int64
	DeckType            int32
	BattleNpcDeckNumber int32
}

// SceneChoiceKey is a composite key for looking up quest scene choices by
// the combination of QuestSceneId, QuestFlowType, and ChoiceNumber.
type SceneChoiceKey struct {
	QuestSceneId  int32
	QuestFlowType int32
	ChoiceNumber  int32
}

type WeaponEvolutionInfo struct {
	GroupId int32
	Order   int32
}

type QuestCatalog struct {
	SceneById                          map[int32]EntityMQuestScene
	MissionById                        map[int32]EntityMQuestMission
	QuestById                          map[int32]EntityMQuest
	QuestReleaseConditionsByListId     map[int32]QuestReleaseConditionGroup
	MissionIdsByQuestId                map[int32][]int32
	RouteIdByQuestId                   map[int32]int32
	SceneIdsByQuestId                  map[int32][]int32
	OrderedQuestIds                    []int32
	FirstClearRewardsByGroupId         map[int32][]EntityMQuestFirstClearRewardGroup
	FirstClearRewardSwitchesByQuestId  map[int32][]EntityMQuestFirstClearRewardSwitch
	MissionRewardsByMissionId          map[int32][]EntityMQuestMissionReward
	WeaponIdsByReleaseConditionGroupId map[int32][]int32
	ReleaseConditionsByGroupId         map[int32][]EntityMWeaponStoryReleaseConditionGroup
	SceneGrantsBySceneId               map[int32][]EntityMUserQuestSceneGrantPossession
	BattleDropRewardById               map[int32]EntityMBattleDropReward
	PickupRewardIdsByGroupId           map[int32][]int32
	BattleDropsByQuestId               map[int32][]BattleDropInfo
	ReplayFlowRewardsByGroupId         map[int32][]EntityMQuestReplayFlowRewardGroup
	RentalQuestIds                     map[int32]bool
	TutorialUnlockConditions           []EntityMTutorialUnlockCondition
	ChapterLastSceneByQuestId          map[int32]int32
	SeasonIdByRouteId                  map[int32]int32
	RoutesBySeason                     map[int32][]int32
	RouteCompletionQuestId             map[int32]int32
	BattleOnlyTargetSceneByQuestId     map[int32]int32
	MainQuestChapterIdByQuestId        map[int32]int32
	MainQuestRouteIdByChapterId        map[int32]int32
	EventQuestTypeByChapterId          map[int32]int32
	EventQuestIdsByChapterId           map[int32][]int32
	EventQuestIdsByChapterDifficulty   map[int32]map[int32][]int32 // chapterId -> difficulty -> questIds
	LimitContentQuestIds               map[int32]bool
	// DeckRestrictionsByGroupId maps QuestDeckRestrictionGroupId -> slot restrictions.
	DeckRestrictionsByGroupId map[int32][]EntityMQuestDeckRestrictionGroup
	// CostumeProperAttributeByCostumeId maps CostumeId -> CostumeProperAttributeType (affinity).
	CostumeProperAttributeByCostumeId map[int32]int32
	// DarkMemoryQuestIds contains all quest IDs in the Dark Memory master-data
	// namespace. This series contains every character's quest stages and their
	// difficulty subgroups.
	DarkMemoryQuestIds map[int32]bool
	// ChapterMemoirsByQuestId maps an event quest id to the memoir (Parts) ids
	// advertised by its chapter's display item group. Event chapters advertise a
	// set of memoirs (one per series) but the per-quest drop data only wires one,
	// leaving the rest unobtainable; these are granted on finish instead.
	ChapterMemoirsByQuestId map[int32][]int32
	// QuestMissionConditionValueGroupsByGroupId maps quest mission condition group IDs to character IDs
	// Used for Type 61 missions (character group-based requirements)
	QuestMissionConditionValueGroupsByGroupId map[int32][]int32

	// SkillBehaviourActionRecoveryIds contains all SkillBehaviourActionIds that are recovery actions
	// Used for Type 57 missions (don't use recovery skills)
	SkillBehaviourActionRecoveryIds map[int32]bool

	// CostumeSkillDetailIds contains all SkillDetailIds that belong to costume active skills.
	// Used to count costume skill usages from FinishWave SkillUseInfo when the client
	// does not populate BattleDetail.playerCostumeActiveSkillUsedCount (Type 6/54 missions).
	CostumeSkillDetailIds map[int32]bool

	// SceneChoiceByKey maps (QuestSceneId, QuestFlowType, ChoiceNumber) -> EntityMQuestSceneChoice
	// Used to resolve the client's choice request to the choice effect
	SceneChoiceByKey map[SceneChoiceKey]EntityMQuestSceneChoice
	// SceneChoiceEffectById maps QuestSceneChoiceEffectId -> EntityMQuestSceneChoiceEffect
	// Used to get groupingId and other effect data
	SceneChoiceEffectById map[int32]EntityMQuestSceneChoiceEffect

	// Battle-related mappings for boss detection
	BattleGroupBySceneId              map[int32]int32
	BattleIdsByGroupId                map[int32][]int32
	BattleByIdMap                     map[int32]EntityMBattle
	BattleNpcDeckByNumber             map[int32]EntityMBattleNpcDeck
	BattleNpcDeckByKey                map[NpcDeckKey]EntityMBattleNpcDeck
	BattleNpcDeckCharacterTypeByNpcId map[int64][]EntityMBattleNpcDeckCharacterType

	UserExpThresholds       []int32
	CharacterExpThresholds  []int32
	CostumeExpByRarity      map[int32][]int32
	CostumeMaxLevelByRarity map[int32]NumericalFunc
	MaxStaminaByLevel       map[int32]int32

	CostumeById           map[int32]EntityMCostume
	CompanionEnhancedById map[int32]EntityMCompanionEnhanced
	WeaponById            map[int32]EntityMWeapon

	// WeaponAttributeById maps WeaponId to its AttributeType (element).
	WeaponAttributeById map[int32]int32

	WeaponSkillSlots   map[int32][]int32
	WeaponAbilitySlots map[int32][]int32

	// Resonant / quest-bonus tables (event medal bonuses from equipped weapons).
	QuestBonusById                map[int32]EntityMQuestBonus
	QuestBonusWeaponRowsByGroupId map[int32][]EntityMQuestBonusWeaponGroup
	QuestBonusEffectsByGroupId    map[int32][]EntityMQuestBonusEffectGroup
	QuestBonusDropByEffectId      map[int32]EntityMQuestBonusDropReward
	QuestBonusTermsByGroupId      map[int32][]EntityMQuestBonusTermGroup
	WeaponEvolutionByWeaponId     map[int32]WeaponEvolutionInfo

	*PartsCatalog
}


func buildEventQuestIndexes(
	chapters []EntityMEventQuestChapter,
	groups []EntityMEventQuestSequenceGroup,
	sequences []EntityMEventQuestSequence,
) (map[int32][]int32, map[int32]map[int32][]int32, map[int32]map[int32][]int32) {
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].EventQuestSequenceGroupId != groups[j].EventQuestSequenceGroupId {
			return groups[i].EventQuestSequenceGroupId < groups[j].EventQuestSequenceGroupId
		}
		if groups[i].DifficultyType != groups[j].DifficultyType {
			return groups[i].DifficultyType < groups[j].DifficultyType
		}
		return groups[i].EventQuestSequenceId < groups[j].EventQuestSequenceId
	})
	sort.Slice(sequences, func(i, j int) bool {
		if sequences[i].EventQuestSequenceId != sequences[j].EventQuestSequenceId {
			return sequences[i].EventQuestSequenceId < sequences[j].EventQuestSequenceId
		}
		if sequences[i].SortOrder != sequences[j].SortOrder {
			return sequences[i].SortOrder < sequences[j].SortOrder
		}
		return sequences[i].QuestId < sequences[j].QuestId
	})

	type sequenceRef struct {
		id         int32
		difficulty int32
	}
	sequencesByGroup := make(map[int32][]sequenceRef)
	for _, row := range groups {
		sequencesByGroup[row.EventQuestSequenceGroupId] = append(sequencesByGroup[row.EventQuestSequenceGroupId], sequenceRef{
			id: row.EventQuestSequenceId, difficulty: row.DifficultyType,
		})
	}
	questRowsBySequence := make(map[int32][]EntityMEventQuestSequence)
	for _, row := range sequences {
		questRowsBySequence[row.EventQuestSequenceId] = append(questRowsBySequence[row.EventQuestSequenceId], row)
	}

	questIdsByChapter := make(map[int32][]int32)
	questIdsByChapterSortOrder := make(map[int32]map[int32][]int32)
	questIdsByChapterDifficulty := make(map[int32]map[int32][]int32)
	for _, chapter := range chapters {
		seen := make(map[int32]bool)
		seenBySortOrder := make(map[int32]map[int32]bool)
		bySortOrder := make(map[int32][]int32)
		seenByDifficulty := make(map[int32]map[int32]bool)
		byDifficulty := make(map[int32][]int32)
		for _, sequence := range sequencesByGroup[chapter.EventQuestSequenceGroupId] {
			for _, row := range questRowsBySequence[sequence.id] {
				if !seen[row.QuestId] {
					seen[row.QuestId] = true
					questIdsByChapter[chapter.EventQuestChapterId] = append(questIdsByChapter[chapter.EventQuestChapterId], row.QuestId)
				}
				if seenByDifficulty[sequence.difficulty] == nil {
					seenByDifficulty[sequence.difficulty] = make(map[int32]bool)
				}
				if !seenByDifficulty[sequence.difficulty][row.QuestId] {
					seenByDifficulty[sequence.difficulty][row.QuestId] = true
					byDifficulty[sequence.difficulty] = append(byDifficulty[sequence.difficulty], row.QuestId)
				}
				if seenBySortOrder[row.SortOrder] == nil {
					seenBySortOrder[row.SortOrder] = make(map[int32]bool)
				}
				if !seenBySortOrder[row.SortOrder][row.QuestId] {
					seenBySortOrder[row.SortOrder][row.QuestId] = true
					bySortOrder[row.SortOrder] = append(bySortOrder[row.SortOrder], row.QuestId)
				}
			}
		}
		if len(byDifficulty) > 0 {
			questIdsByChapterDifficulty[chapter.EventQuestChapterId] = byDifficulty
		}
		if len(bySortOrder) > 0 {
			questIdsByChapterSortOrder[chapter.EventQuestChapterId] = bySortOrder
		}
	}
	return questIdsByChapter, questIdsByChapterSortOrder, questIdsByChapterDifficulty
}

func LoadQuestCatalog(partsCatalog *PartsCatalog) (*QuestCatalog, error) {
	scenes, err := utils.ReadTable[EntityMQuestScene]("m_quest_scene")
	if err != nil {
		return nil, fmt.Errorf("load quest scene table: %w", err)
	}
	sort.Slice(scenes, func(i, j int) bool {
		if scenes[i].QuestId != scenes[j].QuestId {
			return scenes[i].QuestId < scenes[j].QuestId
		}
		if scenes[i].SortOrder != scenes[j].SortOrder {
			return scenes[i].SortOrder < scenes[j].SortOrder
		}
		return scenes[i].QuestSceneId < scenes[j].QuestSceneId
	})

	missions, err := utils.ReadTable[EntityMQuestMission]("m_quest_mission")
	if err != nil {
		return nil, fmt.Errorf("load quest mission table: %w", err)
	}

	missionConditionValueGroups, err := utils.ReadTable[EntityMQuestMissionConditionValueGroup]("m_quest_mission_condition_value_group")
	if err != nil {
		return nil, fmt.Errorf("load quest mission condition value group table: %w", err)
	}

	quests, err := utils.ReadTable[EntityMQuest]("m_quest")
	if err != nil {
		return nil, fmt.Errorf("load quest table: %w", err)
	}

	questReleaseConditionsByListId, err := loadQuestReleaseConditions()
	if err != nil {
		return nil, err
	}

	missionGroups, err := utils.ReadTable[EntityMQuestMissionGroup]("m_quest_mission_group")
	if err != nil {
		return nil, fmt.Errorf("load quest mission group table: %w", err)
	}
	sort.Slice(missionGroups, func(i, j int) bool {
		if missionGroups[i].QuestMissionGroupId != missionGroups[j].QuestMissionGroupId {
			return missionGroups[i].QuestMissionGroupId < missionGroups[j].QuestMissionGroupId
		}
		if missionGroups[i].SortOrder != missionGroups[j].SortOrder {
			return missionGroups[i].SortOrder < missionGroups[j].SortOrder
		}
		return missionGroups[i].QuestMissionId < missionGroups[j].QuestMissionId
	})

	sequences, err := utils.ReadTable[EntityMMainQuestSequence]("m_main_quest_sequence")
	if err != nil {
		return nil, fmt.Errorf("load main quest sequence table: %w", err)
	}
	sort.Slice(sequences, func(i, j int) bool {
		if sequences[i].MainQuestSequenceId != sequences[j].MainQuestSequenceId {
			return sequences[i].MainQuestSequenceId < sequences[j].MainQuestSequenceId
		}
		if sequences[i].SortOrder != sequences[j].SortOrder {
			return sequences[i].SortOrder < sequences[j].SortOrder
		}
		return sequences[i].QuestId < sequences[j].QuestId
	})

	chapters, err := utils.ReadTable[EntityMMainQuestChapter]("m_main_quest_chapter")
	if err != nil {
		return nil, fmt.Errorf("load main quest chapter table: %w", err)
	}

	routes, err := utils.ReadTable[EntityMMainQuestRoute]("m_main_quest_route")
	if err != nil {
		return nil, fmt.Errorf("load main quest route table: %w", err)
	}
	seasonIdByRouteId := make(map[int32]int32, len(routes))
	routesBySeason := make(map[int32][]int32, len(routes))
	sortOrderByRoute := make(map[int32]int32, len(routes))
	for _, r := range routes {
		seasonIdByRouteId[r.MainQuestRouteId] = r.MainQuestSeasonId
		routesBySeason[r.MainQuestSeasonId] = append(routesBySeason[r.MainQuestSeasonId], r.MainQuestRouteId)
		sortOrderByRoute[r.MainQuestRouteId] = r.SortOrder
	}
	for seasonId, ids := range routesBySeason {
		s := ids
		sort.Slice(s, func(i, j int) bool { return sortOrderByRoute[s[i]] > sortOrderByRoute[s[j]] })
		routesBySeason[seasonId] = s
	}

	anotherReplayConds, err := utils.ReadTable[EntityMMainQuestRouteAnotherReplayFlowUnlockCondition]("m_main_quest_route_another_replay_flow_unlock_condition")
	if err != nil {
		return nil, fmt.Errorf("load main quest route another replay flow unlock condition table: %w", err)
	}
	evaluateConds, err := utils.ReadTable[EntityMEvaluateCondition]("m_evaluate_condition")
	if err != nil {
		return nil, fmt.Errorf("load evaluate condition table: %w", err)
	}
	valueGroupByConditionId := make(map[int32]int32, len(evaluateConds))
	for _, c := range evaluateConds {
		valueGroupByConditionId[c.EvaluateConditionId] = c.EvaluateConditionValueGroupId
	}
	evaluateValueGroups, err := utils.ReadTable[EntityMEvaluateConditionValueGroup]("m_evaluate_condition_value_group")
	if err != nil {
		return nil, fmt.Errorf("load evaluate condition value group table: %w", err)
	}
	valueByGroupId := make(map[int32]int32, len(evaluateValueGroups))
	for _, vg := range evaluateValueGroups {
		if _, exists := valueByGroupId[vg.EvaluateConditionValueGroupId]; exists {
			continue
		}
		valueByGroupId[vg.EvaluateConditionValueGroupId] = int32(vg.Value)
	}
	routeCompletionQuestId := make(map[int32]int32, len(anotherReplayConds))
	for _, c := range anotherReplayConds {
		valueGroupId, ok := valueGroupByConditionId[c.UnlockEvaluateConditionId]
		if !ok {
			continue
		}
		questId, ok := valueByGroupId[valueGroupId]
		if !ok {
			continue
		}
		routeCompletionQuestId[c.MainQuestRouteId] = questId
	}

	firstClearSwitches, err := utils.ReadTable[EntityMQuestFirstClearRewardSwitch]("m_quest_first_clear_reward_switch")
	if err != nil {
		return nil, fmt.Errorf("load quest first clear reward switch table: %w", err)
	}

	firstClearRewards, err := utils.ReadTable[EntityMQuestFirstClearRewardGroup]("m_quest_first_clear_reward_group")
	if err != nil {
		return nil, fmt.Errorf("load quest first clear reward group table: %w", err)
	}
	sort.Slice(firstClearRewards, func(i, j int) bool {
		if firstClearRewards[i].QuestFirstClearRewardGroupId != firstClearRewards[j].QuestFirstClearRewardGroupId {
			return firstClearRewards[i].QuestFirstClearRewardGroupId < firstClearRewards[j].QuestFirstClearRewardGroupId
		}
		if firstClearRewards[i].SortOrder != firstClearRewards[j].SortOrder {
			return firstClearRewards[i].SortOrder < firstClearRewards[j].SortOrder
		}
		return firstClearRewards[i].QuestFirstClearRewardType < firstClearRewards[j].QuestFirstClearRewardType
	})

	replayFlowRewards, err := utils.ReadTable[EntityMQuestReplayFlowRewardGroup]("m_quest_replay_flow_reward_group")
	if err != nil {
		return nil, fmt.Errorf("load quest replay flow reward group table: %w", err)
	}
	sort.Slice(replayFlowRewards, func(i, j int) bool {
		if replayFlowRewards[i].QuestReplayFlowRewardGroupId != replayFlowRewards[j].QuestReplayFlowRewardGroupId {
			return replayFlowRewards[i].QuestReplayFlowRewardGroupId < replayFlowRewards[j].QuestReplayFlowRewardGroupId
		}
		return replayFlowRewards[i].SortOrder < replayFlowRewards[j].SortOrder
	})

	missionRewards, err := utils.ReadTable[EntityMQuestMissionReward]("m_quest_mission_reward")
	if err != nil {
		return nil, fmt.Errorf("load quest mission reward table: %w", err)
	}

	weapons, err := utils.ReadTable[EntityMWeapon]("m_weapon")
	if err != nil {
		return nil, fmt.Errorf("load weapon table: %w", err)
	}

	weaponSkillGroups, err := utils.ReadTable[EntityMWeaponSkillGroup]("m_weapon_skill_group")
	if err != nil {
		return nil, fmt.Errorf("load weapon skill group table: %w", err)
	}

	skillBehaviourActionRecoveries, err := utils.ReadTable[EntityMSkillBehaviourActionRecovery]("m_skill_behaviour_action_recovery")
	if err != nil {
		return nil, fmt.Errorf("load skill behaviour action recovery table: %w", err)
	}

	costumeActiveSkillGroups, err := utils.ReadTable[EntityMCostumeActiveSkillGroup]("m_costume_active_skill_group")
	if err != nil {
		return nil, fmt.Errorf("load costume active skill group table: %w", err)
	}

	skills, err := utils.ReadTable[EntityMSkill]("m_skill")
	if err != nil {
		return nil, fmt.Errorf("load skill table: %w", err)
	}

	skillLevelGroups, err := utils.ReadTable[EntityMSkillLevelGroup]("m_skill_level_group")
	if err != nil {
		return nil, fmt.Errorf("load skill level group table: %w", err)
	}

	weaponAbilityGroups, err := utils.ReadTable[EntityMWeaponAbilityGroup]("m_weapon_ability_group")
	if err != nil {
		return nil, fmt.Errorf("load weapon ability group table: %w", err)
	}

	releaseConditions, err := utils.ReadTable[EntityMWeaponStoryReleaseConditionGroup]("m_weapon_story_release_condition_group")
	if err != nil {
		return nil, fmt.Errorf("load weapon story release condition table: %w", err)
	}

	costumeMasters, err := utils.ReadTable[EntityMCostume]("m_costume")
	if err != nil {
		return nil, fmt.Errorf("load costume table: %w", err)
	}

	companionEnhancedRows, err := utils.ReadTable[EntityMCompanionEnhanced]("m_companion_enhanced")
	if err != nil {
		return nil, fmt.Errorf("load enhanced companion table: %w", err)
	}

	costumeRarities, err := utils.ReadTable[EntityMCostumeRarity]("m_costume_rarity")
	if err != nil {
		return nil, fmt.Errorf("load costume rarity table: %w", err)
	}

	sceneGrants, err := utils.ReadTable[EntityMUserQuestSceneGrantPossession]("m_user_quest_scene_grant_possession")
	if err != nil {
		return nil, fmt.Errorf("load quest scene grant table: %w", err)
	}

	sceneChoices, err := utils.ReadTable[EntityMQuestSceneChoice]("m_quest_scene_choice")
	if err != nil {
		return nil, fmt.Errorf("load quest scene choice table: %w", err)
	}

	sceneChoiceEffects, err := utils.ReadTable[EntityMQuestSceneChoiceEffect]("m_quest_scene_choice_effect")
	if err != nil {
		return nil, fmt.Errorf("load quest scene choice effect table: %w", err)
	}

	battleDropRewards, err := utils.ReadTable[EntityMBattleDropReward]("m_battle_drop_reward")
	if err != nil {
		return nil, fmt.Errorf("load battle drop reward table: %w", err)
	}

	pickupRewardGroups, err := utils.ReadTable[EntityMQuestPickupRewardGroup]("m_quest_pickup_reward_group")
	if err != nil {
		return nil, fmt.Errorf("load quest pickup reward group table: %w", err)
	}
	sort.Slice(pickupRewardGroups, func(i, j int) bool {
		if pickupRewardGroups[i].QuestPickupRewardGroupId != pickupRewardGroups[j].QuestPickupRewardGroupId {
			return pickupRewardGroups[i].QuestPickupRewardGroupId < pickupRewardGroups[j].QuestPickupRewardGroupId
		}
		return pickupRewardGroups[i].SortOrder < pickupRewardGroups[j].SortOrder
	})

	sceneBattles, err := utils.ReadTable[EntityMQuestSceneBattle]("m_quest_scene_battle")
	if err != nil {
		return nil, fmt.Errorf("load quest scene battle table: %w", err)
	}

	battleGroups, err := utils.ReadTable[EntityMBattleGroup]("m_battle_group")
	if err != nil {
		return nil, fmt.Errorf("load battle group table: %w", err)
	}

	battles, err := utils.ReadTable[EntityMBattle]("m_battle")
	if err != nil {
		return nil, fmt.Errorf("load battle table: %w", err)
	}

	npcDecks, err := utils.ReadTable[EntityMBattleNpcDeck]("m_battle_npc_deck")
	if err != nil {
		return nil, fmt.Errorf("load battle npc deck table: %w", err)
	}

	npcDropCategories, err := utils.ReadTable[EntityMBattleNpcDeckCharacterDropCategory]("m_battle_npc_deck_character_drop_category")
	if err != nil {
		return nil, fmt.Errorf("load battle npc drop category table: %w", err)
	}

	npcCharacterTypes, err := utils.ReadTable[EntityMBattleNpcDeckCharacterType]("m_battle_npc_deck_character_type")
	if err != nil {
		return nil, fmt.Errorf("load battle npc character type table: %w", err)
	}

	battleNpcDecks, err := utils.ReadTable[EntityMBattleNpcDeck]("m_battle_npc_deck")
	if err != nil {
		return nil, fmt.Errorf("load battle npc deck table: %w", err)
	}

	rentalDecks, err := utils.ReadTable[EntityMBattleRentalDeck]("m_battle_rental_deck")
	if err != nil {
		return nil, fmt.Errorf("load battle rental deck table: %w", err)
	}

	tutorialUnlockConds, err := utils.ReadTable[EntityMTutorialUnlockCondition]("m_tutorial_unlock_condition")
	if err != nil {
		return nil, fmt.Errorf("load tutorial unlock condition table: %w", err)
	}

	battleOnlyTargetSceneByQuestId := make(map[int32]int32)
	for _, scene := range scenes {
		if scene.IsBattleOnlyTarget {
			if _, exists := battleOnlyTargetSceneByQuestId[scene.QuestId]; !exists {
				battleOnlyTargetSceneByQuestId[scene.QuestId] = scene.QuestSceneId
			}
		}
	}

	paramMapRows, err := LoadParameterMap()
	if err != nil {
		return nil, err
	}

	userLevels, err := utils.ReadTable[EntityMUserLevel]("m_user_level")
	if err != nil {
		return nil, fmt.Errorf("load user level table: %w", err)
	}
	maxStaminaByLevel := make(map[int32]int32, len(userLevels))
	for _, ul := range userLevels {
		maxStaminaByLevel[ul.UserLevel] = ul.MaxStamina
	}

	funcResolver, err := LoadFunctionResolver()
	if err != nil {
		return nil, fmt.Errorf("load function resolver: %w", err)
	}

	costumeExpByRarity := make(map[int32][]int32, len(costumeRarities))
	costumeMaxLevelByRarity := make(map[int32]NumericalFunc, len(costumeRarities))
	for _, r := range costumeRarities {
		if _, ok := costumeExpByRarity[r.RarityType]; !ok {
			costumeExpByRarity[r.RarityType] = BuildExpThresholds(paramMapRows, r.RequiredExpForLevelUpNumericalParameterMapId)
		}
		if _, ok := costumeMaxLevelByRarity[r.RarityType]; !ok {
			if f, found := funcResolver.Resolve(r.MaxLevelNumericalFunctionId); found {
				costumeMaxLevelByRarity[r.RarityType] = f
			}
		}
	}

	costumeById := make(map[int32]EntityMCostume, len(costumeMasters))
	for _, cm := range costumeMasters {
		costumeById[cm.CostumeId] = cm
	}

	companionEnhancedById := make(map[int32]EntityMCompanionEnhanced, len(companionEnhancedRows))
	for _, enhanced := range companionEnhancedRows {
		companionEnhancedById[enhanced.CompanionEnhancedId] = enhanced
	}

	weaponById := make(map[int32]EntityMWeapon, len(weapons))
	for _, w := range weapons {
		weaponById[w.WeaponId] = w
	}

	weaponAttributeById := make(map[int32]int32, len(weapons))
	for _, w := range weapons {
		weaponAttributeById[w.WeaponId] = w.AttributeType
	}

	skillSlots := make(map[int32][]int32)
	for _, row := range weaponSkillGroups {
		skillSlots[row.WeaponSkillGroupId] = append(skillSlots[row.WeaponSkillGroupId], row.SlotNumber)
	}
	abilitySlots := make(map[int32][]int32)
	for _, row := range weaponAbilityGroups {
		abilitySlots[row.WeaponAbilityGroupId] = append(abilitySlots[row.WeaponAbilityGroupId], row.SlotNumber)
	}

	sceneById := make(map[int32]EntityMQuestScene, len(scenes))
	sceneIdsByQuestId := make(map[int32][]int32)
	for _, scene := range scenes {
		sceneById[scene.QuestSceneId] = scene
		sceneIdsByQuestId[scene.QuestId] = append(sceneIdsByQuestId[scene.QuestId], scene.QuestSceneId)
	}

	missionById := make(map[int32]EntityMQuestMission, len(missions))
	for _, mission := range missions {
		missionById[mission.QuestMissionId] = mission
	}

	questById := make(map[int32]EntityMQuest, len(quests))
	darkMemoryQuestIds := make(map[int32]bool)
	for _, quest := range quests {
		questById[quest.QuestId] = quest
		// 110xxx is the dedicated Dark Memory quest namespace in this master
		// data. It covers the full series rather than individual quest IDs.
		if quest.QuestId >= 110000 && quest.QuestId < 111000 {
			darkMemoryQuestIds[quest.QuestId] = true
		}
	}

	missionIdsByGroupId := make(map[int32][]int32, len(missionGroups))
	for _, mg := range missionGroups {
		missionIdsByGroupId[mg.QuestMissionGroupId] = append(
			missionIdsByGroupId[mg.QuestMissionGroupId], mg.QuestMissionId)
	}
	missionIdsByQuestId := make(map[int32][]int32)
	for questId, quest := range questById {
		missionIds := missionIdsByGroupId[quest.QuestMissionGroupId]
		if len(missionIds) == 0 {
			continue
		}
		missionIdsByQuestId[questId] = append([]int32(nil), missionIds...)
	}

	chapterBySequenceId := make(map[int32]EntityMMainQuestChapter, len(chapters))
	for _, chapter := range chapters {
		chapterBySequenceId[chapter.MainQuestSequenceGroupId] = chapter
	}
	routeIdByQuestId := make(map[int32]int32)
	mainQuestChapterIdByQuestId := make(map[int32]int32)
	mainQuestRouteIdByChapterId := make(map[int32]int32)
	for _, sequence := range sequences {
		if chapter, ok := chapterBySequenceId[sequence.MainQuestSequenceId]; ok {
			routeIdByQuestId[sequence.QuestId] = chapter.MainQuestRouteId
			mainQuestChapterIdByQuestId[sequence.QuestId] = chapter.MainQuestChapterId
			mainQuestRouteIdByChapterId[chapter.MainQuestChapterId] = chapter.MainQuestRouteId
		}
	}

	eventChapters, err := utils.ReadTable[EntityMEventQuestChapter]("m_event_quest_chapter")
	if err != nil {
		return nil, fmt.Errorf("load event quest chapter table: %w", err)
	}
	eventQuestTypeByChapterId := make(map[int32]int32, len(eventChapters))
	for _, ec := range eventChapters {
		eventQuestTypeByChapterId[ec.EventQuestChapterId] = ec.EventQuestType
	}

	// Map each event quest to the memoir (Parts) set its chapter advertises in
	// the display item group, resolving chapter -> sequence group -> sequences ->
	// quests. The advertised memoirs span several series but the per-quest drop
	// data only wires one of them, so the others are otherwise unobtainable.
	eventDisplayItems, err := utils.ReadTable[EntityMEventQuestDisplayItemGroup]("m_event_quest_display_item_group")
	if err != nil {
		return nil, fmt.Errorf("load event quest display item group table: %w", err)
	}
	eventSequences, err := utils.ReadTable[EntityMEventQuestSequence]("m_event_quest_sequence")
	if err != nil {
		return nil, fmt.Errorf("load event quest sequence table: %w", err)
	}
	eventSequenceGroups, err := utils.ReadTable[EntityMEventQuestSequenceGroup]("m_event_quest_sequence_group")
	if err != nil {
		return nil, fmt.Errorf("load event quest sequence group table: %w", err)
	}
	eventLimitRelations, err := utils.ReadTable[EntityMEventQuestChapterLimitContentRelation]("m_event_quest_chapter_limit_content_relation")
	if err != nil {
		return nil, fmt.Errorf("load event limit content relations: %w", err)
	}
	eventQuestIdsByChapterId, _, eventQuestIdsByChapterDifficulty := buildEventQuestIndexes(eventChapters, eventSequenceGroups, eventSequences)
	limitContentQuestIds := make(map[int32]bool)
	for _, relation := range eventLimitRelations {
		for _, questId := range eventQuestIdsByChapterId[relation.EventQuestChapterId] {
			limitContentQuestIds[questId] = true
		}
	}
	memoirsByDisplayGroupId := make(map[int32][]int32)
	for _, d := range eventDisplayItems {
		if d.PossessionType == 4 { // PossessionTypeParts == memoirs
			memoirsByDisplayGroupId[d.EventQuestDisplayItemGroupId] = append(
				memoirsByDisplayGroupId[d.EventQuestDisplayItemGroupId], d.PossessionId)
		}
	}
	questIdsBySequenceId := make(map[int32][]int32)
	for _, s := range eventSequences {
		questIdsBySequenceId[s.EventQuestSequenceId] = append(questIdsBySequenceId[s.EventQuestSequenceId], s.QuestId)
	}
	sequenceIdsByGroupId := make(map[int32][]int32)
	for _, sg := range eventSequenceGroups {
		sequenceIdsByGroupId[sg.EventQuestSequenceGroupId] = append(
			sequenceIdsByGroupId[sg.EventQuestSequenceGroupId], sg.EventQuestSequenceId)
	}
	chapterMemoirsByQuestId := make(map[int32][]int32)
	for _, ec := range eventChapters {
		memoirs := memoirsByDisplayGroupId[ec.EventQuestDisplayItemGroupId]
		if len(memoirs) == 0 {
			continue
		}
		for _, seqId := range sequenceIdsByGroupId[ec.EventQuestSequenceGroupId] {
			for _, qid := range questIdsBySequenceId[seqId] {
				chapterMemoirsByQuestId[qid] = memoirs
			}
		}
	}

	sortedChapters := make([]EntityMMainQuestChapter, len(chapters))
	copy(sortedChapters, chapters)
	sort.Slice(sortedChapters, func(i, j int) bool {
		return sortedChapters[i].SortOrder < sortedChapters[j].SortOrder
	})
	sequencesByGroupId := make(map[int32][]EntityMMainQuestSequence)
	for _, seq := range sequences {
		sequencesByGroupId[seq.MainQuestSequenceId] = append(sequencesByGroupId[seq.MainQuestSequenceId], seq)
	}
	var orderedQuestIds []int32
	for _, chapter := range sortedChapters {
		for _, seq := range sequencesByGroupId[chapter.MainQuestSequenceGroupId] {
			orderedQuestIds = append(orderedQuestIds, seq.QuestId)
		}
	}

	chapterLastSceneByQuestId := make(map[int32]int32)
	for _, chapter := range sortedChapters {
		seqs := sequencesByGroupId[chapter.MainQuestSequenceGroupId]
		var chapterLastScene int32
		for i := len(seqs) - 1; i >= 0; i-- {
			if sids := sceneIdsByQuestId[seqs[i].QuestId]; len(sids) > 0 {
				chapterLastScene = sids[len(sids)-1]
				break
			}
		}
		if chapterLastScene != 0 {
			for _, seq := range seqs {
				chapterLastSceneByQuestId[seq.QuestId] = chapterLastScene
			}
		}
	}

	firstClearRewardsByGroupId := make(map[int32][]EntityMQuestFirstClearRewardGroup, len(firstClearRewards))
	for _, reward := range firstClearRewards {
		firstClearRewardsByGroupId[reward.QuestFirstClearRewardGroupId] = append(
			firstClearRewardsByGroupId[reward.QuestFirstClearRewardGroupId], reward)
	}

	replayFlowRewardsByGroupId := make(map[int32][]EntityMQuestReplayFlowRewardGroup, len(replayFlowRewards))
	for _, reward := range replayFlowRewards {
		replayFlowRewardsByGroupId[reward.QuestReplayFlowRewardGroupId] = append(
			replayFlowRewardsByGroupId[reward.QuestReplayFlowRewardGroupId], reward)
	}

	firstClearRewardSwitchesByQuestId := make(map[int32][]EntityMQuestFirstClearRewardSwitch, len(firstClearSwitches))
	for _, switchRow := range firstClearSwitches {
		firstClearRewardSwitchesByQuestId[switchRow.QuestId] = append(
			firstClearRewardSwitchesByQuestId[switchRow.QuestId], switchRow)
	}

	missionRewardsByMissionId := make(map[int32][]EntityMQuestMissionReward, len(missionRewards))
	for _, reward := range missionRewards {
		missionRewardsByMissionId[reward.QuestMissionRewardId] = append(
			missionRewardsByMissionId[reward.QuestMissionRewardId], reward)
	}

	weaponIdsByReleaseConditionGroupId := make(map[int32][]int32)
	for _, w := range weaponById {
		if w.WeaponStoryReleaseConditionGroupId != 0 {
			weaponIdsByReleaseConditionGroupId[w.WeaponStoryReleaseConditionGroupId] = append(
				weaponIdsByReleaseConditionGroupId[w.WeaponStoryReleaseConditionGroupId], w.WeaponId)
		}
	}

	releaseConditionsByGroupId := make(map[int32][]EntityMWeaponStoryReleaseConditionGroup)
	for _, c := range releaseConditions {
		releaseConditionsByGroupId[c.WeaponStoryReleaseConditionGroupId] = append(
			releaseConditionsByGroupId[c.WeaponStoryReleaseConditionGroupId], c)
	}

	sceneGrantsBySceneId := make(map[int32][]EntityMUserQuestSceneGrantPossession)
	for _, sg := range sceneGrants {
		sceneGrantsBySceneId[sg.QuestSceneId] = append(sceneGrantsBySceneId[sg.QuestSceneId], sg)
	}

	// Build scene choice maps
	sceneChoiceByKey := make(map[SceneChoiceKey]EntityMQuestSceneChoice, len(sceneChoices))
	for _, sc := range sceneChoices {
		key := SceneChoiceKey{
			QuestSceneId:  sc.MainFlowQuestSceneId,
			QuestFlowType: sc.QuestFlowType,
			ChoiceNumber:  sc.ChoiceNumber,
		}
		sceneChoiceByKey[key] = sc
	}

	sceneChoiceEffectById := make(map[int32]EntityMQuestSceneChoiceEffect, len(sceneChoiceEffects))
	for _, sce := range sceneChoiceEffects {
		sceneChoiceEffectById[sce.QuestSceneChoiceEffectId] = sce
	}

	battleDropRewardById := make(map[int32]EntityMBattleDropReward, len(battleDropRewards))
	for _, bdr := range battleDropRewards {
		battleDropRewardById[bdr.BattleDropRewardId] = bdr
	}

	pickupRewardIdsByGroupId := make(map[int32][]int32)
	for _, pg := range pickupRewardGroups {
		pickupRewardIdsByGroupId[pg.QuestPickupRewardGroupId] = append(
			pickupRewardIdsByGroupId[pg.QuestPickupRewardGroupId], pg.BattleDropRewardId)
	}

	battleGroupBySceneId := make(map[int32]int32, len(sceneBattles))
	for _, sb := range sceneBattles {
		battleGroupBySceneId[sb.QuestSceneId] = sb.BattleGroupId
	}

	battleIdsByGroupId := make(map[int32][]int32)
	for _, bg := range battleGroups {
		battleIdsByGroupId[bg.BattleGroupId] = append(battleIdsByGroupId[bg.BattleGroupId], bg.BattleId)
	}

	// Build the composite-key map for looking up battle NPC decks
	battleNpcDeckByKey := make(map[NpcDeckKey]EntityMBattleNpcDeck, len(npcDecks))
	for _, d := range npcDecks {
		battleNpcDeckByKey[NpcDeckKey{d.BattleNpcId, d.DeckType, d.BattleNpcDeckNumber}] = d
	}

	battleByIdMap := make(map[int32]EntityMBattle, len(battles))
	for _, b := range battles {
		battleByIdMap[b.BattleId] = b
	}

	battleNpcDeckCharacterTypeByNpcId := make(map[int64][]EntityMBattleNpcDeckCharacterType, len(npcCharacterTypes))
	for _, ct := range npcCharacterTypes {
		battleNpcDeckCharacterTypeByNpcId[ct.BattleNpcId] = append(battleNpcDeckCharacterTypeByNpcId[ct.BattleNpcId], ct)
	}

	battleNpcDeckByNumber := make(map[int32]EntityMBattleNpcDeck, len(battleNpcDecks))
	for _, deck := range battleNpcDecks {
		battleNpcDeckByNumber[deck.BattleNpcDeckNumber] = deck
	}

	type dropCatKey struct {
		BattleNpcId int64
		Uuid        string
	}
	dropCategoryByKey := make(map[dropCatKey]int32, len(npcDropCategories))
	for _, dc := range npcDropCategories {
		dropCategoryByKey[dropCatKey{dc.BattleNpcId, dc.BattleNpcDeckCharacterUuid}] = dc.BattleDropCategoryId
	}

	battleDropsByQuestId := make(map[int32][]BattleDropInfo)
	for questId := range questById {
		sids := sceneIdsByQuestId[questId]
		seen := make(map[BattleDropInfo]bool)
		var drops []BattleDropInfo
		for _, sceneId := range sids {
			groupId, ok := battleGroupBySceneId[sceneId]
			if !ok {
				continue
			}
			for _, battleId := range battleIdsByGroupId[groupId] {
				b, ok := battleByIdMap[battleId]
				if !ok {
					continue
				}
				dk := NpcDeckKey{b.BattleNpcId, b.DeckType, b.BattleNpcDeckNumber}
				deck, ok := battleNpcDeckByKey[dk]
				if !ok {
					continue
				}
				for _, uuid := range []string{deck.BattleNpcDeckCharacterUuid01, deck.BattleNpcDeckCharacterUuid02, deck.BattleNpcDeckCharacterUuid03} {
					if uuid == "" {
						continue
					}
					catId, ok := dropCategoryByKey[dropCatKey{b.BattleNpcId, uuid}]
					if !ok {
						continue
					}
					info := BattleDropInfo{QuestSceneId: sceneId, BattleDropCategoryId: catId}
					if !seen[info] {
						seen[info] = true
						drops = append(drops, info)
					}
				}
			}
		}
		if len(drops) > 0 {
			battleDropsByQuestId[questId] = drops
		}
	}

	rentalBattleGroups := make(map[int32]bool, len(rentalDecks))
	for _, rd := range rentalDecks {
		rentalBattleGroups[rd.BattleGroupId] = true
	}
	rentalQuestIds := make(map[int32]bool)
	for questId := range questById {
		for _, sceneId := range sceneIdsByQuestId[questId] {
			if groupId, ok := battleGroupBySceneId[sceneId]; ok && rentalBattleGroups[groupId] {
				rentalQuestIds[questId] = true
				break
			}
		}
	}

	// Build map of quest mission condition value groups by group ID
	questMissionConditionValueGroupsByGroupId := make(map[int32][]int32)
	for _, mcvg := range missionConditionValueGroups {
		questMissionConditionValueGroupsByGroupId[mcvg.QuestMissionConditionValueGroupId] = append(
			questMissionConditionValueGroupsByGroupId[mcvg.QuestMissionConditionValueGroupId], mcvg.ConditionValue)
	}

	// Build set of skill behaviour action IDs that are recovery actions
	skillBehaviourActionRecoveryIds := make(map[int32]bool, len(skillBehaviourActionRecoveries))
	for _, recovery := range skillBehaviourActionRecoveries {
		skillBehaviourActionRecoveryIds[recovery.SkillBehaviourActionId] = true
	}

	// Build the set of SkillDetailIds that belong to costume active skills:
	// m_costume_active_skill_group.CostumeActiveSkillId -> m_skill.SkillLevelGroupId -> m_skill_level_group.SkillDetailId
	skillLevelGroupBySkillId := make(map[int32]int32, len(skills))
	for _, s := range skills {
		skillLevelGroupBySkillId[s.SkillId] = s.SkillLevelGroupId
	}
	detailIdsByLevelGroup := make(map[int32][]int32, len(skillLevelGroups))
	for _, slg := range skillLevelGroups {
		detailIdsByLevelGroup[slg.SkillLevelGroupId] = append(detailIdsByLevelGroup[slg.SkillLevelGroupId], slg.SkillDetailId)
	}
	costumeSkillDetailIds := make(map[int32]bool)
	for _, casg := range costumeActiveSkillGroups {
		if lvlGroupId, ok := skillLevelGroupBySkillId[casg.CostumeActiveSkillId]; ok {
			for _, detailId := range detailIdsByLevelGroup[lvlGroupId] {
				costumeSkillDetailIds[detailId] = true
			}
		}
	}

	deckRestrictionRows, err := utils.ReadTable[EntityMQuestDeckRestrictionGroup]("m_quest_deck_restriction_group")
	if err != nil {
		return nil, fmt.Errorf("load quest deck restriction group: %w", err)
	}
	deckRestrictionsByGroupId := make(map[int32][]EntityMQuestDeckRestrictionGroup)
	for _, row := range deckRestrictionRows {
		deckRestrictionsByGroupId[row.QuestDeckRestrictionGroupId] = append(
			deckRestrictionsByGroupId[row.QuestDeckRestrictionGroupId], row)
	}

	costumeProperAttrRows, err := utils.ReadTable[EntityMCostumeProperAttributeHpBonus]("m_costume_proper_attribute_hp_bonus")
	if err != nil {
		return nil, fmt.Errorf("load costume proper attribute: %w", err)
	}
	costumeProperAttributeByCostumeId := make(map[int32]int32, len(costumeProperAttrRows))
	for _, row := range costumeProperAttrRows {
		// First row wins; master data usually has one primary affinity per costume.
		if _, exists := costumeProperAttributeByCostumeId[row.CostumeId]; !exists {
			costumeProperAttributeByCostumeId[row.CostumeId] = row.CostumeProperAttributeType
		}
	}

	
	questBonuses, err := utils.ReadTable[EntityMQuestBonus]("m_quest_bonus")
	if err != nil {
		return nil, fmt.Errorf("load quest bonus: %w", err)
	}
	questBonusWeapons, err := utils.ReadTable[EntityMQuestBonusWeaponGroup]("m_quest_bonus_weapon_group")
	if err != nil {
		return nil, fmt.Errorf("load quest bonus weapon group: %w", err)
	}
	questBonusEffects, err := utils.ReadTable[EntityMQuestBonusEffectGroup]("m_quest_bonus_effect_group")
	if err != nil {
		return nil, fmt.Errorf("load quest bonus effect group: %w", err)
	}
	questBonusDrops, err := utils.ReadTable[EntityMQuestBonusDropReward]("m_quest_bonus_drop_reward")
	if err != nil {
		return nil, fmt.Errorf("load quest bonus drop reward: %w", err)
	}
	questBonusTerms, err := utils.ReadTable[EntityMQuestBonusTermGroup]("m_quest_bonus_term_group")
	if err != nil {
		return nil, fmt.Errorf("load quest bonus term group: %w", err)
	}
	weaponEvolutions, err := utils.ReadTable[EntityMWeaponEvolutionGroup]("m_weapon_evolution_group")
	if err != nil {
		return nil, fmt.Errorf("load weapon evolution group: %w", err)
	}

	questBonusById := make(map[int32]EntityMQuestBonus, len(questBonuses))
	for _, bonus := range questBonuses {
		questBonusById[bonus.QuestBonusId] = bonus
	}
	questBonusWeaponRowsByGroupId := make(map[int32][]EntityMQuestBonusWeaponGroup)
	for _, row := range questBonusWeapons {
		questBonusWeaponRowsByGroupId[row.QuestBonusWeaponGroupId] = append(
			questBonusWeaponRowsByGroupId[row.QuestBonusWeaponGroupId], row)
	}
	questBonusEffectsByGroupId := make(map[int32][]EntityMQuestBonusEffectGroup)
	for _, row := range questBonusEffects {
		questBonusEffectsByGroupId[row.QuestBonusEffectGroupId] = append(
			questBonusEffectsByGroupId[row.QuestBonusEffectGroupId], row)
	}
	questBonusDropByEffectId := make(map[int32]EntityMQuestBonusDropReward, len(questBonusDrops))
	for _, row := range questBonusDrops {
		questBonusDropByEffectId[row.QuestBonusEffectId] = row
	}
	questBonusTermsByGroupId := make(map[int32][]EntityMQuestBonusTermGroup)
	for _, row := range questBonusTerms {
		questBonusTermsByGroupId[row.QuestBonusTermGroupId] = append(
			questBonusTermsByGroupId[row.QuestBonusTermGroupId], row)
	}
	weaponEvolutionByWeaponId := make(map[int32]WeaponEvolutionInfo, len(weaponEvolutions))
	for _, evolution := range weaponEvolutions {
		weaponEvolutionByWeaponId[evolution.WeaponId] = WeaponEvolutionInfo{
			GroupId: evolution.WeaponEvolutionGroupId,
			Order:   evolution.EvolutionOrder,
		}
	}
	log.Printf("quest bonus loaded: %d bonuses, %d weapon rows, %d drop effects",
		len(questBonusById), len(questBonusWeapons), len(questBonusDropByEffectId))

return &QuestCatalog{
		SceneById:                                 sceneById,
		MissionById:                               missionById,
		QuestById:                                 questById,
		QuestReleaseConditionsByListId:            questReleaseConditionsByListId,
		MissionIdsByQuestId:                       missionIdsByQuestId,
		RouteIdByQuestId:                          routeIdByQuestId,
		SceneIdsByQuestId:                         sceneIdsByQuestId,
		OrderedQuestIds:                           orderedQuestIds,
		FirstClearRewardsByGroupId:                firstClearRewardsByGroupId,
		FirstClearRewardSwitchesByQuestId:         firstClearRewardSwitchesByQuestId,
		MissionRewardsByMissionId:                 missionRewardsByMissionId,
		WeaponIdsByReleaseConditionGroupId:        weaponIdsByReleaseConditionGroupId,
		ReleaseConditionsByGroupId:                releaseConditionsByGroupId,
		SceneGrantsBySceneId:                      sceneGrantsBySceneId,
		BattleDropRewardById:                      battleDropRewardById,
		PickupRewardIdsByGroupId:                  pickupRewardIdsByGroupId,
		BattleDropsByQuestId:                      battleDropsByQuestId,
		ReplayFlowRewardsByGroupId:                replayFlowRewardsByGroupId,
		RentalQuestIds:                            rentalQuestIds,
		TutorialUnlockConditions:                  tutorialUnlockConds,
		ChapterLastSceneByQuestId:                 chapterLastSceneByQuestId,
		SeasonIdByRouteId:                         seasonIdByRouteId,
		RoutesBySeason:                            routesBySeason,
		RouteCompletionQuestId:                    routeCompletionQuestId,
		BattleOnlyTargetSceneByQuestId:            battleOnlyTargetSceneByQuestId,
		MainQuestChapterIdByQuestId:               mainQuestChapterIdByQuestId,
		MainQuestRouteIdByChapterId:               mainQuestRouteIdByChapterId,
		EventQuestTypeByChapterId:                 eventQuestTypeByChapterId,
		EventQuestIdsByChapterId:                  eventQuestIdsByChapterId,
		EventQuestIdsByChapterDifficulty:          eventQuestIdsByChapterDifficulty,
		LimitContentQuestIds:                      limitContentQuestIds,
		DeckRestrictionsByGroupId:                 deckRestrictionsByGroupId,
		CostumeProperAttributeByCostumeId:        costumeProperAttributeByCostumeId,
		DarkMemoryQuestIds:                        darkMemoryQuestIds,
		ChapterMemoirsByQuestId:                   chapterMemoirsByQuestId,
		QuestMissionConditionValueGroupsByGroupId: questMissionConditionValueGroupsByGroupId,
		SkillBehaviourActionRecoveryIds:           skillBehaviourActionRecoveryIds,
		CostumeSkillDetailIds:                     costumeSkillDetailIds,

		// Scene choice mappings
		SceneChoiceByKey:     sceneChoiceByKey,
		SceneChoiceEffectById: sceneChoiceEffectById,

		// Battle-related mappings for boss detection
		BattleGroupBySceneId:              battleGroupBySceneId,
		BattleIdsByGroupId:                battleIdsByGroupId,
		BattleByIdMap:                     battleByIdMap,
		BattleNpcDeckByNumber:             battleNpcDeckByNumber,
		BattleNpcDeckByKey:                battleNpcDeckByKey,
		BattleNpcDeckCharacterTypeByNpcId: battleNpcDeckCharacterTypeByNpcId,

		UserExpThresholds:       BuildExpThresholds(paramMapRows, 1),
		CharacterExpThresholds:  BuildExpThresholds(paramMapRows, 31),
		CostumeExpByRarity:      costumeExpByRarity,
		CostumeMaxLevelByRarity: costumeMaxLevelByRarity,
		MaxStaminaByLevel:       maxStaminaByLevel,

		CostumeById:           costumeById,
		CompanionEnhancedById: companionEnhancedById,
		WeaponById:            weaponById,
		WeaponAttributeById:   weaponAttributeById,

		WeaponSkillSlots:   skillSlots,
		WeaponAbilitySlots: abilitySlots,

		QuestBonusById:                        questBonusById,
		QuestBonusWeaponRowsByGroupId:         questBonusWeaponRowsByGroupId,
		QuestBonusEffectsByGroupId:            questBonusEffectsByGroupId,
		QuestBonusDropByEffectId:              questBonusDropByEffectId,
		QuestBonusTermsByGroupId:              questBonusTermsByGroupId,
		WeaponEvolutionByWeaponId:             weaponEvolutionByWeaponId,

		PartsCatalog: partsCatalog,
	}, nil
}

func (q *QuestCatalog) BattleOnlyTargetSceneIdFor(questId int32) (int32, bool) {
	v, ok := q.BattleOnlyTargetSceneByQuestId[questId]
	return v, ok
}


// QuestHasAffinityRestriction reports whether the quest requires a proper
// attribute (affinity) on any deck slot (QuestDeckRestrictionType = 3).
func (c *QuestCatalog) QuestHasAffinityRestriction(questId int32) bool {
	if c == nil {
		return false
	}
	quest, ok := c.QuestById[questId]
	if !ok || quest.QuestDeckRestrictionGroupId == 0 {
		return false
	}
	for _, row := range c.DeckRestrictionsByGroupId[quest.QuestDeckRestrictionGroupId] {
		if row.QuestDeckRestrictionType == QuestDeckRestrictionTypeProperAttributeType {
			return true
		}
	}
	return false
}

// DeckRestrictionsForQuest returns slot restrictions for a quest (may be empty).
func (c *QuestCatalog) DeckRestrictionsForQuest(questId int32) []EntityMQuestDeckRestrictionGroup {
	if c == nil {
		return nil
	}
	quest, ok := c.QuestById[questId]
	if !ok || quest.QuestDeckRestrictionGroupId == 0 {
		return nil
	}
	return c.DeckRestrictionsByGroupId[quest.QuestDeckRestrictionGroupId]
}
