package questflow

import (
	"testing"

	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
)

// A quest with the authored shape of every mission-bearing quest: three
// challenge missions plus the "Clear all missions" meta-mission (type 9999),
// which the BigWin logic handles separately. The challenge missions use a max
// damage threshold nobody can reach, so the only way they clear is the option.
func forceMissionsFixture() (*QuestHandler, *store.UserState) {
	const questId = int32(100)
	impossibleDamage := int32(1_000_000_000)
	handler := &QuestHandler{QuestCatalog: &masterdata.QuestCatalog{
		QuestById: map[int32]masterdata.EntityMQuest{
			questId: {QuestId: questId, IsCountedAsQuest: true},
		},
		MissionById: map[int32]masterdata.EntityMQuestMission{
			1: {QuestMissionId: 1, QuestMissionConditionType: int32(model.QuestMissionConditionTypeComplete)},
			10: {QuestMissionId: 10, QuestMissionConditionType: int32(model.QuestMissionConditionTypeMaxDamage),
				ConditionValue: impossibleDamage, QuestMissionRewardId: 10},
			11: {QuestMissionId: 11, QuestMissionConditionType: int32(model.QuestMissionConditionTypeMaxDamage),
				ConditionValue: impossibleDamage, QuestMissionRewardId: 11},
			12: {QuestMissionId: 12, QuestMissionConditionType: int32(model.QuestMissionConditionTypeMaxDamage),
				ConditionValue: impossibleDamage, QuestMissionRewardId: 12},
		},
		MissionIdsByQuestId: map[int32][]int32{questId: {1, 10, 11, 12}},
	}}
	user := &store.UserState{Options: store.UserOptionsState{}}
	user.EnsureMaps()
	return handler, user
}

func clearedMissionIds(user *store.UserState, questId int32) []int32 {
	var out []int32
	for key, state := range user.QuestMissions {
		if key.QuestId == questId && state.IsClear {
			out = append(out, key.QuestMissionId)
		}
	}
	return out
}

func countIds(ids []int32, want int32) int {
	n := 0
	for _, id := range ids {
		if id == want {
			n++
		}
	}
	return n
}

// With the option off (the default) the impossible missions stay unclaimed.
// clearQuestMissions still records its evaluation as ordinary not-cleared rows
// (pre-existing behaviour), so the assertion is about clears, not row count.
func TestClearQuestMissions_DefaultStillRequiresLegitClear(t *testing.T) {
	handler, user := forceMissionsFixture()
	handler.clearQuestMissions(user, 100, 5000)

	if got := clearedMissionIds(user, 100); len(got) != 0 {
		t.Fatalf("expected no cleared missions without the option, got %+v", got)
	}
	for key, state := range user.QuestMissions {
		if state.LatestClearDatetime != 0 {
			t.Fatalf("mission %d must not carry a clear timestamp: %+v", key, state)
		}
	}
}

// With the option on, the quest being cleared records its three challenge
// missions as ordinary clears — the state the finish path then turns into real
// DB rows and reward payouts.
func TestClearQuestMissions_ForceOptionClearsCurrentQuest(t *testing.T) {
	handler, user := forceMissionsFixture()
	user.Options.ForceQuestMissions = true
	handler.clearQuestMissions(user, 100, 5000)

	for _, missionId := range []int32{10, 11, 12} {
		key := store.QuestMissionKey{QuestId: 100, QuestMissionId: missionId}
		state, ok := user.QuestMissions[key]
		if !ok || !state.IsClear {
			t.Fatalf("mission %d must be recorded as cleared, got %+v ok=%v", missionId, state, ok)
		}
		if state.ProgressValue != 1 {
			t.Errorf("mission %d: expected progress 1, got %d", missionId, state.ProgressValue)
		}
		if state.LatestClearDatetime != 5000 {
			t.Errorf("mission %d: expected clear time from the finish, got %d", missionId, state.LatestClearDatetime)
		}
	}

	// The "Clear all missions" meta-mission is left to the BigWin logic, which
	// unlocks it because all three regular missions now read as cleared.
	if n := countIds(clearedMissionIds(user, 100), 1); n != 0 {
		t.Errorf("complete mission must not be cleared directly, it goes through BigWin")
	}
	cleared := 0
	for _, missionId := range []int32{10, 11, 12} {
		if user.QuestMissions[store.QuestMissionKey{QuestId: 100, QuestMissionId: missionId}].IsClear {
			cleared++
		}
	}
	if cleared != 3 {
		t.Fatalf("BigWin requires all 3 regular missions cleared, got %d", cleared)
	}
}

// The option is scoped to the quest being played: another quest's missions are
// untouched, so nothing retroactive happens to already-finished content.
func TestClearQuestMissions_ForceOptionDoesNotTouchOtherQuests(t *testing.T) {
	handler, user := forceMissionsFixture()
	user.Options.ForceQuestMissions = true
	handler.clearQuestMissions(user, 100, 5000)

	if len(clearedMissionIds(user, 200)) != 0 {
		t.Fatalf("only the finished quest may gain cleared missions, got %+v", user.QuestMissions)
	}
}
