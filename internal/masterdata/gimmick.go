package masterdata

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"sync"

	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
	"lunar-tear/server/internal/utils"
)

// MaxUserGimmickRows is the server's per-table cap for gimmick projections and the
// in-memory user.Gimmick.Sequences map. The client hard-caps each of its
// IUserGimmick* tables at 1024 rows; we project up to this many with a small
// safety margin so we never overflow client buffers. Shared by InitSequenceSchedule
// (limits user.Gimmick.Sequences map size) and the IUserGimmick / Ornament /
// Sequence projections.
const MaxUserGimmickRows = 1000

// fickleBirdsPerDay is how many of a chapter's fickle birds are surfaced on any given
// server day. The live service released these birds in small daily batches; on a private
// server their historical StartDatetimes are all long past, so without throttling a whole
// chapter pool (8-23 birds) floods the map at once. We instead split each chapter's pool
// into blocks of this size and show one block per day, cycling back to the first block
// once the pool has been exhausted ("по кругу"). Override with FICKLE_BIRD_PER_DAY
// (integer); <=0 disables the rotation and shows the whole pool immediately.
var fickleBirdsPerDay = sync.OnceValue(func() int {
	n := 4
	if s := os.Getenv("FICKLE_BIRD_PER_DAY"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			n = v
		} else {
			log.Printf("[gimmick] FICKLE_BIRD_PER_DAY=%q invalid, using %d", s, n)
		}
	}
	return n
})

type gimmickScheduleEntry struct {
	ScheduleId         int32
	StartDatetime      int64
	EndDatetime        int64
	FirstSequenceId    int32
	Type               model.GimmickType // the first sequence's gimmick type
	GateQuestId        int32             // last Normal main-quest of the gimmick's chapter; 0 = none
	ReleaseConditionId int32             // m_gimmick_sequence_schedule.ReleaseEvaluateConditionId
	IsHidden           bool              // report / cage-memory sequence — used only for load logging
	Rank               int               // trim priority — see gimmickTypeRank
	RotIndex           int32             // fickle-birds only: position within its chapter's rotation pool (0-based)
	RotSize            int32             // fickle-birds only: number of birds in the chapter's rotation pool
}

func readGimmickTable[T any](name, what string) ([]T, bool) {
	rows, err := utils.ReadTable[T](name)
	if err != nil {
		log.Printf("[gimmick] %s unavailable, %s empty: %v", name, what, err)
		return nil, false
	}
	return rows, true
}

func gimmickTypeRank(t model.GimmickType) int {
	switch t {
	case model.GimmickTypeReport: // hidden missions / stories
		return 0
	case model.GimmickTypeCageMemory: // lost archives
		return 1
	case model.GimmickTypeCageTreasureHunt: // treasure
		return 2
	case model.GimmickTypeBrokenObelisk, model.GimmickTypeFirstBrokenObelisk:
		return 3
	case model.GimmickTypeIronGrill:
		return 4
	case model.GimmickTypeRadioMessage:
		return 5
	case model.GimmickTypeMapOnlyCageTreasureHunt, model.GimmickTypeMapOnlyHideObelisk:
		return 6
	case model.GimmickTypeCageIntervalDropItem, model.GimmickTypeMapOnlyCageIntervalDrop:
		return 7 // birds — bottom
	}
	return 8
}

type gimmickTypeTables struct {
	byGimmick  map[int32]model.GimmickType
	bySequence map[int32]model.GimmickType
}

var gimmickTypes = sync.OnceValue(loadGimmickTypes)

func loadGimmickTypes() gimmickTypeTables {
	empty := gimmickTypeTables{
		byGimmick:  map[int32]model.GimmickType{},
		bySequence: map[int32]model.GimmickType{},
	}

	gimmicks, ok := readGimmickTable[EntityMGimmick]("m_gimmick", "type tables")
	if !ok {
		return empty
	}
	groups, ok := readGimmickTable[EntityMGimmickGroup]("m_gimmick_group", "type tables")
	if !ok {
		return empty
	}
	sequences, ok := readGimmickTable[EntityMGimmickSequence]("m_gimmick_sequence", "type tables")
	if !ok {
		return empty
	}

	byGimmick := make(map[int32]model.GimmickType, len(gimmicks))
	for _, g := range gimmicks {
		byGimmick[g.GimmickId] = model.GimmickType(g.GimmickType)
	}
	typeByGroup := make(map[int32]model.GimmickType, len(groups))
	for _, grp := range groups {
		if _, seen := typeByGroup[grp.GimmickGroupId]; seen {
			continue
		}
		if t, ok := byGimmick[grp.GimmickId]; ok {
			typeByGroup[grp.GimmickGroupId] = t
		}
	}
	bySequence := make(map[int32]model.GimmickType, len(sequences))
	for _, seq := range sequences {
		if t, ok := typeByGroup[seq.GimmickGroupId]; ok {
			bySequence[seq.GimmickSequenceId] = t
		}
	}
	return gimmickTypeTables{byGimmick: byGimmick, bySequence: bySequence}
}

func gimmickSequenceTypes() map[int32]model.GimmickType {
	return gimmickTypes().bySequence
}

func LoadGimmickSequenceRanks() map[int32]int {
	types := gimmickSequenceTypes()
	out := make(map[int32]int, len(types))
	for sid, t := range types {
		out[sid] = gimmickTypeRank(t)
	}
	return out
}

// normalDifficultyType is the MainQuestSequenceGroup.DifficultyType for the Normal
// lane. A chapter's "last quest" — the gimmick release gate — is taken from that
// lane so gimmick icons unlock exactly when the player finishes the chapter's main
// story, matching the way the client presents the area as clearable.
const normalDifficultyType int32 = 1

// loadGimmickChapterGates maps each gimmick sequence to the last Normal main-quest
// of the chapter its ornaments live in. The chain is:
//
//	m_gimmick_sequence.GimmickGroupId -> m_gimmick_group -> m_gimmick.GimmickOrnamentGroupId
//	m_gimmick_ornament.ChapterId (= MainQuestChapterId)
//	m_main_quest_chapter.MainQuestSequenceGroupId -> m_main_quest_sequence_group (Normal)
//	m_main_quest_sequence (highest SortOrder) -> QuestId
//
// Every gimmick sequence resolves to exactly one chapter in the shipped data. If any
// source table is unavailable the map is returned empty, so gating simply falls back
// to "always visible" rather than hiding content.
// LoadGimmickChapterGates returns the memoised map produced by loadGimmickChapterGates.
// Projection code uses it to gate chain-walk expansion by the same chapter-clear rule
// the catalog's schedule gate applies, so a schedule whose chain crosses chapter
// boundaries never leaks future chapters' ornaments onto the world map.
func LoadGimmickChapterGates() map[int32]int32 { return chapterGatesOnce() }

var chapterGatesOnce = sync.OnceValue(loadGimmickChapterGates)

// LoadHiddenBirdSequenceIDs returns the set of gimmick sequence IDs whose first
// gimmick is a "Hidden Black Bird" (map-only cage treasure hunt, type 7). These are
// the only world-map gimmicks that are meant to be visible from the start of the
// game, so the projection walk treats them as gate-exempt.
func LoadHiddenBirdSequenceIDs() map[int32]bool { return hiddenBirdSeqOnce() }

var hiddenBirdSeqOnce = sync.OnceValue(loadHiddenBirdSequenceIDs)

func loadHiddenBirdSequenceIDs() map[int32]bool {
	types := gimmickSequenceTypes()
	out := make(map[int32]bool, len(types))
	for sid, t := range types {
		if t == model.GimmickTypeMapOnlyCageTreasureHunt {
			out[sid] = true
		}
	}
	return out
}

// LoadGimmickSequenceTypes exposes the per-sequence gimmick-type map so the
// projection layer can decide, chain member by chain member, whether the
// hidden-bird exemption applies.
func LoadGimmickSequenceTypes() map[int32]model.GimmickType { return gimmickSequenceTypes() }

// LoadGimmickScheduleFirstSeqs returns the set of gimmick sequence IDs that are the
// FirstGimmickSequenceId of at least one schedule. The projection layer uses this to
// distinguish chain-only content (no own schedule, safe to project if gate is cleared)
// from sequences that have their own schedule (cross-chapter projection should rely on
// independent eligibility, i.e. presence in user.Gimmick.Sequences).
func LoadGimmickScheduleFirstSeqs() map[int32]bool { return schedFirstSeqsOnce() }

var schedFirstSeqsOnce = sync.OnceValue(loadGimmickScheduleFirstSeqs)

func loadGimmickScheduleFirstSeqs() map[int32]bool {
	rows, ok := readGimmickTable[EntityMGimmickSequenceSchedule]("m_gimmick_sequence_schedule", "schedule firsts")
	if !ok {
		return map[int32]bool{}
	}
	out := make(map[int32]bool, len(rows))
	for _, r := range rows {
		out[r.FirstGimmickSequenceId] = true
	}
	return out
}

func loadGimmickChapterGates() map[int32]int32 {
	empty := map[int32]int32{}

	chapters, ok := readGimmickTable[EntityMMainQuestChapter]("m_main_quest_chapter", "chapter gates")
	if !ok {
		return empty
	}
	seqGroups, ok := readGimmickTable[EntityMMainQuestSequenceGroup]("m_main_quest_sequence_group", "chapter gates")
	if !ok {
		return empty
	}
	mainSeqs, ok := readGimmickTable[EntityMMainQuestSequence]("m_main_quest_sequence", "chapter gates")
	if !ok {
		return empty
	}
	sequences, ok := readGimmickTable[EntityMGimmickSequence]("m_gimmick_sequence", "chapter gates")
	if !ok {
		return empty
	}
	groups, ok := readGimmickTable[EntityMGimmickGroup]("m_gimmick_group", "chapter gates")
	if !ok {
		return empty
	}
	gimmicks, ok := readGimmickTable[EntityMGimmick]("m_gimmick", "chapter gates")
	if !ok {
		return empty
	}
	ornaments, ok := readGimmickTable[EntityMGimmickOrnament]("m_gimmick_ornament", "chapter gates")
	if !ok {
		return empty
	}

	// chapter's sequence group -> last Normal quest (max SortOrder).
	sortById := make(map[int32]EntityMMainQuestSequence, len(mainSeqs))
	for _, s := range mainSeqs {
		sortById[s.MainQuestSequenceId] = s
	}
	type lastEntry struct {
		sortOrder int32
		questId   int32
	}
	bestByGroup := make(map[int32]lastEntry)
	seen := make(map[int32]bool)
	for _, g := range seqGroups {
		if g.DifficultyType != normalDifficultyType {
			continue
		}
		sq, ok := sortById[g.MainQuestSequenceId]
		if !ok {
			continue
		}
		cur := bestByGroup[g.MainQuestSequenceGroupId]
		if !seen[g.MainQuestSequenceGroupId] || sq.SortOrder > cur.sortOrder {
			bestByGroup[g.MainQuestSequenceGroupId] = lastEntry{sq.SortOrder, sq.QuestId}
			seen[g.MainQuestSequenceGroupId] = true
		}
	}
	lastQuestByChapter := make(map[int32]int32, len(chapters))
	for _, c := range chapters {
		if e, ok := bestByGroup[c.MainQuestSequenceGroupId]; ok && e.questId != 0 {
			lastQuestByChapter[c.MainQuestChapterId] = e.questId
		}
	}

	// gimmick sequence -> chapter (via its gimmicks' ornament groups).
	chapterByOrnGroup := make(map[int32]int32)
	for _, o := range ornaments {
		if o.ChapterId == 0 {
			continue
		}
		if _, done := chapterByOrnGroup[o.GimmickOrnamentGroupId]; !done {
			chapterByOrnGroup[o.GimmickOrnamentGroupId] = o.ChapterId
		}
	}
	ornGroupByGimmick := make(map[int32]int32, len(gimmicks))
	for _, g := range gimmicks {
		ornGroupByGimmick[g.GimmickId] = g.GimmickOrnamentGroupId
	}
	gimmicksByGroup := make(map[int32][]int32)
	for _, grp := range groups {
		gimmicksByGroup[grp.GimmickGroupId] = append(gimmicksByGroup[grp.GimmickGroupId], grp.GimmickId)
	}

	out := make(map[int32]int32, len(sequences))
	for _, seq := range sequences {
		for _, gid := range gimmicksByGroup[seq.GimmickGroupId] {
			chapter, ok := chapterByOrnGroup[ornGroupByGimmick[gid]]
			if !ok {
				continue
			}
			if last, ok := lastQuestByChapter[chapter]; ok && last != 0 {
				out[seq.GimmickSequenceId] = last
				break
			}
		}
	}
	return out
}

type SequenceReward struct {
	PossessionType int32
	PossessionId   int32
	Count          int32
}

type GimmickCatalog struct {
	schedules              []gimmickScheduleEntry
	scheduleByKey          map[store.GimmickSequenceKey]gimmickScheduleEntry // (schedId, chain member seqId) -> schedule entry
	sequenceChains         map[int32][]int32                                 // first seq id -> full chain (BFS via NextGimmickSequenceGroupId)
	sequencePredecessors   map[int32][]int32                                 // seq id -> its immediate chain predecessors
	gimmicksBySequence     map[int32]map[int32]bool                          // seq id -> set of gimmick ids in its group
	clearConditionByGimmick map[int32]int32                                  // gimmick id -> ClearEvaluateConditionId
	hiddenSequences        map[int32]bool                                    // GimmickSequenceId -> report/cage-memory/hidden-bird
	sequenceRewards        map[int32][]SequenceReward                        // GimmickSequenceId -> clear rewards
	gimmickTypes           map[int32]model.GimmickType
	cageMemoryItems        map[int32]int32 // CageMemory GimmickId -> ImportantItemId (type 4)
	hiddenBirdRewards      map[GimmickOrnamentRef]SequenceReward
	fickleBirdOrnaments    map[GimmickOrnamentRef]*EntityMGimmickOrnament // (GimmickId, OrnamentIndex) -> ornament
	conditions             *ConditionResolver
}

func LoadGimmickCatalog(resolver *ConditionResolver, cageOrnaments *CageOrnamentCatalog) (*GimmickCatalog, error) {
	rows, err := utils.ReadTable[EntityMGimmickSequenceSchedule]("m_gimmick_sequence_schedule")
	if err != nil {
		return nil, fmt.Errorf("load gimmick sequence schedule table: %w", err)
	}

	seqTypes := gimmickSequenceTypes()
	chapterGates := LoadGimmickChapterGates()
	hiddenSeq := make(map[int32]bool, len(seqTypes))
	for sid, t := range seqTypes {
		if t == model.GimmickTypeReport || t == model.GimmickTypeCageMemory {
			hiddenSeq[sid] = true
		}
	}

	// Pick rule: prefer schedules with EndDatetime in the future (so the client's
	// in-cage filter doesn't drop them), then by lowest StartDatetime (longest
	// historical validity), tie-broken by lowest ScheduleId for determinism. The
	// future-end preference matters for "Fickle Black Birds" (type 1) where real
	// expiry dates vary; other types have EndDatetime = 9999-03-31 so the preference
	// is a no-op.
	now := gametime.NowMillis()
	bestBySeq := make(map[int32]gimmickScheduleEntry, len(rows))
	for _, r := range rows {
		entry := gimmickScheduleEntry{
			ScheduleId:         r.GimmickSequenceScheduleId,
			StartDatetime:      r.StartDatetime,
			EndDatetime:        r.EndDatetime,
			FirstSequenceId:    r.FirstGimmickSequenceId,
			Type:               seqTypes[r.FirstGimmickSequenceId],
			IsHidden:           hiddenSeq[r.FirstGimmickSequenceId],
			Rank:               gimmickTypeRank(seqTypes[r.FirstGimmickSequenceId]),
			GateQuestId:        chapterGates[r.FirstGimmickSequenceId],
			ReleaseConditionId: r.ReleaseEvaluateConditionId,
		}
		if existing, ok := bestBySeq[entry.FirstSequenceId]; ok {
			existingFuture := existing.EndDatetime > now
			entryFuture := entry.EndDatetime > now
			if existingFuture != entryFuture {
				// Future-end schedule wins over expired one.
				if existingFuture {
					continue
				}
			} else if existing.StartDatetime < entry.StartDatetime ||
				(existing.StartDatetime == entry.StartDatetime && existing.ScheduleId <= entry.ScheduleId) {
				continue
			}
		}
		bestBySeq[entry.FirstSequenceId] = entry
	}

	entries := make([]gimmickScheduleEntry, 0, len(bestBySeq))
	hiddenCount := 0
	for _, entry := range bestBySeq {
		if entry.IsHidden {
			hiddenCount++
		}
		entries = append(entries, entry)
	}
	dedupedCount := len(rows) - len(entries)

	// Sort by (Rank, ScheduleId) so ActiveScheduleKeys returns the priority order
	// directly: treasure/lost-archives/hidden-missions first, birds last. This is
	// what lets InitSequenceSchedule's 1000-row cap trim from the bottom.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Rank != entries[j].Rank {
			return entries[i].Rank < entries[j].Rank
		}
		return entries[i].ScheduleId < entries[j].ScheduleId
	})

	assignFickleBirdRotation(entries)

	sequenceRewards := loadGimmickSequenceRewards()
	cageMemoryItems := loadCageMemoryImportantItems(gimmickTypes().byGimmick)
	hiddenBirdRewards := loadHiddenBirdRewards(cageOrnaments)
	fickleBirdOrnaments := loadFickleBirdOrnaments(gimmickTypes().byGimmick)
	gimmicksBySequence, clearConditionByGimmick, err := loadGimmickConditionsBySequence()
	if err != nil {
		return nil, err
	}
	chains, predecessors := loadGimmickSequenceGraph()
	scheduleByKey := make(map[store.GimmickSequenceKey]gimmickScheduleEntry)
	for _, entry := range entries {
		for _, sequenceId := range chains[entry.FirstSequenceId] {
			scheduleByKey[store.GimmickSequenceKey{GimmickSequenceScheduleId: entry.ScheduleId, GimmickSequenceId: sequenceId}] = entry
		}
	}

	log.Printf("gimmick catalog loaded: %d schedules (%d hidden-content, %d duplicates dropped), %d reward sequences, %d cage-memory items, %d hidden-bird rewards, %d fickle-bird ornaments",
		len(entries), hiddenCount, dedupedCount, len(sequenceRewards), len(cageMemoryItems), len(hiddenBirdRewards), len(fickleBirdOrnaments))
	return &GimmickCatalog{
		schedules:               entries,
		scheduleByKey:           scheduleByKey,
		sequenceChains:          chains,
		sequencePredecessors:    predecessors,
		gimmicksBySequence:      gimmicksBySequence,
		clearConditionByGimmick: clearConditionByGimmick,
		hiddenSequences:         hiddenSeq,
		sequenceRewards:         sequenceRewards,
		gimmickTypes:            gimmickTypes().byGimmick,
		cageMemoryItems:         cageMemoryItems,
		hiddenBirdRewards:       hiddenBirdRewards,
		fickleBirdOrnaments:     fickleBirdOrnaments,
		conditions:              resolver,
	}, nil
}

// assignFickleBirdRotation orders each chapter's fickle birds and records their position
// in the chapter's rotation pool. Birds are grouped by GateQuestId (the chapter's gating
// quest) and sorted by historical StartDatetime (tie-broken by ScheduleId), so the daily
// blocks replay the live-service release order. scheduleEligible turns (RotIndex, RotSize)
// into "is this bird in today's block?". Entries in a chapter with no gate quest, or when
// the rotation is disabled (FICKLE_BIRD_PER_DAY <= 0), keep RotSize = 0 (always visible).
func assignFickleBirdRotation(entries []gimmickScheduleEntry) {
	if fickleBirdsPerDay() <= 0 {
		return
	}
	byGate := map[int32][]*gimmickScheduleEntry{}
	for i := range entries {
		e := &entries[i]
		if e.Type != model.GimmickTypeCageTreasureHunt || e.GateQuestId == 0 {
			continue
		}
		byGate[e.GateQuestId] = append(byGate[e.GateQuestId], e)
	}
	for _, group := range byGate {
		sort.Slice(group, func(i, j int) bool {
			if group[i].StartDatetime != group[j].StartDatetime {
				return group[i].StartDatetime < group[j].StartDatetime
			}
			return group[i].ScheduleId < group[j].ScheduleId
		})
		n := int32(len(group))
		for idx, e := range group {
			e.RotIndex = int32(idx)
			e.RotSize = n
		}
	}
}

// HiddenBirdReward returns the per-tap reward for a MAP_ONLY_CAGE_TREASURE_HUNT
// ("Hidden Black Birds", type 7) ornament. Returns false if there's no mapping
// (e.g. the ornament view has no corresponding cage-ornament-reward entry).
func (c *GimmickCatalog) HiddenBirdReward(gimmickId, ornamentIndex int32) (SequenceReward, bool) {
	r, ok := c.hiddenBirdRewards[GimmickOrnamentRef{GimmickId: gimmickId, OrnamentIndex: ornamentIndex}]
	return r, ok
}

// FickleBirdCageOrnamentId calculates the cage ornament ID for a CAGE_TREASURE_HUNT
// ("Fickle Black Birds", type 1) ornament. Each chapter's m_cage_ornament exposes up to
// four reward slots, 1_000_000 + ChapterId*1_000 + {101,102,103,104}. Almost every fickle
// bird carries GimmickOrnamentViewId==101, so the view id alone cannot pick a slot — the
// differentiator is IconDifficultyValue (the bird's tier). primary applies that tiering:
//
//	view 101, difficulty 1 -> 101 (common)
//	view 101, difficulty 2 -> 102 (uncommon)
//	view 101, difficulty 3 -> 104 (rare; 103 is reserved for the view-103 birds)
//	otherwise             -> the view id itself (e.g. the few view 102 / 103 birds)
//
// A handful of chapters are missing one of the slots (e.g. ch2 has no 104, ch17 no 103),
// so primary may not resolve to a reward. fallback is the original view-only id; callers
// should try primary first and fall back to it (and ultimately to the generic material
// grant) so a bird never loses the reward it had before this tiering was added.
func (c *GimmickCatalog) FickleBirdCageOrnamentId(gimmickId, ornamentIndex int32) (primary, fallback int32, ok bool) {
	ornament, ok := c.fickleBirdOrnaments[GimmickOrnamentRef{GimmickId: gimmickId, OrnamentIndex: ornamentIndex}]
	if !ok {
		return 0, 0, false
	}
	base := 1_000_000 + ornament.ChapterId*1_000
	fallback = base + ornament.GimmickOrnamentViewId
	primary = base + fickleBirdRewardSuffix(ornament.GimmickOrnamentViewId, ornament.IconDifficultyValue)
	return primary, fallback, true
}

// fickleBirdRewardSuffix maps a fickle bird's (view, difficulty) to its per-chapter reward
// slot (101..104). See FickleBirdCageOrnamentId for the rationale.
func fickleBirdRewardSuffix(view, difficulty int32) int32 {
	if view == 101 {
		switch difficulty {
		case 2:
			return 102
		case 3:
			return 104
		default: // 1 (or any unexpected value) -> common tier
			return 101
		}
	}
	return view
}

// loadHiddenBirdRewards resolves (GimmickId, OrnamentIndex) -> CageOrnamentReward for
// every type-7 ("Hidden Black Birds") gimmick. The mapping is structural:
//
//	m_gimmick (GimmickType == 7) -> GimmickOrnamentGroupId
//	m_gimmick_ornament (matching group) -> GimmickOrnamentViewId
//	m_cage_ornament (CageOrnamentId == ViewId) -> CageOrnamentRewardId
//	m_cage_ornament_reward (matching id) -> PossessionType / PossessionId / Count
//
// 110 of 114 type-7 ornaments have a matching m_cage_ornament row in the current
// data; the rest log a warning and are silently skipped so the player just gets
// no reward on those (no crash).
func loadHiddenBirdRewards(cageOrnaments *CageOrnamentCatalog) map[GimmickOrnamentRef]SequenceReward {
	empty := map[GimmickOrnamentRef]SequenceReward{}
	if cageOrnaments == nil {
		return empty
	}

	gimmicks, ok := readGimmickTable[EntityMGimmick]("m_gimmick", "hidden-bird rewards")
	if !ok {
		return empty
	}
	ornaments, ok := readGimmickTable[EntityMGimmickOrnament]("m_gimmick_ornament", "hidden-bird rewards")
	if !ok {
		return empty
	}

	gimmicksByGroup := make(map[int32][]int32)
	for _, g := range gimmicks {
		if model.GimmickType(g.GimmickType) == model.GimmickTypeMapOnlyCageTreasureHunt {
			gimmicksByGroup[g.GimmickOrnamentGroupId] = append(gimmicksByGroup[g.GimmickOrnamentGroupId], g.GimmickId)
		}
	}

	out := make(map[GimmickOrnamentRef]SequenceReward)
	missing := 0
	for _, o := range ornaments {
		gids, ok := gimmicksByGroup[o.GimmickOrnamentGroupId]
		if !ok {
			continue
		}
		reward, ok := cageOrnaments.LookupReward(o.GimmickOrnamentViewId)
		if !ok {
			missing++
			continue
		}
		entry := SequenceReward{
			PossessionType: reward.PossessionType,
			PossessionId:   reward.PossessionId,
			Count:          reward.Count,
		}
		for _, gid := range gids {
			out[GimmickOrnamentRef{GimmickId: gid, OrnamentIndex: o.GimmickOrnamentIndex}] = entry
		}
	}
	if missing > 0 {
		log.Printf("[gimmick] %d hidden-bird ornaments had no m_cage_ornament_reward row", missing)
	}
	return out
}

// loadFickleBirdOrnaments resolves (GimmickId, OrnamentIndex) -> EntityMGimmickOrnament for
// every type-1 ("Fickle Black Birds") gimmick. The ornament data is used to calculate
// cageOrnamentId via the formula: 1_000_000 + ChapterId*1_000 + GimmickOrnamentViewId
func loadFickleBirdOrnaments(typeByGimmick map[int32]model.GimmickType) map[GimmickOrnamentRef]*EntityMGimmickOrnament {
	empty := map[GimmickOrnamentRef]*EntityMGimmickOrnament{}

	gimmicks, ok := readGimmickTable[EntityMGimmick]("m_gimmick", "fickle-bird ornaments")
	if !ok {
		return empty
	}
	ornaments, ok := readGimmickTable[EntityMGimmickOrnament]("m_gimmick_ornament", "fickle-bird ornaments")
	if !ok {
		return empty
	}

	// Build map: GimmickId -> GimmickOrnamentGroupId for type-1 gimmicks
	ornamentGroupByGimmick := make(map[int32]int32)
	for _, g := range gimmicks {
		if model.GimmickType(g.GimmickType) == model.GimmickTypeCageTreasureHunt {
			ornamentGroupByGimmick[g.GimmickId] = g.GimmickOrnamentGroupId
		}
	}

	// Build result map: (GimmickId, OrnamentIndex) -> ornament
	out := make(map[GimmickOrnamentRef]*EntityMGimmickOrnament)
	for i, o := range ornaments {
		// Find all type-1 gimmicks using this ornament group
		for gimmickId, groupId := range ornamentGroupByGimmick {
			if groupId == o.GimmickOrnamentGroupId {
				out[GimmickOrnamentRef{GimmickId: gimmickId, OrnamentIndex: o.GimmickOrnamentIndex}] = &ornaments[i]
			}
		}
	}
	return out
}

func (c *GimmickCatalog) GimmickType(gimmickId int32) model.GimmickType {
	return c.gimmickTypes[gimmickId]
}

// CageMemoryImportantItem returns the ImportantItemId (type 4) that the library uses
// to mark a tapped cage memory as collected, given the world-gimmick id. The mapping
// is derived from m_gimmick_additional_asset texture suffixes — see
// loadCageMemoryImportantItems.
func (c *GimmickCatalog) CageMemoryImportantItem(gimmickId int32) (int32, bool) {
	id, ok := c.cageMemoryItems[gimmickId]
	return id, ok
}

// importantItemTypeCageMemory mirrors EntityMImportantItem.ImportantItemType==4 — the
// CageMemory entry that the library's HasCageMemory check resolves to.
const importantItemTypeCageMemory int32 = 4

func loadCageMemoryImportantItems(typeByGimmick map[int32]model.GimmickType) map[int32]int32 {
	empty := map[int32]int32{}

	ornaments, ok := readGimmickTable[EntityMGimmickOrnament]("m_gimmick_ornament", "cage-memory items")
	if !ok {
		return empty
	}
	chapters, ok := readGimmickTable[EntityMMainQuestChapter]("m_main_quest_chapter", "cage-memory items")
	if !ok {
		return empty
	}
	routes, ok := readGimmickTable[EntityMMainQuestRoute]("m_main_quest_route", "cage-memory items")
	if !ok {
		return empty
	}
	cageMemories, ok := readGimmickTable[EntityMCageMemory]("m_cage_memory", "cage-memory items")
	if !ok {
		return empty
	}
	items, ok := readGimmickTable[EntityMImportantItem]("m_important_item", "cage-memory items")
	if !ok {
		return empty
	}

	chapterByOrnamentGroup := make(map[int32]int32, len(ornaments))
	for _, o := range ornaments {
		if _, seen := chapterByOrnamentGroup[o.GimmickOrnamentGroupId]; seen {
			continue
		}
		chapterByOrnamentGroup[o.GimmickOrnamentGroupId] = o.ChapterId
	}
	routeByChapter := make(map[int32]int32, len(chapters))
	for _, c := range chapters {
		routeByChapter[c.MainQuestChapterId] = c.MainQuestRouteId
	}
	seasonByRoute := make(map[int32]int32, len(routes))
	for _, r := range routes {
		seasonByRoute[r.MainQuestRouteId] = r.MainQuestSeasonId
	}
	cmsBySeason := make(map[int32][]int32)
	for _, c := range cageMemories {
		cmsBySeason[c.MainQuestSeasonId] = append(cmsBySeason[c.MainQuestSeasonId], c.CageMemoryId)
	}
	for s := range cmsBySeason {
		sort.Slice(cmsBySeason[s], func(i, j int) bool { return cmsBySeason[s][i] < cmsBySeason[s][j] })
	}
	itemByCageMemory := make(map[int32]int32)
	for _, it := range items {
		if it.ImportantItemType == importantItemTypeCageMemory && it.CageMemoryId != 0 {
			itemByCageMemory[it.CageMemoryId] = it.ImportantItemId
		}
	}

	gimmicksByRoute := make(map[int32][]int32)
	for gid, t := range typeByGimmick {
		if t != model.GimmickTypeCageMemory {
			continue
		}
		chapter, ok := chapterByOrnamentGroup[gid]
		if !ok {
			log.Printf("[gimmick] cage-memory %d has no ornament row, skipping mapping", gid)
			continue
		}
		route, ok := routeByChapter[chapter]
		if !ok {
			log.Printf("[gimmick] cage-memory %d chapter %d has no route, skipping mapping", gid, chapter)
			continue
		}
		gimmicksByRoute[route] = append(gimmicksByRoute[route], gid)
	}
	for r := range gimmicksByRoute {
		sort.Slice(gimmicksByRoute[r], func(i, j int) bool { return gimmicksByRoute[r][i] < gimmicksByRoute[r][j] })
	}

	out := make(map[int32]int32)
	for route, gids := range gimmicksByRoute {
		season, ok := seasonByRoute[route]
		if !ok {
			log.Printf("[gimmick] route %d has no season, skipping %d cage-memory gimmicks", route, len(gids))
			continue
		}
		seasonCms := cmsBySeason[season]
		for i, gid := range gids {
			if i >= len(seasonCms) {
				log.Printf("[gimmick] route %d (season %d) has %d cage-memory gimmicks but only %d cage memories; gimmick %d skipped",
					route, season, len(gids), len(seasonCms), gid)
				continue
			}
			cageMemoryId := seasonCms[i]
			itemId, ok := itemByCageMemory[cageMemoryId]
			if !ok {
				log.Printf("[gimmick] cage memory %d (gimmick %d) has no m_important_item row (type 4), skipping",
					cageMemoryId, gid)
				continue
			}
			out[gid] = itemId
		}
	}
	return out
}

func loadGimmickSequenceRewards() map[int32][]SequenceReward {
	empty := map[int32][]SequenceReward{}

	sequences, ok := readGimmickTable[EntityMGimmickSequence]("m_gimmick_sequence", "sequence rewards")
	if !ok {
		return empty
	}
	rewardGroups, ok := readGimmickTable[EntityMGimmickSequenceRewardGroup]("m_gimmick_sequence_reward_group", "sequence rewards")
	if !ok {
		return empty
	}

	rewardsByGroup := make(map[int32][]SequenceReward)
	for _, rg := range rewardGroups {
		if rg.PossessionType == 0 || rg.PossessionId == 0 {
			continue
		}
		rewardsByGroup[rg.GimmickSequenceRewardGroupId] = append(
			rewardsByGroup[rg.GimmickSequenceRewardGroupId], SequenceReward{
				PossessionType: rg.PossessionType,
				PossessionId:   rg.PossessionId,
				Count:          rg.Count,
			})
	}

	rewardsBySequence := make(map[int32][]SequenceReward, len(sequences))
	for _, seq := range sequences {
		if rewards := rewardsByGroup[seq.GimmickSequenceRewardGroupId]; len(rewards) > 0 {
			rewardsBySequence[seq.GimmickSequenceId] = rewards
		}
	}
	return rewardsBySequence
}

func (c *GimmickCatalog) IsHiddenSequence(sequenceId int32) bool {
	return c.hiddenSequences[sequenceId]
}

// IsReportSequence returns true when the sequence contains at least one
// GimmickType==Report gimmick (a hidden story). Chain-walk projection uses this
// to decide whether a projected record must pass the availability filter before
// reaching the client.
func (c *GimmickCatalog) IsReportSequence(sequenceId int32) bool {
	for gimmickId := range c.gimmicksBySequence[sequenceId] {
		if c.gimmickTypes[gimmickId] == model.GimmickTypeReport {
			return true
		}
	}
	return false
}

func (c *GimmickCatalog) SequenceRewards(sequenceId int32) []SequenceReward {
	return c.sequenceRewards[sequenceId]
}

func (c *GimmickCatalog) ActiveScheduleKeys(user store.UserState, nowMillis int64) []store.GimmickSequenceKey {
	keys := make([]store.GimmickSequenceKey, 0, len(c.schedules))
	for _, s := range c.schedules {
		if !c.scheduleEligible(&user, s, nowMillis) {
			continue
		}
		// Iterate the schedule's chain and emit keys for every sequence that is
		// available right now (first sequence always, later sequences only after
		// their predecessor's gimmick has been cleared). This is what stops a
		// chapter-4 report schedule from independently storing chapter-13 story
		// keys before the player has progressed through the chain.
		for _, sequenceId := range c.sequenceChains[s.FirstSequenceId] {
			if c.SequenceAvailable(&user, s.ScheduleId, sequenceId, nowMillis) {
				keys = append(keys, store.GimmickSequenceKey{
					GimmickSequenceScheduleId: s.ScheduleId,
					GimmickSequenceId:         sequenceId,
				})
			}
		}
	}
	return keys
}

// scheduleEligible decides whether a gimmick schedule's icon should be on the map.
//
// The time window always applies. "Hidden Black Birds" (type 7) are world-map
// birds that are meant to be visible from the start, so they skip the story gates.
// Every other gimmick (fickle birds, lost items, lost archives, hidden stories,
// stray obelisks) must clear BOTH story gates:
//
//  1. the last Normal main-quest of the chapter the gimmick lives in, so its icon
//     stays hidden until the player finishes that chapter, and
//  2. the schedule's own ReleaseEvaluateConditionId, the developer-authored gate
//     (evaluated with the full condition resolver). Requiring it closes the cases
//     where the chapter heuristic alone mis-assigns a gimmick (e.g. a hidden story
//     whose ornament points at an already-cleared chapter).
//
// A GateQuestId of 0 (chapter had no resolvable last quest) simply skips gate 1,
// so we never hide content permanently because of missing mapping data.
//
// Fickle birds (type 1) get one extra throttle on top of gate 1: a daily "rotation" that
// surfaces only a small block of each chapter's bird pool per server day and cycles
// through the pool, mirroring the live-service cadence. Without it all ~8-23 birds of a
// chapter (whose authored StartDatetimes are all long past) appear simultaneously on
// chapter clear. See assignFickleBirdRotation / FICKLE_BIRD_PER_DAY.
func (c *GimmickCatalog) scheduleEligible(user *store.UserState, s gimmickScheduleEntry, nowMillis int64) bool {
	if nowMillis < s.StartDatetime {
		return false
	}
	if s.Type == model.GimmickTypeMapOnlyCageTreasureHunt {
		return true // hidden birds always visible
	}
	if s.GateQuestId != 0 {
		q, ok := user.Quests[s.GateQuestId]
		if !ok || q.QuestStateType != model.UserQuestStateTypeCleared {
			return false
		}
		// Fickle-bird rotation: within a cleared chapter only today's block of birds is
		// surfaced; the block advances each server day and cycles back after the pool is
		// exhausted, so a chapter's 8-23 birds appear ~4-at-a-time "по кругу" instead of
		// all at once (their historical StartDatetimes are all long past on a private server).
		if s.Type == model.GimmickTypeCageTreasureHunt && s.RotSize > 0 && !fickleBirdInTodaysBlock(s, nowMillis) {
			return false
		}
	}
	return c.conditions.Satisfied(s.ReleaseConditionId, user)
}

// fickleBirdInTodaysBlock reports whether a fickle bird belongs to the rotation block
// surfaced on the current server day. The chapter's pool (RotSize) is split into blocks
// of fickleBirdsPerDay(); the active block index advances by one each local calendar day
// and wraps via modulo, so the pool cycles repeatedly ("по кругу"). Callers only reach
// this when fickleBirdsPerDay() > 0 and RotSize > 0.
func fickleBirdInTodaysBlock(s gimmickScheduleEntry, nowMillis int64) bool {
	perDay := int32(fickleBirdsPerDay())
	if perDay <= 0 {
		return true
	}
	numBlocks := (s.RotSize + perDay - 1) / perDay
	if numBlocks <= 1 {
		return true // whole pool fits in one block; nothing to rotate
	}
	const dayMillis = int64(24 * 60 * 60 * 1000)
	dayNumber := gametime.StartOfDayMillisAt(nowMillis) / dayMillis
	activeBlock := int32(dayNumber % int64(numBlocks))
	return s.RotIndex/perDay == activeBlock
}

// SequenceAvailable reports whether a specific (scheduleId, sequenceId) pair is
// eligible for the world-map projection right now. Two-tier rule:
//
//  1. The first sequence of a schedule is gated purely by the schedule itself
//     (time window + chapter gate + developer ReleaseEvaluateConditionId).
//  2. Later chain members must additionally satisfy chain progression: their
//     immediate predecessor in the chain has to have been cleared by the
//     player (or, for backward compatibility with older servers that let
//     stories be collected out of order, they must already be recorded as
//     unlocked/cleared in user.Gimmick.*).
//
// This mirrors the client's own list-ordering logic — the game's chain graph
// encodes the intended narrative order, not chapter number. Two 5th stories
// (900605, 901205) have ornaments physically placed in the tutorial-prologue
// map area despite belonging later in the story flow, so an ornament-chapter
// gate alone leaks them from the start; chain progression correctly hides them
// until the player has collected the earlier stories in the same chain.
//
// Note on parallel per-chapter chains: the master data intentionally ships
// 900X01..900X04 and 900X06..900X10 as two independent chains within the
// same chapter, both anchored to schedules that share the same
// ReleaseEvaluateConditionId ("chapter X cleared"). Both open together by
// design; the reference fork treats them the same way. Only 5th stories
// (900X05) sit in a cross-chapter chain that has to be walked in narrative
// order.
func (c *GimmickCatalog) SequenceAvailable(user *store.UserState, scheduleId, sequenceId int32, nowMillis int64) bool {
	entry, ok := c.scheduleByKey[store.GimmickSequenceKey{GimmickSequenceScheduleId: scheduleId, GimmickSequenceId: sequenceId}]
	if !ok {
		return false
	}
	if !c.scheduleEligible(user, entry, nowMillis) {
		return false
	}
	if sequenceId == entry.FirstSequenceId {
		return true
	}
	// Backward-compat: a chain member the player has fully CLEARED (Progress
	// IsGimmickCleared=true or Sequence IsGimmickSequenceCleared=true) stays
	// available even if the predecessor's state was recorded out of order by an
	// earlier server version. Without this, restoring such rows would silently
	// hide stories the player already paid the reward for. Note we do NOT
	// extend this courtesy to Unlock-only rows (icon tapped but story not
	// completed): older server builds wrote Unlock rows without enforcing
	// chain progression, so treating them as evidence would let stale test
	// state keep leaking icons onto the map.
	if _, cleared := c.reportSequenceState(user, scheduleId, sequenceId); cleared {
		return true
	}
	// Chain-progression: at least one immediate predecessor in the same
	// schedule's chain must have been collected (sequence cleared OR any of its
	// Report gimmicks cleared — see ReportSequenceCleared).
	for _, predecessor := range c.sequencePredecessors[sequenceId] {
		predKey := store.GimmickSequenceKey{GimmickSequenceScheduleId: scheduleId, GimmickSequenceId: predecessor}
		if _, belongs := c.scheduleByKey[predKey]; !belongs {
			continue
		}
		if user.Gimmick.Sequences[predKey].IsGimmickSequenceCleared || c.ReportSequenceCleared(user, scheduleId, predecessor) {
			return true
		}
	}
	return false
}

// reportSequenceState inspects a sequence's Report-type gimmicks and returns
// two flags: `unlocked` (touched by the player via Unlock/Progress/Sequence
// row) and `cleared` (fully collected). Only cleared counts as completing the
// story — merely creating a Sequence row during InitSequenceSchedule does not
// advance the chain.
func (c *GimmickCatalog) reportSequenceState(user *store.UserState, scheduleId, sequenceId int32) (unlocked, cleared bool) {
	seqKey := store.GimmickSequenceKey{GimmickSequenceScheduleId: scheduleId, GimmickSequenceId: sequenceId}
	if _, exists := c.scheduleByKey[seqKey]; !exists {
		return false, false
	}
	for gimmickId := range c.gimmicksBySequence[sequenceId] {
		if c.gimmickTypes[gimmickId] != model.GimmickTypeReport {
			continue
		}
		key := store.GimmickKey{GimmickSequenceScheduleId: scheduleId, GimmickSequenceId: sequenceId, GimmickId: gimmickId}
		cleared = cleared || user.Gimmick.Sequences[seqKey].IsGimmickSequenceCleared || user.Gimmick.Progress[key].IsGimmickCleared
		unlocked = unlocked || cleared || user.Gimmick.Unlocks[key].IsUnlocked
	}
	return unlocked, cleared
}

// ReportSequenceCleared also recognises a collected story whose sequence row
// is missing, so restoring that row never grants the reward again.
func (c *GimmickCatalog) ReportSequenceCleared(user *store.UserState, scheduleId, sequenceId int32) bool {
	_, cleared := c.reportSequenceState(user, scheduleId, sequenceId)
	return cleared
}

// GimmickUnlockAvailable gates whether a specific gimmick marker can be tapped
// on the world map (i.e. its containing sequence is available AND the gimmick
// belongs to that sequence's group).
func (c *GimmickCatalog) GimmickUnlockAvailable(user *store.UserState, scheduleId, sequenceId, gimmickId int32, nowMillis int64) bool {
	return c.SequenceAvailable(user, scheduleId, sequenceId, nowMillis) && c.gimmicksBySequence[sequenceId][gimmickId]
}

// GimmickAvailable additionally enforces the gimmick's own ClearEvaluateConditionId
// (hidden mission prerequisite) on top of the unlock gate. Report-type gimmicks
// use it so the client doesn't render the marker before the required hidden
// mission is eligible; other gimmick types skip the check.
func (c *GimmickCatalog) GimmickAvailable(user *store.UserState, scheduleId, sequenceId, gimmickId int32, nowMillis int64) bool {
	if !c.GimmickUnlockAvailable(user, scheduleId, sequenceId, gimmickId, nowMillis) {
		return false
	}
	if c.gimmickTypes[gimmickId] != model.GimmickTypeReport {
		return true
	}
	return c.conditions.Satisfied(c.clearConditionByGimmick[gimmickId], user)
}

type GimmickOrnamentRef struct {
	GimmickId     int32
	OrnamentIndex int32
}

func LoadGimmickOrnamentRefs() map[int32][]GimmickOrnamentRef {
	empty := map[int32][]GimmickOrnamentRef{}

	sequences, ok := readGimmickTable[EntityMGimmickSequence]("m_gimmick_sequence", "ornament refs")
	if !ok {
		return empty
	}
	groups, ok := readGimmickTable[EntityMGimmickGroup]("m_gimmick_group", "ornament refs")
	if !ok {
		return empty
	}
	gimmicks, ok := readGimmickTable[EntityMGimmick]("m_gimmick", "ornament refs")
	if !ok {
		return empty
	}
	ornaments, ok := readGimmickTable[EntityMGimmickOrnament]("m_gimmick_ornament", "ornament refs")
	if !ok {
		return empty
	}

	indicesByOrnamentGroup := make(map[int32][]int32)
	for _, o := range ornaments {
		indicesByOrnamentGroup[o.GimmickOrnamentGroupId] = append(
			indicesByOrnamentGroup[o.GimmickOrnamentGroupId], o.GimmickOrnamentIndex)
	}
	ornamentGroupByGimmick := make(map[int32]int32, len(gimmicks))
	for _, g := range gimmicks {
		ornamentGroupByGimmick[g.GimmickId] = g.GimmickOrnamentGroupId
	}
	gimmicksByGroup := make(map[int32][]int32)
	for _, grp := range groups {
		gimmicksByGroup[grp.GimmickGroupId] = append(gimmicksByGroup[grp.GimmickGroupId], grp.GimmickId)
	}

	refsBySequence := make(map[int32][]GimmickOrnamentRef, len(sequences))
	for _, seq := range sequences {
		var refs []GimmickOrnamentRef
		for _, gimmickId := range gimmicksByGroup[seq.GimmickGroupId] {
			for _, ornamentIndex := range indicesByOrnamentGroup[ornamentGroupByGimmick[gimmickId]] {
				refs = append(refs, GimmickOrnamentRef{GimmickId: gimmickId, OrnamentIndex: ornamentIndex})
			}
		}
		if len(refs) > 0 {
			refsBySequence[seq.GimmickSequenceId] = refs
		}
	}
	log.Printf("gimmick ornament refs loaded: %d sequences", len(refsBySequence))
	return refsBySequence
}

func LoadHiddenGimmickSequenceIDs() map[int32]bool {
	types := gimmickSequenceTypes()
	out := make(map[int32]bool, len(types))
	for sid, t := range types {
		if t == model.GimmickTypeReport || t == model.GimmickTypeCageMemory || t == model.GimmickTypeMapOnlyCageTreasureHunt {
			out[sid] = true
		}
	}
	return out
}

func LoadBirdGimmickIDs() map[int32]bool {
	byGimmick := gimmickTypes().byGimmick
	out := make(map[int32]bool, len(byGimmick))
	for gid, t := range byGimmick {
		if t == model.GimmickTypeCageIntervalDropItem || t == model.GimmickTypeMapOnlyCageIntervalDrop {
			out[gid] = true
		}
	}
	return out
}

func LoadGimmickSequenceChains() map[int32][]int32 {
	chains, _ := loadGimmickSequenceGraph()
	return chains
}

// loadGimmickSequenceGraph builds both the forward chain (first-sequence-id ->
// all reachable members via NextGimmickSequenceGroupId) and the reverse
// predecessor map (sequence-id -> its immediate predecessors). The predecessor
// map is what lets us gate chain members by "did the player collect the previous
// story in this chain?" — a schedule's chain is the game's authored story
// sequence, and each member only becomes available after its predecessor's
// gimmicks have been tapped.
func loadGimmickSequenceGraph() (map[int32][]int32, map[int32][]int32) {
	empty := map[int32][]int32{}

	sequences, ok := readGimmickTable[EntityMGimmickSequence]("m_gimmick_sequence", "sequence chains")
	if !ok {
		return empty, nil
	}
	groups, ok := readGimmickTable[EntityMGimmickSequenceGroup]("m_gimmick_sequence_group", "sequence chains")
	if !ok {
		return empty, nil
	}

	membersByGroup := make(map[int32][]int32)
	for _, g := range groups {
		membersByGroup[g.GimmickSequenceGroupId] = append(membersByGroup[g.GimmickSequenceGroupId], g.GimmickSequenceId)
	}
	nextGroupBySequence := make(map[int32]int32, len(sequences))
	predecessors := make(map[int32][]int32)
	for _, seq := range sequences {
		nextGroupBySequence[seq.GimmickSequenceId] = seq.NextGimmickSequenceGroupId
		for _, next := range membersByGroup[seq.NextGimmickSequenceGroupId] {
			predecessors[next] = append(predecessors[next], seq.GimmickSequenceId)
		}
	}

	chains := make(map[int32][]int32, len(sequences))
	for _, seq := range sequences {
		start := seq.GimmickSequenceId
		seen := map[int32]bool{start: true}
		chain := []int32{start}
		for queue := []int32{start}; len(queue) > 0; {
			cur := queue[0]
			queue = queue[1:]
			nextGroup := nextGroupBySequence[cur]
			if nextGroup == 0 {
				continue
			}
			for _, member := range membersByGroup[nextGroup] {
				if !seen[member] {
					seen[member] = true
					chain = append(chain, member)
					queue = append(queue, member)
				}
			}
		}
		chains[start] = chain
	}
	return chains, predecessors
}

// loadGimmickConditionsBySequence maps each gimmick sequence to its gimmick-id
// set (so we can iterate a sequence's gimmicks when checking per-gimmick unlock
// and clear conditions) and each gimmick to its ClearEvaluateConditionId.
// Both are needed by the chain-progression availability check.
func loadGimmickConditionsBySequence() (map[int32]map[int32]bool, map[int32]int32, error) {
	gimmicks, err := utils.ReadTable[EntityMGimmick]("m_gimmick")
	if err != nil {
		return nil, nil, fmt.Errorf("load gimmick conditions: %w", err)
	}
	groups, err := utils.ReadTable[EntityMGimmickGroup]("m_gimmick_group")
	if err != nil {
		return nil, nil, fmt.Errorf("load gimmick groups for conditions: %w", err)
	}
	sequences, err := utils.ReadTable[EntityMGimmickSequence]("m_gimmick_sequence")
	if err != nil {
		return nil, nil, fmt.Errorf("load gimmick sequences for conditions: %w", err)
	}
	gimmicksByGroup := make(map[int32][]int32)
	for _, group := range groups {
		gimmicksByGroup[group.GimmickGroupId] = append(gimmicksByGroup[group.GimmickGroupId], group.GimmickId)
	}
	gimmicksBySequence := make(map[int32]map[int32]bool)
	for _, sequence := range sequences {
		members := make(map[int32]bool)
		for _, gimmickId := range gimmicksByGroup[sequence.GimmickGroupId] {
			members[gimmickId] = true
		}
		gimmicksBySequence[sequence.GimmickSequenceId] = members
	}
	clearConditionByGimmick := make(map[int32]int32, len(gimmicks))
	for _, gimmick := range gimmicks {
		clearConditionByGimmick[gimmick.GimmickId] = gimmick.ClearEvaluateConditionId
	}
	return gimmicksBySequence, clearConditionByGimmick, nil
}
