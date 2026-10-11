package main

import (
	"errors"
	"fmt"
	"log"
	"sort"
	"time"

	"lunar-tear/server/internal/campaign"
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/masterdata/memorydb"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/questflow"
	"lunar-tear/server/internal/store"
	"lunar-tear/server/internal/utils"
)

// questInfo is one row in the list_quests response. Display names are resolved
// on the Python side from data/names/*.json (keyed by quest_id), so the shim
// only emits ids, grouping, and per-user cleared status.
type questInfo struct {
	QuestID   int32  `json:"quest_id"`
	Kind      string `json:"kind"` // "main" | "event" | "other"
	ChapterID int32  `json:"chapter_id"`
	Cleared   bool   `json:"cleared"`
}

// questEnv bundles the catalog + handler + derived lookups needed to both list
// and clear quests. Built once per shim invocation.
type questEnv struct {
	catalog      *masterdata.QuestCatalog
	handler      *questflow.QuestHandler
	eventChapter map[int32]int32 // questId -> eventQuestChapterId (event quests only)
	mainOrder    map[int32]int   // questId -> index in OrderedQuestIds (main quests only)
}

// loadQuestEnv mirrors runtime/build.go's construction of the quest handler so a
// faithful clear runs exactly the same code path the live server's FinishQuest
// RPCs use.
func loadQuestEnv(masterDataPath string) (*questEnv, error) {
	if masterDataPath == "" {
		return nil, errors.New("master_data_path required")
	}
	if err := memorydb.Init(masterDataPath); err != nil {
		return nil, fmt.Errorf("init master data: %w", err)
	}
	gameConfig, err := masterdata.LoadGameConfig()
	if err != nil {
		return nil, fmt.Errorf("load game config: %w", err)
	}
	partsCatalog, err := masterdata.LoadPartsCatalog()
	if err != nil {
		return nil, fmt.Errorf("load parts catalog: %w", err)
	}
	catalog, err := masterdata.LoadQuestCatalog(partsCatalog)
	if err != nil {
		return nil, fmt.Errorf("load quest catalog: %w", err)
	}
	sideStory := masterdata.LoadSideStoryCatalog()
	campaignCatalog, err := campaign.Load()
	if err != nil {
		return nil, fmt.Errorf("load campaign catalog: %w", err)
	}
	rebirth, err := masterdata.LoadCharacterRebirthCatalog()
	if err != nil {
		return nil, fmt.Errorf("load character rebirth catalog: %w", err)
	}
	materialCatalog, err := masterdata.LoadMaterialCatalog()
	if err != nil {
		return nil, fmt.Errorf("load material catalog: %w", err)
	}
	consumableItemCatalog, err := masterdata.LoadConsumableItemCatalog()
	if err != nil {
		return nil, fmt.Errorf("load consumable item catalog: %w", err)
	}
	handler := questflow.NewQuestHandler(catalog, gameConfig, sideStory, campaignCatalog, rebirth, materialCatalog, consumableItemCatalog)

	eventChapter, err := buildEventChapterByQuestId()
	if err != nil {
		return nil, err
	}

	mainOrder := make(map[int32]int, len(catalog.OrderedQuestIds))
	for i, qid := range catalog.OrderedQuestIds {
		mainOrder[qid] = i
	}

	return &questEnv{
		catalog:      catalog,
		handler:      handler,
		eventChapter: eventChapter,
		mainOrder:    mainOrder,
	}, nil
}

// buildEventChapterByQuestId resolves each event quest to its chapter by walking
// chapter -> sequence group -> sequences -> quests, the same traversal the quest
// catalog uses for chapter memoirs (masterdata/quest.go).
func buildEventChapterByQuestId() (map[int32]int32, error) {
	chapters, err := utils.ReadTable[masterdata.EntityMEventQuestChapter]("m_event_quest_chapter")
	if err != nil {
		return nil, fmt.Errorf("load event quest chapter table: %w", err)
	}
	groups, err := utils.ReadTable[masterdata.EntityMEventQuestSequenceGroup]("m_event_quest_sequence_group")
	if err != nil {
		return nil, fmt.Errorf("load event quest sequence group table: %w", err)
	}
	seqs, err := utils.ReadTable[masterdata.EntityMEventQuestSequence]("m_event_quest_sequence")
	if err != nil {
		return nil, fmt.Errorf("load event quest sequence table: %w", err)
	}
	questIdsBySeq := make(map[int32][]int32)
	for _, s := range seqs {
		questIdsBySeq[s.EventQuestSequenceId] = append(questIdsBySeq[s.EventQuestSequenceId], s.QuestId)
	}
	seqIdsByGroup := make(map[int32][]int32)
	for _, g := range groups {
		seqIdsByGroup[g.EventQuestSequenceGroupId] = append(seqIdsByGroup[g.EventQuestSequenceGroupId], g.EventQuestSequenceId)
	}
	out := make(map[int32]int32)
	for _, ec := range chapters {
		for _, seqId := range seqIdsByGroup[ec.EventQuestSequenceGroupId] {
			for _, qid := range questIdsBySeq[seqId] {
				out[qid] = ec.EventQuestChapterId
			}
		}
	}
	return out, nil
}

func (e *questEnv) kindAndChapter(questId int32) (kind string, chapterId int32) {
	if c, ok := e.catalog.MainQuestChapterIdByQuestId[questId]; ok {
		return "main", c
	}
	if c, ok := e.eventChapter[questId]; ok {
		return "event", c
	}
	return "other", 0
}

// rank yields a canonical (group, subrank) so a batch is always cleared in story
// order: main-flow quests first by their OrderedQuestIds index (so scene
// progression advances correctly), then other main quests, then events.
func (e *questEnv) rank(questId int32) (int, int) {
	if _, isEvent := e.eventChapter[questId]; isEvent {
		return 1, int(questId)
	}
	if o, ok := e.mainOrder[questId]; ok {
		return 0, o
	}
	return 0, 1_000_000 + int(questId)
}

// ensureQuestState mirrors questflow.initQuestState (unexported). The event
// finish assumes HandleEventQuestStart already created the quest + its mission
// rows; an offline finish-only clear skips Start, so without this
// evaluateFinishOutcome panics ("unknown questId"). The main finish inits this
// itself, so the call is a harmless no-op there.
func (e *questEnv) ensureQuestState(u *store.UserState, questId int32) {
	qs := u.Quests[questId]
	qs.QuestId = questId
	u.Quests[questId] = qs
	for _, missionId := range e.catalog.MissionIdsByQuestId[questId] {
		key := store.QuestMissionKey{QuestId: questId, QuestMissionId: missionId}
		m := u.QuestMissions[key]
		m.QuestId = questId
		m.QuestMissionId = missionId
		u.QuestMissions[key] = m
	}
}

func runListQuests(req *request) (int, error) {
	env, err := loadQuestEnv(req.MasterDataPath)
	if err != nil {
		return 0, err
	}
	db, st, err := openDB(req.DBPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	user, err := st.LoadUser(req.UserID)
	if err != nil {
		return 0, fmt.Errorf("load user: %w", err)
	}

	out := make([]questInfo, 0, len(env.catalog.QuestById))
	for questId := range env.catalog.QuestById {
		kind, chapterId := env.kindAndChapter(questId)
		cleared := user.Quests[questId].QuestStateType == model.UserQuestStateTypeCleared
		out = append(out, questInfo{QuestID: questId, Kind: kind, ChapterID: chapterId, Cleared: cleared})
	}
	sort.Slice(out, func(i, j int) bool {
		gi, si := env.rank(out[i].QuestID)
		gj, sj := env.rank(out[j].QuestID)
		if gi != gj {
			return gi < gj
		}
		if out[i].ChapterID != out[j].ChapterID {
			return out[i].ChapterID < out[j].ChapterID
		}
		return si < sj
	})
	queryQuests = out
	return len(out), nil
}

func runClearQuests(req *request) (int, error) {
	if len(req.QuestIDs) == 0 {
		return 0, errors.New("quest_ids list is empty")
	}
	env, err := loadQuestEnv(req.MasterDataPath)
	if err != nil {
		return 0, err
	}
	db, st, err := openDB(req.DBPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	// Clear in canonical order so a main-story batch advances the scene pointer
	// in the right sequence.
	ids := append([]int32(nil), req.QuestIDs...)
	sort.Slice(ids, func(i, j int) bool {
		gi, si := env.rank(ids[i])
		gj, sj := env.rank(ids[j])
		if gi != gj {
			return gi < gj
		}
		return si < sj
	})

	now := time.Now().UnixMilli()
	applied := 0
	_, err = st.UpdateUser(req.UserID, func(u *store.UserState) {
		u.EnsureMaps()
		for _, questId := range ids {
			if _, ok := env.catalog.QuestById[questId]; !ok {
				continue // unknown id
			}
			if u.Quests[questId].QuestStateType == model.UserQuestStateTypeCleared {
				continue // already cleared -- skip so drops are not re-rolled
			}
			func() {
				// One quest's failure must not abort the rest of the batch.
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[clear_quests] quest %d failed: %v", questId, r)
					}
				}()
				env.ensureQuestState(u, questId)
				// Event quests are exactly those resolvable to an event chapter;
				// everything else (all main-story quests, incl. non-sequence
				// ones) goes through the main finish.
				if chapterId, isEvent := env.eventChapter[questId]; isEvent {
					env.handler.HandleEventQuestFinish(u, chapterId, questId, false, false, now)
				} else {
					env.handler.HandleQuestFinish(u, questId, false, false, now)
				}
				applied++
			}()
		}
	})
	if err != nil {
		return 0, fmt.Errorf("clear quests: %w", err)
	}
	return applied, nil
}
