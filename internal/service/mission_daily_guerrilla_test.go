package service

import (
	"testing"

	"lunar-tear/server/internal/model"
)

// The "Clear a daily quest N times" hidden-story missions use condition option
// group 30029; the "Clear guerrilla quests N times" missions use 30030. Both
// are satisfied by EVENT-quest clears of their respective quest families
// (dailies: EventQuestType=4 ids 100001-100024/100073-100076; guerrilla:
// EventQuestType=5 ids 100025-100048). A nil catalog is safe here because the
// matcher returns from the family switch before touching it.
func TestDailyGuerrillaQuestMissionMatches(t *testing.T) {
	cases := []struct {
		name      string
		option    int32
		questType model.QuestType
		questID   int32
		want      bool
	}{
		{"daily event clear counts", 30029, model.QuestTypeEvent, 100002, true},
		{"daily master event clear counts", 30029, model.QuestTypeEvent, 100076, true},
		{"guerrilla quest is not a daily", 30029, model.QuestTypeEvent, 100030, false},
		{"daily as extra clear does not count", 30029, model.QuestTypeExtra, 100002, false},
		{"guerrilla event clear counts", 30030, model.QuestTypeEvent, 100025, true},
		{"guerrilla upper bound counts", 30030, model.QuestTypeEvent, 100048, true},
		{"daily quest is not guerrilla", 30030, model.QuestTypeEvent, 100002, false},
		{"guerrilla as extra clear does not count", 30030, model.QuestTypeExtra, 100025, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := questMissionOptionMatches(nil, tc.option, 1, tc.questType, tc.questID, 10)
			if got != tc.want {
				t.Fatalf("questMissionOptionMatches(option=%d type=%v quest=%d) = %v, want %v",
					tc.option, tc.questType, tc.questID, got, tc.want)
			}
		})
	}
}
