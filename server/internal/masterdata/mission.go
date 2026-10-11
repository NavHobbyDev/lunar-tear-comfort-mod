package masterdata

import (
	"log"
	"sort"

	"lunar-tear/server/internal/utils"
)

type MainQuestPosition struct {
	DifficultyType int32
	QuestNumber    int32
}

// MissionCatalog is an immutable snapshot of every mission-related master-data
// table, indexed for fast lookup by the mission service.
type MissionCatalog struct {
	Missions         map[int32]EntityMMission         // by MissionId
	RewardsByGroupId map[int32][]EntityMMissionReward // by MissionRewardId
	TermById         map[int32]EntityMMissionTerm     // by MissionTermId
	GroupById        map[int32]EntityMMissionGroup    // by MissionGroupId
	LinkById         map[int32]EntityMMissionLink     // by MissionLinkId

	PassById         map[int32]EntityMMissionPass               // by MissionPassId
	PassLevelGroup   map[int32][]EntityMMissionPassLevelGroup   // by MissionPassLevelGroupId (sorted by Level)
	PassRewardGroup  map[int32][]EntityMMissionPassRewardGroup  // by MissionPassRewardGroupId
	PassMissionGroup map[int32][]EntityMMissionPassMissionGroup // by MissionPassId

	// QuestToMainChapter maps any main-quest QuestId to its MainQuestChapterId.
	// Built from m_main_quest_chapter -> m_main_quest_sequence_group ->
	// m_main_quest_sequence at startup.
	QuestToMainChapter map[int32]int32

	// QuestToEventChapter maps any event-quest QuestId to its EventQuestChapterId.
	// Built from m_event_quest_chapter -> m_event_quest_sequence_group ->
	// m_event_quest_sequence at startup.
	QuestToEventChapter map[int32]int32

	// MainChapterFinalQuestIds contains the last quest of every difficulty in a
	// main-quest chapter. Chapter-clear missions complete only on these quests.
	MainChapterFinalQuestIds          map[int32]map[int32]bool
	MainChapterFinalQuestByDifficulty map[int32]map[int32]int32
	MainQuestByPosition               map[int32]map[MainQuestPosition]int32
	MainMissionTargetQuest            map[int32]int32
	// MainMissionConcreteTemplate marks missions that belong to a concrete
	// Quest 1/4/7/10 template even if their target cannot be resolved. Such a
	// mission must never fall back to treating OptionGroupId as a QuestId.
	MainMissionConcreteTemplate map[int32]bool
	MainChapterClearMission     map[int32]int32 // mission -> MainQuestChapterId
	MainChapterClearDifficulty  map[int32]int32 // mission -> DifficultyType

	// GameToDataChapter maps game-visible chapter numbers (used in mission
	// OptionGroupId as 100+N) to actual data chapter IDs. Single-quest
	// chapters (prologues/epilogues) are skipped in the game numbering.
	GameToDataChapter map[int32]int32

	// EventChapterQuestIds contains every quest belonging to an event chapter;
	// EventChapterByAssetId resolves a mission group's AssetId to that chapter.
	EventChapterQuestIds  map[int32]map[int32]bool
	EventChapterByAssetId map[int32]int32
	// EventChapterRerunAlias maps rerun event-chapter ids that exist only in
	// m_mission_link (their m_event_quest_chapter rows were never shipped in
	// this master-data revision) to the original chapter whose BannerAssetId
	// equals the missing chapter id. Record/Variation rerun missions then
	// count clears of the original chapter's quests instead of being forever
	// uncompletable.
	EventChapterRerunAlias map[int32]int32
	// EventChapterFinalQuestByDifficulty contains the final quest for each
	// difficulty of an event chapter. Mission templates target this quest when
	// they require clearing a Record on a concrete difficulty.
	EventChapterFinalQuestByDifficulty map[int32]map[int32]int32
	// EventQuestPosition locates an event quest inside its chapter: the
	// difficulty sequence it belongs to and its 1-based number within it.
	// Used by hidden-story mission 500013 ("Clear Quest 3 ... on EX Hard").
	EventQuestPosition map[int32]MainQuestPosition
	// EventMissionTargetDifficulty is derived from the mission template itself:
	// condition group, option group and order inside the mission group. A
	// missing entry means "any quest of the linked event chapter".
	EventMissionTargetDifficulty map[int32]int32

	// CompleteMissionCostumesByMissionId maps a type-49 mission ("Acquire
	// costumes for X and Y") to the concrete list of costumes that count
	// toward it. Master data stores the tuples in m_complete_mission_group
	// with PossessionType=1 (costume).
	CompleteMissionCostumesByMissionId map[int32][]int32

	// WeaponIdsByOptionGroup resolves a MissionClearConditionOptionGroupId
	// used by weapon-specific missions (condition types 6, 8 and 43) to the
	// WeaponIds of the targeted weapon family. An empty map entry means the
	// option group is not weapon-related and the caller keeps its own
	// matching rules.
	WeaponIdsByOptionGroup map[int32]map[int32]bool
}

func LoadMissionCatalog() *MissionCatalog {
	cat := &MissionCatalog{
		Missions:                           map[int32]EntityMMission{},
		RewardsByGroupId:                   map[int32][]EntityMMissionReward{},
		TermById:                           map[int32]EntityMMissionTerm{},
		GroupById:                          map[int32]EntityMMissionGroup{},
		LinkById:                           map[int32]EntityMMissionLink{},
		PassById:                           map[int32]EntityMMissionPass{},
		PassLevelGroup:                     map[int32][]EntityMMissionPassLevelGroup{},
		PassRewardGroup:                    map[int32][]EntityMMissionPassRewardGroup{},
		PassMissionGroup:                   map[int32][]EntityMMissionPassMissionGroup{},
		QuestToMainChapter:                 map[int32]int32{},
		QuestToEventChapter:                map[int32]int32{},
		MainChapterFinalQuestIds:           map[int32]map[int32]bool{},
		MainChapterFinalQuestByDifficulty:  map[int32]map[int32]int32{},
		MainQuestByPosition:                map[int32]map[MainQuestPosition]int32{},
		MainMissionTargetQuest:             map[int32]int32{},
		MainMissionConcreteTemplate:        map[int32]bool{},
		MainChapterClearMission:            map[int32]int32{},
		MainChapterClearDifficulty:         map[int32]int32{},
		GameToDataChapter:                  map[int32]int32{},
		EventChapterQuestIds:               map[int32]map[int32]bool{},
		EventChapterByAssetId:              map[int32]int32{},
		EventChapterRerunAlias:             map[int32]int32{},
		EventChapterFinalQuestByDifficulty: map[int32]map[int32]int32{},
		EventQuestPosition:                 map[int32]MainQuestPosition{},
		EventMissionTargetDifficulty:       map[int32]int32{},
		CompleteMissionCostumesByMissionId: map[int32][]int32{},
		WeaponIdsByOptionGroup:             map[int32]map[int32]bool{},
	}

	// Load mission groups first to enable category-based filtering
	if rows, err := utils.ReadTable[EntityMMissionGroup]("m_mission_group"); err == nil {
		for _, r := range rows {
			cat.GroupById[r.MissionGroupId] = r
		}
	} else {
		log.Printf("[MissionCatalog] load m_mission_group: %v", err)
	}

	// Load missions and filter by MissionCategoryType
	if rows, err := utils.ReadTable[EntityMMission]("m_mission"); err == nil {
		for _, r := range rows {
			if group, ok := cat.GroupById[r.MissionGroupId]; ok {
				if group.MissionCategoryType == 2 || // Challenge
					group.MissionCategoryType == 5 || // Costumes
					group.MissionCategoryType == 7 { // For Hidden Story
					// group.MissionCategoryType == 3 || // Event
					// group.MissionCategoryType == 1 // Daily
					// group.MissionCategoryType == 4 // Panel (Panel not work)
					// group.MissionCategoryType == 6 // Packs
					// group.MissionCategoryType == 9 // Pass daily (Pass not work)
					// group.MissionCategoryType == 10 // Pass (Pass not work)
					cat.Missions[r.MissionId] = r
				}
			}
		}
	} else {
		log.Printf("[MissionCatalog] load m_mission: %v", err)
	}

	if rows, err := utils.ReadTable[EntityMMissionReward]("m_mission_reward"); err == nil {
		for _, r := range rows {
			cat.RewardsByGroupId[r.MissionRewardId] = append(cat.RewardsByGroupId[r.MissionRewardId], r)
		}
	} else {
		log.Printf("[MissionCatalog] load m_mission_reward: %v", err)
	}

	if rows, err := utils.ReadTable[EntityMMissionTerm]("m_mission_term"); err == nil {
		for _, r := range rows {
			cat.TermById[r.MissionTermId] = r
		}
	} else {
		log.Printf("[MissionCatalog] load m_mission_term: %v", err)
	}

	if rows, err := utils.ReadTable[EntityMMissionLink]("m_mission_link"); err == nil {
		for _, r := range rows {
			cat.LinkById[r.MissionLinkId] = r
		}
	} else {
		log.Printf("[MissionCatalog] load m_mission_link: %v", err)
	}

	if rows, err := utils.ReadTable[EntityMMissionPass]("m_mission_pass"); err == nil {
		for _, r := range rows {
			cat.PassById[r.MissionPassId] = r
		}
	}
	if rows, err := utils.ReadTable[EntityMMissionPassLevelGroup]("m_mission_pass_level_group"); err == nil {
		for _, r := range rows {
			cat.PassLevelGroup[r.MissionPassLevelGroupId] = append(cat.PassLevelGroup[r.MissionPassLevelGroupId], r)
		}
		for k := range cat.PassLevelGroup {
			levels := cat.PassLevelGroup[k]
			sort.SliceStable(levels, func(i, j int) bool { return levels[i].Level < levels[j].Level })
		}
	}
	if rows, err := utils.ReadTable[EntityMMissionPassRewardGroup]("m_mission_pass_reward_group"); err == nil {
		for _, r := range rows {
			cat.PassRewardGroup[r.MissionPassRewardGroupId] = append(cat.PassRewardGroup[r.MissionPassRewardGroupId], r)
		}
	}
	if rows, err := utils.ReadTable[EntityMMissionPassMissionGroup]("m_mission_pass_mission_group"); err == nil {
		for _, r := range rows {
			cat.PassMissionGroup[r.MissionPassId] = append(cat.PassMissionGroup[r.MissionPassId], r)
		}
	}

	if rows, err := utils.ReadTable[EntityMCompleteMissionGroup]("m_complete_mission_group"); err == nil {
		// Sort so that a mission's costume list is deterministic across loads.
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].MissionId != rows[j].MissionId {
				return rows[i].MissionId < rows[j].MissionId
			}
			return rows[i].SortOrder < rows[j].SortOrder
		})
		for _, r := range rows {
			// Only PossessionType 1 (costume) is observed in this table today;
			// guard defensively in case future data introduces other targets.
			if r.PossessionType != 1 {
				continue
			}
			cat.CompleteMissionCostumesByMissionId[r.MissionId] = append(
				cat.CompleteMissionCostumesByMissionId[r.MissionId], r.PossessionId,
			)
		}
	} else {
		log.Printf("[MissionCatalog] load m_complete_mission_group: %v", err)
	}

	for optionGroup, weaponIds := range weaponMissionOptionGroups {
		ids := make(map[int32]bool, len(weaponIds))
		for _, id := range weaponIds {
			ids[id] = true
		}
		cat.WeaponIdsByOptionGroup[optionGroup] = ids
	}

	cat.buildMainQuestChapterIndex()
	cat.buildEventQuestChapterIndex()
	cat.buildMainMissionTargetIndex()
	cat.buildEventMissionTargetIndex()

	log.Printf("[MissionCatalog] loaded: %d missions, %d reward groups, %d terms, %d groups, %d passes, %d main-quest-chapter entries, %d event-quest-chapter entries, %d weapon mission option groups",
		len(cat.Missions), len(cat.RewardsByGroupId), len(cat.TermById), len(cat.GroupById), len(cat.PassById),
		len(cat.QuestToMainChapter), len(cat.QuestToEventChapter), len(cat.WeaponIdsByOptionGroup))

	return cat
}

// buildMainQuestChapterIndex builds QuestToMainChapter from:
// m_main_quest_chapter -> m_main_quest_sequence_group -> m_main_quest_sequence
func (c *MissionCatalog) buildMainQuestChapterIndex() {
	chapters, err := utils.ReadTable[EntityMMainQuestChapter]("m_main_quest_chapter")
	if err != nil {
		log.Printf("[MissionCatalog] load m_main_quest_chapter: %v", err)
		return
	}
	seqGroups, err := utils.ReadTable[EntityMMainQuestSequenceGroup]("m_main_quest_sequence_group")
	if err != nil {
		log.Printf("[MissionCatalog] load m_main_quest_sequence_group: %v", err)
		return
	}
	sequences, err := utils.ReadTable[EntityMMainQuestSequence]("m_main_quest_sequence")
	if err != nil {
		log.Printf("[MissionCatalog] load m_main_quest_sequence: %v", err)
		return
	}

	// chapter -> sequenceGroupId
	chapToSeqGroup := make(map[int32]int32, len(chapters))
	for _, ch := range chapters {
		chapToSeqGroup[ch.MainQuestChapterId] = ch.MainQuestSequenceGroupId
	}
	// sequenceGroupId -> difficulty/sequence rows
	seqGroupToSeqs := make(map[int32][]EntityMMainQuestSequenceGroup)
	for _, sg := range seqGroups {
		seqGroupToSeqs[sg.MainQuestSequenceGroupId] = append(seqGroupToSeqs[sg.MainQuestSequenceGroupId], sg)
	}
	// sequenceId -> quest rows in sequence order
	seqToQuests := make(map[int32][]EntityMMainQuestSequence)
	for _, s := range sequences {
		seqToQuests[s.MainQuestSequenceId] = append(seqToQuests[s.MainQuestSequenceId], s)
	}
	for seqID, rows := range seqToQuests {
		sort.Slice(rows, func(i, j int) bool { return rows[i].SortOrder < rows[j].SortOrder })
		seqToQuests[seqID] = rows
	}

	for chapId, sgId := range chapToSeqGroup {
		for _, sequenceGroup := range seqGroupToSeqs[sgId] {
			quests := seqToQuests[sequenceGroup.MainQuestSequenceId]
			if c.MainQuestByPosition[chapId] == nil {
				c.MainQuestByPosition[chapId] = map[MainQuestPosition]int32{}
			}
			for _, q := range quests {
				c.QuestToMainChapter[q.QuestId] = chapId
				c.MainQuestByPosition[chapId][MainQuestPosition{DifficultyType: sequenceGroup.DifficultyType, QuestNumber: q.SortOrder}] = q.QuestId
			}
			if len(quests) > 0 {
				if c.MainChapterFinalQuestIds[chapId] == nil {
					c.MainChapterFinalQuestIds[chapId] = map[int32]bool{}
				}
				c.MainChapterFinalQuestIds[chapId][quests[len(quests)-1].QuestId] = true
				if c.MainChapterFinalQuestByDifficulty[chapId] == nil {
					c.MainChapterFinalQuestByDifficulty[chapId] = map[int32]int32{}
				}
				c.MainChapterFinalQuestByDifficulty[chapId][sequenceGroup.DifficultyType] = quests[len(quests)-1].QuestId
			}
		}
	}

	// Build game-to-data chapter mapping. Chapters with only one quest
	// (difficulty 1) are prologues/epilogues and don't count as game chapters.
	sortedChapIds := make([]int, 0, len(chapToSeqGroup))
	for chapId := range chapToSeqGroup {
		sortedChapIds = append(sortedChapIds, int(chapId))
	}
	sort.Ints(sortedChapIds)
	gameCh := int32(0)
	for _, chapId := range sortedChapIds {
		numQuests := int32(0)
		for pos := range c.MainQuestByPosition[int32(chapId)] {
			if pos.DifficultyType == 1 {
				numQuests++
			}
		}
		if numQuests <= 1 {
			continue
		}
		gameCh++
		c.GameToDataChapter[gameCh] = int32(chapId)
	}
}

// buildMainMissionTargetIndex uses only m_mission and the main-quest tables.
// Chapter-completion missions point to a chapter (the oldest template stores
// chapter 1..8 as 101..108). Sun/Moon character templates are stored in
// four-entry cycles targeting Quest 1, 4, 7 and 10.
func (c *MissionCatalog) buildMainMissionTargetIndex() {
	byGroup := map[int32][]EntityMMission{}
	for _, mission := range c.Missions {
		if mission.MissionClearConditionType != 1 {
			continue
		}
		chapter, isMainChapter := c.mainChapterForMissionOption(mission.MissionClearConditionOptionGroupId)
		if !isMainChapter {
			continue
		}
		if mission.ClearConditionValue == 1 && mission.MissionClearConditionGroupId != mission.MissionId {
			c.MainChapterClearMission[mission.MissionId] = chapter
			c.MainChapterClearDifficulty[mission.MissionId] = mainDifficultyForMissionOption(mission.MissionClearConditionOptionGroupId)
		}
		if mission.MissionClearConditionGroupId == mission.MissionId {
			// Missions using the +100 convention (e.g. 103 = chapter 3 normal)
			// target a specific quest in a main chapter. The index-based
			// 1/4/7/10 cycle only works when all 4 entries share the same
			// chapter; these missions span different chapters, so treat them
			// as chapter-clear missions (target the final quest).
			if mission.MissionClearConditionOptionGroupId > 100 {
				c.MainChapterClearMission[mission.MissionId] = chapter
				c.MainChapterClearDifficulty[mission.MissionId] = mainDifficultyForMissionOption(mission.MissionClearConditionOptionGroupId)
				continue
			}
			byGroup[mission.MissionGroupId] = append(byGroup[mission.MissionGroupId], mission)
		}
	}

	for _, missions := range byGroup {
		if len(missions) < 4 || len(missions)%4 != 0 {
			continue
		}
		sort.Slice(missions, func(i, j int) bool { return missions[i].SortOrderInMissionGroup < missions[j].SortOrderInMissionGroup })
		for index, mission := range missions {
			c.MainMissionConcreteTemplate[mission.MissionId] = true
			chapter, ok := c.mainChapterForMissionOption(mission.MissionClearConditionOptionGroupId)
			if !ok {
				continue
			}
			position := MainQuestPosition{DifficultyType: 1, QuestNumber: int32(1 + index)}
			if questID := c.MainQuestByPosition[chapter][position]; questID != 0 {
				c.MainMissionTargetQuest[mission.MissionId] = questID
			}
		}
	}
}

func mainDifficultyForMissionOption(option int32) int32 {
	switch {
	case option > 30000:
		return 4
	case option > 20000:
		return 3
	case option > 10000:
		return 2
	default:
		return 1
	}
}

func (c *MissionCatalog) mainChapterForMissionOption(option int32) (int32, bool) {
	if len(c.MainChapterFinalQuestIds[option]) != 0 {
		return option, true
	}
	// The +100/+10000/+20000 convention encodes a GAME chapter number
	// (not a data chapter). Convert via GameToDataChapter to get the actual
	// data chapter ID, since single-quest chapters (prologues) are skipped
	// in game numbering.
	var gameChapter int32
	if option > 20000 {
		gameChapter = option - 20000
	} else if option > 10000 {
		gameChapter = option - 10000
	} else if option > 100 {
		gameChapter = option - 100
	} else {
		return 0, false
	}
	if dataChapter, ok := c.GameToDataChapter[gameChapter]; ok {
		return dataChapter, true
	}
	return 0, false
}

// buildEventQuestChapterIndex builds QuestToEventChapter from:
// m_event_quest_chapter -> m_event_quest_sequence_group -> m_event_quest_sequence
func (c *MissionCatalog) buildEventQuestChapterIndex() {
	chapters, err := utils.ReadTable[EntityMEventQuestChapter]("m_event_quest_chapter")
	if err != nil {
		log.Printf("[MissionCatalog] load m_event_quest_chapter: %v", err)
		return
	}
	seqGroups, err := utils.ReadTable[EntityMEventQuestSequenceGroup]("m_event_quest_sequence_group")
	if err != nil {
		log.Printf("[MissionCatalog] load m_event_quest_sequence_group: %v", err)
		return
	}
	sequences, err := utils.ReadTable[EntityMEventQuestSequence]("m_event_quest_sequence")
	if err != nil {
		log.Printf("[MissionCatalog] load m_event_quest_sequence: %v", err)
		return
	}

	chapToSeqGroup := make(map[int32]int32, len(chapters))
	for _, ch := range chapters {
		chapToSeqGroup[ch.EventQuestChapterId] = ch.EventQuestSequenceGroupId
		c.EventChapterByAssetId[ch.EventQuestChapterId] = ch.EventQuestChapterId
		if ch.BannerAssetId != 0 {
			c.EventChapterByAssetId[ch.BannerAssetId] = ch.EventQuestChapterId
		}
	}
	seqGroupToSeqs := make(map[int32][]EntityMEventQuestSequenceGroup)
	for _, sg := range seqGroups {
		seqGroupToSeqs[sg.EventQuestSequenceGroupId] = append(seqGroupToSeqs[sg.EventQuestSequenceGroupId], sg)
	}
	seqToQuests := make(map[int32][]EntityMEventQuestSequence)
	for _, s := range sequences {
		seqToQuests[s.EventQuestSequenceId] = append(seqToQuests[s.EventQuestSequenceId], s)
	}
	for seqID, rows := range seqToQuests {
		sort.Slice(rows, func(i, j int) bool { return rows[i].SortOrder < rows[j].SortOrder })
		seqToQuests[seqID] = rows
	}

	for chapId, sgId := range chapToSeqGroup {
		if c.EventChapterQuestIds[chapId] == nil {
			c.EventChapterQuestIds[chapId] = map[int32]bool{}
		}
		for _, sequenceGroup := range seqGroupToSeqs[sgId] {
			quests := seqToQuests[sequenceGroup.EventQuestSequenceId]
			for i, quest := range quests {
				c.QuestToEventChapter[quest.QuestId] = chapId
				c.EventChapterQuestIds[chapId][quest.QuestId] = true
				c.EventQuestPosition[quest.QuestId] = MainQuestPosition{
					DifficultyType: sequenceGroup.DifficultyType,
					QuestNumber:    int32(i + 1),
				}
			}
			if len(quests) > 0 {
				if c.EventChapterFinalQuestByDifficulty[chapId] == nil {
					c.EventChapterFinalQuestByDifficulty[chapId] = map[int32]int32{}
				}
				c.EventChapterFinalQuestByDifficulty[chapId][sequenceGroup.DifficultyType] = quests[len(quests)-1].QuestId
			}
		}
	}

	// Rerun aliases: some mission links point at event chapters (record /
	// variation reruns) whose rows are absent from this data revision, making
	// those missions uncompletable. The original chapter reuses the rerun's
	// banner asset id, so EventChapterByAssetId resolves the missing chapter
	// back to the original one whose quests actually exist.
	for _, link := range c.LinkById {
		if link.DestinationDomainType != 4 {
			continue
		}
		dest := link.DestinationDomainId
		if dest == 0 || c.EventChapterQuestIds[dest] != nil || c.MainChapterFinalQuestIds[dest] != nil {
			continue
		}
		if actual := c.EventChapterByAssetId[dest]; actual != 0 && actual != dest && c.EventChapterQuestIds[actual] != nil {
			c.EventChapterRerunAlias[dest] = actual
		}
	}
	if len(c.EventChapterRerunAlias) > 0 {
		log.Printf("[MissionCatalog] rerun chapter aliases: %v", c.EventChapterRerunAlias)
	}
}

// buildEventMissionTargetIndex decodes the two event-mission templates found
// in m_mission. Older Record groups use condition group 2 and list Normal,
// Hard, Very Hard (and, where present, EX Hard) in SortOrder 1..4. Newer
// Record groups list their concrete completion targets first: Normal, then
// EX Hard. Repeated conditions reuse the same OptionGroupId as that target.
//
// This is based exclusively on master-table fields; it has no mission-ID list
// and does not read names or admin-panel data.
func (c *MissionCatalog) buildEventMissionTargetIndex() {
	byGroup := map[int32][]EntityMMission{}
	for _, mission := range c.Missions {
		if mission.MissionClearConditionType != 1 {
			continue
		}
		link, ok := c.LinkById[mission.MissionLinkId]
		if !ok {
			continue
		}
		dest := link.DestinationDomainId
		if alias, isRerun := c.EventChapterRerunAlias[dest]; isRerun {
			dest = alias
		}
		if c.EventChapterQuestIds[dest] == nil {
			continue
		}
		byGroup[mission.MissionGroupId] = append(byGroup[mission.MissionGroupId], mission)
	}

	for _, missions := range byGroup {
		sort.Slice(missions, func(i, j int) bool {
			return missions[i].SortOrderInMissionGroup < missions[j].SortOrderInMissionGroup
		})
		difficultyByOption := map[int32]int32{}

		// Legacy Record template: the mission order is the difficulty order.
		for _, mission := range missions {
			if mission.ClearConditionValue == 1 && mission.MissionClearConditionGroupId == 2 &&
				mission.SortOrderInMissionGroup >= 1 && mission.SortOrderInMissionGroup <= 4 {
				difficultyByOption[mission.MissionClearConditionOptionGroupId] = mission.SortOrderInMissionGroup
			}
		}

		// Current Record template: first concrete target is Normal; second is
		// EX Hard. Sort order, rather than the numeric mission ID, is the
		// stable master-data relation.
		modernTargets := make([]EntityMMission, 0, 2)
		for _, mission := range missions {
			if mission.ClearConditionValue == 1 && mission.MissionClearConditionGroupId != 2 {
				modernTargets = append(modernTargets, mission)
			}
		}
		if len(modernTargets) > 0 {
			difficultyByOption[modernTargets[0].MissionClearConditionOptionGroupId] = 1
		}
		if len(modernTargets) > 1 {
			difficultyByOption[modernTargets[1].MissionClearConditionOptionGroupId] = 4
		}

		for _, mission := range missions {
			if difficulty, ok := difficultyByOption[mission.MissionClearConditionOptionGroupId]; ok {
				c.EventMissionTargetDifficulty[mission.MissionId] = difficulty
			}
		}
	}
}

func (c *MissionCatalog) CategoryTypeOf(m EntityMMission) int32 {
	if g, ok := c.GroupById[m.MissionGroupId]; ok {
		return g.MissionCategoryType
	}
	return 0
}

func (c *MissionCatalog) IsActiveAt(m EntityMMission, nowMillis int64) bool {
	term, ok := c.TermById[m.MissionTermId]
	if !ok {
		return true
	}
	return nowMillis >= term.StartDatetime && nowMillis <= term.EndDatetime
}

func (c *MissionCatalog) ActiveMissionsAt(nowMillis int64) []EntityMMission {
	out := make([]EntityMMission, 0, len(c.Missions))
	for _, m := range c.Missions {
		if c.IsActiveAt(m, nowMillis) {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MissionId < out[j].MissionId })
	return out
}
