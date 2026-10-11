package questflow

import (
	"fmt"
	"log"

	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
)

func (h *QuestHandler) HandleExtraQuestStart(user *store.UserState, questId, userDeckNumber int32, nowMillis int64) {
	quest, ok := h.QuestById[questId]
	if !ok {
		panic(fmt.Sprintf("unknown questId=%d for HandleExtraQuestStart", questId))
	}
	if !h.QuestReleased(user, quest) {
		log.Printf("[HandleExtraQuestStart] quest %d is locked by release conditions", questId)
		return
	}

	h.initQuestState(user, questId)

	// A new attempt begins: battle statistics must start from zero.
	user.Battle.ResetQuestBattleStats()

	// Stamina is intentionally never consumed on this server — quest starts are free.

	questState := user.Quests[questId]
	questState.UserDeckNumber = userDeckNumber
	questState.QuestStateType = model.UserQuestStateTypeActive
	questState.LatestStartDatetime = nowMillis
	user.Quests[questId] = questState

	user.ExtraQuest.CurrentQuestId = questId
	if sceneIds := h.SceneIdsByQuestId[questId]; len(sceneIds) > 0 {
		user.ExtraQuest.CurrentQuestSceneId = sceneIds[0]
		user.ExtraQuest.HeadQuestSceneId = sceneIds[0]
	}
}

func (h *QuestHandler) HandleExtraQuestFinish(user *store.UserState, questId int32, isRetired, isAnnihilated bool, nowMillis int64) FinishOutcome {
	_, ok := h.QuestById[questId]
	if !ok {
		panic(fmt.Sprintf("unknown questId=%d for HandleExtraQuestFinish", questId))
	}

	target := h.targetForExtra(questId)
	var outcome FinishOutcome
	if !isRetired && !isAnnihilated {
		oldMissionStates := make(map[int32]bool)
		for _, missionId := range h.MissionIdsByQuestId[questId] {
			key := store.QuestMissionKey{QuestId: questId, QuestMissionId: missionId}
			oldMissionStates[missionId] = user.QuestMissions[key].IsClear
		}
		h.clearQuestMissions(user, questId, nowMillis)
		// Extra quests have no replay flow; never inherit the main quest's state.
		outcome = h.evaluateFinishOutcome(user, questId, target, nowMillis, oldMissionStates, false)
		h.applyQuestVictory(user, questId, target, &outcome, nowMillis, false)
	} else {
		// On retire/annihilate: keep Cleared if already cleared once, otherwise
		// mark Challenged so dependent quests (e.g. linked Scarecrow) can unlock.
		questState := user.Quests[questId]
		if questState.ClearCount > 0 {
			questState.QuestStateType = model.UserQuestStateTypeCleared
		} else {
			questState.QuestStateType = model.UserQuestStateTypeChallenged
		}
		questState.LatestVersion = nowMillis
		user.Quests[questId] = questState
		log.Printf("[HandleExtraQuestFinish] retired/annihilated: reset quest %d state to %d (clearCount=%d)", questId, questState.QuestStateType, questState.ClearCount)
	}

	// No stamina refund on retire: stamina is never consumed on this server.

	user.ExtraQuest.CurrentQuestId = 0
	user.ExtraQuest.CurrentQuestSceneId = 0
	user.ExtraQuest.HeadQuestSceneId = 0

	return outcome
}

func (h *QuestHandler) HandleExtraQuestRestart(user *store.UserState, questId int32, nowMillis int64) {
	h.HandleQuestRestart(user, questId, nowMillis)

	user.ExtraQuest.CurrentQuestId = questId
}

func (h *QuestHandler) HandleExtraQuestSceneProgress(user *store.UserState, questSceneId int32, nowMillis int64) {
	if _, ok := h.SceneById[questSceneId]; !ok {
		log.Printf("[HandleExtraQuestSceneProgress] unknown sceneId=%d, skipping", questSceneId)
		return
	}

	user.ExtraQuest.CurrentQuestSceneId = questSceneId
	if h.isSceneAhead(questSceneId, user.ExtraQuest.HeadQuestSceneId) {
		user.ExtraQuest.HeadQuestSceneId = questSceneId
	}

	h.applySceneGrants(user, questSceneId, nowMillis)
}
