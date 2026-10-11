// Mass-upgrade actions for the lunar-base-grant shim:
//   - upgrade_all_companions
//   - upgrade_all_weapons
//   - upgrade_all_costumes
//   - grant_thought_batch
//   - fill_karma_slots
//
// These mirror lunar-tear's gRPC service-handler logic
// (CompanionService.Enhance, WeaponService.{Enhance,Evolve,LimitBreak,Awaken,
// EnhanceSkill,EnhanceAbility}, CostumeService.{Enhance,Awaken,LimitBreak,
// EnhanceActiveSkill,UnlockLotteryEffectSlot,DrawLotteryEffect}) but skip
// cost consumption. lunar-base treats the post-upgrade state as authoritative
// — the player asked for "everything maxed" and we set the result without
// funding the transaction.
//
// fill_karma_slots replaces the deferred-Karma-Selector design: rather than
// roll OddsNumber via rand.Int31n, we deterministically pick the rarest
// (highest RarityType, ties broken by lowest OddsNumber) entry from each
// slot's odds pool. Only fills already-unlocked slots whose OddsNumber is
// still 0 — so the player's existing rolls are preserved and Upgrade All
// Costumes must run first to unlock the slots.

package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"lunar-tear/server/internal/gameutil"
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/masterdata/memorydb"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
)

// Mirrors m_config table values. Verified against EntityMConfigTable.json
// 2026-05-02: WEAPON_LIMIT_BREAK_AVAILABLE_COUNT=4,
// COSTUME_LIMIT_BREAK_AVAILABLE_COUNT=4, COSTUME_AWAKEN_AVAILABLE_COUNT=5.
// CompanionMaxLevel=50 is `companionMaxLevel` in lunar-tear's
// service.companion.go (unexported).
const (
	massCompanionMaxLevel    = int32(50)
	massWeaponLimitBreakMax  = int32(4)
	massCostumeLimitBreakMax = int32(4)
	massCostumeAwakenMax     = int32(5)
	massCostumeLotterySlots  = int32(3)
	massWeaponSkillMaxLevel  = int32(15)
)

// runUpgradeAllCompanions sets every owned companion's Level to the
// CompanionService cap (50), bumping LatestVersion. Self-skips companions
// already at max so we don't pay the diff-and-write cost for them.
func runUpgradeAllCompanions(req *request) (int, error) {
	db, st, err := openDB(req.DBPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	now := time.Now().UnixMilli()
	applied := 0
	_, err = st.UpdateUser(req.UserID, func(u *store.UserState) {
		for compUuid, comp := range u.Companions {
			if comp.Level >= massCompanionMaxLevel {
				continue
			}
			comp.Level = massCompanionMaxLevel
			comp.LatestVersion = now
			u.Companions[compUuid] = comp
			applied++
		}
	})
	if err != nil {
		return 0, fmt.Errorf("upgrade companions: %w", err)
	}
	return applied, nil
}

// runThoughtBatch grants every thought_id in the request, self-skipping any
// already in user.Thoughts. Mirrors applyCostumeAwakenItemAcquire from
// lunar-tear's costume service: a single ThoughtState row keyed by a fresh
// UUID, with AcquisitionDatetime/LatestVersion = now.
func runThoughtBatch(req *request) (int, error) {
	if len(req.ThoughtIDs) == 0 {
		return 0, errors.New("thought_ids list is empty")
	}

	db, st, err := openDB(req.DBPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	now := time.Now().UnixMilli()
	applied := 0
	_, err = st.UpdateUser(req.UserID, func(u *store.UserState) {
		owned := make(map[int32]bool, len(u.Thoughts))
		for _, t := range u.Thoughts {
			owned[t.ThoughtId] = true
		}
		for _, tid := range req.ThoughtIDs {
			if owned[tid] {
				continue
			}
			key := uuid.New().String()
			u.Thoughts[key] = store.ThoughtState{
				UserThoughtUuid:     key,
				ThoughtId:           tid,
				AcquisitionDatetime: now,
				LatestVersion:       now,
			}
			owned[tid] = true
			applied++
		}
	})
	if err != nil {
		return 0, fmt.Errorf("grant thoughts: %w", err)
	}
	return applied, nil
}

// loadMassWeaponCatalog loads the WeaponCatalog the same way runtime.buildCatalogs
// does. Material catalog must be loaded first because LoadWeaponCatalog enriches
// itself from MaterialTypeWeaponEnhancement entries.
func loadMassWeaponCatalog(masterDataPath string) (*masterdata.WeaponCatalog, error) {
	if err := memorydb.Init(masterDataPath); err != nil {
		return nil, fmt.Errorf("init master data: %w", err)
	}
	matCatalog, err := masterdata.LoadMaterialCatalog()
	if err != nil {
		return nil, fmt.Errorf("load material catalog: %w", err)
	}
	cat, err := masterdata.LoadWeaponCatalog(matCatalog)
	if err != nil {
		return nil, fmt.Errorf("load weapon catalog: %w", err)
	}
	return cat, nil
}

// runUpgradeAllWeapons applies the per-class upgrade path to every owned
// weapon. The path is the same in shape regardless of class — evolve to
// chain end, ascend to LB cap, refine if eligible, enhance to level cap,
// max all skill + ability slots. Class-specific behavior (e.g. RoD/Dark
// Memory don't evolve because they were granted in their final form) falls
// out naturally because EvolutionNextWeaponId only chains the unfinished
// part of each evolution group.
func runUpgradeAllWeapons(req *request) (int, error) {
	if req.MasterDataPath == "" {
		return 0, errors.New("master_data_path required for upgrade_all_weapons")
	}
	catalog, err := loadMassWeaponCatalog(req.MasterDataPath)
	if err != nil {
		return 0, err
	}

	db, st, err := openDB(req.DBPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	now := time.Now().UnixMilli()
	applied := 0
	_, err = st.UpdateUser(req.UserID, func(u *store.UserState) {
		for wUuid, weapon := range u.Weapons {
			oldId := weapon.WeaponId

			// 1. Evolve to chain end. Walk EvolutionNextWeaponId, forwarding
			//    WeaponStories and seeding WeaponNotes for each id we touch.
			finalId := weapon.WeaponId
			oldStory, hadOldStory := u.WeaponStories[oldId]
			for {
				next, ok := catalog.EvolutionNextWeaponId[finalId]
				if !ok {
					break
				}
				finalId = next
			}
			if finalId != oldId {
				weapon.WeaponId = finalId
				if hadOldStory {
					newStory, hasNewStory := u.WeaponStories[finalId]
					if !hasNewStory || newStory.ReleasedMaxStoryIndex < oldStory.ReleasedMaxStoryIndex {
						u.WeaponStories[finalId] = store.WeaponStoryState{
							WeaponId:              finalId,
							ReleasedMaxStoryIndex: oldStory.ReleasedMaxStoryIndex,
							LatestVersion:         now,
						}
					}
				}
			}

			finalMaster, ok := catalog.Weapons[finalId]
			if !ok {
				// Master entry missing for this id — leave the weapon alone
				// rather than corrupting it.
				continue
			}

			// 2. Ascend to LB cap.
			weapon.LimitBreakCount = massWeaponLimitBreakMax

			// 3. Refine if eligible (R50 refinable: listed in m_weapon_awaken).
			awakenRow, awakenEligible := catalog.AwakenByWeaponId[finalId]
			if awakenEligible {
				if _, already := u.WeaponAwakens[wUuid]; !already {
					u.WeaponAwakens[wUuid] = store.WeaponAwakenState{
						UserWeaponUuid: wUuid,
						LatestVersion:  now,
					}
				}
			}

			// 4. Enhance to level cap. Level cap is per-WeaponSpecificEnhanceId
			//    and depends on LimitBreakCount; exp thresholds come from the
			//    chain root's enhance id (LevelingEnhanceIdByWeaponId).
			//    Refined weapons get +awakenRow.LevelLimitUp on top (90 -> 100
			//    for R50 LB4 refinable: Subjugation, Other-4-Star post-evolve).
			capLevel := int32(0)
			if maxFunc, ok := catalog.MaxLevelByEnhanceId[finalMaster.WeaponSpecificEnhanceId]; ok {
				capLevel = maxFunc.Evaluate(weapon.LimitBreakCount)
			}
			if awakenEligible {
				capLevel += awakenRow.LevelLimitUp
			}
			levelingEnhanceId := catalog.LevelingEnhanceIdByWeaponId[finalId]
			if thresholds, ok := catalog.ExpByEnhanceId[levelingEnhanceId]; ok && capLevel > 0 {
				if int(capLevel) < len(thresholds) {
					weapon.Exp = thresholds[capLevel]
				} else {
					weapon.Exp = thresholds[len(thresholds)-1]
				}
				weapon.Level, weapon.Exp = gameutil.LevelAndCap(weapon.Exp, thresholds)
				if weapon.Level > capLevel {
					weapon.Level = capLevel
				}
			} else if capLevel > 0 {
				weapon.Level = capLevel
			}

			// 5. Rebuild WeaponSkills and WeaponAbilities for the final
			//    weapon id, with every slot at its max level. The final
			//    master's group ids dictate which slots exist.
			rebuildWeaponSkills(catalog, u, wUuid, finalMaster, weapon.LimitBreakCount)
			rebuildWeaponAbilities(catalog, u, wUuid, finalMaster, weapon.LimitBreakCount)

			// 6. WeaponNotes track the per-weapon-id high-water mark. Update
			//    for the final id so the in-game collection screen reads
			//    correctly.
			note := u.WeaponNotes[finalId]
			if note.WeaponId == 0 {
				note.WeaponId = finalId
				note.FirstAcquisitionDatetime = now
			}
			if note.MaxLevel < weapon.Level {
				note.MaxLevel = weapon.Level
			}
			if note.MaxLimitBreakCount < weapon.LimitBreakCount {
				note.MaxLimitBreakCount = weapon.LimitBreakCount
			}
			note.LatestVersion = now
			u.WeaponNotes[finalId] = note

			weapon.LatestVersion = now
			u.Weapons[wUuid] = weapon

			// 7. Replicate lunar-tear's checkWeaponStoryUnlocks so stories
			//    2-4 unlock at the relevant level / evolution / max-level
			//    milestones we just hit.
			checkWeaponStoryUnlocksMass(catalog, u, finalId, weapon.Level, now)
			applied++
		}
	})
	if err != nil {
		return 0, fmt.Errorf("upgrade weapons: %w", err)
	}
	return applied, nil
}

func rebuildWeaponSkills(catalog *masterdata.WeaponCatalog, u *store.UserState, wUuid string, finalMaster masterdata.EntityMWeapon, limitBreakCount int32) {
	groupRows := catalog.SkillGroupsByGroupId[finalMaster.WeaponSkillGroupId]
	if len(groupRows) == 0 {
		return
	}
	maxLevel := massWeaponSkillMaxLevel
	if maxFunc, ok := catalog.SkillMaxLevelByEnhanceId[finalMaster.WeaponSpecificEnhanceId]; ok {
		maxLevel = maxFunc.Evaluate(limitBreakCount)
	}
	skills := make([]store.WeaponSkillState, 0, len(groupRows))
	seenSlot := make(map[int32]bool, len(groupRows))
	for _, g := range groupRows {
		if seenSlot[g.SlotNumber] {
			continue
		}
		seenSlot[g.SlotNumber] = true
		skills = append(skills, store.WeaponSkillState{
			UserWeaponUuid: wUuid,
			SlotNumber:     g.SlotNumber,
			Level:          maxLevel,
		})
	}
	u.WeaponSkills[wUuid] = skills
}

func rebuildWeaponAbilities(catalog *masterdata.WeaponCatalog, u *store.UserState, wUuid string, finalMaster masterdata.EntityMWeapon, limitBreakCount int32) {
	slots := catalog.AbilitySlots[finalMaster.WeaponAbilityGroupId]
	if len(slots) == 0 {
		return
	}
	maxLevel := massWeaponSkillMaxLevel
	if maxFunc, ok := catalog.AbilityMaxLevelByEnhanceId[finalMaster.WeaponSpecificEnhanceId]; ok {
		maxLevel = maxFunc.Evaluate(limitBreakCount)
	}
	abilities := make([]store.WeaponAbilityState, 0, len(slots))
	seenSlot := make(map[int32]bool, len(slots))
	for _, slot := range slots {
		if seenSlot[slot] {
			continue
		}
		seenSlot[slot] = true
		abilities = append(abilities, store.WeaponAbilityState{
			UserWeaponUuid: wUuid,
			SlotNumber:     slot,
			Level:          maxLevel,
		})
	}
	u.WeaponAbilities[wUuid] = abilities
}

// checkWeaponStoryUnlocksMass mirrors lunar-tear's checkWeaponStoryUnlocks
// (unexported in service/weapon.go). After the mass upgrade the weapon is at
// final form + max level + maxed evolution count, so all condition types
// fire.
func checkWeaponStoryUnlocksMass(catalog *masterdata.WeaponCatalog, u *store.UserState, weaponId, level int32, now int64) {
	wm, ok := catalog.Weapons[weaponId]
	if !ok || wm.WeaponStoryReleaseConditionGroupId == 0 {
		return
	}
	evoOrder, hasEvo := catalog.EvolutionOrder[weaponId]
	conditions := catalog.ReleaseConditionsByGroupId[wm.WeaponStoryReleaseConditionGroupId]
	for _, cond := range conditions {
		switch model.WeaponStoryReleaseConditionType(cond.WeaponStoryReleaseConditionType) {
		case model.WeaponStoryReleaseConditionTypeAcquisition:
			store.GrantWeaponStoryUnlock(u, weaponId, cond.StoryIndex, now)
		case model.WeaponStoryReleaseConditionTypeReachSpecifiedLevel:
			if level >= cond.ConditionValue {
				store.GrantWeaponStoryUnlock(u, weaponId, cond.StoryIndex, now)
			}
		case model.WeaponStoryReleaseConditionTypeReachInitialMaxLevel:
			if maxFunc, ok := catalog.MaxLevelByEnhanceId[wm.WeaponSpecificEnhanceId]; ok {
				if level >= maxFunc.Evaluate(0) {
					store.GrantWeaponStoryUnlock(u, weaponId, cond.StoryIndex, now)
				}
			}
		case model.WeaponStoryReleaseConditionTypeReachOnceEvolvedMaxLevel:
			if hasEvo && evoOrder >= 1 {
				if maxFunc, ok := catalog.MaxLevelByEnhanceId[wm.WeaponSpecificEnhanceId]; ok {
					if level >= maxFunc.Evaluate(0) {
						store.GrantWeaponStoryUnlock(u, weaponId, cond.StoryIndex, now)
					}
				}
			}
		case model.WeaponStoryReleaseConditionTypeReachSpecifiedEvolutionCount:
			if hasEvo && evoOrder >= cond.ConditionValue {
				store.GrantWeaponStoryUnlock(u, weaponId, cond.StoryIndex, now)
			}
		}
	}
}

// loadMassCostumeCatalog mirrors loadMassWeaponCatalog for costumes. Also
// loads the CharacterRebirthCatalog so per-character CostumeLevelLimitUp can
// be added to the rarity-based cap (rebirth count 5 = +10 levels for SSR,
// taking the cap from 90 to the in-game-visible 100).
func loadMassCostumeCatalog(masterDataPath string) (*masterdata.CostumeCatalog, *masterdata.CharacterRebirthCatalog, error) {
	if err := memorydb.Init(masterDataPath); err != nil {
		return nil, nil, fmt.Errorf("init master data: %w", err)
	}
	matCatalog, err := masterdata.LoadMaterialCatalog()
	if err != nil {
		return nil, nil, fmt.Errorf("load material catalog: %w", err)
	}
	cat, err := masterdata.LoadCostumeCatalog(matCatalog)
	if err != nil {
		return nil, nil, fmt.Errorf("load costume catalog: %w", err)
	}
	rebirthCat, err := masterdata.LoadCharacterRebirthCatalog()
	if err != nil {
		return nil, nil, fmt.Errorf("load character rebirth catalog: %w", err)
	}
	return cat, rebirthCat, nil
}

// costumeRebirthBonus sums CostumeLevelLimitUp for steps 0..rebirthCount-1
// of the given character's rebirth step group. Returns 0 if the character has
// no rebirth data or rebirth count is 0.
func costumeRebirthBonus(rebirthCat *masterdata.CharacterRebirthCatalog, characterId, rebirthCount int32) int32 {
	if rebirthCount <= 0 {
		return 0
	}
	stepGroupId, ok := rebirthCat.StepGroupByCharacterId[characterId]
	if !ok {
		return 0
	}
	bonus := int32(0)
	for i := int32(0); i < rebirthCount; i++ {
		step, ok := rebirthCat.StepByGroupAndCount[masterdata.StepKey{GroupId: stepGroupId, BeforeRebirthCount: i}]
		if !ok {
			continue
		}
		bonus += step.CostumeLevelLimitUp
	}
	return bonus
}

// runUpgradeAllCostumes awakens (with each step's effects), ascends, levels
// and skill-maxes every owned costume. SSR costumes also get all 3 lottery
// (karma) slots unlocked with OddsNumber=0 so the player rolls in-game.
func runUpgradeAllCostumes(req *request) (int, error) {
	if req.MasterDataPath == "" {
		return 0, errors.New("master_data_path required for upgrade_all_costumes")
	}
	catalog, rebirthCat, err := loadMassCostumeCatalog(req.MasterDataPath)
	if err != nil {
		return 0, err
	}

	db, st, err := openDB(req.DBPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	now := time.Now().UnixMilli()
	applied := 0
	_, err = st.UpdateUser(req.UserID, func(u *store.UserState) {
		for cUuid, costume := range u.Costumes {
			cm, ok := catalog.Costumes[costume.CostumeId]
			if !ok {
				continue
			}

			// 1. Awaken from current AwakenCount up to the cap, applying each
			//    step's effect (StatusUp / Ability / ItemAcquire). Lunar-tear
			//    treats Ability effects as client-resolved, so they are a
			//    server-side no-op.
			if awakenRow, ok := catalog.AwakenByCostumeId[costume.CostumeId]; ok {
				effectSteps := catalog.AwakenEffectsByGroupAndStep[awakenRow.CostumeAwakenEffectGroupId]
				for step := costume.AwakenCount + 1; step <= massCostumeAwakenMax; step++ {
					eff, has := effectSteps[step]
					if !has {
						continue
					}
					switch model.CostumeAwakenEffectType(eff.CostumeAwakenEffectType) {
					case model.CostumeAwakenEffectTypeStatusUp:
						applyMassCostumeStatusUp(catalog, u, cUuid, eff.CostumeAwakenEffectId, now)
					case model.CostumeAwakenEffectTypeItemAcquire:
						applyMassCostumeItemAcquire(catalog, u, eff.CostumeAwakenEffectId, now)
					}
				}
				if costume.AwakenCount < massCostumeAwakenMax {
					costume.AwakenCount = massCostumeAwakenMax
				}
			}

			// 2. Ascend to LB cap.
			if costume.LimitBreakCount < massCostumeLimitBreakMax {
				costume.LimitBreakCount = massCostumeLimitBreakMax
			}

			// 3. Enhance to rarity-driven level cap, plus the per-character
			//    rebirth bonus (CostumeLevelLimitUp summed across completed
			//    rebirth steps). For SSR @ LB4 + 5 rebirths this gives the
			//    in-game-visible cap of 100 (90 base + 10 from rebirths).
			capLevel := int32(0)
			if maxFunc, ok := catalog.MaxLevelByRarity[cm.RarityType]; ok {
				capLevel = maxFunc.Evaluate(costume.LimitBreakCount)
			}
			capLevel += costumeRebirthBonus(rebirthCat, cm.CharacterId, u.CharacterRebirths[cm.CharacterId].RebirthCount)
			if thresholds, ok := catalog.ExpByRarity[cm.RarityType]; ok && capLevel > 0 {
				if int(capLevel) < len(thresholds) {
					costume.Exp = thresholds[capLevel]
				} else {
					costume.Exp = thresholds[len(thresholds)-1]
				}
				costume.Level, costume.Exp = gameutil.LevelAndCap(costume.Exp, thresholds)
				if costume.Level > capLevel {
					costume.Level = capLevel
				}
			} else if capLevel > 0 {
				costume.Level = capLevel
			}

			// 4. Active skill to its rarity-defined max level (15 for SSR
			//    according to the user's spec; rarity functions return the
			//    correct cap for SR/R too).
			if maxFunc, ok := catalog.ActiveSkillMaxLevelByRarity[cm.RarityType]; ok {
				skillMax := maxFunc.Evaluate(1)
				skill := u.CostumeActiveSkills[cUuid]
				skill.UserCostumeUuid = cUuid
				if skill.AcquisitionDatetime == 0 {
					skill.AcquisitionDatetime = now
				}
				if skill.Level < skillMax {
					skill.Level = skillMax
				}
				skill.LatestVersion = now
				u.CostumeActiveSkills[cUuid] = skill
			}

			// 5. SSR-only: unlock the 3 lottery (karma) slots. Leave
			//    OddsNumber=0 so the player rolls in-game (DrawLotteryEffect
			//    is server-side rand.Int31n; we don't pre-roll here).
			if cm.RarityType == 40 {
				for slot := int32(1); slot <= massCostumeLotterySlots; slot++ {
					if _, hasOdds := catalog.LotteryEffects[[2]int32{costume.CostumeId, slot}]; !hasOdds {
						continue
					}
					key := store.CostumeLotteryEffectKey{
						UserCostumeUuid: cUuid,
						SlotNumber:      slot,
					}
					existing, exists := u.CostumeLotteryEffects[key]
					if exists {
						continue
					}
					existing.UserCostumeUuid = cUuid
					existing.SlotNumber = slot
					existing.OddsNumber = 0
					existing.LatestVersion = now
					u.CostumeLotteryEffects[key] = existing
				}
				if costume.CostumeLotteryEffectUnlockedSlotCount < massCostumeLotterySlots {
					costume.CostumeLotteryEffectUnlockedSlotCount = massCostumeLotterySlots
				}
			}

			costume.LatestVersion = now
			u.Costumes[cUuid] = costume
			applied++
		}
	})
	if err != nil {
		return 0, fmt.Errorf("upgrade costumes: %w", err)
	}
	return applied, nil
}

// runFillKarmaSlots writes a chosen OddsNumber into every already-unlocked
// karma slot. For each costume's slot the shim walks the user's preference
// list (in order) and picks the first preference whose (EffectType,
// TargetId) is present in that costume's odds pool. If no preference
// matches, falls back to the rarest pool entry (highest RarityType, ties
// broken by lowest OddsNumber).
//
// Always overwrites — even if the slot already had an OddsNumber rolled,
// it is replaced. The slot must already be unlocked
// (CostumeLotteryEffects[(uuid, slot)] exists), which Upgrade All Costumes
// handles for SSR.
func runFillKarmaSlots(req *request) (int, error) {
	if req.MasterDataPath == "" {
		return 0, errors.New("master_data_path required for fill_karma_slots")
	}
	catalog, _, err := loadMassCostumeCatalog(req.MasterDataPath)
	if err != nil {
		return 0, err
	}

	db, st, err := openDB(req.DBPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	now := time.Now().UnixMilli()
	applied := 0
	_, err = st.UpdateUser(req.UserID, func(u *store.UserState) {
		for cUuid, costume := range u.Costumes {
			touched := false
			for slot := int32(1); slot <= massCostumeLotterySlots; slot++ {
				key := store.CostumeLotteryEffectKey{
					UserCostumeUuid: cUuid,
					SlotNumber:      slot,
				}
				effect, exists := u.CostumeLotteryEffects[key]
				if !exists {
					continue
				}
				effectRow, ok := catalog.LotteryEffects[[2]int32{costume.CostumeId, slot}]
				if !ok {
					continue
				}
				oddsPool := catalog.LotteryEffectOdds[effectRow.CostumeLotteryEffectOddsGroupId]
				if len(oddsPool) == 0 {
					continue
				}
				prefs := req.KarmaPreferences[fmt.Sprintf("%d", slot)]
				picked := pickKarmaEntry(oddsPool, prefs)
				if picked.OddsNumber == 0 {
					continue
				}
				if effect.OddsNumber == picked.OddsNumber {
					continue
				}
				effect.OddsNumber = picked.OddsNumber
				effect.LatestVersion = now
				u.CostumeLotteryEffects[key] = effect
				touched = true
				applied++
			}
			if touched {
				costume.LatestVersion = now
				u.Costumes[cUuid] = costume
			}
		}
	})
	if err != nil {
		return 0, fmt.Errorf("fill karma slots: %w", err)
	}
	return applied, nil
}

// runSetCostumeKarmaBatch writes per-costume karma. Each spec carries
// {costume_id, karma: {slot: odds_number}}. For each spec we find the
// user_costume by costume_id (one per id is the lunar-tear invariant),
// validate the slot is already unlocked, and write the OddsNumber.
// No-op writes (same value already there) are skipped so the diff stays
// minimal.
func runSetCostumeKarmaBatch(req *request) (int, error) {
	if len(req.CostumeKarma) == 0 {
		return 0, errors.New("costume_karma list is empty")
	}

	db, st, err := openDB(req.DBPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	now := time.Now().UnixMilli()
	applied := 0
	_, err = st.UpdateUser(req.UserID, func(u *store.UserState) {
		// Build costume_id -> uuid index once. lunar-tear's GrantCostume
		// self-skips already-owned, so we expect at most one user_costume
		// per costume_id.
		uuidByCostumeId := make(map[int32]string, len(u.Costumes))
		for uid, c := range u.Costumes {
			uuidByCostumeId[c.CostumeId] = uid
		}

		for _, spec := range req.CostumeKarma {
			cUuid, ok := uuidByCostumeId[spec.CostumeID]
			if !ok {
				continue
			}
			costume := u.Costumes[cUuid]
			touched := false
			for slotStr, oddsNumber := range spec.Karma {
				var slot int32
				if _, err := fmt.Sscanf(slotStr, "%d", &slot); err != nil || slot < 1 || slot > 3 {
					continue
				}
				key := store.CostumeLotteryEffectKey{
					UserCostumeUuid: cUuid,
					SlotNumber:      slot,
				}
				effect, exists := u.CostumeLotteryEffects[key]
				if !exists {
					// Slot not yet unlocked — skip rather than silently
					// inserting, so callers know they need to run Upgrade
					// All Costumes (or unlock individually) first.
					continue
				}
				if effect.OddsNumber == oddsNumber {
					continue
				}
				effect.OddsNumber = oddsNumber
				effect.LatestVersion = now
				u.CostumeLotteryEffects[key] = effect
				touched = true
				applied++
			}
			if touched {
				costume.LatestVersion = now
				u.Costumes[cUuid] = costume
			}
		}
	})
	if err != nil {
		return 0, fmt.Errorf("set costume karma: %w", err)
	}
	return applied, nil
}

// pickKarmaEntry walks the preference list in order and returns the first
// pool entry matching a preference's (EffectType, TargetId). If none
// match, returns the rarest entry in the pool (highest RarityType, ties
// broken by lowest OddsNumber).
func pickKarmaEntry(pool []masterdata.EntityMCostumeLotteryEffectOddsGroup, prefs []karmaPref) masterdata.EntityMCostumeLotteryEffectOddsGroup {
	for _, pref := range prefs {
		for _, e := range pool {
			if e.CostumeLotteryEffectType == pref.EffectType && e.CostumeLotteryEffectTargetId == pref.TargetID {
				return e
			}
		}
	}
	var best masterdata.EntityMCostumeLotteryEffectOddsGroup
	bestSet := false
	for _, e := range pool {
		if !bestSet {
			best = e
			bestSet = true
			continue
		}
		if e.RarityType > best.RarityType ||
			(e.RarityType == best.RarityType && e.OddsNumber < best.OddsNumber) {
			best = e
		}
	}
	return best
}

// applyMassCostumeStatusUp mirrors applyCostumeAwakenStatusUp from
// lunar-tear's costume service. Adds per-(uuid, calc_type) status rows to
// CostumeAwakenStatusUps; values accumulate across awaken steps.
func applyMassCostumeStatusUp(catalog *masterdata.CostumeCatalog, u *store.UserState, costumeUuid string, statusUpGroupId int32, now int64) {
	rows, ok := catalog.AwakenStatusUpByGroup[statusUpGroupId]
	if !ok {
		return
	}
	for _, row := range rows {
		calcType := model.StatusCalculationType(row.StatusCalculationType)
		key := store.CostumeAwakenStatusKey{
			UserCostumeUuid:       costumeUuid,
			StatusCalculationType: calcType,
		}
		state := u.CostumeAwakenStatusUps[key]
		state.UserCostumeUuid = costumeUuid
		state.StatusCalculationType = calcType
		switch model.StatusKindType(row.StatusKindType) {
		case model.StatusKindTypeHp:
			state.Hp += row.EffectValue
		case model.StatusKindTypeAttack:
			state.Attack += row.EffectValue
		case model.StatusKindTypeVitality:
			state.Vitality += row.EffectValue
		case model.StatusKindTypeAgility:
			state.Agility += row.EffectValue
		case model.StatusKindTypeCriticalRatio:
			state.CriticalRatio += row.EffectValue
		case model.StatusKindTypeCriticalAttack:
			state.CriticalAttack += row.EffectValue
		}
		state.LatestVersion = now
		u.CostumeAwakenStatusUps[key] = state
	}
}

// applyMassCostumeItemAcquire mirrors applyCostumeAwakenItemAcquire. The
// awaken step 5 effect is typically a Thought (Debris) grant; self-skip if
// the player already has it.
func applyMassCostumeItemAcquire(catalog *masterdata.CostumeCatalog, u *store.UserState, itemAcquireId int32, now int64) {
	acq, ok := catalog.AwakenItemAcquireById[itemAcquireId]
	if !ok {
		return
	}
	if model.PossessionType(acq.PossessionType) != model.PossessionTypeThought {
		// Defensive: every awaken-step-5 reward we have observed is a
		// Thought. If the master data ever ships a non-Thought reward here,
		// route it through GrantPossession explicitly to handle it.
		store.GrantPossession(u, model.PossessionType(acq.PossessionType), acq.PossessionId, acq.Count)
		return
	}
	for _, t := range u.Thoughts {
		if t.ThoughtId == acq.PossessionId {
			return
		}
	}
	key := uuid.New().String()
	u.Thoughts[key] = store.ThoughtState{
		UserThoughtUuid:     key,
		ThoughtId:           acq.PossessionId,
		AcquisitionDatetime: now,
		LatestVersion:       now,
	}
}
