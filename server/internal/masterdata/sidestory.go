package masterdata

import (
	"log"
	"sort"

	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/utils"
)

type SideStorySceneInfo struct {
	SceneId int32
	Type    model.SideStorySceneIdType
}

type SideStoryQuestInfo struct {
	SideStoryQuestId int32
	Scenes           []SideStorySceneInfo // the 7 scenes, one per type
	Quests           []int32              // ordered event quests (the chapter+difficulty sequence)
}

type SideStoryCatalog struct {
	QuestById             map[int32]*SideStoryQuestInfo
	ChapterByEventQuestId map[int32]int32 // event quest id -> side story chapter id
}

func (q *SideStoryQuestInfo) SceneIdByType(t model.SideStorySceneIdType) (int32, bool) {
	for _, s := range q.Scenes {
		if s.Type == t {
			return s.SceneId, true
		}
	}
	return 0, false
}

func LoadSideStoryCatalog() *SideStoryCatalog {
	scenes, err := utils.ReadTable[EntityMSideStoryQuestScene]("m_side_story_quest_scene")
	if err != nil {
		log.Fatalf("load side story quest scene table: %v", err)
	}
	limitContents, err := utils.ReadTable[EntityMSideStoryQuestLimitContent]("m_side_story_quest_limit_content")
	if err != nil {
		log.Fatalf("load side story quest limit content table: %v", err)
	}
	seqGroups, err := utils.ReadTable[EntityMEventQuestSequenceGroup]("m_event_quest_sequence_group")
	if err != nil {
		log.Fatalf("load event quest sequence group table: %v", err)
	}
	sequences, err := utils.ReadTable[EntityMEventQuestSequence]("m_event_quest_sequence")
	if err != nil {
		log.Fatalf("load event quest sequence table: %v", err)
	}

	seqRows := make(map[int32][]EntityMEventQuestSequence)
	for _, s := range sequences {
		seqRows[s.EventQuestSequenceId] = append(seqRows[s.EventQuestSequenceId], s)
	}
	orderedQuestIds := make(map[int32][]int32, len(seqRows))
	for seqId, rows := range seqRows {
		sort.Slice(rows, func(i, j int) bool { return rows[i].SortOrder < rows[j].SortOrder })
		ids := make([]int32, len(rows))
		for i, r := range rows {
			ids[i] = r.QuestId
		}
		orderedQuestIds[seqId] = ids
	}

	// (chapterId, difficulty) -> sequenceId. Sequence group id == chapter id.
	type chapDiff struct{ chapter, difficulty int32 }
	sequenceByChapterDiff := make(map[chapDiff]int32, len(seqGroups))
	for _, g := range seqGroups {
		sequenceByChapterDiff[chapDiff{g.EventQuestSequenceGroupId, g.DifficultyType}] = g.EventQuestSequenceId
	}

	// sideStoryQuestId -> limit content row. Limit content id == side story quest id.
	limitByQuest := make(map[int32]EntityMSideStoryQuestLimitContent, len(limitContents))
	for _, lc := range limitContents {
		limitByQuest[lc.SideStoryQuestLimitContentId] = lc
	}

	// sideStoryQuestId -> scene rows
	scenesByQuest := make(map[int32][]EntityMSideStoryQuestScene)
	for _, sc := range scenes {
		scenesByQuest[sc.SideStoryQuestId] = append(scenesByQuest[sc.SideStoryQuestId], sc)
	}

	questById := make(map[int32]*SideStoryQuestInfo, len(scenesByQuest))
	chapterByEventQuest := make(map[int32]int32)

	for ssqId, rows := range scenesByQuest {
		sort.Slice(rows, func(i, j int) bool { return rows[i].SortOrder < rows[j].SortOrder })

		var orderedQuests []int32
		var chapterId, difficulty int32
		if lc, ok := limitByQuest[ssqId]; ok {
			chapterId = lc.EventQuestChapterId
			difficulty = lc.DifficultyType
			if seqId, ok := sequenceByChapterDiff[chapDiff{chapterId, difficulty}]; ok {
				orderedQuests = orderedQuestIds[seqId]
			}
		}
		if chapterId != 0 {
			for _, questId := range orderedQuests {
				chapterByEventQuest[questId] = chapterId
			}
		}

		info := &SideStoryQuestInfo{
			SideStoryQuestId: ssqId,
			Scenes:           make([]SideStorySceneInfo, 0, len(rows)),
			Quests:           orderedQuests,
		}
		for _, sc := range rows {
			info.Scenes = append(info.Scenes, SideStorySceneInfo{
				SceneId: sc.SideStoryQuestSceneId,
				Type:    model.SideStorySceneIdType(sc.SortOrder),
			})
		}
		questById[ssqId] = info
	}

	log.Printf("side story catalog loaded: %d quests, %d scenes", len(questById), len(scenes))
	return &SideStoryCatalog{
		QuestById:             questById,
		ChapterByEventQuestId: chapterByEventQuest,
	}
}
