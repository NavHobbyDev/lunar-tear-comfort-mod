package userdata

import (
	"sort"

	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
	"lunar-tear/server/internal/utils"
)

func sortedQuestRecords(user store.UserState) []map[string]any {
	ids := make([]int, 0, len(user.Quests))
	for id := range user.Quests {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)

	var replayQuestId int32
	if user.MainQuest.SavedContext.Active && questHandler != nil {
		if scene, ok := questHandler.SceneById[user.MainQuest.ProgressQuestSceneId]; ok {
			replayQuestId = scene.QuestId
		}
	}

	records := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		row := user.Quests[int32(id)]
		stateType := row.QuestStateType
		if replayQuestId != 0 {
			switch {
			case int32(id) == replayQuestId:
				stateType = model.UserQuestStateTypeActive
			case stateType == model.UserQuestStateTypeActive:
				stateType = model.UserQuestStateTypeCleared
			}
		}
		records = append(records, map[string]any{
			"userId":              user.UserId,
			"questId":             row.QuestId,
			"questStateType":      stateType,
			"isBattleOnly":        row.IsBattleOnly,
			"latestStartDatetime": row.LatestStartDatetime,
			"clearCount":          row.ClearCount,
			"dailyClearCount":     row.DailyClearCount,
			"lastClearDatetime":   row.LastClearDatetime,
			"shortestClearFrames": row.ShortestClearFrames,
			"latestVersion":       row.LatestVersion,
		})
	}
	return records
}

func sortedQuestMissionRecords(user store.UserState) []map[string]any {
	questMissions := make(map[store.QuestMissionKey]store.UserQuestMissionState, len(user.QuestMissions))
	for key, qm := range user.QuestMissions {
		questMissions[key] = qm
	}
	// Force-clear hidden-story quest-missions so their report gimmicks unlock.
	settings := ResolveOptions(user.Options)
	if settings.ForceHiddenStoryMissions {
		for _, key := range hiddenStoryRequirements().QuestMissions {
			if existing, ok := questMissions[key]; ok && existing.IsClear {
				continue
			}
			questMissions[key] = store.UserQuestMissionState{
				QuestId:             key.QuestId,
				QuestMissionId:      key.QuestMissionId,
				IsClear:             true,
				LatestClearDatetime: user.GameStartDatetime,
				LatestVersion:       user.GameStartDatetime,
			}
		}
	}
	// The "Force quest missions" option is deliberately absent here: it is not a
	// projection trick. It makes the quest being cleared right now record its
	// missions as really cleared (with rewards) in questflow's clearQuestMissions,
	// so the state lands in the database through the normal finish path.

	keys := make([]store.QuestMissionKey, 0, len(questMissions))
	for key := range questMissions {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].QuestId != keys[j].QuestId {
			return keys[i].QuestId < keys[j].QuestId
		}
		return keys[i].QuestMissionId < keys[j].QuestMissionId
	})
	records := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		row := questMissions[key]
		records = append(records, map[string]any{
			"userId":              user.UserId,
			"questId":             row.QuestId,
			"questMissionId":      row.QuestMissionId,
			"progressValue":       row.ProgressValue,
			"isClear":             row.IsClear,
			"latestClearDatetime": row.LatestClearDatetime,
			"latestVersion":       row.LatestVersion,
		})
	}
	return records
}

func init() {
	register("IUserQuest", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(sortedQuestRecords(user)...)
		return s
	})
	register("IUserQuestMission", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(sortedQuestMissionRecords(user)...)
		return s
	})
	register("IUserMainQuestFlowStatus", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(map[string]any{
			"userId":               user.UserId,
			"currentQuestFlowType": user.MainQuest.CurrentQuestFlowType,
			"latestVersion":        user.MainQuest.LatestVersion,
		})
		return s
	})
	register("IUserMainQuestMainFlowStatus", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(map[string]any{
			"userId":                  user.UserId,
			"currentMainQuestRouteId": user.MainQuest.CurrentMainQuestRouteId,
			"currentQuestSceneId":     user.MainQuest.CurrentQuestSceneId,
			"headQuestSceneId":        user.MainQuest.HeadQuestSceneId,
			"isReachedLastQuestScene": user.MainQuest.IsReachedLastQuestScene,
			"latestVersion":           user.MainQuest.LatestVersion,
		})
		return s
	})
	register("IUserMainQuestProgressStatus", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(map[string]any{
			"userId":               user.UserId,
			"currentQuestSceneId":  user.MainQuest.ProgressQuestSceneId,
			"headQuestSceneId":     user.MainQuest.ProgressHeadQuestSceneId,
			"currentQuestFlowType": user.MainQuest.ProgressQuestFlowType,
			"latestVersion":        user.MainQuest.LatestVersion,
		})
		return s
	})
	register("IUserMainQuestSeasonRoute", func(user store.UserState) string {
		if questHandler == nil {
			return "[]"
		}
		pairs := questHandler.SeasonRoutesFor(&user)
		if len(pairs) == 0 {
			return "[]"
		}
		seasons := make([]int32, 0, len(pairs))
		for s := range pairs {
			seasons = append(seasons, s)
		}
		sort.Slice(seasons, func(i, j int) bool { return seasons[i] < seasons[j] })
		records := make([]map[string]any, 0, len(seasons))
		for _, s := range seasons {
			records = append(records, map[string]any{
				"userId":            user.UserId,
				"mainQuestSeasonId": s,
				"mainQuestRouteId":  pairs[s],
				"latestVersion":     user.MainQuest.LatestVersion,
			})
		}
		out, _ := utils.EncodeJSONMaps(records...)
		return out
	})
	register("IUserEventQuestProgressStatus", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(map[string]any{
			"userId":                     user.UserId,
			"currentEventQuestChapterId": user.EventQuest.CurrentEventQuestChapterId,
			"currentQuestId":             user.EventQuest.CurrentQuestId,
			"currentQuestSceneId":        user.EventQuest.CurrentQuestSceneId,
			"headQuestSceneId":           user.EventQuest.HeadQuestSceneId,
			"latestVersion":              user.EventQuest.LatestVersion,
		})
		return s
	})
	register("IUserExtraQuestProgressStatus", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(map[string]any{
			"userId":              user.UserId,
			"currentQuestId":      user.ExtraQuest.CurrentQuestId,
			"currentQuestSceneId": user.ExtraQuest.CurrentQuestSceneId,
			"headQuestSceneId":    user.ExtraQuest.HeadQuestSceneId,
			"latestVersion":       user.ExtraQuest.LatestVersion,
		})
		return s
	})
	register("IUserMainQuestReplayFlowStatus", func(user store.UserState) string {
		if user.MainQuest.ReplayFlowCurrentQuestSceneId == 0 && user.MainQuest.ReplayFlowHeadQuestSceneId == 0 {
			return "[]"
		}
		s, _ := utils.EncodeJSONMaps(map[string]any{
			"userId":                  user.UserId,
			"currentHeadQuestSceneId": user.MainQuest.ReplayFlowHeadQuestSceneId,
			"currentQuestSceneId":     user.MainQuest.ReplayFlowCurrentQuestSceneId,
			"latestVersion":           user.MainQuest.LatestVersion,
		})
		return s
	})
	register("IUserSideStoryQuestSceneProgressStatus", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(map[string]any{
			"userId":                       user.UserId,
			"currentSideStoryQuestId":      user.SideStoryActiveProgress.CurrentSideStoryQuestId,
			"currentSideStoryQuestSceneId": user.SideStoryActiveProgress.CurrentSideStoryQuestSceneId,
			"latestVersion":                user.SideStoryActiveProgress.LatestVersion,
		})
		return s
	})
	register("IUserSideStoryQuest", func(user store.UserState) string {
		if len(user.SideStoryQuests) == 0 {
			return "[]"
		}
		ids := make([]int, 0, len(user.SideStoryQuests))
		for id := range user.SideStoryQuests {
			ids = append(ids, int(id))
		}
		sort.Ints(ids)
		records := make([]map[string]any, 0, len(ids))
		for _, id := range ids {
			progress := user.SideStoryQuests[int32(id)]
			records = append(records, map[string]any{
				"userId":                    user.UserId,
				"sideStoryQuestId":          int32(id),
				"headSideStoryQuestSceneId": progress.HeadSideStoryQuestSceneId,
				"sideStoryQuestStateType":   progress.SideStoryQuestStateType,
				"latestVersion":             progress.LatestVersion,
			})
		}
		s, _ := utils.EncodeJSONMaps(records...)
		return s
	})
	register("IUserQuestLimitContentStatus", func(user store.UserState) string {
		if len(user.QuestLimitContentStatus) == 0 {
			return "[]"
		}
		ids := make([]int, 0, len(user.QuestLimitContentStatus))
		for id := range user.QuestLimitContentStatus {
			ids = append(ids, int(id))
		}
		sort.Ints(ids)
		records := make([]map[string]any, 0, len(ids))
		for _, id := range ids {
			st := user.QuestLimitContentStatus[int32(id)]
			records = append(records, map[string]any{
				"userId":                      user.UserId,
				"questId":                     int32(id),
				"limitContentQuestStatusType": st.LimitContentQuestStatusType,
				"eventQuestChapterId":         st.EventQuestChapterId,
				"latestVersion":               st.LatestVersion,
			})
		}
		s, _ := utils.EncodeJSONMaps(records...)
		return s
	})
	register("IUserEventQuestTowerAccumulationReward", func(user store.UserState) string {
		if len(user.TowerAccumulationRewards) == 0 {
			return "[]"
		}
		ids := make([]int, 0, len(user.TowerAccumulationRewards))
		for id := range user.TowerAccumulationRewards {
			ids = append(ids, int(id))
		}
		sort.Ints(ids)
		records := make([]map[string]any, 0, len(ids))
		for _, id := range ids {
			st := user.TowerAccumulationRewards[int32(id)]
			records = append(records, map[string]any{
				"userId":              user.UserId,
				"eventQuestChapterId": st.EventQuestChapterId,
				"latestRewardReceiveQuestMissionClearCount": st.LatestRewardReceiveQuestMissionClearCount,
				"latestVersion": st.LatestVersion,
			})
		}
		s, _ := utils.EncodeJSONMaps(records...)
		return s
	})
	register("IUserQuestAutoOrbit", func(user store.UserState) string {
		s := user.QuestAutoOrbit
		if s.MaxAutoOrbitCount <= 0 {
			return "[]"
		}
		out, _ := utils.EncodeJSONMaps(map[string]any{
			"userId":                user.UserId,
			"questType":             s.QuestType,
			"chapterId":             s.ChapterId,
			"questId":               s.QuestId,
			"maxAutoOrbitCount":     s.MaxAutoOrbitCount,
			"clearedAutoOrbitCount": s.ClearedAutoOrbitCount,
			"lastClearDatetime":     s.LastClearDatetime,
			"latestVersion":         s.LatestVersion,
		})
		return out
	})
	register("IUserQuestSceneChoice", func(user store.UserState) string {
		if len(user.QuestSceneChoices) == 0 {
			return "[]"
		}
		ids := make([]int32, 0, len(user.QuestSceneChoices))
		for gid := range user.QuestSceneChoices {
			ids = append(ids, gid)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		records := make([]map[string]any, 0, len(ids))
		for _, gid := range ids {
			choice := user.QuestSceneChoices[gid]
			records = append(records, map[string]any{
				"userId":                          user.UserId,
				"questSceneChoiceGroupingId":      choice.QuestSceneChoiceGroupingId,
				"questSceneChoiceEffectId":        choice.QuestSceneChoiceEffectId,
				"latestVersion":                   choice.LatestVersion,
			})
		}
		s, _ := utils.EncodeJSONMaps(records...)
		return s
	})
	register("IUserQuestSceneChoiceHistory", func(user store.UserState) string {
		if len(user.QuestSceneChoiceHistory) == 0 {
			return "[]"
		}
		ids := make([]int32, 0, len(user.QuestSceneChoiceHistory))
		for eid := range user.QuestSceneChoiceHistory {
			ids = append(ids, eid)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		records := make([]map[string]any, 0, len(ids))
		for _, eid := range ids {
			history := user.QuestSceneChoiceHistory[eid]
			records = append(records, map[string]any{
				"userId":                     user.UserId,
				"questSceneChoiceEffectId":   history.QuestSceneChoiceEffectId,
				"choiceDatetime":             history.ChoiceDatetime,
				"latestVersion":              history.LatestVersion,
			})
		}
		s, _ := utils.EncodeJSONMaps(records...)
		return s
	})
	registerStatic(
		"IUserEventQuestDailyGroupCompleteReward",
		"IUserQuestReplayFlowRewardGroup",
	)
}
