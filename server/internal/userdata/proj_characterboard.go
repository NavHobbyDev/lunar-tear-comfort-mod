package userdata

import (
	"sort"

	"lunar-tear/server/internal/store"
	"lunar-tear/server/internal/utils"
)

func init() {
	register("IUserCharacterBoard", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(sortedCharacterBoardRecords(user)...)
		return s
	})
	register("IUserCharacterBoardAbility", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(sortedCharacterBoardAbilityRecords(user)...)
		return s
	})
	register("IUserCharacterBoardStatusUp", func(user store.UserState) string {
		s, _ := utils.EncodeJSONMaps(sortedCharacterBoardStatusUpRecords(user)...)
		return s
	})
	registerStatic("IUserCharacterBoardCompleteReward")
}

func sortedCharacterBoardRecords(user store.UserState) []map[string]any {
	ids := make([]int, 0, len(user.CharacterBoards))
	for id := range user.CharacterBoards {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)

	records := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		row := user.CharacterBoards[int32(id)]
		records = append(records, map[string]any{
			"userId":           user.UserId,
			"characterBoardId": row.CharacterBoardId,
			"panelReleaseBit1": row.PanelReleaseBit1,
			"panelReleaseBit2": row.PanelReleaseBit2,
			"panelReleaseBit3": row.PanelReleaseBit3,
			"panelReleaseBit4": row.PanelReleaseBit4,
			"latestVersion":    row.LatestVersion,
		})
	}
	return records
}

func sortedCharacterBoardAbilityRecords(user store.UserState) []map[string]any {
	type entry struct {
		key   store.CharacterBoardAbilityKey
		state store.CharacterBoardAbilityState
	}
	entries := make([]entry, 0, len(user.CharacterBoardAbilities))
	for k, v := range user.CharacterBoardAbilities {
		entries = append(entries, entry{k, v})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].key.CharacterId != entries[j].key.CharacterId {
			return entries[i].key.CharacterId < entries[j].key.CharacterId
		}
		return entries[i].key.AbilityId < entries[j].key.AbilityId
	})

	records := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		records = append(records, map[string]any{
			"userId":        user.UserId,
			"characterId":   e.state.CharacterId,
			"abilityId":     e.state.AbilityId,
			"level":         e.state.Level,
			"latestVersion": e.state.LatestVersion,
		})
	}
	return records
}

func sortedCharacterBoardStatusUpRecords(user store.UserState) []map[string]any {
	type entry struct {
		key   store.CharacterBoardStatusUpKey
		state store.CharacterBoardStatusUpState
	}
	entries := make([]entry, 0, len(user.CharacterBoardStatusUps))
	for k, v := range user.CharacterBoardStatusUps {
		entries = append(entries, entry{k, v})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].key.CharacterId != entries[j].key.CharacterId {
			return entries[i].key.CharacterId < entries[j].key.CharacterId
		}
		return entries[i].key.StatusCalculationType < entries[j].key.StatusCalculationType
	})

	records := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		records = append(records, map[string]any{
			"userId":                user.UserId,
			"characterId":           e.state.CharacterId,
			"statusCalculationType": e.state.StatusCalculationType,
			"hp":                    e.state.Hp,
			"attack":                e.state.Attack,
			"vitality":              e.state.Vitality,
			"agility":               e.state.Agility,
			"criticalRatio":         e.state.CriticalRatio,
			"criticalAttack":        e.state.CriticalAttack,
			"latestVersion":         e.state.LatestVersion,
		})
	}
	return records
}
