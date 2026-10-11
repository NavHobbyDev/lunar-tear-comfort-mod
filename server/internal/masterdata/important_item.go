package masterdata

import (
	"log"

	"lunar-tear/server/internal/utils"
)

// ImportantItemEffectType values from m_important_item_effect.
const (
	importantItemEffectTypeUnlockFunction = 1 // not a drop effect
	ImportantItemEffectTypeDropRate       = 2 // RatePermil bonus on matching drops
	ImportantItemEffectTypeDropCount      = 3 // CountPermil bonus on matching drops
)

// ImportantItemEffectTargetQuestGroupType values — mirror the campaign target
// types in internal/campaign (QuestCampaignTargetType), plus type 8 which is
// specific to important items: a main-quest DifficultyType (3 = Very Hard).
const (
	importantItemQuestGroupTypeWholeQuest          = 1 // all quests
	importantItemQuestGroupTypeQuestType           = 2 // campaign.QuestType value
	importantItemQuestGroupTypeEventQuestType      = 3 // EventQuestType sub-category
	importantItemQuestGroupTypeMainChapterId       = 4 // main-quest chapter ID
	importantItemQuestGroupTypeMainQuestId         = 5 // specific main-quest ID
	importantItemQuestGroupTypeSubQuestChapterId   = 6 // sub-quest (event) chapter ID
	importantItemQuestGroupTypeSubQuestId          = 7 // specific sub-quest (event) quest ID
	importantItemQuestGroupTypeMainQuestDifficulty = 8 // main-quest DifficultyType
)

// Quest type values as used by campaign.QuestType, duplicated here to avoid
// an import in the other direction.
const (
	importantItemQuestTypeMainQuest  = 1
	importantItemQuestTypeEventQuest = 2
)

type QuestTarget struct {
	QuestId        int32
	QuestType      int32
	EventQuestType int32
	ChapterId      int32
	// Difficulty is the main-quest DifficultyType (from
	// m_main_quest_sequence_group), 0 for non-main quests.
	Difficulty int32
}

// ImportantItemDropEffect is one pre-resolved drop bonus for a single important
// item effect row. Stored in ImportantItemCatalog indexed by ImportantItemId.
type ImportantItemDropEffect struct {
	CountPermil  int32 // from m_important_item_effect_drop_count
	RatePermil   int32 // from m_important_item_effect_drop_rate
	QuestFilters []importantItemQuestFilter
	ItemFilter   map[importantItemPossKey]bool // empty = all items
	StartMillis  int64                         // effect.StartDatetime (inclusive)
	EndMillis    int64                         // effect.EndDatetime (inclusive); 0 = no upper bound
}

type importantItemQuestFilter struct{ groupType, value int32 }
type importantItemPossKey struct{ possessionType, possessionId int32 }

// ImportantItemCatalog indexes drop effects by ImportantItemId (= PossessionId).
type ImportantItemCatalog struct {
	EffectByItemId map[int32][]ImportantItemDropEffect
	// MainQuestDifficultyByQuestId resolves a main-quest id to its
	// DifficultyType (1=Normal 2=Hard 3=Very Hard) for quest filter type 8.
	MainQuestDifficultyByQuestId map[int32]int32
}

func LoadImportantItemCatalog() *ImportantItemCatalog {
	cat := &ImportantItemCatalog{
		EffectByItemId:               map[int32][]ImportantItemDropEffect{},
		MainQuestDifficultyByQuestId: map[int32]int32{},
	}

	var skipped int // broken / unsupported effects that were silently skipped

	items, err := utils.ReadTable[EntityMImportantItem]("m_important_item")
	if err != nil {
		log.Printf("[ImportantItemCatalog] ERROR loading m_important_item: %v", err)
		return cat
	}
	log.Printf("[ImportantItemCatalog] loaded %d items from m_important_item", len(items))

	effects, err := utils.ReadTable[EntityMImportantItemEffect]("m_important_item_effect")
	if err != nil {
		log.Printf("[ImportantItemCatalog] ERROR loading m_important_item_effect: %v", err)
		return cat
	}
	log.Printf("[ImportantItemCatalog] loaded %d effects from m_important_item_effect", len(effects))

	dropCounts, err := utils.ReadTable[EntityMImportantItemEffectDropCount]("m_important_item_effect_drop_count")
	if err != nil {
		log.Printf("[ImportantItemCatalog] WARNING loading m_important_item_effect_drop_count: %v", err)
	} else {
		log.Printf("[ImportantItemCatalog] loaded %d drop counts from m_important_item_effect_drop_count", len(dropCounts))
	}

	dropRates, err := utils.ReadTable[EntityMImportantItemEffectDropRate]("m_important_item_effect_drop_rate")
	if err != nil {
		log.Printf("[ImportantItemCatalog] WARNING loading m_important_item_effect_drop_rate: %v", err)
	} else {
		log.Printf("[ImportantItemCatalog] loaded %d drop rates from m_important_item_effect_drop_rate", len(dropRates))
	}

	questGroupRows, err := utils.ReadTable[EntityMImportantItemEffectTargetQuestGroup]("m_important_item_effect_target_quest_group")
	if err != nil {
		log.Printf("[ImportantItemCatalog] WARNING loading m_important_item_effect_target_quest_group: %v", err)
	} else {
		log.Printf("[ImportantItemCatalog] loaded %d quest groups from m_important_item_effect_target_quest_group", len(questGroupRows))
	}

	itemGroupRows, err := utils.ReadTable[EntityMImportantItemEffectTargetItemGroup]("m_important_item_effect_target_item_group")
	if err != nil {
		log.Printf("[ImportantItemCatalog] WARNING loading m_important_item_effect_target_item_group: %v", err)
	} else {
		log.Printf("[ImportantItemCatalog] loaded %d item groups from m_important_item_effect_target_item_group", len(itemGroupRows))
	}

	// Index quest group filters by group id.
	questGroups := map[int32][]importantItemQuestFilter{}
	for _, r := range questGroupRows {
		questGroups[r.ImportantItemEffectTargetQuestGroupId] = append(
			questGroups[r.ImportantItemEffectTargetQuestGroupId],
			importantItemQuestFilter{r.ImportantItemEffectTargetQuestGroupType, r.TargetValue},
		)
	}
	// Index item group sets by group id.
	itemSets := map[int32]map[importantItemPossKey]bool{}
	for _, r := range itemGroupRows {
		if itemSets[r.ImportantItemEffectTargetItemGroupId] == nil {
			itemSets[r.ImportantItemEffectTargetItemGroupId] = map[importantItemPossKey]bool{}
		}
		itemSets[r.ImportantItemEffectTargetItemGroupId][importantItemPossKey{r.PossessionType, r.PossessionId}] = true
	}
	// Index drop tables by id.
	dcById := map[int32]EntityMImportantItemEffectDropCount{}
	for _, r := range dropCounts {
		dcById[r.ImportantItemEffectDropCountId] = r
	}
	drById := map[int32]EntityMImportantItemEffectDropRate{}
	for _, r := range dropRates {
		drById[r.ImportantItemEffectDropRateId] = r
	}
	// Index effects by effect id (= ImportantItemEffectId on the item row).
	effById := map[int32][]EntityMImportantItemEffect{}
	for _, e := range effects {
		effById[e.ImportantItemEffectId] = append(effById[e.ImportantItemEffectId], e)
	}

	for _, item := range items {
		if item.ImportantItemEffectId == 0 {
			continue
		}
		for _, eff := range effById[item.ImportantItemEffectId] {
			var resolved ImportantItemDropEffect
			switch eff.ImportantItemEffectType {
			case ImportantItemEffectTypeDropCount:
				dc, ok := dcById[eff.ImportantItemEffectTargetId]
				if !ok {
					skipped++
					log.Printf("[ImportantItemCatalog] skip item=%d effect=%d: missing drop-count target %d",
						item.ImportantItemId, eff.ImportantItemEffectId, eff.ImportantItemEffectTargetId)
					continue
				}
				resolved = ImportantItemDropEffect{
					CountPermil:  dc.CountPermil,
					QuestFilters: questGroups[dc.ImportantItemEffectTargetQuestGroupId],
					ItemFilter:   itemSets[dc.ImportantItemEffectTargetItemGroupId],
					StartMillis:  eff.StartDatetime,
					EndMillis:    eff.EndDatetime,
				}
				if len(resolved.QuestFilters) == 0 {
					skipped++
					log.Printf("[ImportantItemCatalog] skip item=%d effect=%d: empty quest-target group %d",
						item.ImportantItemId, eff.ImportantItemEffectId, dc.ImportantItemEffectTargetQuestGroupId)
					continue
				}
			case ImportantItemEffectTypeDropRate:
				dr, ok := drById[eff.ImportantItemEffectTargetId]
				if !ok {
					skipped++
					log.Printf("[ImportantItemCatalog] skip item=%d effect=%d: missing drop-rate target %d",
						item.ImportantItemId, eff.ImportantItemEffectId, eff.ImportantItemEffectTargetId)
					continue
				}
				resolved = ImportantItemDropEffect{
					RatePermil:   dr.RatePermil,
					QuestFilters: questGroups[dr.ImportantItemEffectTargetQuestGroupId],
					ItemFilter:   itemSets[dr.ImportantItemEffectTargetItemGroupId],
					StartMillis:  eff.StartDatetime,
					EndMillis:    eff.EndDatetime,
				}
				if len(resolved.QuestFilters) == 0 {
					skipped++
					log.Printf("[ImportantItemCatalog] skip item=%d effect=%d: empty quest-target group %d",
						item.ImportantItemId, eff.ImportantItemEffectId, dr.ImportantItemEffectTargetQuestGroupId)
					continue
				}
			default:
				// UnlockFunction and any unknown types are intentionally ignored
				// (client-side only or unsupported).
				continue
			}
			cat.EffectByItemId[item.ImportantItemId] = append(cat.EffectByItemId[item.ImportantItemId], resolved)
		}
	}

	// Resolve main-quest difficulty for quest filter type 8:
	// m_main_quest_sequence_group carries the DifficultyType of each sequence,
	// m_main_quest_sequence lists the quests inside a sequence.
	seqGroups, err := utils.ReadTable[EntityMMainQuestSequenceGroup]("m_main_quest_sequence_group")
	if err != nil {
		log.Printf("[ImportantItemCatalog] WARNING loading m_main_quest_sequence_group: %v", err)
	}
	sequences, err := utils.ReadTable[EntityMMainQuestSequence]("m_main_quest_sequence")
	if err != nil {
		log.Printf("[ImportantItemCatalog] WARNING loading m_main_quest_sequence: %v", err)
	}
	difficultyBySequenceId := map[int32]int32{}
	for _, sg := range seqGroups {
		difficultyBySequenceId[sg.MainQuestSequenceId] = sg.DifficultyType
	}
	for _, s := range sequences {
		if d, ok := difficultyBySequenceId[s.MainQuestSequenceId]; ok {
			cat.MainQuestDifficultyByQuestId[s.QuestId] = d
		}
	}

	log.Printf("[ImportantItemCatalog] loaded: %d items with drop effects, %d broken/unsupported effects skipped",
		len(cat.EffectByItemId), skipped)
	return cat
}

// Active reports whether the effect is within its valid time window.
// EndMillis == 0 means no upper bound (open-ended).
func (e *ImportantItemDropEffect) Active(nowMillis int64) bool {
	if nowMillis < e.StartMillis {
		return false
	}
	if e.EndMillis > 0 && nowMillis > e.EndMillis {
		return false
	}
	return true
}

// QuestMatches reports whether target is covered by this effect's quest
// filters. The filter rows form a whitelist of alternative targets, so a
// single matching row is enough (OR semantics, mirroring the campaign matcher
// in internal/campaign). An empty filter list matches every quest.
func (e *ImportantItemDropEffect) QuestMatches(target QuestTarget) bool {
	if len(e.QuestFilters) == 0 {
		return true
	}
	for _, f := range e.QuestFilters {
		switch f.groupType {
		case importantItemQuestGroupTypeWholeQuest:
			return true
		case importantItemQuestGroupTypeQuestType:
			if target.QuestType == f.value {
				return true
			}
		case importantItemQuestGroupTypeEventQuestType:
			if target.QuestType == importantItemQuestTypeEventQuest && target.EventQuestType == f.value {
				return true
			}
		case importantItemQuestGroupTypeMainChapterId:
			if target.QuestType == importantItemQuestTypeMainQuest && target.ChapterId == f.value {
				return true
			}
		case importantItemQuestGroupTypeMainQuestId:
			if target.QuestType == importantItemQuestTypeMainQuest && target.QuestId == f.value {
				return true
			}
		case importantItemQuestGroupTypeSubQuestChapterId:
			if target.QuestType == importantItemQuestTypeEventQuest && target.ChapterId == f.value {
				return true
			}
		case importantItemQuestGroupTypeSubQuestId:
			if target.QuestType == importantItemQuestTypeEventQuest && target.QuestId == f.value {
				return true
			}
		case importantItemQuestGroupTypeMainQuestDifficulty:
			if target.QuestType == importantItemQuestTypeMainQuest && target.Difficulty == f.value {
				return true
			}
		default:
			log.Printf("[ImportantItemBonus] QuestMatches WARNING: unknown groupType=%d", f.groupType)
		}
	}
	return false
}

// ItemMatches reports whether a drop item is covered by the item filter.
// An empty filter matches everything.
func (e *ImportantItemDropEffect) ItemMatches(possType, possId int32) bool {
	if len(e.ItemFilter) == 0 {
		return true
	}
	key := importantItemPossKey{possessionType: possType, possessionId: possId}
	return e.ItemFilter[key]
}

// Permil returns the combined drop bonus in parts per thousand.
// RatePermil and CountPermil are mutually exclusive per master-data row, but
// summing is safe when multiple effects stack. Application (including
// probabilistic rounding of the fractional part) happens in questflow.
func (e *ImportantItemDropEffect) Permil() int32 {
	return e.CountPermil + e.RatePermil
}
