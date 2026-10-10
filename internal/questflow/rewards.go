package questflow

import (
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"

	"lunar-tear/server/internal/campaign"
	"lunar-tear/server/internal/gameutil"
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
)

func shouldMultiplyQuestReward(possType model.PossessionType) bool {
	switch possType {
	case model.PossessionTypeParts,
		model.PossessionTypePartsEnhanced,
		model.PossessionTypeMaterial,
		model.PossessionTypeConsumableItem:
		return true
	default:
		return false
	}
}

// shouldDropReward returns true if the reward should be dropped.
// Items with rarity 40 have reduced drop chance in dark memory quests:
// - Daily dark quests: 5% drop chance
// - Regular dark quests: 1% drop chance
func (h *QuestHandler) shouldDropReward(questDef masterdata.EntityMQuest, rarityType int32) bool {
	// Apply probability check only for dark memory quests
	if !h.isDarkMemoryQuest(questDef.QuestId) {
		return true // Non-dark quests always drop everything
	}

	// For dark memory quests, rarity 40 items have reduced probability
	if rarityType == 40 {
		// Daily dark quests (DailyClearableCount > 0) have higher drop rate
		if questDef.DailyClearableCount > 0 {
			// 10% chance for daily dark quests
			return rand.Intn(50) == 0
		} else {
			// 1% chance for regular dark quests
			return rand.Intn(100) == 0
		}
	}

	// All other rarities in dark quests are always dropped
	return true
}

func scaleQuestRewardCount(possType model.PossessionType, count int32) int32 {
	if count <= 0 || !shouldMultiplyQuestReward(possType) {
		return count
	}
	return count * QuestRewardMultiplier
}

// scaleDropMaterialCount applies the quest reward multiplier to a drop, but
// only to materials of rarity 30 or below. High-rarity materials (rarity 40+)
// keep their authored master-data quantity; non-material drops are unaffected.
func scaleDropMaterialCount(grant RewardGrant, catalog *masterdata.MaterialCatalog) RewardGrant {
	if grant.Count <= 0 {
		return grant
	}
	switch grant.PossessionType {
	case model.PossessionTypeMaterial:
		if grant.RarityType <= 0 && catalog != nil {
			if mat, ok := catalog.All[grant.PossessionId]; ok {
				grant.RarityType = mat.RarityType
			}
		}
		grant.Count *= QuestRewardMultiplier
	case model.PossessionTypeConsumableItem:
		grant.Count *= QuestRewardMultiplier
	}
	return grant
}

func scaleQuestGoldReward(gold int32) int32 {
	if gold <= 0 {
		return gold
	}
	return gold * GoldBonusRate
}

func scaleRewardGrant(grant RewardGrant) RewardGrant {
	// Apply gem bonus to all gems
	if grant.PossessionType == model.PossessionTypeFreeGem || grant.PossessionType == model.PossessionTypePaidGem {
		grant.Count = grant.Count * RewardGemBonus
	}
	return grant
}

// consolidateRewards merges rewards of the same type/id into single entries
// This is used to show combined amounts (e.g., all Free Gems together)
func consolidateRewards(grants []RewardGrant) []RewardGrant {
	if len(grants) == 0 {
		return grants
	}

	// Map to track consolidated rewards by (PossessionType, PossessionId)
	consolidated := make(map[string]RewardGrant)
	order := []string{}

	for _, g := range grants {
		key := fmt.Sprintf("%d-%d", g.PossessionType, g.PossessionId)
		if existing, ok := consolidated[key]; ok {
			// Already have this type/id, just add to count
			existing.Count += g.Count
			consolidated[key] = existing
		} else {
			// New type/id, track the order for output
			order = append(order, key)
			consolidated[key] = g
		}
	}

	// Build result in order of first appearance
	result := make([]RewardGrant, 0, len(consolidated))
	for _, key := range order {
		result = append(result, consolidated[key])
	}
	return result
}

// scaleDropReward applies the server's ordinary quest multiplier to drops on
// every clear path — normal finish, auto orbit and skip alike. The reward
// multiplier only applies to materials of rarity 30 or below; Dark Memory
// quest material drops keep their authored quantities entirely. Gems receive
// their own bonus. EX Character Quests are Extra quests; their material drops
// deliberately retain the quantities authored in master data.
func (h *QuestHandler) scaleDropReward(target campaign.QuestTarget, grant RewardGrant) RewardGrant {
	if h.isDarkMemoryQuest(target.QuestId) &&
		(grant.PossessionType == model.PossessionTypeMaterial ||
			grant.PossessionType == model.PossessionTypeConsumableItem) {
		return grant
	}
	grant = scaleDropMaterialCount(grant, h.MaterialCatalog)
	return scaleRewardGrant(grant)
}

func (h *QuestHandler) isDarkMemoryQuest(questId int32) bool {
	return h != nil && h.QuestCatalog != nil && h.DarkMemoryQuestIds[questId]
}

// scaleFirstClearRewardCount leaves a first-clear memoir as one item. Memoirs
// are individual randomized inventory objects, so multiplying a guaranteed
// first-clear reward would create ten rolls where the quest promises one.
// Ascension books retain their exact master-data quantity (often 10), rather
// than being multiplied into 100.
// For first clear rewards, only gems receive bonus (if configured).
func (h *QuestHandler) scaleFirstClearRewardCount(target campaign.QuestTarget, possType model.PossessionType, possessionId, count int32) int32 {
	if possType == model.PossessionTypeParts || possType == model.PossessionTypePartsEnhanced {
		if count > 0 {
			return 1
		}
		return 0
	}
	if h.isDarkMemoryQuest(target.QuestId) &&
		(possType == model.PossessionTypeMaterial || possType == model.PossessionTypeConsumableItem) {
		return count
	}
	if possType == model.PossessionTypeMaterial && h.FirstClearUnmultipliedMaterialIds[possessionId] {
		return count
	}
	// Apply gem bonus to all gems (including first clear)
	if possType == model.PossessionTypeFreeGem || possType == model.PossessionTypePaidGem {
		if RewardGemBonus > 1 {
			return count * RewardGemBonus
		}
	}
	return count
}

func (h *QuestHandler) isQuestCleared(user *store.UserState, questId int32) bool {
	quest, ok := user.Quests[questId]
	if !ok {
		return false
	}
	return quest.QuestStateType == model.UserQuestStateTypeCleared
}

func appendMissionRewards(dst []RewardGrant, src []masterdata.EntityMQuestMissionReward) []RewardGrant {
	for _, r := range src {
		grant := RewardGrant{
			PossessionType: model.PossessionType(r.PossessionType),
			PossessionId:   r.PossessionId,
			Count:          r.Count,
			RarityType:     0,
		}
		grant = scaleRewardGrant(grant)
		dst = append(dst, grant)
	}
	return dst
}

func (h *QuestHandler) firstClearRewardGroupId(user *store.UserState, questDef masterdata.EntityMQuest) int32 {
	rewardGroupId := questDef.QuestFirstClearRewardGroupId
	for _, switchRow := range h.FirstClearRewardSwitchesByQuestId[questDef.QuestId] {
		if h.isQuestCleared(user, switchRow.SwitchConditionClearQuestId) {
			rewardGroupId = switchRow.QuestFirstClearRewardGroupId
			break
		}
	}
	return rewardGroupId
}

// evaluateFinishOutcome builds the reward list for one quest finish.
//
// isReplay is supplied by the caller rather than read from
// user.MainQuest.CurrentQuestFlowType: "am I replaying?" is a property of the
// flow that started this particular quest, and only the main-quest path owns
// that state. A replay flow deliberately stays set after its finish (the client
// needs it to keep the replay result UI), and event/extra/big-hunt quest starts
// never touch it — so reading it here made every non-main finish that happens
// while a replay flow is lingering look like a replay, silently dropping its
// first-clear popup, its mission rewards and its BigWin bonus.
func (h *QuestHandler) evaluateFinishOutcome(user *store.UserState, questId int32, target campaign.QuestTarget, nowMillis int64, oldMissionStates map[int32]bool, isReplay bool) FinishOutcome {
	outcome := FinishOutcome{}
	questState, ok := user.Quests[questId]
	if !ok {
		panic(fmt.Sprintf("unknown questId=%d for evaluateFinishOutcome", questId))
	}
	questDef, ok := h.QuestById[questId]
	if !ok {
		panic(fmt.Sprintf("unknown questId=%d for evaluateFinishOutcome", questId))
	}

	log.Printf("[FinishOutcome] quest=%d isReplay=%v firstRewardAlreadyGranted=%v", questId, isReplay, questState.IsRewardGranted)

	if !questState.IsRewardGranted && !isReplay {
		rewardGroupId := h.firstClearRewardGroupId(user, questDef)
		var firstClearConverted []RewardGrant
		for _, reward := range h.FirstClearRewardsByGroupId[rewardGroupId] {
			grant := RewardGrant{
				PossessionType: model.PossessionType(reward.PossessionType),
				PossessionId:   reward.PossessionId,
				Count:          h.scaleFirstClearRewardCount(target, model.PossessionType(reward.PossessionType), reward.PossessionId, reward.Count),
				RarityType:     0,
			}
			firstClearConverted = append(firstClearConverted, grant)
		}
		outcome.FirstClearRewards = consolidateRewards(firstClearConverted)
	}

	if isReplay && questDef.QuestReplayFlowRewardGroupId > 0 {
		for _, reward := range h.ReplayFlowRewardsByGroupId[questDef.QuestReplayFlowRewardGroupId] {
			outcome.ReplayFlowFirstClearRewards = append(outcome.ReplayFlowFirstClearRewards, RewardGrant{
				PossessionType: model.PossessionType(reward.PossessionType),
				PossessionId:   reward.PossessionId,
				Count:          scaleQuestRewardCount(model.PossessionType(reward.PossessionType), reward.Count),
				RarityType:     0,
			})
		}
	}

	// Mission rewards / BigWin are first-clear concepts. Reference
	// IUserQuestMissionTable has no rows for replay-variant ids (30000+):
	// the popup is empty on replay in the original game.
	if !isReplay {
		clearedThisAttemptCount := 0
		regularMissionCount := 0
		for _, questMissionId := range h.MissionIdsByQuestId[questId] {
			missionDef, ok := h.MissionById[questMissionId]
			if !ok || model.QuestMissionConditionType(missionDef.QuestMissionConditionType) == model.QuestMissionConditionTypeComplete {
				continue
			}
			regularMissionCount++

			key := store.QuestMissionKey{QuestId: questId, QuestMissionId: questMissionId}
			mission := user.QuestMissions[key]
			wasAlreadyClear := oldMissionStates[questMissionId]
			isNowClear := mission.IsClear

			// Grant rewards only for missions newly cleared THIS ATTEMPT (was false, now true)
			if !wasAlreadyClear && isNowClear {
				clearedThisAttemptCount++
				outcome.MissionClearRewards = appendMissionRewards(
					outcome.MissionClearRewards,
					h.MissionRewardsByMissionId[missionDef.QuestMissionRewardId],
				)
			}
		}

		// BigWin bonus: if all 3 regular missions cleared (including from previous attempts),
		// also unlock the "Complete" mission (always mission #1, condition type 9999).
		// But only award the bonus once (when Complete mission is not yet cleared).
		allRegularCleared := false
		if regularMissionCount > 0 {
			totalClearedCount := 0
			for _, questMissionId := range h.MissionIdsByQuestId[questId] {
				missionDef, ok := h.MissionById[questMissionId]
				if !ok || model.QuestMissionConditionType(missionDef.QuestMissionConditionType) == model.QuestMissionConditionTypeComplete {
					continue
				}
				key := store.QuestMissionKey{QuestId: questId, QuestMissionId: questMissionId}
				mission := user.QuestMissions[key]
				if mission.IsClear {
					totalClearedCount++
				}
			}
			allRegularCleared = totalClearedCount == regularMissionCount
			log.Printf("[BigWin] questId=%d regularMissionCount=%d totalClearedCount=%d allRegularCleared=%v", questId, regularMissionCount, totalClearedCount, allRegularCleared)
		}

		if allRegularCleared {
			for _, questMissionId := range h.MissionIdsByQuestId[questId] {
				missionDef, ok := h.MissionById[questMissionId]
				if !ok || model.QuestMissionConditionType(missionDef.QuestMissionConditionType) != model.QuestMissionConditionTypeComplete {
					continue
				}
				key := store.QuestMissionKey{QuestId: questId, QuestMissionId: questMissionId}
				mission := user.QuestMissions[key]
				log.Printf("[BigWin] Found Complete mission questMissionId=%d isClear=%v", questMissionId, mission.IsClear)
				if !mission.IsClear {
					// Mark the Complete mission as cleared and grant its reward (only once)
					mission.IsClear = true
					mission.ProgressValue = 1
					mission.LatestClearDatetime = nowMillis
					user.QuestMissions[key] = mission
					outcome.MissionClearCompleteRewards = appendMissionRewards(
						outcome.MissionClearCompleteRewards,
						h.MissionRewardsByMissionId[missionDef.QuestMissionRewardId],
					)
					outcome.BigWinClearedQuestMissionIds = append(outcome.BigWinClearedQuestMissionIds, questMissionId)
					log.Printf("[BigWin] Granted Complete mission bonus for questId=%d", questId)
				}
			}
			outcome.IsBigWin = len(outcome.BigWinClearedQuestMissionIds) > 0
			log.Printf("[BigWin] IsBigWin=%v rewardCount=%d", outcome.IsBigWin, len(outcome.MissionClearCompleteRewards))
		}
	}

	drops := h.computeDropRewards(questDef, target, nowMillis)
	outcome.DropRewards = h.applyImportantItemDropBonuses(drops, user.ImportantItems, target, nowMillis)

	outcome.DropRewards = consolidateRewards(outcome.DropRewards)

	for i := range outcome.DropRewards {
		outcome.DropRewards[i] = h.scaleDropReward(target, outcome.DropRewards[i])
	}

	// Resonant weapon quest bonuses (flat AdditionalCount). Applied AFTER
	// scaleDropReward so event medals are not multiplied by QuestRewardMultiplier.
	if bonusGrants := h.questBonusDropRewards(user, questDef, nowMillis); len(bonusGrants) > 0 {
		for i := range bonusGrants {
			bonusGrants[i] = h.scaleDropReward(target, bonusGrants[i])
			logQuestLoot(questId, "quest-bonus", bonusGrants[i].PossessionType, bonusGrants[i].PossessionId, bonusGrants[i].Count)
		}
		outcome.DropRewards = append(outcome.DropRewards, bonusGrants...)
	}

	// Consolidate remaining reward types for display (combine like items).
	// FirstClearRewards are already consolidated when created above.
	// Mission rewards are intentionally NOT consolidated: each cleared
	// mission's reward (e.g. free gems) is displayed individually.
	outcome.ReplayFlowFirstClearRewards = consolidateRewards(outcome.ReplayFlowFirstClearRewards)
	outcome.DropRewards = consolidateRewards(outcome.DropRewards)

	return outcome
}

var autoSaleRarityTiers = map[int32]bool{10: true, 20: true, 30: true, 40: true, 50: true}

// Rarity tiers (10..50) and ranks (1..5) are disjoint, so the delimited values
// are classified by range — independent of the client's map key or delimiter.
func parseAutoSaleRules(settings map[int32]store.AutoSaleSettingState) (raritySet, rankSet map[int32]bool) {
	raritySet = map[int32]bool{}
	rankSet = map[int32]bool{}
	for _, s := range settings {
		for _, n := range extractInts(s.PossessionAutoSaleItemValue) {
			switch {
			case autoSaleRarityTiers[n]:
				raritySet[n] = true
			case n >= 1 && n <= 5:
				rankSet[n] = true
			}
		}
	}
	return raritySet, rankSet
}

func extractInts(s string) []int32 {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r < '0' || r > '9' })
	out := make([]int32, 0, len(fields))
	for _, f := range fields {
		if v, err := strconv.Atoi(f); err == nil {
			out = append(out, int32(v))
		}
	}
	return out
}

// grantDropRewards grants the computed drops and returns the list to show in
// the reward popup. Parts (memoirs) entries carry only the drop-preview
// representative ids; together they define the quest's drop pool (every
// piece of every advertised set), from which PartsDropCountPerQuest memoirs
// are drawn deck-style inside GrantOrSellPartsPoolDrop. The popup list is
// rebuilt from the actual rolls (grouped by rolled id) instead of echoing
// the preview back to the client.
func (h *QuestHandler) grantDropRewards(user *store.UserState, drops []RewardGrant, questDef masterdata.EntityMQuest, raritySet, rankSet map[int32]bool, nowMillis int64) []RewardGrant {
	out := make([]RewardGrant, 0, len(drops))
	var (
		wiredIds   []int32
		partsType  model.PossessionType
		rarityCap  int32
		multiplier int32 = 1
	)
	for i := range drops {
		d := drops[i]
		if d.PossessionType == model.PossessionTypeParts || d.PossessionType == model.PossessionTypePartsEnhanced {
			// Pool the preview rows: RarityType carries the quest's max
			// droppable rarity; campaign drop-rate multipliers can raise
			// Count above 1 and scale the whole haul.
			if len(wiredIds) == 0 {
				partsType = d.PossessionType
			}
			wiredIds = append(wiredIds, d.PossessionId)
			if d.RarityType > rarityCap {
				rarityCap = d.RarityType
			}
			if d.Count > multiplier {
				multiplier = d.Count
			}
			continue
		}
		if h.shouldDropReward(questDef, d.RarityType) {
			h.applyRewardPossession(user, d.PossessionType, d.PossessionId, d.Count, nowMillis)
			out = append(out, d)
		}
	}
	if len(wiredIds) == 0 {
		return out
	}
	type rolledKey struct {
		partsId int32
		sold    bool
	}
	rolledIdx := map[rolledKey]int{}
	rolls := h.Granter.GrantOrSellPartsPoolDrop(user, wiredIds, PartsDropCountPerQuest*multiplier, rarityCap, raritySet, rankSet, nowMillis)
	for _, r := range rolls {
		key := rolledKey{partsId: r.PartsId, sold: r.Sold}
		if j, ok := rolledIdx[key]; ok {
			out[j].Count++
			continue
		}
		rarity := int32(0)
		if p, ok := h.PartsById[r.PartsId]; ok {
			rarity = p.RarityType
		}
		rolledIdx[key] = len(out)
		out = append(out, RewardGrant{
			PossessionType: partsType,
			PossessionId:   r.PartsId,
			Count:          1,
			IsAutoSale:     r.Sold,
			RarityType:     rarity,
		})
	}
	return out
}

func (h *QuestHandler) computeDropRewards(questDef masterdata.EntityMQuest, target campaign.QuestTarget, nowMillis int64) []RewardGrant {
	var drops []RewardGrant
	var dropRate campaign.DropRateMul
	if h.Campaigns != nil {
		dropRate = h.Campaigns.QuestDropRate(target, h.campaignFilter(nowMillis))
	}

	// Event chapters advertise a memoir set (one Parts per series) in their
	// display item group, but the per-quest drop data only wires one of them, so
	// the rest are unobtainable. When a quest has a chapter memoir set, drop the
	// quest's single memoir Parts and grant the chapter's full advertised set.
	chapterMemoirs := h.ChapterMemoirsByQuestId[questDef.QuestId]

	// The parts entries wired into the drop data mirror the quest's drop
	// preview: one representative piece per memoir set, each shown at a single
	// rarity. The highest advertised rarity is the quest's actual drop cap for
	// every set, so collect it up front and stamp it on each parts grant.
	maxPartsRarity := int32(0)
	considerPartsRarity := func(partId int32) {
		if p, ok := h.PartsById[partId]; ok && p.RarityType > maxPartsRarity {
			maxPartsRarity = p.RarityType
		}
	}
	if questDef.QuestPickupRewardGroupId != 0 {
		for _, dropId := range h.PickupRewardIdsByGroupId[questDef.QuestPickupRewardGroupId] {
			if bdr, ok := h.BattleDropRewardById[dropId]; ok {
				pt := model.PossessionType(bdr.PossessionType)
				if pt == model.PossessionTypeParts || pt == model.PossessionTypePartsEnhanced {
					considerPartsRarity(bdr.PossessionId)
				}
			}
		}
	}
	for _, partId := range chapterMemoirs {
		considerPartsRarity(partId)
	}

	if questDef.QuestPickupRewardGroupId != 0 {
		for _, dropId := range h.PickupRewardIdsByGroupId[questDef.QuestPickupRewardGroupId] {
			if bdr, ok := h.BattleDropRewardById[dropId]; ok {
				pt := model.PossessionType(bdr.PossessionType)
				if len(chapterMemoirs) > 0 && (pt == model.PossessionTypeParts || pt == model.PossessionTypePartsEnhanced) {
					continue // replaced by the chapter memoir set below
				}
				rarityType := int32(0)
				if pt == model.PossessionTypeMaterial && h.MaterialCatalog != nil {
					if mat, ok := h.MaterialCatalog.All[bdr.PossessionId]; ok {
						rarityType = mat.RarityType
					}
				}
				if pt == model.PossessionTypeParts || pt == model.PossessionTypePartsEnhanced {
					rarityType = maxPartsRarity
				}
				drops = append(drops, RewardGrant{
					PossessionType: pt,
					PossessionId:   bdr.PossessionId,
					Count:          dropRate.Apply(bdr.Count),
					RarityType:     rarityType,
				})
			}
		}
	}
	for _, partId := range chapterMemoirs {
		drops = append(drops, RewardGrant{
			PossessionType: model.PossessionTypeParts,
			PossessionId:   partId,
			Count:          1,
			RarityType:     maxPartsRarity,
		})
	}
	drops = h.appendBonusDrops(drops, target, nowMillis)
	return drops
}

func (h *QuestHandler) applyExpRewards(user *store.UserState, questId int32, nowMillis int64) {
	questDef, ok := h.QuestById[questId]
	if !ok {
		return
	}

	oldLevel := user.Status.Level
	user.Status.Exp += questDef.UserExp
	user.Status.Level, user.Status.Exp = gameutil.LevelAndCap(user.Status.Exp, h.UserExpThresholds)
	log.Printf("[applyExpRewards] questId=%d user: +%d exp -> total=%d level=%d", questId, questDef.UserExp, user.Status.Exp, user.Status.Level)

	//Fix stamina reset when account levels up
	if user.Status.Level > oldLevel {
		if maxStamina, ok := h.MaxStaminaByLevel[user.Status.Level]; ok {
			maxStaminaMillis := maxStamina * 1000
			if user.Status.StaminaMilliValue < maxStaminaMillis {
				store.ReplenishStamina(user, maxStaminaMillis, nowMillis)
			}
		}
		// Advance "Reach player level N" missions (condition type 22).
		if h.MissionCatalog != nil {
			applyPlayerLevelMissionProgress(user, h.MissionCatalog, user.Status.Level, nowMillis)
		}
	}

	if h.RentalQuestIds[questId] {
		log.Printf("[applyExpRewards] questId=%d skipping character/costume exp (rental deck)", questId)
		return
	}

	if questDef.CharacterExp == 0 && questDef.CostumeExp == 0 {
		return
	}

	deckCostumeUuids, deckCharacterIds := h.resolveDeckUnits(user, questId)
	if deckCostumeUuids == nil {
		log.Printf("[applyExpRewards] questId=%d skipping character/costume exp (deck not resolved)", questId)
		return
	}

	if questDef.CharacterExp != 0 {
		for id := range deckCharacterIds {
			row := user.Characters[id]
			row.Exp += questDef.CharacterExp
			row.Level, row.Exp = gameutil.LevelAndCap(row.Exp, h.CharacterExpThresholds)
			user.Characters[id] = row
			log.Printf("[applyExpRewards] questId=%d character=%d: +%d exp -> total=%d level=%d", questId, id, questDef.CharacterExp, row.Exp, row.Level)
		}
	}

	if questDef.CostumeExp != 0 {
		for key := range deckCostumeUuids {
			row := user.Costumes[key]
			cm, ok := h.CostumeById[row.CostumeId]
			if !ok {
				continue
			}
			var maxLevel int32
			if maxLevelFunc, hasMax := h.CostumeMaxLevelByRarity[cm.RarityType]; hasMax {
				maxLevel = maxLevelFunc.Evaluate(row.LimitBreakCount) +
					h.CharacterRebirth.CostumeLevelLimitUp(cm.CharacterId, user.CharacterRebirths[cm.CharacterId].RebirthCount)
				if row.Level >= maxLevel {
					log.Printf("[applyExpRewards] questId=%d costume=%d (key=%s): at max level %d, skipping", questId, row.CostumeId, key, row.Level)
					continue
				}
			}
			row.Exp += questDef.CostumeExp
			if thresholds, ok := h.CostumeExpByRarity[cm.RarityType]; ok {
				row.Level, row.Exp = gameutil.ApplyExpWithMaxLevel(row.Exp, thresholds, maxLevel)
			}
			user.Costumes[key] = row
			log.Printf("[applyExpRewards] questId=%d costume=%d (key=%s): +%d exp -> total=%d level=%d", questId, row.CostumeId, key, questDef.CostumeExp, row.Exp, row.Level)
		}
	}
}

func (h *QuestHandler) resolveDeckUnits(user *store.UserState, questId int32) (costumeUuids map[string]bool, characterIds map[int32]bool) {
	dn := user.Quests[questId].UserDeckNumber
	if dn == 0 {
		return nil, nil
	}
	deck, ok := user.Decks[store.DeckKey{DeckType: model.DeckTypeQuest, UserDeckNumber: dn}]
	if !ok {
		return nil, nil
	}

	costumeUuids = make(map[string]bool)
	characterIds = make(map[int32]bool)
	for _, dcUuid := range []string{deck.UserDeckCharacterUuid01, deck.UserDeckCharacterUuid02, deck.UserDeckCharacterUuid03} {
		if dcUuid == "" {
			continue
		}
		dc, ok := user.DeckCharacters[dcUuid]
		if !ok || dc.UserCostumeUuid == "" {
			continue
		}
		costumeUuids[dc.UserCostumeUuid] = true
		if costume, ok := user.Costumes[dc.UserCostumeUuid]; ok {
			if cm, ok := h.CostumeById[costume.CostumeId]; ok {
				characterIds[cm.CharacterId] = true
			}
		}
	}

	if len(costumeUuids) == 0 {
		return nil, nil
	}
	return costumeUuids, characterIds
}

func (h *QuestHandler) applyExpAndGoldRewards(user *store.UserState, questId int32, nowMillis int64) {
	questDef, ok := h.QuestById[questId]
	if !ok {
		return
	}

	h.applyExpRewards(user, questId, nowMillis)

	if questDef.Gold != 0 {
		goldReward := scaleQuestGoldReward(questDef.Gold)
		user.ConsumableItems[h.Config.ConsumableItemIdForGold] += goldReward
		log.Printf("[applyQuestRewards] questId=%d gold: +%d -> total=%d", questId, goldReward, user.ConsumableItems[h.Config.ConsumableItemIdForGold])
	}
}

func (h *QuestHandler) applyFirstClearItemRewards(user *store.UserState, questId int32, target campaign.QuestTarget, nowMillis int64) {
	questDef, ok := h.QuestById[questId]
	if !ok {
		return
	}
	rewardGroupId := h.firstClearRewardGroupId(user, questDef)
	for _, reward := range h.FirstClearRewardsByGroupId[rewardGroupId] {
		grant := RewardGrant{
			PossessionType: model.PossessionType(reward.PossessionType),
			PossessionId:   reward.PossessionId,
			Count:          h.scaleFirstClearRewardCount(target, model.PossessionType(reward.PossessionType), reward.PossessionId, reward.Count),
			RarityType:     0,
		}
		logQuestLoot(questId, "first-clear", grant.PossessionType, grant.PossessionId, grant.Count)
		h.grantQuestReward(user, grant, nowMillis)
	}
}

func (h *QuestHandler) applyQuestRewards(user *store.UserState, questId int32, nowMillis int64) {
	h.applyExpAndGoldRewards(user, questId, nowMillis)
	h.applyFirstClearItemRewards(user, questId, h.targetForMain(questId), nowMillis)
}

func (h *QuestHandler) applyRewardPossession(user *store.UserState, possType model.PossessionType, possId, count int32, nowMillis int64) {
	h.Granter.GrantFull(user, possType, possId, count, nowMillis)
}

func (h *QuestHandler) grantQuestReward(user *store.UserState, reward RewardGrant, nowMillis int64) {
	if reward.Count <= 0 {
		return
	}
	if reward.PossessionType == model.PossessionTypeParts || reward.PossessionType == model.PossessionTypePartsEnhanced {
		for i := int32(0); i < reward.Count; i++ {
			h.Granter.GrantFull(user, reward.PossessionType, reward.PossessionId, 1, nowMillis)
		}
		return
	}
	h.applyRewardPossession(user, reward.PossessionType, reward.PossessionId, reward.Count, nowMillis)
}

// possessionTypeLabel gives a human-readable name for a possession type, for
// diagnostic logging.
func possessionTypeLabel(t model.PossessionType) string {
	switch t {
	case model.PossessionTypeCostume:
		return "Costume"
	case model.PossessionTypeCostumeEnhanced:
		return "CostumeEnh"
	case model.PossessionTypeWeapon:
		return "Weapon"
	case model.PossessionTypeWeaponEnhanced:
		return "WeaponEnh"
	case model.PossessionTypeCompanion:
		return "Companion"
	case model.PossessionTypeCompanionEnhanced:
		return "CompanionEnh"
	case model.PossessionTypeParts:
		return "Parts"
	case model.PossessionTypePartsEnhanced:
		return "PartsEnh"
	case model.PossessionTypeMaterial:
		return "Material"
	case model.PossessionTypeConsumableItem:
		return "Consumable"
	case model.PossessionTypeImportantItem:
		return "Important"
	case model.PossessionTypePremiumItem:
		return "Premium"
	case model.PossessionTypePaidGem:
		return "PaidGem"
	case model.PossessionTypeFreeGem:
		return "FreeGem"
	default:
		return fmt.Sprintf("Type%d", int32(t))
	}
}

// logQuestLoot logs one reward and the source it came from, for diagnosing what
// a quest actually grants.
func logQuestLoot(questId int32, source string, possType model.PossessionType, possId, count int32) {
	log.Printf("[QuestLoot] quest=%d source=%-18s type=%-11s id=%-8d count=%d",
		questId, source, possessionTypeLabel(possType), possId, count)
}

func logQuestLootList(questId int32, source string, grants []RewardGrant) {
	for _, g := range grants {
		logQuestLoot(questId, source, g.PossessionType, g.PossessionId, g.Count)
	}
}

func (h *QuestHandler) grantWeaponStoryUnlock(user *store.UserState, weaponId, storyIndex int32, nowMillis int64) bool {
	return store.GrantWeaponStoryUnlock(user, weaponId, storyIndex, nowMillis)
}

var tutorialCompanionChoices = map[int32]int32{
	1: 2,  // bear + fire (Cat=1, Attr=2)
	2: 1,  // bear + wind (Cat=1, Attr=6)
	3: 7,  // doll + fire (Cat=3, Attr=2)
	4: 10, // doll + wind (Cat=3, Attr=6)
}

func (h *QuestHandler) ApplyTutorialReward(user *store.UserState, tutorialType model.TutorialType, choiceId int32, nowMillis int64) []RewardGrant {
	switch tutorialType {
	case model.TutorialTypeCompanion:
		return h.applyCompanionTutorialReward(user, choiceId, nowMillis)
	default:
		return nil
	}
}

func (h *QuestHandler) applyCompanionTutorialReward(user *store.UserState, choiceId int32, nowMillis int64) []RewardGrant {
	companionId, ok := tutorialCompanionChoices[choiceId]
	if !ok {
		log.Printf("[QuestHandler] unknown companion tutorial choiceId=%d", choiceId)
		return nil
	}
	h.Granter.GrantCompanion(user, companionId, nowMillis)
	return []RewardGrant{{
		PossessionType: model.PossessionTypeCompanion,
		PossessionId:   companionId,
		Count:          1,
		RarityType:     0,
	}}
}

func (h *QuestHandler) BattleDropRewards(questId int32) []masterdata.BattleDropInfo {
	return h.BattleDropsByQuestId[questId]
}

func (h *QuestHandler) grantWeaponStoryUnlocksForQuestScene(user *store.UserState, questId int32, resultType model.QuestResultType, nowMillis int64) []int32 {
	var changedIds []int32
	if resultType == model.QuestResultTypeHalfResult {
		questDef, ok := h.QuestById[questId]
		if !ok {
			return nil
		}
		rewardGroupId := h.firstClearRewardGroupId(user, questDef)
		for _, reward := range h.FirstClearRewardsByGroupId[rewardGroupId] {
			if model.PossessionType(reward.PossessionType) != model.PossessionTypeWeapon {
				continue
			}
			weaponId := reward.PossessionId
			weapon, ok := h.WeaponById[weaponId]
			if !ok || weapon.WeaponStoryReleaseConditionGroupId == 0 {
				continue
			}
			groupId := weapon.WeaponStoryReleaseConditionGroupId
			for _, cond := range h.ReleaseConditionsByGroupId[groupId] {
				if model.WeaponStoryReleaseConditionType(cond.WeaponStoryReleaseConditionType) == model.WeaponStoryReleaseConditionTypeAcquisition && cond.ConditionValue == 0 {
					if h.grantWeaponStoryUnlock(user, weaponId, cond.StoryIndex, nowMillis) {
						changedIds = append(changedIds, weaponId)
					}
				}
			}
		}
		return changedIds
	}
	if resultType == model.QuestResultTypeFullResult {
		for groupId, conditions := range h.ReleaseConditionsByGroupId {
			for _, cond := range conditions {
				if model.WeaponStoryReleaseConditionType(cond.WeaponStoryReleaseConditionType) == model.WeaponStoryReleaseConditionTypeQuestClear && cond.ConditionValue == questId {
					for _, weaponId := range h.WeaponIdsByReleaseConditionGroupId[groupId] {
						if h.grantWeaponStoryUnlock(user, weaponId, cond.StoryIndex, nowMillis) {
							changedIds = append(changedIds, weaponId)
						}
					}
					break
				}
			}
		}
	}
	return changedIds
}

// missionConditionPlayerLevel is the MissionClearConditionType for
// "Reach player level N" missions (type 22 in EntityMMission).
const missionConditionPlayerLevel int32 = 22

// applyPlayerLevelMissionProgress advances every active "Reach player level N"
// mission whose threshold the player's new level has met or exceeded.
// Uses CurrentValue semantics: progress is updated only when newLevel exceeds
// the stored ProgressValue, so each threshold triggers exactly once.
func applyPlayerLevelMissionProgress(user *store.UserState, cat *masterdata.MissionCatalog, newLevel int32, nowMillis int64) {
	if cat == nil || newLevel <= 0 {
		return
	}
	const clearStatus int32 = 2 // model.MissionProgressStatusTypeClear
	for _, mission := range cat.ActiveMissionsAt(nowMillis) {
		if mission.MissionClearConditionType != missionConditionPlayerLevel {
			continue
		}
		progress := user.Missions[mission.MissionId]
		if progress.MissionProgressStatusType >= clearStatus {
			continue
		}
		if newLevel <= progress.ProgressValue {
			continue
		}
		if progress.MissionId == 0 {
			progress.MissionId = mission.MissionId
			progress.StartDatetime = nowMillis
		}
		progress.ProgressValue = newLevel
		progress.LatestVersion = nowMillis
		if progress.ProgressValue >= mission.ClearConditionValue {
			progress.ProgressValue = mission.ClearConditionValue
			progress.MissionProgressStatusType = clearStatus
			progress.ClearDatetime = nowMillis
			log.Printf("[Mission] mission %d CLEARED by player level %d", mission.MissionId, newLevel)
		} else {
			progress.MissionProgressStatusType = 1 // InProgress
		}
		user.Missions[mission.MissionId] = progress
	}
	// The type-25 "clear N missions" meta-counter intentionally advances only
	// when rewards are claimed (ReceiveMissionRewardsById), not at clear time.
}

func (h *QuestHandler) applyImportantItemDropBonuses(
	drops []RewardGrant,
	userImportantItems map[int32]int32,
	target campaign.QuestTarget,
	nowMillis int64,
) []RewardGrant {
	if h.ImportantItems == nil {
		return drops
	}
	if len(userImportantItems) == 0 {
		return drops
	}

	// Convert campaign.QuestTarget to masterdata.QuestTarget
	masterTarget := masterdata.QuestTarget{
		QuestId:        target.QuestId,
		QuestType:      int32(target.QuestType),
		EventQuestType: target.EventQuestType,
		ChapterId:      target.ChapterId,
	}
	if target.QuestType == campaign.QuestTypeMainQuest {
		masterTarget.Difficulty = h.ImportantItems.MainQuestDifficultyByQuestId[target.QuestId]
	}

	// Sum the permil bonus of every matching *active* effect per drop, then
	// apply once with probabilistic rounding. Applying each effect separately
	// with truncating integer division silently discarded every sub-100% bonus
	// on count-1 drops (1 * 1500 / 1000 = 1), which is what almost all wired
	// drop rows are.
	permilByDrop := make([]int32, len(drops))
	for itemId, count := range userImportantItems {
		if count <= 0 {
			continue
		}
		effects, ok := h.ImportantItems.EffectByItemId[itemId]
		if !ok {
			continue
		}
		for _, eff := range effects {
			if !eff.Active(nowMillis) {
				continue
			}
			permil := eff.Permil()
			if permil == 0 || !eff.QuestMatches(masterTarget) {
				continue
			}
			for i := range drops {
				if eff.ItemMatches(int32(drops[i].PossessionType), drops[i].PossessionId) {
					permilByDrop[i] += permil
					log.Printf("[ImportantItemBonus] itemId=%d quest=%d matched type=%s id=%d permil=+%d",
						itemId, target.QuestId,
						possessionTypeLabel(drops[i].PossessionType), drops[i].PossessionId, permil)
				}
			}
		}
	}
	for i := range drops {
		if permilByDrop[i] <= 0 || drops[i].Count <= 0 {
			continue
		}
		before := drops[i].Count
		drops[i].Count = applyPermilBonus(before, permilByDrop[i])
		if drops[i].Count != before {
			log.Printf("[ImportantItemBonus] quest=%d type=%s id=%d permil=%d count %d->%d",
				target.QuestId,
				possessionTypeLabel(drops[i].PossessionType), drops[i].PossessionId,
				permilByDrop[i], before, drops[i].Count)
		}
	}
	return drops
}

// applyPermilBonus adds count*permil/1000 extra items. The fractional
// remainder becomes a chance for one more item (e.g. +50% on a single drop
// grants a second one half the time), mimicking the original game's
// probabilistic drop-rate bonuses on a server with deterministic drops.
func applyPermilBonus(count, permil int32) int32 {
	extra := int64(count) * int64(permil)
	whole := extra / 1000
	if frac := extra % 1000; frac > 0 && rand.Int63n(1000) < frac {
		whole++
	}
	return count + int32(whole)
}
