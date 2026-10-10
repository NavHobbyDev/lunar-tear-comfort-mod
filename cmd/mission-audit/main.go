// Command mission-audit is a READ-ONLY diagnostic that loads the same encrypted
// master data the game server uses and reports, for every quest mission, which
// QuestMissionConditionType values actually appear in the data and whether the
// quest-flow evaluator (internal/questflow.evaluateQuestMissionCondition) handles
// each one.
//
// It never opens the game database and never mutates any file. It only reads
// assets/release/20240404193219.bin.e.
//
// Usage:
//
//	go run ./cmd/mission-audit [path/to/masterdata.bin.e]
package main

import (
	"fmt"
	"os"
	"sort"

	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/masterdata/memorydb"
)

const defaultMasterData = "assets/release/20240404193219.bin.e"

// handledTypes mirrors the cases explicitly implemented in
// internal/questflow/quest.go evaluateQuestMissionCondition. Type 9999
// (Complete) is skipped before evaluation, so it is treated as handled too.
var handledTypes = map[int32]bool{
	1:    true, // LessThanOrEqualXPeopleNotAlive
	2:    true, // MaxDamage
	4:    true, // SpecifiedCharacterIsInDeck
	5:    true, // SpecifiedAttributeMainWeaponIsInDeck
	6:    true, // GreaterThanOrEqualXCostumeSkillUseCount
	7:    true, // GreaterThanOrEqualXWeaponSkillUseCount
	8:    true, // GreaterThanOrEqualXCompanionSkillUseCount
	10:   true, // CostumeSkillfulWeaponAnyCharacter
	19:   true, // SpecifiedAttributeMainWeaponAllCharacter
	49:   true, // DeckCostumeNumLe
	50:   true, // CriticalCountGe
	51:   true, // MinHpPercentageGe
	52:   true, // ComboCountGe
	54:   true, // LessThanOrEqualXCostumeSkillUseCount
	55:   true, // LessThanOrEqualXWeaponSkillUseCount
	56:   true, // LessThanOrEqualXCompanionSkillUseCount
	57:   true, // WithoutRecoverySkill
	61:   true, // CharacterContainAll (group based)
	65:   true, // CostumeSkillfulWeaponContainAll (group based)
	9999: true, // Complete (skipped in clearQuestMissions)
}

// conditionTypeName mirrors the model.QuestMissionConditionType constant names so
// the report is human-readable, without adding a String() method to the model.
var conditionTypeName = map[int32]string{
	0: "Unknown", 1: "LessThanOrEqualXPeopleNotAlive", 2: "MaxDamage",
	3: "SpecifiedCostumeIsInDeck", 4: "SpecifiedCharacterIsInDeck",
	5: "SpecifiedAttributeMainWeaponIsInDeck", 6: "GreaterThanOrEqualXCostumeSkillUseCount",
	7: "GreaterThanOrEqualXWeaponSkillUseCount", 8: "GreaterThanOrEqualXCompanionSkillUseCount",
	9: "CostumeSkillfulWeaponAllCharacter", 10: "CostumeSkillfulWeaponAnyCharacter",
	11: "CostumeRarityEqAllCharacter", 12: "CostumeRarityGeAllCharacter",
	13: "CostumeRarityLeAllCharacter", 14: "CostumeRarityEqAnyCharacter",
	15: "CostumeRarityGeAnyCharacter", 16: "CostumeRarityLeAnyCharacter",
	17: "WeaponEvolutionGroupId", 18: "SpecifiedAttributeWeaponIsInDeck",
	19: "SpecifiedAttributeMainWeaponAllCharacter", 20: "SpecifiedAttributeWeaponAllCharacter",
	21: "WeaponManSkillfulWeaponAllCharacter", 22: "WeaponSkillfulWeaponAllCharacter",
	23: "WeaponManSkillfulWeaponAnyCharacter", 24: "WeaponSkillfulWeaponAnyCharacter",
	25: "WeaponRarityEqAllCharacter", 26: "WeaponRarityGeAllCharacter",
	27: "WeaponRarityLeAllCharacter", 28: "WeaponMainRarityEqAllCharacter",
	29: "WeaponMainRarityGeAllCharacter", 30: "WeaponMainRarityLeAllCharacter",
	31: "WeaponRarityEqAnyCharacter", 32: "WeaponRarityGeAnyCharacter",
	33: "WeaponRarityLeAnyCharacter", 34: "WeaponMainRarityEqAnyCharacter",
	35: "WeaponMainRarityGeAnyCharacter", 36: "WeaponMainRarityLeAnyCharacter",
	37: "CompanionId", 38: "CompanionAttribute", 39: "CompanionCategory",
	40: "PartsId", 41: "PartsGroupId", 42: "PartsRarityEq", 43: "PartsRarityGe",
	44: "PartsRarityLe", 45: "DeckPowerGe", 46: "DeckPowerLe", 47: "DeckCostumeNumEq",
	48: "DeckCostumeNumGe", 49: "DeckCostumeNumLe", 50: "CriticalCountGe",
	51: "MinHpPercentageGe", 52: "ComboCountGe", 53: "ComboMaxDamageGe",
	54: "LessThanOrEqualXCostumeSkillUseCount", 55: "LessThanOrEqualXWeaponSkillUseCount",
	56: "LessThanOrEqualXCompanionSkillUseCount", 57: "WithoutRecoverySkill",
	58: "WithoutCostumeSkill", 59: "WithoutWeaponSkill", 60: "WithoutCompanionSkill",
	61: "CharacterContainAll", 62: "CharacterContainAny", 63: "CostumeContainAll",
	64: "CostumeContainAny", 65: "CostumeSkillfulWeaponContainAll",
	66: "CostumeSkillfulWeaponContainAny", 67: "AttributeMainWeaponContainAll",
	68: "AttributeMainWeaponContainAny", 69: "AttributeWeaponContainAll",
	70: "AttributeWeaponContainAny", 71: "WeaponManSkillfulWeaponContainAll",
	72: "WeaponManSkillfulWeaponContainAny", 73: "WeaponSkillfulWeaponContainAll",
	74: "WeaponSkillfulWeaponContainAny", 9999: "Complete",
}

func conditionName(t int32) string {
	if n, ok := conditionTypeName[t]; ok {
		return n
	}
	return fmt.Sprintf("UnknownType(%d)", t)
}

type stat struct {
	total     int
	mainRoute int
	// examples collected for unhandled types
	exampleQuests   []int32
	exampleMissions []int32
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

	partsCatalog, err := masterdata.LoadPartsCatalog()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load parts catalog: %v\n", err)
		os.Exit(1)
	}
	catalog, err := masterdata.LoadQuestCatalog(partsCatalog)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load quest catalog: %v\n", err)
		os.Exit(1)
	}

	// membership sets to flag main-route quests
	routeByQuest := catalog.RouteIdByQuestId

	stats := map[int32]*stat{}
	missionsSeen := 0
	questsSeen := 0

	for questId, missionIds := range catalog.MissionIdsByQuestId {
		_, isMain := routeByQuest[questId]
		questsSeen++
		for _, missionId := range missionIds {
			m, ok := catalog.MissionById[missionId]
			if !ok {
				continue
			}
			missionsSeen++
			ct := m.QuestMissionConditionType
			s := stats[ct]
			if s == nil {
				s = &stat{}
				stats[ct] = s
			}
			s.total++
			if isMain {
				s.mainRoute++
			}
			if !handledTypes[ct] && len(s.exampleQuests) < 12 {
				s.exampleQuests = append(s.exampleQuests, questId)
				s.exampleMissions = append(s.exampleMissions, missionId)
			}
		}
	}

	types := make([]int, 0, len(stats))
	for t := range stats {
		types = append(types, int(t))
	}
	sort.Ints(types)

	fmt.Printf("master data: %s\n", binPath)
	fmt.Printf("quests with missions: %d, missions evaluated: %d\n\n", questsSeen, missionsSeen)
	fmt.Printf("%-6s %-6s %-6s %-9s %s\n", "TYPE", "TOTAL", "MAIN", "HANDLED", "NAME")
	fmt.Println("------------------------------------------------------------")
	unhandledUsed := false
	for _, t := range types {
		s := stats[int32(t)]
		handled := "OK"
		if !handledTypes[int32(t)] {
			handled = "MISSING"
			unhandledUsed = true
		}
		fmt.Printf("%-6d %-6d %-6d %-9s %s\n",
			t, s.total, s.mainRoute, handled,
			conditionName(int32(t)))
	}

	fmt.Println()
	if !unhandledUsed {
		fmt.Println("RESULT: every quest-mission condition type present in the data is handled by the evaluator.")
		return
	}

	fmt.Println("RESULT: the following condition types appear in quest data but are NOT handled (evaluator returns false => such missions never clear):")
	for _, t := range types {
		s := stats[int32(t)]
		if handledTypes[int32(t)] {
			continue
		}
		fmt.Printf("\n  type %d (%s): total=%d mainRoute=%d\n",
			t, conditionName(int32(t)), s.total, s.mainRoute)
		for i := range s.exampleQuests {
			fmt.Printf("      quest %-6d mission %-6d\n", s.exampleQuests[i], s.exampleMissions[i])
		}
	}
}
