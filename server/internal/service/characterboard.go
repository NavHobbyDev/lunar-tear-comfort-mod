package service

import (
	"context"
	"log"
	"math/bits"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/runtime"
	"lunar-tear/server/internal/store"
)

type CharacterBoardServiceServer struct {
	pb.UnimplementedCharacterBoardServiceServer
	users    store.UserRepository
	sessions store.SessionRepository
	holder   *runtime.Holder
}

func NewCharacterBoardServiceServer(users store.UserRepository, sessions store.SessionRepository, holder *runtime.Holder) *CharacterBoardServiceServer {
	return &CharacterBoardServiceServer{users: users, sessions: sessions, holder: holder}
}

func (s *CharacterBoardServiceServer) ReleasePanel(ctx context.Context, req *pb.ReleasePanelRequest) (*pb.ReleasePanelResponse, error) {
	log.Printf("[CharacterBoardService] ReleasePanel: panelIds=%v", req.CharacterBoardPanelId)

	cat := s.holder.Get()
	boardCatalog := cat.CharacterBoard
	userId := CurrentUserId(ctx, s.users, s.sessions)

	_, err := s.users.UpdateUser(userId, func(user *store.UserState) {
		nowMillis := gametime.NowMillis()
		released := false
		for _, panelId := range req.CharacterBoardPanelId {
			panel, ok := boardCatalog.PanelById[panelId]
			if !ok {
				log.Printf("[CharacterBoardService] unknown panelId=%d, skipping", panelId)
				continue
			}

			consumeBoardCosts(boardCatalog, user, panel)
			setBoardReleaseBit(user, panel, nowMillis)
			applyBoardEffects(boardCatalog, user, panel, nowMillis)
			released = true
		}
		if released {
			applyBoardPanelMissionProgress(boardCatalog, cat.Mission, user, nowMillis)
		}
	})
	if err != nil {
		log.Printf("[CharacterBoardService] ReleasePanel: UpdateUser error: %v", err)
	}

	return &pb.ReleasePanelResponse{}, nil
}

func consumeBoardCosts(catalog *masterdata.CharacterBoardCatalog, user *store.UserState, panel masterdata.EntityMCharacterBoardPanel) {
	costs := catalog.ReleaseCostsByGroupId[panel.CharacterBoardPanelReleasePossessionGroupId]
	for _, cost := range costs {
		store.DeductPossession(user, model.PossessionType(cost.PossessionType), cost.PossessionId, cost.Count)
	}
}

// applyBoardPanelMissionProgress recounts released panels and feeds type-54
// missions with absolute values, so progress stays correct even for panels
// released before a mission system fix or restore. Character-specific
// missions ("Unlock N Stone Tower/Cursed God Monument panels for X",
// 480001-480126) match their option group (310001-310042); the generic
// "Unlock N Mythic Slab panels" missions use option group 0 and take the
// total across all boards.
func applyBoardPanelMissionProgress(catalog *masterdata.CharacterBoardCatalog, missions *masterdata.MissionCatalog, user *store.UserState, nowMillis int64) {
	total := int32(0)
	countByOption := map[int32]int32{}
	for boardId, state := range user.CharacterBoards {
		n := releasedPanelCount(state)
		total += n
		if opt, ok := catalog.MissionOptionGroupByBoardId[boardId]; ok {
			countByOption[opt] += n
		}
	}
	for opt, count := range countByOption {
		ApplyMissionProgressEvent(user, missions, MissionProgressEvent{ConditionType: missionConditionCharacterBoardPanel, TargetId: opt, CurrentValue: count}, nowMillis)
	}
	ApplyMissionProgressEvent(user, missions, MissionProgressEvent{ConditionType: missionConditionCharacterBoardPanel, CurrentValue: total}, nowMillis)
}

func releasedPanelCount(b store.CharacterBoardState) int32 {
	return int32(bits.OnesCount32(uint32(b.PanelReleaseBit1)) +
		bits.OnesCount32(uint32(b.PanelReleaseBit2)) +
		bits.OnesCount32(uint32(b.PanelReleaseBit3)) +
		bits.OnesCount32(uint32(b.PanelReleaseBit4)))
}

func setBoardReleaseBit(user *store.UserState, panel masterdata.EntityMCharacterBoardPanel, nowMillis int64) {
	boardId := panel.CharacterBoardId
	board := user.CharacterBoards[boardId]
	board.CharacterBoardId = boardId

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

	board.LatestVersion = nowMillis
	user.CharacterBoards[boardId] = board
}

func applyBoardEffects(catalog *masterdata.CharacterBoardCatalog, user *store.UserState, panel masterdata.EntityMCharacterBoardPanel, nowMillis int64) {
	effects := catalog.ReleaseEffectsByGroupId[panel.CharacterBoardPanelReleaseEffectGroupId]
	for _, eff := range effects {
		switch model.CharacterBoardEffectType(eff.CharacterBoardEffectType) {
		case model.CharacterBoardEffectTypeAbility:
			applyBoardAbilityEffect(catalog, user, eff, nowMillis)
		case model.CharacterBoardEffectTypeStatusUp:
			applyBoardStatusUpEffect(catalog, user, eff, nowMillis)
		}
	}
}

func applyBoardAbilityEffect(catalog *masterdata.CharacterBoardCatalog, user *store.UserState, eff masterdata.EntityMCharacterBoardPanelReleaseEffectGroup, nowMillis int64) {
	ability, ok := catalog.AbilityById[eff.CharacterBoardEffectId]
	if !ok {
		log.Printf("[CharacterBoardService] unknown abilityId=%d", eff.CharacterBoardEffectId)
		return
	}

	characterId := resolveBoardCharacterId(catalog, ability.CharacterBoardEffectTargetGroupId)
	if characterId == 0 {
		return
	}

	key := store.CharacterBoardAbilityKey{CharacterId: characterId, AbilityId: ability.AbilityId}
	state := user.CharacterBoardAbilities[key]
	state.CharacterId = characterId
	state.AbilityId = ability.AbilityId
	state.Level += eff.EffectValue

	if maxLvl, ok := catalog.AbilityMaxLevel[key]; ok && state.Level > maxLvl {
		state.Level = maxLvl
	}

	state.LatestVersion = nowMillis
	user.CharacterBoardAbilities[key] = state
}

func applyBoardStatusUpEffect(catalog *masterdata.CharacterBoardCatalog, user *store.UserState, eff masterdata.EntityMCharacterBoardPanelReleaseEffectGroup, nowMillis int64) {
	statusUp, ok := catalog.StatusUpById[eff.CharacterBoardEffectId]
	if !ok {
		log.Printf("[CharacterBoardService] unknown statusUpId=%d", eff.CharacterBoardEffectId)
		return
	}

	characterId := resolveBoardCharacterId(catalog, statusUp.CharacterBoardEffectTargetGroupId)
	if characterId == 0 {
		return
	}

	supType := model.CharacterBoardStatusUpType(statusUp.CharacterBoardStatusUpType)
	calcType := model.StatusUpTypeToCalcType(supType)

	key := store.CharacterBoardStatusUpKey{
		CharacterId:           characterId,
		StatusCalculationType: int32(calcType),
	}
	state := user.CharacterBoardStatusUps[key]
	state.CharacterId = characterId
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

	state.LatestVersion = nowMillis
	user.CharacterBoardStatusUps[key] = state
}

func resolveBoardCharacterId(catalog *masterdata.CharacterBoardCatalog, targetGroupId int32) int32 {
	targets := catalog.EffectTargetsByGroupId[targetGroupId]
	for _, t := range targets {
		if t.TargetValue != 0 {
			return t.TargetValue
		}
	}
	log.Printf("[CharacterBoardService] no characterId resolved for targetGroupId=%d", targetGroupId)
	return 0
}
