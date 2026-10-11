package service

import (
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
)

type BannerUnlockRule struct {
	RequiredQuestID int32
	AllowedGachaIDs []int32
}

var bannerUnlockRules = []BannerUnlockRule{
	//{RequiredQuestID: 1, AllowedGachaIDs: []int32{403,40,47,9000023,176,38,9000078,178,34,121,50,594,150,8,188,99,132,168,416,426,489,498,544,613, 200003}},
	{RequiredQuestID: 1, AllowedGachaIDs: []int32{403}},
	{RequiredQuestID: 12, AllowedGachaIDs: []int32{40}},
	{RequiredQuestID: 22, AllowedGachaIDs: []int32{47}},
	{RequiredQuestID: 32, AllowedGachaIDs: []int32{9000023}},
	{RequiredQuestID: 42, AllowedGachaIDs: []int32{176}},
	{RequiredQuestID: 62, AllowedGachaIDs: []int32{38}},
	{RequiredQuestID: 72, AllowedGachaIDs: []int32{9000078}},
	{RequiredQuestID: 82, AllowedGachaIDs: []int32{178}},
	{RequiredQuestID: 92, AllowedGachaIDs: []int32{34}},
	{RequiredQuestID: 102, AllowedGachaIDs: []int32{121}},
	{RequiredQuestID: 121, AllowedGachaIDs: []int32{50, 594}},
	{RequiredQuestID: 305, AllowedGachaIDs: []int32{150}},
	{RequiredQuestID: 325, AllowedGachaIDs: []int32{8}},
	{RequiredQuestID: 351, AllowedGachaIDs: []int32{188}},
	{RequiredQuestID: 405, AllowedGachaIDs: []int32{99}},
	{RequiredQuestID: 415, AllowedGachaIDs: []int32{132}},
	{RequiredQuestID: 425, AllowedGachaIDs: []int32{168}},
	{RequiredQuestID: 441, AllowedGachaIDs: []int32{416}},
	{RequiredQuestID: 451, AllowedGachaIDs: []int32{426}},
	{RequiredQuestID: 461, AllowedGachaIDs: []int32{489}},
	{RequiredQuestID: 470, AllowedGachaIDs: []int32{498, 544}},
	{RequiredQuestID: 531, AllowedGachaIDs: []int32{613}},
	
}

func isQuestCleared(user store.UserState, questId int32) bool {
	q, ok := user.Quests[questId]
	return ok && q.QuestStateType == model.UserQuestStateTypeCleared
}

func isGachaVisible(gachaId int32, user store.UserState) bool {
	for _, rule := range bannerUnlockRules {
		for _, gid := range rule.AllowedGachaIDs {
			if gid == gachaId {
				return isQuestCleared(user, rule.RequiredQuestID)
			}
		}
	}
	return true
}

// gachaUnlockSatisfied reports whether the user meets a banner's unlock
// conditions computed at startup by masterdata.EnrichGachaUnlockConditions.
// Chapter gachas require clearing the prerequisite main quest; banners with no
// condition (or GachaUnlockNone) are always satisfied. This gates chapter
// gachas server-side, mirroring the bannerUnlockRules behavior for premium
// banners, since the client does not lock them from the transmitted condition.
func gachaUnlockSatisfied(entry store.GachaCatalogEntry, user store.UserState) bool {
	for _, uc := range entry.UnlockConditions {
		if uc.GachaUnlockConditionType == model.GachaUnlockMainQuestClear {
			if !isQuestCleared(user, uc.ConditionValue) {
				return false
			}
		}
	}
	return true
}

type gachaUnlockInfo struct {
	QuestID  int32
	Position int
}

func gachaUnlockInfoMap() map[int32]gachaUnlockInfo {
	out := make(map[int32]gachaUnlockInfo)
	for _, rule := range bannerUnlockRules {
		for pos, gid := range rule.AllowedGachaIDs {
			if _, exists := out[gid]; !exists {
				out[gid] = gachaUnlockInfo{QuestID: rule.RequiredQuestID, Position: pos}
			}
		}
	}
	return out
}

func questClearTime(user store.UserState, questId int32) int64 {
	q, ok := user.Quests[questId]
	if !ok {
		return 0
	}
	return q.LastClearDatetime
}
