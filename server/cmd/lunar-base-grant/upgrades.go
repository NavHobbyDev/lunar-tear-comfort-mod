// Upgrade actions for the lunar-base-grant shim:
//   - grant_companion_batch
//   - exalt_characters
//   - release_panels
//
// These mirror lunar-tear's service-layer logic (CharacterService.Rebirth,
// CharacterBoardService.ReleasePanel, CompanionService) but skip material/
// gold consumption — lunar-base treats upgrades as a no-cost grant.

package main

import (
	"errors"
	"fmt"
	"time"

	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/masterdata/memorydb"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/questflow"
	"lunar-tear/server/internal/store"
)

// runCompanionBatch grants every companion id in the request through the
// existing PossessionGranter.GrantCompanion (which self-skips already-owned).
func runCompanionBatch(req *request) (int, error) {
	if req.MasterDataPath == "" {
		return 0, errors.New("master_data_path required for grant_companion_batch")
	}
	if len(req.CompanionIDs) == 0 {
		return 0, errors.New("companion_ids list is empty")
	}

	if err := memorydb.Init(req.MasterDataPath); err != nil {
		return 0, fmt.Errorf("init master data: %w", err)
	}
	partsCatalog, err := masterdata.LoadPartsCatalog()
	if err != nil {
		return 0, fmt.Errorf("load parts catalog: %w", err)
	}
	catalog, err := masterdata.LoadQuestCatalog(partsCatalog)
	if err != nil {
		return 0, fmt.Errorf("load quest catalog: %w", err)
	}
	gameConfig, err := masterdata.LoadGameConfig()
	if err != nil {
		return 0, fmt.Errorf("load game config: %w", err)
	}
	granter := questflow.BuildGranter(catalog, gameConfig)

	db, st, err := openDB(req.DBPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	now := time.Now().UnixMilli()
	_, err = st.UpdateUser(req.UserID, func(u *store.UserState) {
		for _, companionID := range req.CompanionIDs {
			granter.GrantCompanion(u, companionID, now)
		}
	})
	if err != nil {
		return 0, fmt.Errorf("grant companion: %w", err)
	}
	return len(req.CompanionIDs), nil
}

// runExalt sets CharacterRebirths[id] = {id, count, now} for each spec. It
// does NOT consume gold or rebirth materials. lunar-tear's Rebirth handler
// loops once per increment to consume per-step materials; we just want to
// set the final state.
func runExalt(req *request) (int, error) {
	if len(req.Exaltations) == 0 {
		return 0, errors.New("exaltations list is empty")
	}

	db, st, err := openDB(req.DBPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	now := time.Now().UnixMilli()
	applied := 0
	_, err = st.UpdateUser(req.UserID, func(u *store.UserState) {
		for _, e := range req.Exaltations {
			if e.RebirthCount <= 0 {
				continue
			}
			cur := u.CharacterRebirths[e.CharacterID]
			if cur.RebirthCount >= e.RebirthCount {
				continue
			}
			u.CharacterRebirths[e.CharacterID] = store.CharacterRebirthState{
				CharacterId:   e.CharacterID,
				RebirthCount:  e.RebirthCount,
				LatestVersion: now,
			}
			applied++
		}
	})
	if err != nil {
		return 0, fmt.Errorf("exalt: %w", err)
	}
	return applied, nil
}

// runReleasePanels mirrors lunar-tear's CharacterBoardService.ReleasePanel
// flow, minus cost consumption. For every panel id in the request we:
//   1. Set the appropriate panel-release bit on the panel's board.
//   2. Apply each release effect (status-up or ability) to the user state.
//
// The catalog is loaded fresh per invocation; cold cost ~300-600ms but
// batches of thousands of panel ids share that single load.
func runReleasePanels(req *request) (int, error) {
	if req.MasterDataPath == "" {
		return 0, errors.New("master_data_path required for release_panels")
	}
	if len(req.PanelIDs) == 0 {
		return 0, errors.New("panel_ids list is empty")
	}

	if err := memorydb.Init(req.MasterDataPath); err != nil {
		return 0, fmt.Errorf("init master data: %w", err)
	}
	catalog, err := masterdata.LoadCharacterBoardCatalog()
	if err != nil {
		return 0, fmt.Errorf("load character board catalog: %w", err)
	}

	db, st, err := openDB(req.DBPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	now := time.Now().UnixMilli()
	applied := 0
	_, err = st.UpdateUser(req.UserID, func(u *store.UserState) {
		for _, panelID := range req.PanelIDs {
			panel, ok := catalog.PanelById[panelID]
			if !ok {
				continue
			}
			setPanelReleaseBit(u, panel, now)
			applyPanelEffects(catalog, u, panel, now)
			applied++
		}
	})
	if err != nil {
		return 0, fmt.Errorf("release panels: %w", err)
	}
	return applied, nil
}

// setPanelReleaseBit replicates lunar-tear's
// service.setBoardReleaseBit (an unexported helper). The release bits are
// packed into 4 int32 fields; sort_order N (1-indexed) maps to bit (N-1)%32
// in field bit{(N-1)/32 + 1}.
func setPanelReleaseBit(u *store.UserState, panel masterdata.EntityMCharacterBoardPanel, now int64) {
	board := u.CharacterBoards[panel.CharacterBoardId]
	board.CharacterBoardId = panel.CharacterBoardId
	board.LatestVersion = now

	bitFieldIndex := (panel.SortOrder - 1) / 32
	bitPosition := (panel.SortOrder - 1) % 32
	mask := int32(1 << uint(bitPosition))

	switch bitFieldIndex {
	case 0:
		board.PanelReleaseBit1 |= mask
	case 1:
		board.PanelReleaseBit2 |= mask
	case 2:
		board.PanelReleaseBit3 |= mask
	case 3:
		board.PanelReleaseBit4 |= mask
	}

	u.CharacterBoards[panel.CharacterBoardId] = board
}

// applyPanelEffects replicates lunar-tear's service.applyBoardEffects.
func applyPanelEffects(catalog *masterdata.CharacterBoardCatalog, u *store.UserState, panel masterdata.EntityMCharacterBoardPanel, now int64) {
	effects := catalog.ReleaseEffectsByGroupId[panel.CharacterBoardPanelReleaseEffectGroupId]
	for _, eff := range effects {
		switch model.CharacterBoardEffectType(eff.CharacterBoardEffectType) {
		case model.CharacterBoardEffectTypeAbility:
			applyPanelAbilityEffect(catalog, u, eff, now)
		case model.CharacterBoardEffectTypeStatusUp:
			applyPanelStatusUpEffect(catalog, u, eff, now)
		}
	}
}

func applyPanelAbilityEffect(catalog *masterdata.CharacterBoardCatalog, u *store.UserState, eff masterdata.EntityMCharacterBoardPanelReleaseEffectGroup, now int64) {
	ability, ok := catalog.AbilityById[eff.CharacterBoardEffectId]
	if !ok {
		return
	}
	characterID := resolveTargetCharacter(catalog, ability.CharacterBoardEffectTargetGroupId)
	if characterID == 0 {
		return
	}
	key := store.CharacterBoardAbilityKey{CharacterId: characterID, AbilityId: ability.AbilityId}
	state := u.CharacterBoardAbilities[key]
	state.CharacterId = characterID
	state.AbilityId = ability.AbilityId
	state.Level += eff.EffectValue
	if maxLvl, ok := catalog.AbilityMaxLevel[key]; ok && state.Level > maxLvl {
		state.Level = maxLvl
	}
	u.CharacterBoardAbilities[key] = state
}

func applyPanelStatusUpEffect(catalog *masterdata.CharacterBoardCatalog, u *store.UserState, eff masterdata.EntityMCharacterBoardPanelReleaseEffectGroup, now int64) {
	statusUp, ok := catalog.StatusUpById[eff.CharacterBoardEffectId]
	if !ok {
		return
	}
	characterID := resolveTargetCharacter(catalog, statusUp.CharacterBoardEffectTargetGroupId)
	if characterID == 0 {
		return
	}
	supType := model.CharacterBoardStatusUpType(statusUp.CharacterBoardStatusUpType)
	calcType := model.StatusUpTypeToCalcType(supType)
	key := store.CharacterBoardStatusUpKey{
		CharacterId:           characterID,
		StatusCalculationType: int32(calcType),
	}
	state := u.CharacterBoardStatusUps[key]
	state.CharacterId = characterID
	state.StatusCalculationType = int32(calcType)
	switch supType {
	case model.CharacterBoardStatusUpTypeAgilityAdd, model.CharacterBoardStatusUpTypeAgilityMultiply:
		state.Agility += eff.EffectValue
	case model.CharacterBoardStatusUpTypeAttackAdd, model.CharacterBoardStatusUpTypeAttackMultiply:
		state.Attack += eff.EffectValue
	case model.CharacterBoardStatusUpTypeCritAttackAdd:
		state.CriticalAttack += eff.EffectValue
	case model.CharacterBoardStatusUpTypeCritRatioAdd:
		state.CriticalRatio += eff.EffectValue
	case model.CharacterBoardStatusUpTypeHpAdd, model.CharacterBoardStatusUpTypeHpMultiply:
		state.Hp += eff.EffectValue
	case model.CharacterBoardStatusUpTypeVitalityAdd, model.CharacterBoardStatusUpTypeVitalityMultiply:
		state.Vitality += eff.EffectValue
	}
	u.CharacterBoardStatusUps[key] = state
}

func resolveTargetCharacter(catalog *masterdata.CharacterBoardCatalog, targetGroupID int32) int32 {
	for _, t := range catalog.EffectTargetsByGroupId[targetGroupID] {
		if t.TargetValue != 0 {
			return t.TargetValue
		}
	}
	return 0
}
