package questflow

import (
	"testing"

	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
)

// A quest whose three challenge missions are already cleared this attempt, plus
// first-clear and BigWin rewards wired, so evaluateFinishOutcome has something
// to pay out. Mission 1 is the "Clear all missions" meta-mission (type 9999),
// which only the BigWin branch may clear.
func rewardsFixture() (*QuestHandler, *store.UserState, map[int32]bool) {
	const questId = int32(100)
	missionReward := func(id int32, count int32) []masterdata.EntityMQuestMissionReward {
		return []masterdata.EntityMQuestMissionReward{
			{QuestMissionRewardId: id, PossessionType: int32(model.PossessionTypeMaterial), PossessionId: 500 + id, Count: count},
		}
	}
	handler := &QuestHandler{QuestCatalog: &masterdata.QuestCatalog{
		QuestById: map[int32]masterdata.EntityMQuest{
			questId: {QuestId: questId, IsCountedAsQuest: true, QuestFirstClearRewardGroupId: 7},
		},
		MissionById: map[int32]masterdata.EntityMQuestMission{
			1:  {QuestMissionId: 1, QuestMissionConditionType: int32(model.QuestMissionConditionTypeComplete), QuestMissionRewardId: 90},
			10: {QuestMissionId: 10, QuestMissionConditionType: int32(model.QuestMissionConditionTypeMaxDamage), QuestMissionRewardId: 10},
			11: {QuestMissionId: 11, QuestMissionConditionType: int32(model.QuestMissionConditionTypeMaxDamage), QuestMissionRewardId: 11},
			12: {QuestMissionId: 12, QuestMissionConditionType: int32(model.QuestMissionConditionTypeMaxDamage), QuestMissionRewardId: 12},
		},
		MissionIdsByQuestId: map[int32][]int32{questId: {1, 10, 11, 12}},
		MissionRewardsByMissionId: map[int32][]masterdata.EntityMQuestMissionReward{
			10: missionReward(10, 1), 11: missionReward(11, 1), 12: missionReward(12, 1),
			90: missionReward(90, 5),
		},
		FirstClearRewardsByGroupId: map[int32][]masterdata.EntityMQuestFirstClearRewardGroup{
			7: {{QuestFirstClearRewardGroupId: 7, PossessionType: int32(model.PossessionTypeMaterial), PossessionId: 900, Count: 1}},
		},
	}}

	user := &store.UserState{}
	user.EnsureMaps()
	handler.initQuestState(user, questId)
	for _, missionId := range []int32{10, 11, 12} {
		key := store.QuestMissionKey{QuestId: questId, QuestMissionId: missionId}
		state := user.QuestMissions[key]
		state.ProgressValue = 1
		state.IsClear = true
		state.LatestClearDatetime = 5000
		user.QuestMissions[key] = state
	}

	// None of them was cleared before this attempt.
	oldMissionStates := map[int32]bool{1: false, 10: false, 11: false, 12: false}
	return handler, user, oldMissionStates
}

const testQuestId = int32(100)

// A replay flow stays recorded on the main quest after its finish (the client
// needs it for the replay result UI) and event/extra quest starts never clear
// it. A non-main finish while that state lingers must still pay first-clear,
// mission and BigWin rewards.
func TestEvaluateFinishOutcomeNonMainPathIgnoresLingeringReplayFlow(t *testing.T) {
	handler, user, old := rewardsFixture()
	user.MainQuest.CurrentQuestFlowType = int32(model.QuestFlowTypeReplayFlow)

	outcome := handler.evaluateFinishOutcome(user, testQuestId, handler.targetForMain(testQuestId), 5000, old, false)

	if len(outcome.FirstClearRewards) == 0 {
		t.Errorf("first-clear reward must be paid, got none")
	}
	if n := len(outcome.MissionClearRewards); n != 3 {
		t.Errorf("expected 3 mission rewards, got %d (%+v)", n, outcome.MissionClearRewards)
	}
	if !outcome.IsBigWin {
		t.Errorf("all 3 regular missions are cleared, BigWin must fire")
	}
	if len(outcome.MissionClearCompleteRewards) == 0 {
		t.Errorf("BigWin bonus reward must be paid, got none")
	}
	if !user.QuestMissions[store.QuestMissionKey{QuestId: testQuestId, QuestMissionId: 1}].IsClear {
		t.Errorf("BigWin must clear the meta-mission so the bonus is paid only once")
	}
}

// The main-quest path keeps its behaviour: a genuine replay finish shows no
// first-clear popup and no mission rewards (replay-variant quests have no
// mission rows in the reference data).
func TestEvaluateFinishOutcomeRealReplayStillSuppressesRewards(t *testing.T) {
	handler, user, old := rewardsFixture()

	outcome := handler.evaluateFinishOutcome(user, testQuestId, handler.targetForMain(testQuestId), 5000, old, true)

	if len(outcome.FirstClearRewards) != 0 {
		t.Errorf("replay must not pay a first-clear reward, got %+v", outcome.FirstClearRewards)
	}
	if len(outcome.MissionClearRewards) != 0 {
		t.Errorf("replay must not pay mission rewards, got %+v", outcome.MissionClearRewards)
	}
	if outcome.IsBigWin {
		t.Errorf("replay must not fire BigWin")
	}
	if user.QuestMissions[store.QuestMissionKey{QuestId: testQuestId, QuestMissionId: 1}].IsClear {
		t.Errorf("replay must not clear the meta-mission")
	}
}
