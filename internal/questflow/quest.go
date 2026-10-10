package questflow

import (
	"fmt"
	"log"

	"lunar-tear/server/internal/campaign"
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
)

func (h *QuestHandler) initQuestState(user *store.UserState, questId int32) {
	quest := user.Quests[questId]
	quest.QuestId = questId
	user.Quests[questId] = quest

	for _, missionId := range h.MissionIdsByQuestId[questId] {
		key := store.QuestMissionKey{QuestId: questId, QuestMissionId: missionId}
		mission := user.QuestMissions[key]
		mission.QuestId = questId
		mission.QuestMissionId = missionId
		user.QuestMissions[key] = mission
	}
}

func isMainQuestPlayable(quest masterdata.EntityMQuest) bool {
	if quest.IsRunInTheBackground {
		// A background quest is still actively played — and must NOT be
		// auto-cleared on start — when it carries battle content (a non-zero
		// recommended deck power, e.g. quests 500/515/30515). Pure cutscene
		// background quests have RecommendedDeckPower == 0.
		return quest.RecommendedDeckPower > 0
	}
	return quest.IsCountedAsQuest
}

func (h *QuestHandler) clearQuestMissions(user *store.UserState, questId int32, nowMillis int64) {
	if _, ok := h.QuestById[questId]; !ok {
		return
	}

	clearedCount := 0
	totalCount := 0
	// "Force quest missions" (the octo /information options page): every challenge
	// mission of the quest being cleared right now counts as achieved, whatever
	// the battle actually did. Nothing else is touched — no retroactive pass over
	// past quests, no power requirement. Because the missions land in
	// user.QuestMissions as ordinary clears, the existing outcome logic below
	// pays their authored rewards once (only newly-cleared ones) and the
	// "Clear all missions" BigWin bonus falls out of it naturally.
	forceClear := user.Options.ForceQuestMissions
	for _, missionId := range h.MissionIdsByQuestId[questId] {
		key := store.QuestMissionKey{QuestId: questId, QuestMissionId: missionId}
		mission, ok := h.MissionById[missionId]
		if !ok {
			// Skip missions not in masterdata (like Complete missions with type 9999)
			// They are handled separately via BigWin logic
			continue
		}

		// Skip Complete missions (type 9999) - they get special handling
		if model.QuestMissionConditionType(mission.QuestMissionConditionType) == model.QuestMissionConditionTypeComplete {
			continue
		}
		totalCount++

		isClear := forceClear || h.evaluateQuestMissionCondition(user, questId, mission)
		qm := user.QuestMissions[key]
		if isClear {
			qm.IsClear = true
			qm.ProgressValue = 1
			qm.LatestClearDatetime = nowMillis
		} else {
			// Do NOT overwrite already-cleared missions. Only mark as not clear if it wasn't already clear.
			if !qm.IsClear {
				qm.IsClear = false
				qm.ProgressValue = 0
			}
		}
		if qm.IsClear {
			clearedCount++
		}
		user.QuestMissions[key] = qm
	}
	log.Printf("[QuestMissions] quest=%d evaluated: %d/%d cleared (force=%v) (stats: costumeSkills=%d weaponSkills=%d companionSkills=%d crits=%d combo=%d maxDmg=%d recover=%d party=%d alive=%d)",
		questId, clearedCount, totalCount, forceClear,
		user.Battle.LastCostumeSkillUsedCount, user.Battle.LastWeaponSkillUsedCount,
		user.Battle.LastCompanionSkillUsedCount, user.Battle.LastCriticalCount,
		user.Battle.LastComboCount, user.Battle.LastComboMaxDamage,
		user.Battle.LastTotalRecoverPoint, user.Battle.LastCostumePartySize, user.Battle.LastCostumeAliveCount)
}

// evaluateQuestMissionCondition checks whether a quest mission's condition was
// satisfied based on the deck configuration and the battle statistics stored in
// user.Battle. Quest missions are evaluated ONLY during quest clear, checking
// deck setup (before battle) and battle results (after battle).
// Returns true when the mission should be marked cleared.
func (h *QuestHandler) evaluateQuestMissionCondition(user *store.UserState, questId int32, m masterdata.EntityMQuestMission) bool {
	ct := model.QuestMissionConditionType(m.QuestMissionConditionType)

	switch ct {
	// Complete mission (meta-mission that clears when all others are cleared)
	case model.QuestMissionConditionTypeComplete:
		return true

	// ---- Type 1: Deaths <= X ----
	case model.QuestMissionConditionTypeLessThanOrEqualXPeopleNotAlive:
		deathCount := user.Battle.LastCostumePartySize - user.Battle.LastCostumeAliveCount
		result := deathCount <= m.ConditionValue
		log.Printf("[EvalMission] type=1_DeathsLE: partySize=%d aliveCount=%d deaths=%d threshold=%d result=%v", user.Battle.LastCostumePartySize, user.Battle.LastCostumeAliveCount, deathCount, m.ConditionValue, result)
		return result

	// ---- Type 2: Max Damage >= X ----
	case model.QuestMissionConditionTypeMaxDamage:
		result := user.Battle.LastComboMaxDamage >= m.ConditionValue
		log.Printf("[EvalMission] type=2_MaxDamage: maxDamage=%d threshold=%d result=%v", user.Battle.LastComboMaxDamage, m.ConditionValue, result)
		return result

	// ---- Type 4: Specific Character ID in Deck ----
	case model.QuestMissionConditionTypeSpecifiedCharacterIsInDeck:
		return h.deckHasSpecifiedCharacter(user, questId, m.ConditionValue)

	// ---- Type 5: Main Weapon with Specific Attribute ----
	case model.QuestMissionConditionTypeSpecifiedAttributeMainWeaponIsInDeck:
		return h.deckHasAnyMainWeaponAttribute(user, questId, m.ConditionValue)

	// ---- Type 6: Costume Skills Used >= X ----
	case model.QuestMissionConditionTypeGreaterThanOrEqualXCostumeSkillUseCount:
		result := user.Battle.LastCostumeSkillUsedCount >= m.ConditionValue
		log.Printf("[EvalMission] type=6_CostumeSkillGE: count=%d threshold=%d result=%v", user.Battle.LastCostumeSkillUsedCount, m.ConditionValue, result)
		return result

	// ---- Type 7: Weapon Skills Used >= X ----
	case model.QuestMissionConditionTypeGreaterThanOrEqualXWeaponSkillUseCount:
		result := user.Battle.LastWeaponSkillUsedCount >= m.ConditionValue
		log.Printf("[EvalMission] type=7_WeaponSkillGE: count=%d threshold=%d result=%v", user.Battle.LastWeaponSkillUsedCount, m.ConditionValue, result)
		return result

	// ---- Type 8: Companion Skills Used >= X ----
	case model.QuestMissionConditionTypeGreaterThanOrEqualXCompanionSkillUseCount:
		result := user.Battle.LastCompanionSkillUsedCount >= m.ConditionValue
		log.Printf("[EvalMission] type=8_CompanionSkillGE: count=%d threshold=%d result=%v", user.Battle.LastCompanionSkillUsedCount, m.ConditionValue, result)
		return result

	// ---- Type 10: Any Character with Their Skillful Weapon ----
	// Check if ANY character in deck has the required skillful weapon type
	case model.QuestMissionConditionTypeCostumeSkillfulWeaponAnyCharacter:
		log.Printf("[EvalMission] type=10_CostumeSkillfulWeaponAny: checking for skillful weapon type=%d", m.ConditionValue)
		return h.deckHasAnyCharacterWithSkillfulWeapon(user, questId, m.ConditionValue)

	// ---- Type 19: All Main Weapons have Specific Attribute ----
	case model.QuestMissionConditionTypeSpecifiedAttributeMainWeaponAllCharacter:
		result := h.deckAllMainWeaponsHaveAttribute(user, questId, m.ConditionValue)
		log.Printf("[EvalMission] type=19_AllMainWeaponAttribute: attribute=%d result=%v", m.ConditionValue, result)
		return result

	// ---- Type 49: Party Size <= X ----
	case model.QuestMissionConditionTypeDeckCostumeNumLe:
		return h.deckCostumeCount(user, questId, m.ConditionValue, "le")

	// ---- Type 50: Critical Hits >= X ----
	case model.QuestMissionConditionTypeCriticalCountGe:
		result := user.Battle.LastCriticalCount >= m.ConditionValue
		log.Printf("[EvalMission] type=50_CriticalGE: count=%d threshold=%d result=%v", user.Battle.LastCriticalCount, m.ConditionValue, result)
		return result

	// ---- Type 51: All Party Members' HP >= X% ----
	case model.QuestMissionConditionTypeMinHpPercentageGe:
		// Uses the final-wave HP snapshot collected in FinishWave. Anyone dead
		// fails the mission; otherwise every occupied slot must be at or above
		// the threshold percentage.
		partySize := user.Battle.LastCostumePartySize
		if partySize == 0 {
			log.Printf("[EvalMission] type=51_MinHpPercent: no battle party info, result=false")
			return false
		}
		deathCount := partySize - user.Battle.LastCostumeAliveCount
		minHp := int32(101)
		for i := int32(0); i < partySize && i < 3; i++ {
			if user.Battle.LastCostumeHpPercent[i] < minHp {
				minHp = user.Battle.LastCostumeHpPercent[i]
			}
		}
		result := deathCount == 0 && minHp >= m.ConditionValue
		log.Printf("[EvalMission] type=51_MinHpPercent: threshold=%d deaths=%d minHp=%d result=%v", m.ConditionValue, deathCount, minHp, result)
		return result

	// ---- Type 52: Combo Chain >= X ----
	case model.QuestMissionConditionTypeComboCountGe:
		result := user.Battle.LastComboCount >= m.ConditionValue
		log.Printf("[EvalMission] type=52_ComboGE: count=%d threshold=%d result=%v", user.Battle.LastComboCount, m.ConditionValue, result)
		return result

	// ---- Type 54: Costume Skills Used <= X ----
	case model.QuestMissionConditionTypeLessThanOrEqualXCostumeSkillUseCount:
		result := user.Battle.LastCostumeSkillUsedCount <= m.ConditionValue
		log.Printf("[EvalMission] type=54_CostumeSkillLE: count=%d threshold=%d result=%v", user.Battle.LastCostumeSkillUsedCount, m.ConditionValue, result)
		return result

	// ---- Type 55: Weapon Skills Used <= X ----
	case model.QuestMissionConditionTypeLessThanOrEqualXWeaponSkillUseCount:
		result := user.Battle.LastWeaponSkillUsedCount <= m.ConditionValue
		log.Printf("[EvalMission] type=55_WeaponSkillLE: count=%d threshold=%d result=%v", user.Battle.LastWeaponSkillUsedCount, m.ConditionValue, result)
		return result

	// ---- Type 56: Companion Skills Used <= X ----
	case model.QuestMissionConditionTypeLessThanOrEqualXCompanionSkillUseCount:
		result := user.Battle.LastCompanionSkillUsedCount <= m.ConditionValue
		log.Printf("[EvalMission] type=56_CompanionSkillLE: count=%d threshold=%d result=%v", user.Battle.LastCompanionSkillUsedCount, m.ConditionValue, result)
		return result

	// ---- Type 57: Don't Use Recovery Skills ----
	case model.QuestMissionConditionTypeWithoutRecoverySkill:
		// Type 57: Mission passes if NO recovery skills or abilities were used during the battle
		// We check TotalRecoverPoint: if > 0, recovery actions were performed
		// This covers both costume skills and abilities with recovery actions
		result := user.Battle.LastTotalRecoverPoint == 0
		if !result {
			log.Printf("[EvalMission] type=57_NoRecovery: FAILED - totalRecoverPoint=%d (recovery was used)", user.Battle.LastTotalRecoverPoint)
		} else {
			log.Printf("[EvalMission] type=57_NoRecovery: PASSED - no recovery used")
		}
		return result

	// ---- Type 61: Character from Specific Group in Deck (uses QuestMissionConditionValueGroupId) ----
	case model.QuestMissionConditionTypeCharacterContainAll:
		log.Printf("[EvalMission] type=61_CharacterContainGroup: using group=%d", m.QuestMissionConditionValueGroupId)
		return h.deckContainsCharacterFromGroup(user, questId, m.QuestMissionConditionValueGroupId)

	// ---- Type 65: Deck Covers All Skillful Weapon Types from Group ----
	// ConditionValue is always 0 in masterdata; the required weapon types
	// (e.g. {sword, greatsword}) are listed in the condition value group.
	case model.QuestMissionConditionTypeCostumeSkillfulWeaponContainAll:
		log.Printf("[EvalMission] type=65_CostumeSkillfulWeaponAll: using group=%d", m.QuestMissionConditionValueGroupId)
		return h.deckCoversAllSkillfulWeaponTypes(user, questId, m.QuestMissionConditionValueGroupId)

	default:
		log.Printf("[EvalMission] WARNING: Unhandled quest mission condition type=%d, returning false for safety", m.QuestMissionConditionType)
		return false
	}
}

// deckAllMainWeaponsHaveAttribute checks if ALL characters in deck have main weapons
// with the specified attribute. Used for Type 19.
func (h *QuestHandler) deckAllMainWeaponsHaveAttribute(user *store.UserState, questId int32, attributeType int32) bool {
	// First, check if we have battle info (during pause or post-battle)
	battleWeaponCount := 0
	battleMatchCount := 0
	for i := 0; i < 3; i++ {
		if user.Battle.LastMainWeaponIds[i] != 0 {
			battleWeaponCount++
			if attr, ok := h.WeaponAttributeById[user.Battle.LastMainWeaponIds[i]]; ok && attr == attributeType {
				battleMatchCount++
			}
		}
	}

	// Trust the battle snapshot only when it covers the whole party; a partial
	// snapshot (weapon lookup failed for some slot) falls back to the deck.
	if battleWeaponCount > 0 && battleWeaponCount == int(user.Battle.LastCostumePartySize) {
		return battleMatchCount == battleWeaponCount
	}

	// If no battle info, fall back to current deck
	questState := user.Quests[questId]
	deckNumber := questState.UserDeckNumber
	if deckNumber == 0 {
		deckNumber = 1
	}
	deckKey := store.DeckKey{DeckType: model.DeckTypeQuest, UserDeckNumber: deckNumber}
	deck, ok := user.Decks[deckKey]
	if !ok {
		return false
	}
	uuids := [3]string{deck.UserDeckCharacterUuid01, deck.UserDeckCharacterUuid02, deck.UserDeckCharacterUuid03}

	partySize := 0
	for _, uuid := range uuids {
		if uuid != "" {
			partySize++
		}
	}
	if partySize == 0 {
		return false
	}

	matchCount := 0
	for _, uuid := range uuids {
		if uuid == "" {
			continue
		}
		dc, ok := user.DeckCharacters[uuid]
		if !ok {
			return false
		}
		if w, ok := user.Weapons[dc.MainUserWeaponUuid]; ok {
			if attr, ok := h.WeaponAttributeById[w.WeaponId]; ok && attr == attributeType {
				matchCount++
			} else {
				return false
			}
		} else {
			return false
		}
	}
	return matchCount == partySize
}

// deckContainsCharacterFromGroup checks if deck contains ALL characters from a specific group.
// Used for Type 61 (character group-based missions).
// groupId contains the QuestMissionConditionValueGroupId which maps to a list of character IDs.
// The mission passes if ALL characters in the group are in the deck.
func (h *QuestHandler) deckContainsCharacterFromGroup(user *store.UserState, questId int32, groupId int32) bool {
	// Get the list of character IDs for this group
	characterIds, ok := h.QuestMissionConditionValueGroupsByGroupId[groupId]
	if !ok {
		log.Printf("[EvalMission] type=61_CharacterContainGroup: group=%d not found in catalog", groupId)
		return false
	}

	if len(characterIds) == 0 {
		log.Printf("[EvalMission] type=61_CharacterContainGroup: group=%d is empty", groupId)
		return true // Empty group means condition is met
	}

	questState := user.Quests[questId]
	deckNumber := questState.UserDeckNumber
	if deckNumber == 0 {
		deckNumber = 1
	}
	deckKey := store.DeckKey{DeckType: model.DeckTypeQuest, UserDeckNumber: deckNumber}
	deck, ok := user.Decks[deckKey]
	if !ok {
		return false
	}
	uuids := [3]string{deck.UserDeckCharacterUuid01, deck.UserDeckCharacterUuid02, deck.UserDeckCharacterUuid03}

	// Build set of character IDs in deck
	deckCharacterIds := make(map[int32]bool)
	for _, uuid := range uuids {
		if uuid == "" {
			continue
		}
		dc, ok := user.DeckCharacters[uuid]
		if !ok {
			continue
		}
		if costume, ok := user.Costumes[dc.UserCostumeUuid]; ok {
			if cm, ok := h.CostumeById[costume.CostumeId]; ok {
				deckCharacterIds[cm.CharacterId] = true
			}
		}
	}

	// Check if ALL required characters are in deck
	for _, requiredCharId := range characterIds {
		if !deckCharacterIds[requiredCharId] {
			log.Printf("[EvalMission] type=61_CharacterContainGroup: missing character id=%d from group=%d", requiredCharId, groupId)
			return false
		}
	}

	log.Printf("[EvalMission] type=61_CharacterContainGroup: all %d characters from group=%d found in deck", len(characterIds), groupId)
	return true
}

// deckHasAnyCharacterWithSkillfulWeapon checks if ANY character in deck uses a weapon
// matching their SkillfulWeaponType (from their costume).
// Used for Type 10 (CostumeSkillfulWeaponAnyCharacter).
func (h *QuestHandler) deckHasAnyCharacterWithSkillfulWeapon(user *store.UserState, questId int32, skillfulWeaponType int32) bool {
	questState := user.Quests[questId]
	deckNumber := questState.UserDeckNumber
	if deckNumber == 0 {
		deckNumber = 1
	}
	deckKey := store.DeckKey{DeckType: model.DeckTypeQuest, UserDeckNumber: deckNumber}
	deck, ok := user.Decks[deckKey]
	if !ok {
		return false
	}
	uuids := [3]string{deck.UserDeckCharacterUuid01, deck.UserDeckCharacterUuid02, deck.UserDeckCharacterUuid03}

	// Check if ANY character in deck has the required skillful weapon type
	for _, uuid := range uuids {
		if uuid == "" {
			continue
		}
		dc, ok := user.DeckCharacters[uuid]
		if !ok {
			continue
		}
		// Get costume to check skillful weapon type
		if costume, ok := user.Costumes[dc.UserCostumeUuid]; ok {
			if cm, ok := h.CostumeById[costume.CostumeId]; ok {
				// Simply check if character has the required skillful weapon type
				if cm.SkillfulWeaponType == skillfulWeaponType {
					log.Printf("[EvalMission] type=10_SkillfulWeapon: found character with skillful weapon type=%d", skillfulWeaponType)
					return true
				}
			}
		}
	}
	log.Printf("[EvalMission] type=10_SkillfulWeapon: no character with skillful weapon type=%d found", skillfulWeaponType)
	return false
}

// deckCoversAllSkillfulWeaponTypes checks that for EVERY skillful weapon type
// listed in the condition value group the deck contains at least one costume
// with that SkillfulWeaponType. Used for Type 65 (CostumeSkillfulWeaponContainAll):
// masterdata keeps ConditionValue at 0 and stores the required weapon types
// (two per mission, e.g. {sword, greatsword}) in the value group table.
func (h *QuestHandler) deckCoversAllSkillfulWeaponTypes(user *store.UserState, questId int32, groupId int32) bool {
	requiredTypes, ok := h.QuestMissionConditionValueGroupsByGroupId[groupId]
	if !ok {
		log.Printf("[EvalMission] type=65_CostumeSkillfulWeaponAll: group=%d not found in catalog", groupId)
		return false
	}
	if len(requiredTypes) == 0 {
		return true // Empty group means condition is met
	}

	questState := user.Quests[questId]
	deckNumber := questState.UserDeckNumber
	if deckNumber == 0 {
		deckNumber = 1
	}
	deckKey := store.DeckKey{DeckType: model.DeckTypeQuest, UserDeckNumber: deckNumber}
	deck, ok := user.Decks[deckKey]
	if !ok {
		return false
	}
	uuids := [3]string{deck.UserDeckCharacterUuid01, deck.UserDeckCharacterUuid02, deck.UserDeckCharacterUuid03}

	// Collect the skillful weapon types present in the deck
	deckTypes := make(map[int32]bool)
	for _, uuid := range uuids {
		if uuid == "" {
			continue
		}
		dc, ok := user.DeckCharacters[uuid]
		if !ok {
			continue
		}
		if costume, ok := user.Costumes[dc.UserCostumeUuid]; ok {
			if cm, ok := h.CostumeById[costume.CostumeId]; ok {
				deckTypes[cm.SkillfulWeaponType] = true
			}
		}
	}

	// Every required type must be covered by at least one deck member
	for _, wt := range requiredTypes {
		if !deckTypes[wt] {
			log.Printf("[EvalMission] type=65_CostumeSkillfulWeaponAll: missing skillful weapon type=%d from group=%d", wt, groupId)
			return false
		}
	}
	log.Printf("[EvalMission] type=65_CostumeSkillfulWeaponAll: all %d skillful weapon types from group=%d covered", len(requiredTypes), groupId)
	return true
}

// deckHasAnyMainWeaponAttribute checks if at least one character in the quest
// deck has a main weapon matching the given attribute type.
func (h *QuestHandler) deckHasAnyMainWeaponAttribute(user *store.UserState, questId int32, attributeType int32) bool {
	// Type 5 ("at least one main weapon of the required element") previously
	// emitted no [EvalMission] log line, so a wrong result was invisible in the
	// request log. Log the decisive inputs (the battle weapon ids and their
	// resolved attributes) alongside the result to make mis-evaluations diagnosable.
	result := h.evalAnyMainWeaponAttribute(user, questId, attributeType)
	log.Printf("[EvalMission] type=5_AnyMainWeaponAttribute: quest=%d wantAttr=%d battleWeapons=%v battleAttrs=[%d %d %d] result=%v",
		questId, attributeType, user.Battle.LastMainWeaponIds,
		h.WeaponAttributeById[user.Battle.LastMainWeaponIds[0]],
		h.WeaponAttributeById[user.Battle.LastMainWeaponIds[1]],
		h.WeaponAttributeById[user.Battle.LastMainWeaponIds[2]], result)
	return result
}

func (h *QuestHandler) evalAnyMainWeaponAttribute(user *store.UserState, questId int32, attributeType int32) bool {
	// First, check if we have battle info (during pause or post-battle)
	for i := 0; i < 3; i++ {
		if user.Battle.LastMainWeaponIds[i] != 0 {
			if attr, ok := h.WeaponAttributeById[user.Battle.LastMainWeaponIds[i]]; ok && attr == attributeType {
				return true // Found at least one from battle
			}
		}
	}

	// If no battle info, fall back to current deck
	questState := user.Quests[questId]
	deckNumber := questState.UserDeckNumber
	if deckNumber == 0 {
		deckNumber = 1
	}
	deckKey := store.DeckKey{DeckType: model.DeckTypeQuest, UserDeckNumber: deckNumber}
	deck, ok := user.Decks[deckKey]
	if !ok {
		return false
	}
	uuids := [3]string{deck.UserDeckCharacterUuid01, deck.UserDeckCharacterUuid02, deck.UserDeckCharacterUuid03}
	for _, uuid := range uuids {
		if uuid == "" {
			continue
		}
		dc, ok := user.DeckCharacters[uuid]
		if !ok {
			continue
		}
		if w, ok := user.Weapons[dc.MainUserWeaponUuid]; ok {
			if attr, ok := h.WeaponAttributeById[w.WeaponId]; ok && attr == attributeType {
				return true // Found at least one
			}
		}
	}
	return false
}

func (h *QuestHandler) HandleQuestStart(user *store.UserState, questId int32, isBattleOnly, isMainFlow bool, userDeckNumber int32, nowMillis int64) {
	h.handleQuestStartInternal(user, questId, isBattleOnly, isMainFlow, userDeckNumber, false, nowMillis)
}

func (h *QuestHandler) HandleQuestStartReplay(user *store.UserState, questId int32, isBattleOnly bool, userDeckNumber int32, nowMillis int64) {
	h.handleQuestStartInternal(user, questId, isBattleOnly, false, userDeckNumber, true, nowMillis)
}

func (h *QuestHandler) handleQuestStartInternal(user *store.UserState, questId int32, isBattleOnly, isMainFlow bool, userDeckNumber int32, isReplayFlow bool, nowMillis int64) {
	quest, ok := h.QuestById[questId]
	if !ok {
		panic(fmt.Sprintf("unknown questId=%d for HandleQuestStart", questId))
	}

	h.initQuestState(user, questId)
	questState := user.Quests[questId]

	// A new attempt begins: battle statistics must start from zero. This is
	// the questflow-side reset; service handlers rely on it via HandleQuestStart.
	user.Battle.ResetQuestBattleStats()

	// Stamina is intentionally never consumed on this server — quest starts are free.

	questState.IsBattleOnly = isBattleOnly
	questState.UserDeckNumber = userDeckNumber

	isCleared := questState.QuestStateType == model.UserQuestStateTypeCleared
	isMenuPick := !isReplayFlow && !isMainFlow

	switch {
	case isMenuPick:
		// Snapshot the player's TRUE context first — including a replay flow they
		// were actively browsing on an already-cleared map. Restoring that snapshot
		// verbatim on finish is what returns them to the exact map they opened the
		// menu from, instead of ejecting them to the story frontier.
		snapshotMainQuestIfNeeded(user)
		// Then enter the pick itself in a clean MainFlow sandbox so its scene
		// updates land in the Progress* fields and never pollute the saved
		// ReplayFlow / MainFlow pointers.
		user.MainQuest.CurrentQuestFlowType = int32(model.QuestFlowTypeMainFlow)
		sceneId := h.menuPickSceneId(questId, isBattleOnly)
		user.MainQuest.ProgressQuestSceneId = sceneId
		user.MainQuest.ProgressHeadQuestSceneId = sceneId
		user.MainQuest.ProgressQuestFlowType = int32(model.QuestFlowTypeMainFlow)
		user.PortalCageStatus.IsCurrentProgress = false
		user.PortalCageStatus.LatestVersion = nowMillis
		user.SideStoryActiveProgress = store.SideStoryActiveProgress{LatestVersion: nowMillis}
		user.MainQuest.LatestVersion = nowMillis
		log.Printf("[HandleQuestStart] QuestMenuPick quest=%d isBattleOnly=%v scene=%d cleared=%v",
			questId, isBattleOnly, sceneId, isCleared)
		if isCleared {
			questState.QuestStateType = model.UserQuestStateTypeActive
			questState.LatestStartDatetime = nowMillis
			user.Quests[questId] = questState
			return
		}

	case isReplayFlow:
		h.applyReplayStart(user, quest, questId, isBattleOnly, nowMillis)
		return
	}

	if isCleared {
		snapshotMainQuestIfNeeded(user)
		log.Printf("[HandleQuestStart] cleared quest=%d isMenuPick=%v isMainFlow=%v snapshot.active=%v scene=%d head=%d",
			questId, isMenuPick, !isReplayFlow && !isMenuPick, user.MainQuest.SavedContext.Active,
			user.MainQuest.CurrentQuestSceneId, user.MainQuest.HeadQuestSceneId)
		questState.QuestStateType = model.UserQuestStateTypeActive
		questState.LatestStartDatetime = nowMillis
		user.Quests[questId] = questState
		return
	}

	if isMainQuestPlayable(quest) {
		user.MainQuest.CurrentQuestFlowType = int32(model.QuestFlowTypeMainFlow)
		questState.QuestStateType = model.UserQuestStateTypeActive
		questState.LatestStartDatetime = nowMillis
	} else {
		questState.QuestStateType = model.UserQuestStateTypeCleared
		questState.ClearCount = 1
		questState.DailyClearCount = 1
		questState.LastClearDatetime = nowMillis
		if sceneIds := h.SceneIdsByQuestId[questId]; len(sceneIds) > 0 {
			firstSceneId := sceneIds[0]
			prevSceneId := user.MainQuest.CurrentQuestSceneId
			h.advanceMainFlowScene(user, questId, firstSceneId)
			user.MainQuest.CurrentQuestFlowType = int32(model.QuestFlowTypeMainFlow)
			log.Printf("[HandleQuestStart] background quest %d auto-cleared, scene %d -> %d", questId, prevSceneId, firstSceneId)
		}
	}
	user.Quests[questId] = questState
}

func snapshotMainQuestIfNeeded(user *store.UserState) {
	if user.MainQuest.SavedContext.Active {
		return
	}
	// Record the player's real current flow. If they were browsing an
	// already-cleared map in a replay flow, the snapshot keeps that replay flow
	// (and the untouched ReplayFlow pointers) so HandleQuestFinish can resume them
	// exactly where they were. Eject protection is handled structurally (the head
	// only advances for a quest that was explicitly started), not by rewriting the
	// saved flow here.
	flow := user.MainQuest.CurrentQuestFlowType
	user.MainQuest.SavedContext = store.SavedQuestContext{
		Active:                  true,
		CurrentQuestSceneId:     user.MainQuest.CurrentQuestSceneId,
		HeadQuestSceneId:        user.MainQuest.HeadQuestSceneId,
		CurrentMainQuestRouteId: user.MainQuest.CurrentMainQuestRouteId,
		MainQuestSeasonId:       user.MainQuest.MainQuestSeasonId,
		IsReachedLastQuestScene: user.MainQuest.IsReachedLastQuestScene,
		PortalCageInProgress:    user.PortalCageStatus.IsCurrentProgress,
		CurrentQuestFlowType:    flow,
	}
}

func (h *QuestHandler) applyReplayStart(user *store.UserState, quest masterdata.EntityMQuest, questId int32, isBattleOnly bool, nowMillis int64) {
	flowType := h.replayFlowTypeFromQuestId(user, questId)
	if model.IsReplayQuestFlowType(user.MainQuest.CurrentQuestFlowType) {
		flowType = model.QuestFlowType(user.MainQuest.CurrentQuestFlowType)
	}
	user.MainQuest.CurrentQuestFlowType = int32(flowType)
	user.MainQuest.LatestVersion = nowMillis

	questState := user.Quests[questId]
	questState.LatestStartDatetime = nowMillis

	if isMainQuestPlayable(quest) {
		questState.QuestStateType = model.UserQuestStateTypeActive
		user.Quests[questId] = questState
	} else {
		if questState.QuestStateType != model.UserQuestStateTypeCleared {
			questState.QuestStateType = model.UserQuestStateTypeCleared
			questState.ClearCount++
			questState.DailyClearCount++
			questState.LastClearDatetime = nowMillis
		}
		user.Quests[questId] = questState
		if sceneIds := h.SceneIdsByQuestId[questId]; len(sceneIds) > 0 {
			h.advanceReplayFlowScene(user, sceneIds[0])
		}
	}

	log.Printf("[HandleQuestStart] replay quest=%d flowType=%s isBattleOnly=%v playable=%v current=%d head=%d",
		questId, flowType, isBattleOnly, isMainQuestPlayable(quest),
		user.MainQuest.ReplayFlowCurrentQuestSceneId,
		user.MainQuest.ReplayFlowHeadQuestSceneId)
}

func (h *QuestHandler) menuPickSceneId(questId int32, isBattleOnly bool) int32 {
	if isBattleOnly {
		if v, ok := h.BattleOnlyTargetSceneIdFor(questId); ok {
			return v
		}
	}
	if scenes := h.SceneIdsByQuestId[questId]; len(scenes) > 0 {
		return scenes[0]
	}
	return 0
}

func (h *QuestHandler) applyQuestVictory(user *store.UserState, questId int32, target campaign.QuestTarget, outcome *FinishOutcome, nowMillis int64, wasReplay bool) {
	questState := user.Quests[questId]
	h.applyExpAndGoldRewards(user, questId, nowMillis)
	if !questState.IsRewardGranted {
		if !wasReplay {
			h.applyFirstClearItemRewards(user, questId, target, nowMillis)
			outcome.ChangedWeaponStoryIds = append(outcome.ChangedWeaponStoryIds,
				h.grantWeaponStoryUnlocksForQuestScene(user, questId, model.QuestResultTypeHalfResult, nowMillis)...)
			outcome.ChangedWeaponStoryIds = append(outcome.ChangedWeaponStoryIds,
				h.grantWeaponStoryUnlocksForQuestScene(user, questId, model.QuestResultTypeFullResult, nowMillis)...)
		}
		questState.IsRewardGranted = true
	}

	// Mission rewards are granted outside the first-clear gate: outcome only
	// lists missions that flipped from not-cleared to cleared this finish, so
	// missions earned on a later attempt still pay out exactly once.
	logQuestLootList(questId, "mission", outcome.MissionClearRewards)
	for _, r := range outcome.MissionClearRewards {
		h.grantQuestReward(user, r, nowMillis)
	}
	logQuestLootList(questId, "mission-complete", outcome.MissionClearCompleteRewards)
	for _, r := range outcome.MissionClearCompleteRewards {
		h.grantQuestReward(user, r, nowMillis)
	}
	raritySet, rankSet := parseAutoSaleRules(user.AutoSaleSettings)
	logQuestLootList(questId, "drop", outcome.DropRewards)
	// Rebuild the drop list from the actual memoir rolls so the reward popup
	// shows what really dropped instead of the drop-preview representatives.
	questDef, _ := h.QuestById[questId]
	outcome.DropRewards = h.grantDropRewards(user, outcome.DropRewards, questDef, raritySet, rankSet, nowMillis)
	logQuestLootList(questId, "drop-rolled", outcome.DropRewards)
	logQuestLootList(questId, "replay-first-clear", outcome.ReplayFlowFirstClearRewards)
	for _, reward := range outcome.ReplayFlowFirstClearRewards {
		h.grantQuestReward(user, reward, nowMillis)
	}
	questState.QuestStateType = model.UserQuestStateTypeCleared
	questState.ClearCount++
	questState.DailyClearCount++
	questState.LastClearDatetime = nowMillis
	questState.IsBattleOnly = false
	user.Quests[questId] = questState
}

func (h *QuestHandler) finalizeChainPreviousQuest(user *store.UserState, questId int32, nowMillis int64) {
	if _, ok := h.QuestById[questId]; !ok {
		return
	}
	h.initQuestState(user, questId)
	questState := user.Quests[questId]
	if questState.QuestStateType == model.UserQuestStateTypeCleared {
		return
	}
	if !questState.IsRewardGranted {
		h.applyQuestRewards(user, questId, nowMillis)
		questState.IsRewardGranted = true
	}
	questState.QuestStateType = model.UserQuestStateTypeCleared
	questState.ClearCount++
	questState.DailyClearCount++
	questState.LastClearDatetime = nowMillis
	questState.IsBattleOnly = false
	user.Quests[questId] = questState
	h.clearQuestMissions(user, questId, nowMillis)
	log.Printf("[HandleMainQuestSceneProgress] finalized chain-previous quest %d (cleared)", questId)
}

func (h *QuestHandler) HandleQuestFinish(user *store.UserState, questId int32, isRetired, isAnnihilated bool, nowMillis int64) FinishOutcome {
	quest, ok := h.QuestById[questId]
	if !ok {
		panic(fmt.Sprintf("unknown questId=%d for HandleQuestFinish", questId))
	}

	h.initQuestState(user, questId)

	wasReplay := model.IsReplayQuestFlowType(user.MainQuest.CurrentQuestFlowType)
	wasMenuReplay := user.MainQuest.SavedContext.Active

	var outcome FinishOutcome
	if !isRetired && !isAnnihilated {
		// Save the OLD state of quest missions before evaluation
		oldMissionStates := make(map[int32]bool)
		for _, missionId := range h.MissionIdsByQuestId[questId] {
			key := store.QuestMissionKey{QuestId: questId, QuestMissionId: missionId}
			oldMissionStates[missionId] = user.QuestMissions[key].IsClear
		}

		// Evaluate and clear missions based on battle conditions
		h.clearQuestMissions(user, questId, nowMillis)

		// Pass old states so rewards.go can identify newly-cleared missions
		outcome = h.evaluateFinishOutcome(user, questId, h.targetForMain(questId), nowMillis, oldMissionStates, wasReplay)
		h.applyQuestVictory(user, questId, h.targetForMain(questId), &outcome, nowMillis, wasReplay)

		// A replay-flow finish must NOT move the MainFlow scene pointer: the
		// finished quest is a replay-variant (30000+) with no chapter, so a
		// replay scene left in CurrentQuestSceneId makes the client world map's
		// CalculatorWorldMap.GetCurrentSeasonId resolve chapter 0 and NRE. The
		// replay's own position is tracked in ReplayFlowCurrentQuestSceneId.
		//
		// Advance the head only for a quest the player explicitly started. When
		// the client chains into the next main quest after a replay (or a world-map
		// walk trigger fires an unselected quest), it finishes that quest with no
		// preceding StartMainQuest, so LastStartedQuestId no longer matches it.
		// Advancing the head for such an orphan finish is exactly what ejects the
		// player onto the last map.
		if isMainQuestPlayable(quest) && !wasMenuReplay && !wasReplay &&
			user.Battle.LastStartedQuestId == questId {
			lastSceneId := h.getLastMainFlowSceneId(questId)
			h.advanceMainFlowScene(user, questId, lastSceneId)
		}
	} else {
		// When retired or annihilated, restore menu replay snapshot if present
		log.Printf("[HandleQuestFinish] quest retired/annihilated: wasMenuReplay=%v, isRetired=%v, isAnnihilated=%v scene=%d head=%d savedScene=%d savedHead=%d",
			wasMenuReplay, isRetired, isAnnihilated,
			user.MainQuest.CurrentQuestSceneId, user.MainQuest.HeadQuestSceneId,
			user.MainQuest.SavedContext.CurrentQuestSceneId, user.MainQuest.SavedContext.HeadQuestSceneId)
		if wasMenuReplay {
			ctx := user.MainQuest.SavedContext
			user.MainQuest.CurrentQuestSceneId = ctx.CurrentQuestSceneId
			user.MainQuest.HeadQuestSceneId = ctx.HeadQuestSceneId
			user.MainQuest.CurrentMainQuestRouteId = ctx.CurrentMainQuestRouteId
			user.MainQuest.MainQuestSeasonId = ctx.MainQuestSeasonId
			user.MainQuest.IsReachedLastQuestScene = ctx.IsReachedLastQuestScene
			user.MainQuest.CurrentQuestFlowType = ctx.CurrentQuestFlowType
			user.PortalCageStatus.IsCurrentProgress = ctx.PortalCageInProgress
			user.PortalCageStatus.LatestVersion = nowMillis
			user.MainQuest.SavedContext = store.SavedQuestContext{}
			user.MainQuest.LatestVersion = nowMillis
			log.Printf("[HandleQuestFinish] retired: restored snapshot for quest %d (route=%d season=%d scene=%d head=%d cage=%v flow=%d)",
				questId, ctx.CurrentMainQuestRouteId, ctx.MainQuestSeasonId,
				ctx.CurrentQuestSceneId, ctx.HeadQuestSceneId, ctx.PortalCageInProgress, ctx.CurrentQuestFlowType)
		}
		// Reset quest state to Unknown on retire/annihilate to prevent blocking
		questState := user.Quests[questId]
		if questState.ClearCount > 0 {
			questState.QuestStateType = model.UserQuestStateTypeCleared
		} else {
			questState.QuestStateType = model.UserQuestStateTypeUnknown
		}
		questState.LatestVersion = nowMillis
		user.Quests[questId] = questState
		log.Printf("[HandleQuestFinish] retired/annihilated: reset quest=%d state=%d clearCount=%d scene=%d head=%d route=%d",
			questId, questState.QuestStateType, questState.ClearCount,
			user.MainQuest.CurrentQuestSceneId, user.MainQuest.HeadQuestSceneId,
			user.MainQuest.CurrentMainQuestRouteId)
	}

	// No stamina refund on retire: stamina is never consumed on this server.

	// Reset progress state (temporary markers)
	user.MainQuest.ProgressQuestSceneId = 0
	user.MainQuest.ProgressHeadQuestSceneId = 0
	if !wasReplay {
		// Keep replay flow types on replay finish so the client's
		// Story.ApplyNewestPlayingScene keeps _isReplayed=true (popup result UI).
		user.MainQuest.ProgressQuestFlowType = 0
		user.MainQuest.CurrentQuestFlowType = int32(model.QuestFlowTypeUnknown)
	}

	if wasMenuReplay && !isRetired && !isAnnihilated {
		ctx := user.MainQuest.SavedContext
		user.MainQuest.CurrentQuestSceneId = ctx.CurrentQuestSceneId
		user.MainQuest.HeadQuestSceneId = ctx.HeadQuestSceneId
		user.MainQuest.CurrentMainQuestRouteId = ctx.CurrentMainQuestRouteId
		user.MainQuest.MainQuestSeasonId = ctx.MainQuestSeasonId
		user.MainQuest.IsReachedLastQuestScene = ctx.IsReachedLastQuestScene
		// Resume the player's original flow verbatim (see the snapshot ordering in
		// HandleQuestStart). If they were on an already-cleared map in a replay flow
		// this returns them there; if they were on the frontier it returns them to
		// the frontier. The ReplayFlow pointers were preserved through the pick's
		// Progress* sandbox, so they must NOT be zeroed here — zeroing them (and
		// forcing MainFlow) is exactly what dropped every replay back onto the last
		// map. SuppressReplayFlow is no longer needed because the head only advances
		// for a quest that was explicitly started.
		user.MainQuest.CurrentQuestFlowType = ctx.CurrentQuestFlowType
		user.PortalCageStatus.IsCurrentProgress = ctx.PortalCageInProgress
		user.PortalCageStatus.LatestVersion = nowMillis

		// Clear only the sandbox markers the menu pick used.
		user.MainQuest.ProgressQuestSceneId = 0
		user.MainQuest.ProgressHeadQuestSceneId = 0
		user.MainQuest.ProgressQuestFlowType = 0

		user.MainQuest.SavedContext = store.SavedQuestContext{}
		user.MainQuest.LatestVersion = nowMillis
		log.Printf("[HandleQuestFinish] restored snapshot for quest %d (route=%d season=%d scene=%d head=%d cage=%v flow=%d)",
			questId, ctx.CurrentMainQuestRouteId, ctx.MainQuestSeasonId,
			ctx.CurrentQuestSceneId, ctx.HeadQuestSceneId, ctx.PortalCageInProgress, ctx.CurrentQuestFlowType)
	}

	return outcome
}

func (h *QuestHandler) HandleQuestSkip(user *store.UserState, questId, skipCount int32, nowMillis int64) (FinishOutcome, bool) {
	questDef, ok := h.QuestById[questId]
	if !ok {
		log.Printf("[HandleQuestSkip] unknown questId=%d, skip ignored", questId)
		return FinishOutcome{}, false
	}
	if skipCount <= 0 {
		return FinishOutcome{}, false
	}

	// A full memoir inventory never blocks skips: drops overflow exactly
	// like on finish paths (sub-SSR sold for gold, SSR mailed to the gift
	// box), so tickets are charged and rewards are paid as usual.

	// Skips cost skip tickets only — stamina is never consumed on this server.
	skipTicketId := h.Config.ConsumableItemIdForQuestSkipTicket
	if user.ConsumableItems[skipTicketId] < skipCount {
		log.Printf("[HandleQuestSkip] questId=%d not enough skip tickets: have=%d need=%d",
			questId, user.ConsumableItems[skipTicketId], skipCount)
		return FinishOutcome{}, false
	}

	target := h.targetForMain(questId)
	user.ConsumableItems[skipTicketId] -= skipCount
	raritySet, rankSet := parseAutoSaleRules(user.AutoSaleSettings)
	var allDrops []RewardGrant
	for range skipCount {
		drops := h.computeDropRewards(questDef, target, nowMillis)
		drops = h.applyImportantItemDropBonuses(drops, user.ImportantItems, target, nowMillis)
		// Mirror the finish path: drops are scaled (gem bonus) exactly as on a
		// normal clear, so skipping never pays out less than playing.
		for i := range drops {
			drops[i] = h.scaleDropReward(target, drops[i])
		}

		// Resonant weapon bonuses (flat); do not run through scaleDropReward.
		if bonusGrants := h.questBonusDropRewards(user, questDef, nowMillis); len(bonusGrants) > 0 {
			drops = append(drops, bonusGrants...)
		}

		drops = h.grantDropRewards(user, drops, questDef, raritySet, rankSet, nowMillis)
		allDrops = append(allDrops, drops...)

		if questDef.Gold != 0 {
			user.ConsumableItems[h.Config.ConsumableItemIdForGold] += scaleQuestGoldReward(questDef.Gold)
		}
		h.applyExpRewards(user, questId, nowMillis)
	}

	questState := user.Quests[questId]
	questState.ClearCount += skipCount
	questState.DailyClearCount += skipCount
	questState.LastClearDatetime = nowMillis
	user.Quests[questId] = questState

	// Consolidate rewards for display
	allDrops = consolidateRewards(allDrops)
	log.Printf("[HandleQuestSkip] questId=%d skipCount=%d drops=%d gold=%d", questId, skipCount, len(allDrops), scaleQuestGoldReward(questDef.Gold)*skipCount)
	return FinishOutcome{DropRewards: allDrops}, true
}

func (h *QuestHandler) HandleQuestRestart(user *store.UserState, questId int32, nowMillis int64) {
	questDef, ok := h.QuestById[questId]
	// Only seed CurrentQuestFlowType when it's not already set (initial
	// natural progression). Don't clobber an in-flight ReplayFlow (Map Play
	// resume).
	if ok && isMainQuestPlayable(questDef) && user.MainQuest.CurrentQuestFlowType == 0 {
		user.MainQuest.CurrentQuestFlowType = int32(model.QuestFlowTypeMainFlow)
	}

	quest := user.Quests[questId]
	quest.QuestId = questId
	quest.QuestStateType = model.UserQuestStateTypeActive
	quest.LatestStartDatetime = nowMillis
	user.Quests[questId] = quest

	// A restart is a fresh attempt: battle statistics collected by the
	// previous attempt must not leak into this one. Mission clear states are
	// deliberately kept — they are permanent achievements; re-evaluation on
	// finish never un-clears an already cleared mission.
	user.Battle.ResetQuestBattleStats()
	h.initQuestState(user, questId)
}

// ---- Type 4: Specified Character in Deck ----
func (h *QuestHandler) deckHasSpecifiedCharacter(user *store.UserState, questId int32, characterId int32) bool {
	questState := user.Quests[questId]
	deckNumber := questState.UserDeckNumber
	if deckNumber == 0 {
		deckNumber = 1
	}
	deckKey := store.DeckKey{DeckType: model.DeckTypeQuest, UserDeckNumber: deckNumber}
	deck, ok := user.Decks[deckKey]
	if !ok {
		return false
	}
	uuids := [3]string{deck.UserDeckCharacterUuid01, deck.UserDeckCharacterUuid02, deck.UserDeckCharacterUuid03}
	for _, uuid := range uuids {
		if uuid == "" {
			continue
		}
		dc, ok := user.DeckCharacters[uuid]
		if !ok {
			continue
		}
		// Look up the character ID from the costume
		if costume, ok := user.Costumes[dc.UserCostumeUuid]; ok {
			if cm, ok := h.CostumeById[costume.CostumeId]; ok && cm.CharacterId == characterId {
				return true
			}
		}
	}
	return false
}

func (h *QuestHandler) deckCostumeCount(user *store.UserState, questId int32, count int32, operator string) bool {
	questState := user.Quests[questId]
	deckNumber := questState.UserDeckNumber
	if deckNumber == 0 {
		deckNumber = 1
	}
	deckKey := store.DeckKey{DeckType: model.DeckTypeQuest, UserDeckNumber: deckNumber}
	deck, ok := user.Decks[deckKey]
	if !ok {
		return false
	}

	// Count non-empty deck slots
	deckCount := int32(0)
	uuids := [3]string{deck.UserDeckCharacterUuid01, deck.UserDeckCharacterUuid02, deck.UserDeckCharacterUuid03}
	for _, uuid := range uuids {
		if uuid != "" {
			deckCount++
		}
	}

	match := false
	switch operator {
	case "eq":
		match = deckCount == count
	case "ge":
		match = deckCount >= count
	case "le":
		match = deckCount <= count
	}
	return match
}
