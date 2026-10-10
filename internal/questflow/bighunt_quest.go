package questflow

import (
	"fmt"
	"log"

	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
)

func (h *QuestHandler) HandleBigHuntQuestStart(user *store.UserState, questId, userDeckNumber int32, nowMillis int64) {
	_, ok := h.QuestById[questId]
	if !ok {
		panic(fmt.Sprintf("unknown questId=%d for HandleBigHuntQuestStart", questId))
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
}

func (h *QuestHandler) HandleBigHuntQuestFinish(user *store.UserState, questId int32, isRetired, isAnnihilated bool, nowMillis int64) FinishOutcome {
	_, ok := h.QuestById[questId]
	if !ok {
		panic(fmt.Sprintf("unknown questId=%d for HandleBigHuntQuestFinish", questId))
	}

	target := h.targetForBigHunt(questId)
	var outcome FinishOutcome
	if !isRetired && !isAnnihilated {
		oldMissionStates := make(map[int32]bool)
		for _, missionId := range h.MissionIdsByQuestId[questId] {
			key := store.QuestMissionKey{QuestId: questId, QuestMissionId: missionId}
			oldMissionStates[missionId] = user.QuestMissions[key].IsClear
		}
		h.clearQuestMissions(user, questId, nowMillis)
		// Big hunt quests have no replay flow; never inherit the main quest's state.
		outcome = h.evaluateFinishOutcome(user, questId, target, nowMillis, oldMissionStates, false)
		h.applyQuestVictory(user, questId, target, &outcome, nowMillis, false)
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
		log.Printf("[HandleBigHuntQuestFinish] retired/annihilated: reset quest %d state to %d (clearCount=%d)", questId, questState.QuestStateType, questState.ClearCount)
	}

	// No stamina refund on retire: stamina is never consumed on this server.

	user.BigHuntProgress.CurrentBigHuntQuestId = 0
	user.BigHuntProgress.CurrentQuestSceneId = 0
	user.BigHuntProgress.LatestVersion = nowMillis

	return outcome
}
