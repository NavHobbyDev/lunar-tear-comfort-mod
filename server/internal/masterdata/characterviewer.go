package masterdata

import (
	"log"
	"sort"

	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
	"lunar-tear/server/internal/utils"
)

type characterViewerFieldEntry struct {
	FieldId         int32
	RequiredQuestId int32
}

type CharacterViewerCatalog struct {
	fields []characterViewerFieldEntry
}

func (c *CharacterViewerCatalog) ReleasedFieldIds(user store.UserState) []int32 {
	var released []int32

	for _, f := range c.fields {
		isReleased := false

		// Check whether the location is unlocked by the quest
		if f.RequiredQuestId == 0 {
			isReleased = true
		} else {
			q, ok := user.Quests[f.RequiredQuestId]
			if ok && q.QuestStateType == model.UserQuestStateTypeCleared {
				isReleased = true
			}
		}

		if isReleased {
			released = append(released, f.FieldId)
		}
	}

	return released
}

// NewlyReleasedFieldIds returns locations that have become available but for which
// the client has not yet been shown the "new location" notice
func (c *CharacterViewerCatalog) NewlyReleasedFieldIds(user store.UserState) []int32 {
	var newlyReleased []int32

	released := c.ReleasedFieldIds(user)
	for _, fieldId := range released {
		// If the location was unlocked but the client has not yet been shown the notice
		if viewed, ok := user.CharacterViewerFields[fieldId]; !ok || !viewed.IsViewed {
			newlyReleased = append(newlyReleased, fieldId)
		}
	}

	return newlyReleased
}

func LoadCharacterViewerCatalog(resolver *ConditionResolver) *CharacterViewerCatalog {
	fields, err := utils.ReadTable[EntityMCharacterViewerField]("m_character_viewer_field")
	if err != nil {
		log.Fatalf("load character viewer field table: %v", err)
	}

	cat := &CharacterViewerCatalog{}
	for _, f := range fields {
		entry := characterViewerFieldEntry{FieldId: f.CharacterViewerFieldId}
		if qid, ok := resolver.RequiredQuestId(f.ReleaseEvaluateConditionId); ok {
			entry.RequiredQuestId = qid
		}
		cat.fields = append(cat.fields, entry)
	}

	sort.Slice(cat.fields, func(i, j int) bool {
		return cat.fields[i].FieldId < cat.fields[j].FieldId
	})

	log.Printf("character viewer catalog loaded: %d fields", len(cat.fields))
	return cat
}
