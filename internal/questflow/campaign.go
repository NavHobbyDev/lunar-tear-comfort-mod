package questflow

import (
	"lunar-tear/server/internal/campaign"
	"lunar-tear/server/internal/model"
)

func (h *QuestHandler) targetForMain(questId int32) campaign.QuestTarget {
	return campaign.QuestTarget{
		QuestId:   questId,
		QuestType: campaign.QuestTypeMainQuest,
		ChapterId: h.MainQuestChapterIdByQuestId[questId],
	}
}

func (h *QuestHandler) targetForEvent(eventChapterId, questId int32) campaign.QuestTarget {
	return campaign.QuestTarget{
		QuestId:        questId,
		QuestType:      campaign.QuestTypeEventQuest,
		EventQuestType: h.EventQuestTypeByChapterId[eventChapterId],
		ChapterId:      eventChapterId,
	}
}

func (h *QuestHandler) targetForExtra(questId int32) campaign.QuestTarget {
	return campaign.QuestTarget{QuestId: questId, QuestType: campaign.QuestTypeExtraQuest}
}

func (h *QuestHandler) targetForBigHunt(questId int32) campaign.QuestTarget {
	return campaign.QuestTarget{QuestId: questId, QuestType: campaign.QuestTypeBigHunt}
}

func (h *QuestHandler) campaignFilter(nowMillis int64) campaign.Filter {
	return campaign.Filter{NowMillis: nowMillis, UserStatus: campaign.TargetUserStatusAll}
}

func (h *QuestHandler) staminaWithCampaign(baseStamina int32, t campaign.QuestTarget, nowMillis int64) int32 {
	if h.Campaigns == nil {
		return baseStamina
	}
	return h.Campaigns.QuestStamina(t, h.campaignFilter(nowMillis)).Apply(baseStamina)
}

// MainQuestStaminaCost returns the stamina that a normal quest would have
// consumed at this moment. Normal quest play is free on this server, but the
// value is still used by missions that track total stamina spent.
func (h *QuestHandler) MainQuestStaminaCost(questId int32, nowMillis int64) int32 {
	return h.questStaminaCost(questId, h.targetForMain(questId), nowMillis)
}

func (h *QuestHandler) EventQuestStaminaCost(eventQuestChapterId, questId int32, nowMillis int64) int32 {
	return h.questStaminaCost(questId, h.targetForEvent(eventQuestChapterId, questId), nowMillis)
}

func (h *QuestHandler) ExtraQuestStaminaCost(questId int32, nowMillis int64) int32 {
	return h.questStaminaCost(questId, h.targetForExtra(questId), nowMillis)
}

func (h *QuestHandler) BigHuntQuestStaminaCost(questId int32, nowMillis int64) int32 {
	return h.questStaminaCost(questId, h.targetForBigHunt(questId), nowMillis)
}

func (h *QuestHandler) questStaminaCost(questId int32, target campaign.QuestTarget, nowMillis int64) int32 {
	quest, ok := h.QuestById[questId]
	if !ok || quest.Stamina <= 0 {
		return 0
	}
	return h.staminaWithCampaign(quest.Stamina, target, nowMillis)
}

func (h *QuestHandler) appendBonusDrops(drops []RewardGrant, t campaign.QuestTarget, nowMillis int64) []RewardGrant {
	if h.Campaigns == nil {
		return drops
	}
	for _, bd := range h.Campaigns.QuestBonusDrops(t, h.campaignFilter(nowMillis)) {
		drops = append(drops, RewardGrant{
			PossessionType: model.PossessionType(bd.PossessionType),
			PossessionId:   bd.PossessionId,
			Count:          bd.Count,
			RarityType:     0,
		})
	}
	return drops
}
