package questflow

import (
	"log"

	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
)

// Quest bonus effect types from m_quest_bonus_effect_group.QuestBonusType.
const (
	questBonusTypeExp        int32 = 2
	questBonusTypeDropReward int32 = 3
)

type questBonusDeckUnit struct {
	costumeUuid string
	characterId int32
	weaponUuids []string
}

func (h *QuestHandler) questDeck(user *store.UserState, questId int32) (store.DeckState, bool) {
	return h.questDeckForNumber(user, questId, user.Quests[questId].UserDeckNumber)
}

func (h *QuestHandler) questDeckForNumber(user *store.UserState, questId, deckNumber int32) (store.DeckState, bool) {
	if deckNumber == 0 {
		return store.DeckState{}, false
	}
	quest := h.QuestById[questId]
	deckTypes := []model.DeckType{model.DeckTypeQuest}
	switch {
	case h.LimitContentQuestIds[questId]:
		deckTypes = []model.DeckType{model.DeckTypeRestrictedLimitContentQuest, model.DeckTypeQuest}
	case quest.QuestDeckRestrictionGroupId != 0:
		deckTypes = []model.DeckType{model.DeckTypeRestrictedQuest, model.DeckTypeQuest}
	}
	for _, deckType := range deckTypes {
		if deck, ok := user.Decks[store.DeckKey{DeckType: deckType, UserDeckNumber: deckNumber}]; ok {
			return deck, true
		}
	}
	return store.DeckState{}, false
}

func (h *QuestHandler) questBonusDeckUnits(user *store.UserState, questId int32) []questBonusDeckUnit {
	deck, ok := h.questDeck(user, questId)
	if !ok {
		return nil
	}
	deckCharacterUuids := []string{
		deck.UserDeckCharacterUuid01,
		deck.UserDeckCharacterUuid02,
		deck.UserDeckCharacterUuid03,
	}
	units := make([]questBonusDeckUnit, 0, len(deckCharacterUuids))
	for _, deckCharacterUuid := range deckCharacterUuids {
		if deckCharacterUuid == "" {
			continue
		}
		deckCharacter, ok := user.DeckCharacters[deckCharacterUuid]
		if !ok {
			continue
		}
		unit := questBonusDeckUnit{costumeUuid: deckCharacter.UserCostumeUuid}
		if costume, ok := user.Costumes[deckCharacter.UserCostumeUuid]; ok {
			if cm, ok := h.CostumeById[costume.CostumeId]; ok {
				unit.characterId = cm.CharacterId
			}
		}
		if deckCharacter.MainUserWeaponUuid != "" {
			unit.weaponUuids = append(unit.weaponUuids, deckCharacter.MainUserWeaponUuid)
		}
		unit.weaponUuids = append(unit.weaponUuids, user.DeckSubWeapons[deckCharacterUuid]...)
		units = append(units, unit)
	}
	return units
}

func (h *QuestHandler) questBonusTermActive(groupId int32, nowMillis int64) bool {
	if groupId == 0 {
		return true
	}
	rows := h.QuestBonusTermsByGroupId[groupId]
	if len(rows) == 0 {
		return true
	}
	for _, row := range rows {
		if (row.StartDatetime == 0 || nowMillis >= row.StartDatetime) &&
			(row.EndDatetime == 0 || nowMillis <= row.EndDatetime) {
			return true
		}
	}
	return false
}

// questBonusWeaponMatches treats evolved forms of the same evolution chain as
// matching the configured resonant weapon (same group, equal or higher order).
func (h *QuestHandler) questBonusWeaponMatches(configuredWeaponId, equippedWeaponId int32) bool {
	if configuredWeaponId == equippedWeaponId {
		return true
	}
	configured, configuredOK := h.WeaponEvolutionByWeaponId[configuredWeaponId]
	equipped, equippedOK := h.WeaponEvolutionByWeaponId[equippedWeaponId]
	return configuredOK && equippedOK && configured.GroupId != 0 &&
		configured.GroupId == equipped.GroupId && equipped.Order >= configured.Order
}

func (h *QuestHandler) bestQuestBonusWeaponRow(
	rows []masterdata.EntityMQuestBonusWeaponGroup,
	weapon store.WeaponState,
	nowMillis int64,
) (masterdata.EntityMQuestBonusWeaponGroup, bool) {
	var best masterdata.EntityMQuestBonusWeaponGroup
	found := false
	bestEvolutionOrder := int32(-1)
	for _, row := range rows {
		if row.LimitBreakCountLowerLimit > weapon.LimitBreakCount ||
			!h.questBonusTermActive(row.QuestBonusTermGroupId, nowMillis) ||
			!h.questBonusWeaponMatches(row.WeaponId, weapon.WeaponId) {
			continue
		}
		evolutionOrder := h.WeaponEvolutionByWeaponId[row.WeaponId].Order
		if !found ||
			row.LimitBreakCountLowerLimit > best.LimitBreakCountLowerLimit ||
			(row.LimitBreakCountLowerLimit == best.LimitBreakCountLowerLimit && evolutionOrder > bestEvolutionOrder) {
			best = row
			bestEvolutionOrder = evolutionOrder
			found = true
		}
	}
	return best, found
}

// questBonusDropRewards returns flat additional drop grants from resonant
// weapons in the quest deck (main + sub weapons, all three slots).
// Amounts come from m_quest_bonus_drop_reward.AdditionalCount and must NOT be
// passed through scaleDropReward / QuestRewardMultiplier.
func (h *QuestHandler) questBonusDropRewards(user *store.UserState, quest masterdata.EntityMQuest, nowMillis int64) []RewardGrant {
	bonus, ok := h.QuestBonusById[quest.QuestBonusId]
	if !ok || bonus.QuestBonusWeaponGroupId == 0 {
		return nil
	}
	rows := h.QuestBonusWeaponRowsByGroupId[bonus.QuestBonusWeaponGroupId]
	if len(rows) == 0 {
		return nil
	}

	seenWeaponUuids := make(map[string]bool)
	grantIndex := make(map[[2]int32]int)
	var grants []RewardGrant

	for _, unit := range h.questBonusDeckUnits(user, quest.QuestId) {
		for _, weaponUuid := range unit.weaponUuids {
			if weaponUuid == "" || seenWeaponUuids[weaponUuid] {
				continue
			}
			seenWeaponUuids[weaponUuid] = true
			weapon, ok := user.Weapons[weaponUuid]
			if !ok {
				continue
			}
			row, ok := h.bestQuestBonusWeaponRow(rows, weapon, nowMillis)
			if !ok {
				continue
			}
			for _, effect := range h.QuestBonusEffectsByGroupId[row.QuestBonusEffectGroupId] {
				if effect.QuestBonusType != questBonusTypeDropReward {
					continue
				}
				reward, ok := h.QuestBonusDropByEffectId[effect.QuestBonusEffectId]
				if !ok || reward.AdditionalCount <= 0 {
					continue
				}
				key := [2]int32{reward.PossessionType, reward.PossessionId}
				if index, exists := grantIndex[key]; exists {
					grants[index].Count += reward.AdditionalCount
					continue
				}
				grantIndex[key] = len(grants)
				grants = append(grants, RewardGrant{
					PossessionType: model.PossessionType(reward.PossessionType),
					PossessionId:   reward.PossessionId,
					Count:          reward.AdditionalCount,
				})
			}
		}
	}
	if len(grants) > 0 {
		log.Printf("[QuestBonus] quest=%d QuestBonusId=%d weaponBonusGrants=%d",
			quest.QuestId, quest.QuestBonusId, len(grants))
	}
	return grants
}
