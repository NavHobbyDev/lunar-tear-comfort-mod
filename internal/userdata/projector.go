package userdata

import (
	"sort"

	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/questflow"
	"lunar-tear/server/internal/store"
)

type Projector func(user store.UserState) string

var projectors = make(map[string]Projector)

var questHandler *questflow.QuestHandler

func SetQuestHandler(h *questflow.QuestHandler) {
	questHandler = h
}

var dokanCatalog *masterdata.DokanCatalog

// SetDokanCatalog publishes the loaded m_dokan rows to the projection layer so
// sortedDokanRecords can fake-confirm active dokans for players who disabled
// news popups (per-user user_options.show_news).
func SetDokanCatalog(cat *masterdata.DokanCatalog) { dokanCatalog = cat }

func register(tableName string, fn Projector) {
	projectors[tableName] = fn
}

func registerStatic(tableNames ...string) {
	for _, name := range tableNames {
		projectors[name] = func(_ store.UserState) string { return "[]" }
	}
}

func projectTable(tableName string, user store.UserState) string {
	fn, ok := projectors[tableName]
	if !ok {
		return "[]"
	}
	s := fn(user)
	if s == "" {
		return "[]"
	}
	return s
}

func sortedStringKeys[T any](rows map[string]T) []string {
	keys := make([]string, 0, len(rows))
	for key := range rows {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func compareGimmickKey(a, b store.GimmickKey) int {
	if a.GimmickSequenceScheduleId != b.GimmickSequenceScheduleId {
		if a.GimmickSequenceScheduleId < b.GimmickSequenceScheduleId {
			return -1
		}
		return 1
	}
	if a.GimmickSequenceId != b.GimmickSequenceId {
		if a.GimmickSequenceId < b.GimmickSequenceId {
			return -1
		}
		return 1
	}
	if a.GimmickId < b.GimmickId {
		return -1
	}
	if a.GimmickId > b.GimmickId {
		return 1
	}
	return 0
}

func compareGimmickOrnamentKey(a, b store.GimmickOrnamentKey) int {
	if cmp := compareGimmickKey(
		store.GimmickKey{
			GimmickSequenceScheduleId: a.GimmickSequenceScheduleId,
			GimmickSequenceId:         a.GimmickSequenceId,
			GimmickId:                 a.GimmickId,
		},
		store.GimmickKey{
			GimmickSequenceScheduleId: b.GimmickSequenceScheduleId,
			GimmickSequenceId:         b.GimmickSequenceId,
			GimmickId:                 b.GimmickId,
		},
	); cmp != 0 {
		return cmp
	}
	if a.GimmickOrnamentIndex < b.GimmickOrnamentIndex {
		return -1
	}
	if a.GimmickOrnamentIndex > b.GimmickOrnamentIndex {
		return 1
	}
	return 0
}
