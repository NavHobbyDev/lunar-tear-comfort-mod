package questflow

import (
	"fmt"
	"log"

	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
)

func (h *QuestHandler) HandleEventQuestStart(user *store.UserState, eventQuestChapterId, questId int32, isBattleOnly bool, userDeckNumber int32, nowMillis int64) {
	_, ok := h.QuestById[questId]
	if !ok {
		panic(fmt.Sprintf("unknown questId=%d for HandleEventQuestStart", questId))
	}

	h.initQuestState(user, questId)

	// A new attempt begins: battle statistics must start from zero.
	user.Battle.ResetQuestBattleStats()

	// Stamina is intentionally never consumed on this server — quest starts are free.

	questState := user.Quests[questId]
	questState.IsBattleOnly = isBattleOnly
	questState.UserDeckNumber = userDeckNumber
	questState.QuestStateType = model.UserQuestStateTypeActive
	questState.LatestStartDatetime = nowMillis
	user.Quests[questId] = questState

	user.EventQuest.CurrentEventQuestChapterId = eventQuestChapterId
	user.EventQuest.CurrentQuestId = questId
	if sceneIds := h.SceneIdsByQuestId[questId]; len(sceneIds) > 0 {
		user.EventQuest.CurrentQuestSceneId = sceneIds[0]
		user.EventQuest.HeadQuestSceneId = sceneIds[0]
	}
}

func (h *QuestHandler) HandleEventQuestFinish(user *store.UserState, eventQuestChapterId, questId int32, isRetired, isAnnihilated bool, nowMillis int64) FinishOutcome {
	_, ok := h.QuestById[questId]
	if !ok {
		panic(fmt.Sprintf("unknown questId=%d for HandleEventQuestFinish", questId))
	}

	target := h.targetForEvent(eventQuestChapterId, questId)
	var outcome FinishOutcome
	if !isRetired && !isAnnihilated {
		oldMissionStates := make(map[int32]bool)
		for _, missionId := range h.MissionIdsByQuestId[questId] {
			key := store.QuestMissionKey{QuestId: questId, QuestMissionId: missionId}
			oldMissionStates[missionId] = user.QuestMissions[key].IsClear
		}
		h.clearQuestMissions(user, questId, nowMillis)
		// Event quests are never part of the main-quest replay flow, so this finish
		// is not a replay — matching the false given to applyQuestVictory below.
		outcome = h.evaluateFinishOutcome(user, questId, target, nowMillis, oldMissionStates, false)
		h.applyQuestVictory(user, questId, target, &outcome, nowMillis, false)
		h.recordSideStoryLimitContentStatus(user, questId, nowMillis)
	} else {
		// Reset quest state to Unknown on retire/annihilate to prevent blocking
		questState := user.Quests[questId]
		if questState.ClearCount > 0 {
			questState.QuestStateType = model.UserQuestStateTypeCleared
		} else {
			questState.QuestStateType = model.UserQuestStateTypeUnknown
		}
		questState.LatestVersion = nowMillis
		user.Quests[questId] = questState
		log.Printf("[HandleEventQuestFinish] retired/annihilated: reset quest %d state to %d (clearCount=%d)", questId, questState.QuestStateType, questState.ClearCount)
	}

	// No stamina refund on retire: stamina is never consumed on this server.

	user.EventQuest.CurrentEventQuestChapterId = 0
	user.EventQuest.CurrentQuestId = 0
	user.EventQuest.CurrentQuestSceneId = 0
	user.EventQuest.HeadQuestSceneId = 0
	user.EventQuest.LatestVersion = nowMillis

	return outcome
}

func (h *QuestHandler) recordSideStoryLimitContentStatus(user *store.UserState, questId int32, nowMillis int64) {
	chapterId, ok := h.SideStoryChapterByEventQuestId[questId]
	if !ok {
		return
	}
	st := user.QuestLimitContentStatus[questId]
	st.LimitContentQuestStatusType = 1
	st.EventQuestChapterId = chapterId
	st.LatestVersion = nowMillis
	user.QuestLimitContentStatus[questId] = st
}

func (h *QuestHandler) HandleEventQuestRestart(user *store.UserState, eventQuestChapterId, questId int32, nowMillis int64) {
	h.HandleQuestRestart(user, questId, nowMillis)

	user.EventQuest.CurrentEventQuestChapterId = eventQuestChapterId
	user.EventQuest.CurrentQuestId = questId
}

func (h *QuestHandler) HandleEventQuestSceneProgress(user *store.UserState, questSceneId int32, nowMillis int64) {
	if _, ok := h.SceneById[questSceneId]; !ok {
		log.Printf("[HandleEventQuestSceneProgress] unknown sceneId=%d, skipping", questSceneId)
		return
	}

	user.EventQuest.CurrentQuestSceneId = questSceneId
	if h.isSceneAhead(questSceneId, user.EventQuest.HeadQuestSceneId) {
		user.EventQuest.HeadQuestSceneId = questSceneId
	}

	h.applySceneGrants(user, questSceneId, nowMillis)
}
