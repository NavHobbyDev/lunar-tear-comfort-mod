// Command mission-clear-audit is a READ-ONLY diagnostic. It loads the same
// encrypted master data the game server uses and lists every quest-clear
// counting mission (MissionClearConditionType == 1) together with its category,
// target count, condition/option/link ids, and whether the server would even
// track it (the runtime catalog only loads categories 2/5/7).
//
// It never opens the game database and never mutates any file.
//
// Usage:
//
//	go run ./cmd/mission-clear-audit [path/to/masterdata.bin.e]
package main

import (
	"fmt"
	"os"
	"sort"

	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/masterdata/memorydb"
	"lunar-tear/server/internal/utils"
)

const defaultMasterData = "assets/release/20240404193219.bin.e"

// questClearConditionType is the MissionClearConditionType value for
// "clear N quests" style missions (mirrors missionClearConditionTypeQuestClearCount).
const questClearConditionType int32 = 1

func categoryKnown(c int32) string {
	switch c {
	case 1:
		return "Daily"
	case 2:
		return "Challenge"
	case 3:
		return "Event"
	case 4:
		return "Panel"
	case 5:
		return "Costumes"
	case 6:
		return "Packs"
	case 7:
		return "HiddenStory"
	case 9:
		return "PassDaily"
	case 10:
		return "Pass"
	default:
		return fmt.Sprintf("cat%d", c)
	}
}

func main() {
	binPath := defaultMasterData
	if len(os.Args) > 1 {
		binPath = os.Args[1]
	}
	if err := memorydb.Init(binPath); err != nil {
		fmt.Fprintf(os.Stderr, "init memorydb (%s): %v\n", binPath, err)
		os.Exit(1)
	}

	missions, err := utils.ReadTable[MissionRow]("m_mission")
	if err != nil {
		fmt.Fprintf(os.Stderr, "read m_mission: %v\n", err)
		os.Exit(1)
	}
	groups, err := utils.ReadTable[masterdata.EntityMMissionGroup]("m_mission_group")
	if err != nil {
		fmt.Fprintf(os.Stderr, "read m_mission_group: %v\n", err)
		os.Exit(1)
	}
	catTypeByGroup := map[int32]int32{}
	for _, g := range groups {
		catTypeByGroup[g.MissionGroupId] = g.MissionCategoryType
	}

	var rows []MissionRow
	for _, m := range missions {
		if m.MissionClearConditionType == questClearConditionType {
			rows = append(rows, m)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		ci := catTypeByGroup[rows[i].MissionGroupId]
		cj := catTypeByGroup[rows[j].MissionGroupId]
		if ci != cj {
			return ci < cj
		}
		if rows[i].ClearConditionValue != rows[j].ClearConditionValue {
			return rows[i].ClearConditionValue < rows[j].ClearConditionValue
		}
		return rows[i].MissionId < rows[j].MissionId
	})

	fmt.Printf("master data: %s\n", binPath)
	fmt.Printf("quest-clear missions (condition type %d): %d\n\n", questClearConditionType, len(rows))
	fmt.Printf("%-9s %-12s %-6s %-8s %-8s %-8s %-8s %-8s\n",
		"MISSION", "CATEGORY", "VAL", "CONDGRP", "OPTION", "LINK", "TRACKED", "NAME_TEXTID")
	fmt.Println("------------------------------------------------------------------------")
	for _, m := range rows {
		cat := catTypeByGroup[m.MissionGroupId]
		tracked := "no"
		if cat == 2 || cat == 5 || cat == 7 {
			tracked = "yes"
		}
		fmt.Printf("%-9d %-12s %-6d %-8d %-8d %-8d %-8s %-8d\n",
			m.MissionId, categoryKnown(cat), m.ClearConditionValue,
			m.MissionClearConditionGroupId, m.MissionClearConditionOptionGroupId,
			m.MissionLinkId, tracked, m.NameMissionTextId)
	}

	dumpQuestFacts()
}

// dumpQuestFacts answers: what are the option-group ids referenced by the
// daily/route hidden-story missions (30029/30030/...), do they exist as real
// quests, and what quest ids does event chapter 10 (the "Daily Quest" chapter)
// actually contain? All read-only.
func dumpQuestFacts() {
	quests, err := utils.ReadTable[masterdata.EntityMQuest]("m_quest")
	if err != nil {
		fmt.Fprintf(os.Stderr, "read m_quest: %v\n", err)
		return
	}
	nameByQuest := map[int32]int32{}
	for _, q := range quests {
		nameByQuest[q.QuestId] = q.NameQuestTextId
	}

	questRow := map[int32]masterdata.EntityMQuest{}
	for _, q := range quests {
		nameByQuest[q.QuestId] = q.NameQuestTextId
		questRow[q.QuestId] = q
	}

	fmt.Println("\n=== do the mission option-group ids exist as real quests? ===")
	for _, id := range []int32{30029, 30030, 30036, 30037, 30038, 100001, 100002, 400017, 400018} {
		if q, ok := questRow[id]; ok {
			fmt.Printf("  quest %-7d EXISTS name_text=%-6d DailyClearable=%d IsCountedAsQuest=%v Stamina=%d\n",
				id, q.NameQuestTextId, q.DailyClearableCount, q.IsCountedAsQuest, q.Stamina)
		} else {
			fmt.Printf("  quest %-7d NOT in m_quest\n", id)
		}
	}

	fmt.Println("\n=== ALL quests with DailyClearableCount > 0 (the true daily-quest set) ===")
	var dailyIds []int32
	for _, q := range quests {
		if q.DailyClearableCount > 0 {
			dailyIds = append(dailyIds, q.QuestId)
		}
	}
	sort.Slice(dailyIds, func(i, j int) bool { return dailyIds[i] < dailyIds[j] })
	for _, id := range dailyIds {
		fmt.Printf("  daily quest %d (name_text=%d, DailyClearable=%d)\n", id, questRow[id].NameQuestTextId, questRow[id].DailyClearableCount)
	}
	fmt.Printf("  total daily-clearable quests: %d\n", len(dailyIds))

	// Map event chapter -> quest ids via chapter -> sequence group -> sequence.
	chapters, _ := utils.ReadTable[masterdata.EntityMEventQuestChapter]("m_event_quest_chapter")
	seqGroups, _ := utils.ReadTable[masterdata.EntityMEventQuestSequenceGroup]("m_event_quest_sequence_group")
	sequences, _ := utils.ReadTable[masterdata.EntityMEventQuestSequence]("m_event_quest_sequence")
	seqIdsByGroup := map[int32][]int32{}
	for _, g := range seqGroups {
		seqIdsByGroup[g.EventQuestSequenceGroupId] = append(seqIdsByGroup[g.EventQuestSequenceGroupId], g.EventQuestSequenceId)
	}
	questIdsBySeq := map[int32][]int32{}
	for _, s := range sequences {
		questIdsBySeq[s.EventQuestSequenceId] = append(questIdsBySeq[s.EventQuestSequenceId], s.QuestId)
	}

	fmt.Println("\n=== quest ids in event chapter 10 (the \"Daily Quest\" chapter) ===")
	for _, ch := range chapters {
		if ch.EventQuestChapterId != 10 {
			continue
		}
		seen := map[int32]bool{}
		for _, seqId := range seqIdsByGroup[ch.EventQuestSequenceGroupId] {
			for _, qid := range questIdsBySeq[seqId] {
				if !seen[qid] {
					seen[qid] = true
					fmt.Printf("  chapter10 quest %d (name_text=%d, DailyClearable=%d)\n", qid, questRow[qid].NameQuestTextId, questRow[qid].DailyClearableCount)
				}
			}
		}
	}

	fmt.Println("\n=== which event chapter (if any) owns daily/guerrilla quests? ===")
	for _, target := range []int32{100001, 100002, 100024, 100025, 100036, 100048, 100073, 100076, 30029, 30030} {
		owner := ""
		for _, ch := range chapters {
			for _, seqId := range seqIdsByGroup[ch.EventQuestSequenceGroupId] {
				for _, qid := range questIdsBySeq[seqId] {
					if qid == target {
						owner = fmt.Sprintf("event chapter %d (EventQuestType=%d)", ch.EventQuestChapterId, ch.EventQuestType)
					}
				}
			}
		}
		if owner == "" {
			owner = "(not an event-quest)"
		}
		fmt.Printf("  quest %d -> %s\n", target, owner)
	}

	fmt.Println("\n=== m_extra_quest_group membership for option ids 30029/30030 ===")
	eqg, err := utils.ReadTable[masterdata.EntityMExtraQuestGroup]("m_extra_quest_group")
	if err != nil {
		fmt.Fprintf(os.Stderr, "read m_extra_quest_group: %v\n", err)
	} else {
		for _, target := range []int32{30029, 30030} {
			fmt.Printf("  group header quest %d members:\n", target)
			n := 0
			for _, e := range eqg {
				if e.QuestId == target {
					n++
					fmt.Printf("    idx=%d extraQuestId=%d (name_text=%d, DailyClearable=%d)\n",
						e.ExtraQuestIndex, e.ExtraQuestId, questRow[e.ExtraQuestId].NameQuestTextId, questRow[e.ExtraQuestId].DailyClearableCount)
				}
			}
			if n == 0 {
				fmt.Printf("    (none - not an extra-quest group header)\n")
			}
		}
	}

	fmt.Println("\n=== chapters referenced by daily groups vs chapter 10 ===")
	dgtc, _ := utils.ReadTable[masterdata.EntityMEventQuestDailyGroupTargetChapter]("m_event_quest_daily_group_target_chapter")
	chaptersInDailyGroups := map[int32]bool{}
	for _, t := range dgtc {
		chaptersInDailyGroups[t.EventQuestChapterId] = true
	}
	questIdsByChapter := map[int32][]int32{}
	for _, ch := range chapters {
		seen := map[int32]bool{}
		for _, seqId := range seqIdsByGroup[ch.EventQuestSequenceGroupId] {
			for _, qid := range questIdsBySeq[seqId] {
				if !seen[qid] {
					seen[qid] = true
					questIdsByChapter[ch.EventQuestChapterId] = append(questIdsByChapter[ch.EventQuestChapterId], qid)
				}
			}
		}
	}
	typeByChapter := map[int32]int32{}
	for _, ch := range chapters {
		typeByChapter[ch.EventQuestChapterId] = ch.EventQuestType
	}
	fmt.Printf("  chapter 10 in a daily group? %v (EventQuestType=%d)\n", chaptersInDailyGroups[10], typeByChapter[10])
	fmt.Printf("  distinct chapters used by daily groups: %d\n", len(chaptersInDailyGroups))
	var dc []int32
	for c := range chaptersInDailyGroups {
		dc = append(dc, c)
	}
	sort.Slice(dc, func(i, j int) bool { return dc[i] < dc[j] })
	for _, c := range dc {
		var dailyN int
		for _, qid := range questIdsByChapter[c] {
			if questRow[qid].DailyClearableCount > 0 {
				dailyN++
			}
		}
		fmt.Printf("    daily-group chapter %d: EventQuestType=%d quests=%d dailyClearable=%d\n", c, typeByChapter[c], len(questIdsByChapter[c]), dailyN)
	}

	fmt.Println("\n=== EventQuestType distribution across chapters ===")
	countByType := map[int32]int{}
	for _, ch := range chapters {
		countByType[ch.EventQuestType]++
	}
	var types []int32
	for t := range countByType {
		types = append(types, t)
	}
	sort.Slice(types, func(i, j int) bool { return types[i] < types[j] })
	for _, t := range types {
		fmt.Printf("    EventQuestType %d: %d chapters\n", t, countByType[t])
	}
	fmt.Println("    chapters with EventQuestType==4:")
	for _, ch := range chapters {
		if ch.EventQuestType == 4 {
			fmt.Printf("      chapter %d quests=%v\n", ch.EventQuestChapterId, questIdsByChapter[ch.EventQuestChapterId])
		}
	}

	fmt.Println("\n=== m_mission_link rows (domain type/id) ===")
	links, _ := utils.ReadTable[masterdata.EntityMMissionLink]("m_mission_link")
	for _, l := range links {
		if l.MissionLinkId <= 2 {
			fmt.Printf("  link %d -> DestinationDomainType=%d DestinationDomainId=%d\n",
				l.MissionLinkId, l.DestinationDomainType, l.DestinationDomainId)
		}
	}
}

// MissionRow is a minimal local view of m_mission (msgpack array layout must
// match masterdata.EntityMMission exactly). Kept local so the tool never has to
// touch the runtime catalog's category filter.
type MissionRow = masterdata.EntityMMission
