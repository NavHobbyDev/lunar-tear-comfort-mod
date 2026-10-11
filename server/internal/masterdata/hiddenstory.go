package masterdata

import (
	"log"

	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
	"lunar-tear/server/internal/utils"
)

type HiddenStoryRequirements struct {
	MissionIds    []int32
	QuestMissions []store.QuestMissionKey
}

func LoadHiddenStoryRequirements() HiddenStoryRequirements {
	var empty HiddenStoryRequirements

	gimmicks, err := utils.ReadTable[EntityMGimmick]("m_gimmick")
	if err != nil {
		log.Printf("[hiddenstory] m_gimmick unavailable: %v", err)
		return empty
	}
	conditions, err := utils.ReadTable[EntityMEvaluateCondition]("m_evaluate_condition")
	if err != nil {
		log.Printf("[hiddenstory] m_evaluate_condition unavailable: %v", err)
		return empty
	}
	valueGroups, err := utils.ReadTable[EntityMEvaluateConditionValueGroup]("m_evaluate_condition_value_group")
	if err != nil {
		log.Printf("[hiddenstory] m_evaluate_condition_value_group unavailable: %v", err)
		return empty
	}

	condById := make(map[int32]EntityMEvaluateCondition, len(conditions))
	for _, c := range conditions {
		condById[c.EvaluateConditionId] = c
	}
	valuesByGroup := make(map[int32]map[int32]int64)
	for _, vg := range valueGroups {
		g := valuesByGroup[vg.EvaluateConditionValueGroupId]
		if g == nil {
			g = make(map[int32]int64)
			valuesByGroup[vg.EvaluateConditionValueGroupId] = g
		}
		g[vg.GroupIndex] = vg.Value
	}

	missionSet := make(map[int32]struct{})
	questMissionSet := make(map[store.QuestMissionKey]struct{})
	seen := make(map[int32]bool)

	var resolve func(conditionId int32, depth int)
	resolve = func(conditionId int32, depth int) {
		if conditionId == 0 || depth > 16 || seen[conditionId] {
			return
		}
		seen[conditionId] = true
		c, ok := condById[conditionId]
		if !ok {
			return
		}
		group := valuesByGroup[c.EvaluateConditionValueGroupId]
		switch model.EvaluateConditionFunctionType(c.EvaluateConditionFunctionType) {
		case model.EvaluateConditionFunctionTypeRecursion:
			// Value-group entries are sub-condition ids; satisfying all leaves makes
			// both AND and OR recursion conditions evaluate true.
			for _, sub := range group {
				resolve(int32(sub), depth+1)
			}
		case model.EvaluateConditionFunctionTypeMissionClear:
			if v, ok := group[defaultGroupIndex]; ok {
				missionSet[int32(v)] = struct{}{}
			}
		case model.EvaluateConditionFunctionTypeQuestMissionClear:
			questId, ok1 := group[1]
			questMissionId, ok2 := group[2]
			if ok1 && ok2 {
				questMissionSet[store.QuestMissionKey{
					QuestId:        int32(questId),
					QuestMissionId: int32(questMissionId),
				}] = struct{}{}
			}
		case model.EvaluateConditionFunctionTypeQuestSceneChoice:
			// Scene choice conditions are evaluated dynamically per user at runtime.
			// They check user.QuestSceneChoiceHistory for selected effects.
			// No static requirement can be determined here.
		}
	}

	for _, g := range gimmicks {
		switch model.GimmickType(g.GimmickType) {
		case model.GimmickTypeReport, model.GimmickTypeCageMemory:
			resolve(g.ClearEvaluateConditionId, 0)
		}
	}

	req := HiddenStoryRequirements{}
	for id := range missionSet {
		req.MissionIds = append(req.MissionIds, id)
	}
	for key := range questMissionSet {
		req.QuestMissions = append(req.QuestMissions, key)
	}
	log.Printf("hidden-story requirements: %d missions, %d quest-missions", len(req.MissionIds), len(req.QuestMissions))
	return req
}
