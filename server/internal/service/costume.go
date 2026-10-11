package service

import (
	"context"
	"fmt"
	"log"
	"math/rand"

	"github.com/google/uuid"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/campaign"
	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/gameutil"
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/runtime"
	"lunar-tear/server/internal/store"
)

type CostumeServiceServer struct {
	pb.UnimplementedCostumeServiceServer
	users    store.UserRepository
	sessions store.SessionRepository
	holder   *runtime.Holder
}

func NewCostumeServiceServer(users store.UserRepository, sessions store.SessionRepository, holder *runtime.Holder) *CostumeServiceServer {
	return &CostumeServiceServer{users: users, sessions: sessions, holder: holder}
}

func (s *CostumeServiceServer) Enhance(ctx context.Context, req *pb.EnhanceRequest) (*pb.EnhanceResponse, error) {
	log.Printf("[CostumeService] Enhance: uuid=%s materials=%v", req.UserCostumeUuid, req.Materials)

	cat := s.holder.Get()
	catalog := cat.Costume
	config := cat.GameConfig
	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()

	_, err := s.users.UpdateUser(userId, func(user *store.UserState) {
		costume, ok := user.Costumes[req.UserCostumeUuid]
		if !ok {
			log.Printf("[CostumeService] Enhance: costume uuid=%s not found", req.UserCostumeUuid)
			return
		}

		cm, ok := catalog.Costumes[costume.CostumeId]
		if !ok {
			log.Printf("[CostumeService] Enhance: costume master id=%d not found", costume.CostumeId)
			return
		}

		expBonus := cat.Campaign.CostumeExpBonus(campaign.CostumeTarget{
			CostumeId:          costume.CostumeId,
			CharacterId:        cm.CharacterId,
			SkillfulWeaponType: cm.SkillfulWeaponType,
		}, campaign.Filter{NowMillis: nowMillis, UserStatus: campaign.TargetUserStatusAll})

		totalExp := int32(0)
		totalMaterialCount := int32(0)
		for materialId, count := range req.Materials {
			mat, ok := catalog.Materials[materialId]
			if !ok {
				log.Printf("[CostumeService] Enhance: material id=%d not found, skipping", materialId)
				continue
			}

			cur := user.Materials[materialId]
			if cur < count {
				log.Printf("[CostumeService] Enhance: insufficient material id=%d have=%d need=%d", materialId, cur, count)
				continue
			}
			user.Materials[materialId] = cur - count
			totalMaterialCount += count

			expPerUnit := mat.EffectValue
			if mat.WeaponType != 0 && mat.WeaponType == cm.SkillfulWeaponType {
				expPerUnit = expPerUnit * config.MaterialSameWeaponExpCoefficientPermil / 1000
			}
			totalExp += expBonus.Apply(expPerUnit * count)
		}

		if costFunc, ok := catalog.EnhanceCostByRarity[cm.RarityType]; ok && totalMaterialCount > 0 {
			goldCost := costFunc.Evaluate(totalMaterialCount)
			user.ConsumableItems[config.ConsumableItemIdForGold] -= goldCost
			log.Printf("[CostumeService] Enhance: gold cost=%d (materials=%d)", goldCost, totalMaterialCount)
		}

		costume.Exp += totalExp
		oldLevel := costume.Level
		if thresholds, ok := catalog.ExpByRarity[cm.RarityType]; ok {
			var maxLevel int32
			if maxLevelFunc, hasMax := catalog.MaxLevelByRarity[cm.RarityType]; hasMax {
				maxLevel = maxLevelFunc.Evaluate(costume.LimitBreakCount) +
					cat.CharacterRebirth.CostumeLevelLimitUp(cm.CharacterId, user.CharacterRebirths[cm.CharacterId].RebirthCount)
			}
			costume.Level, costume.Exp = gameutil.ApplyExpWithMaxLevel(costume.Exp, thresholds, maxLevel)
		}

		costume.LatestVersion = nowMillis
		user.Costumes[req.UserCostumeUuid] = costume
		if costume.Level > 0 {
			rebuildCharacterCostumeLevelBonuses(catalog, user, nowMillis)
		}
		if totalMaterialCount > 0 {
			levelIncrease := costume.Level - oldLevel
			if levelIncrease > 0 {
				ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{ConditionType: 9, TargetId: costume.CostumeId, Delta: levelIncrease}, nowMillis)
			}
			ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{ConditionType: 40, TargetId: costume.CostumeId, CurrentValue: costume.Level}, nowMillis)
		}
		log.Printf("[CostumeService] Enhance: costumeId=%d +%d exp -> total=%d level=%d", costume.CostumeId, totalExp, costume.Exp, costume.Level)
	})
	if err != nil {
		return nil, fmt.Errorf("costume enhance: %w", err)
	}

	return &pb.EnhanceResponse{
		IsGreatSuccess:         false,
		SurplusEnhanceMaterial: map[int32]int32{},
	}, nil
}

func (s *CostumeServiceServer) Awaken(ctx context.Context, req *pb.AwakenRequest) (*pb.AwakenResponse, error) {
	log.Printf("[CostumeService] Awaken: uuid=%s materials=%v", req.UserCostumeUuid, req.Materials)

	cat := s.holder.Get()
	catalog := cat.Costume
	config := cat.GameConfig
	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()

	_, err := s.users.UpdateUser(userId, func(user *store.UserState) {
		costume, ok := user.Costumes[req.UserCostumeUuid]
		if !ok {
			log.Printf("[CostumeService] Awaken: costume uuid=%s not found", req.UserCostumeUuid)
			return
		}

		awakenRow, ok := catalog.AwakenByCostumeId[costume.CostumeId]
		if !ok {
			log.Printf("[CostumeService] Awaken: no awaken data for costumeId=%d", costume.CostumeId)
			return
		}

		nextStep := costume.AwakenCount + 1

		if gold, ok := catalog.AwakenPriceByGroup[awakenRow.CostumeAwakenPriceGroupId]; ok {
			user.ConsumableItems[config.ConsumableItemIdForGold] -= gold
			log.Printf("[CostumeService] Awaken: gold cost=%d", gold)
		}

		for materialId, count := range req.Materials {
			cur := user.Materials[materialId]
			if cur < count {
				log.Printf("[CostumeService] Awaken: insufficient material id=%d have=%d need=%d", materialId, cur, count)
				count = cur
			}
			user.Materials[materialId] = cur - count
		}

		costume.AwakenCount = nextStep
		costume.LatestVersion = nowMillis
		user.Costumes[req.UserCostumeUuid] = costume
		log.Printf("[CostumeService] Awaken: costumeId=%d awakenCount=%d", costume.CostumeId, nextStep)

		effectSteps, ok := catalog.AwakenEffectsByGroupAndStep[awakenRow.CostumeAwakenEffectGroupId]
		if !ok {
			return
		}
		effect, ok := effectSteps[nextStep]
		if !ok {
			return
		}

		switch model.CostumeAwakenEffectType(effect.CostumeAwakenEffectType) {
		case model.CostumeAwakenEffectTypeStatusUp:
			applyCostumeAwakenStatusUp(catalog, user, req.UserCostumeUuid, effect.CostumeAwakenEffectId, nowMillis)
		case model.CostumeAwakenEffectTypeAbility:
			log.Printf("[CostumeService] Awaken: ability effect id=%d (client-resolved)", effect.CostumeAwakenEffectId)
		case model.CostumeAwakenEffectTypeItemAcquire:
			applyCostumeAwakenItemAcquire(catalog, cat.QuestHandler.Granter, user, effect.CostumeAwakenEffectId, nowMillis)
		default:
			log.Printf("[CostumeService] Awaken: unknown effect type=%d", effect.CostumeAwakenEffectType)
		}
		ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{ConditionType: missionConditionCostumeAwaken, TargetId: costume.CostumeId, Delta: 1}, nowMillis)
	})
	if err != nil {
		return nil, fmt.Errorf("costume awaken: %w", err)
	}

	return &pb.AwakenResponse{}, nil
}

func applyCostumeAwakenStatusUp(catalog *masterdata.CostumeCatalog, user *store.UserState, costumeUuid string, statusUpGroupId int32, nowMillis int64) {
	rows, ok := catalog.AwakenStatusUpByGroup[statusUpGroupId]
	if !ok {
		log.Printf("[CostumeService] Awaken: status up group %d not found", statusUpGroupId)
		return
	}

	for _, row := range rows {
		calcType := model.StatusCalculationType(row.StatusCalculationType)
		key := store.CostumeAwakenStatusKey{
			UserCostumeUuid:       costumeUuid,
			StatusCalculationType: calcType,
		}
		state := user.CostumeAwakenStatusUps[key]
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

		state.LatestVersion = nowMillis
		user.CostumeAwakenStatusUps[key] = state
	}
}

func applyCostumeAwakenItemAcquire(catalog *masterdata.CostumeCatalog, granter *store.PossessionGranter, user *store.UserState, itemAcquireId int32, nowMillis int64) {
	acq, ok := catalog.AwakenItemAcquireById[itemAcquireId]
	if !ok {
		log.Printf("[CostumeService] Awaken: item acquire id=%d not found", itemAcquireId)
		return
	}

	for _, t := range user.Thoughts {
		if t.ThoughtId == acq.PossessionId {
			return
		}
	}
	key := uuid.New().String()
	user.Thoughts[key] = store.ThoughtState{
		UserThoughtUuid:     key,
		ThoughtId:           acq.PossessionId,
		AcquisitionDatetime: nowMillis,
		LatestVersion:       nowMillis,
	}
	log.Printf("[CostumeService] Awaken: granted thought id=%d", acq.PossessionId)

	// Update encyclopedia missions (type 39) when a new thought is obtained.
	// The granter callback is the single implementation of that recount; the
	// type-25 meta-counter only advances when rewards are claimed.
	if granter != nil && granter.OnEncyclopediaEntryAdded != nil {
		granter.OnEncyclopediaEntryAdded(user)
	}
}

// RegisterLevelBonusConfirmed applies the per-character permanent "Karma" stat
// bonus that carries across costumes of the same character. Each newly-confirmed
// level (above the costume's existing ConfirmedBonusLevel, up to req.Level) adds
// its EffectValue onto the character's CharacterCostumeLevelBonusState, so the
// call is idempotent: re-calling with the same or a lower level adds nothing.
//
// Level-bonus rows carry no StatusCalculationType column and the observed
// CostumeLevelBonusType values {3,7,9} are all additive, so the accumulated
// state is keyed under a single fixed StatusCalculationTypeAdd.
func (s *CostumeServiceServer) RegisterLevelBonusConfirmed(ctx context.Context, req *pb.RegisterLevelBonusConfirmedRequest) (*pb.RegisterLevelBonusConfirmedResponse, error) {
	log.Printf("[CostumeService] RegisterLevelBonusConfirmed: costumeId=%d level=%d", req.CostumeId, req.Level)

	cat := s.holder.Get()
	catalog := cat.Costume
	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()

	_, err := s.users.UpdateUser(userId, func(user *store.UserState) {
		cm, ok := catalog.Costumes[req.CostumeId]
		if !ok {
			log.Printf("[CostumeService] RegisterLevelBonusConfirmed: costume master id=%d not found", req.CostumeId)
			return
		}

		release := user.CostumeLevelBonusReleaseStatuses[req.CostumeId]
		release.CostumeId = req.CostumeId

		applyCostumeLevelBonus(catalog, user, cm.CharacterId, req.CostumeId, release.ConfirmedBonusLevel, req.Level, nowMillis)

		if req.Level > release.LastReleasedBonusLevel {
			release.LastReleasedBonusLevel = req.Level
		}
		if req.Level > release.ConfirmedBonusLevel {
			release.ConfirmedBonusLevel = req.Level
		}
		release.LatestVersion = nowMillis
		user.CostumeLevelBonusReleaseStatuses[req.CostumeId] = release
	})
	if err != nil {
		return nil, fmt.Errorf("costume register level bonus confirmed: %w", err)
	}

	return &pb.RegisterLevelBonusConfirmedResponse{}, nil
}

// applyCostumeLevelBonus accumulates the EffectValue of every level-bonus row in
// (fromLevel, toLevel] onto the character's bonus state. Only newly-confirmed
// levels contribute, which keeps the operation idempotent.
func applyCostumeLevelBonus(catalog *masterdata.CostumeCatalog, user *store.UserState, characterId, costumeId, fromLevel, toLevel int32, nowMillis int64) {
	if toLevel <= fromLevel {
		return
	}

	calcType := model.StatusCalculationTypeAdd
	key := store.CharacterCostumeLevelBonusKey{
		CharacterId:           characterId,
		StatusCalculationType: calcType,
	}
	state := user.CharacterCostumeLevelBonuses[key]
	state.CharacterId = characterId
	state.StatusCalculationType = calcType

	changed := false
	for _, row := range catalog.BonusesUpToLevel(costumeId, toLevel) {
		if row.Level <= fromLevel {
			continue
		}
		switch model.CostumeLevelBonusTypeToStat(row.CostumeLevelBonusType) {
		case model.CostumeLevelBonusStatHp:
			state.Hp += row.EffectValue
		case model.CostumeLevelBonusStatAttack:
			state.Attack += row.EffectValue
		case model.CostumeLevelBonusStatVitality:
			state.Vitality += row.EffectValue
		case model.CostumeLevelBonusStatAgility:
			state.Agility += row.EffectValue
		case model.CostumeLevelBonusStatCriticalRatio:
			state.CriticalRatio += row.EffectValue
		default:
			log.Printf("[CostumeService] RegisterLevelBonusConfirmed: unknown bonus type=%d (costumeId=%d level=%d)", row.CostumeLevelBonusType, costumeId, row.Level)
			continue
		}
		changed = true
	}
	if !changed {
		return
	}

	state.LatestVersion = nowMillis
	user.CharacterCostumeLevelBonuses[key] = state
}

func (s *CostumeServiceServer) EnhanceActiveSkill(ctx context.Context, req *pb.EnhanceActiveSkillRequest) (*pb.EnhanceActiveSkillResponse, error) {
	log.Printf("[CostumeService] EnhanceActiveSkill: uuid=%s addLevel=%d", req.UserCostumeUuid, req.AddLevelCount)

	cat := s.holder.Get()
	catalog := cat.Costume
	config := cat.GameConfig
	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()

	_, err := s.users.UpdateUser(userId, func(user *store.UserState) {
		costume, ok := user.Costumes[req.UserCostumeUuid]
		if !ok {
			log.Printf("[CostumeService] EnhanceActiveSkill: costume uuid=%s not found", req.UserCostumeUuid)
			return
		}

		cm, ok := catalog.Costumes[costume.CostumeId]
		if !ok {
			log.Printf("[CostumeService] EnhanceActiveSkill: costume master id=%d not found", costume.CostumeId)
			return
		}

		groupRows := catalog.ActiveSkillGroupsByGroupId[cm.CostumeActiveSkillGroupId]
		enhanceMatId := int32(-1)
		for _, g := range groupRows {
			if g.CostumeLimitBreakCountLowerLimit <= costume.LimitBreakCount {
				enhanceMatId = g.CostumeActiveSkillEnhancementMaterialId
				break
			}
		}
		if enhanceMatId < 0 {
			log.Printf("[CostumeService] EnhanceActiveSkill: no skill group for costumeId=%d groupId=%d lb=%d",
				costume.CostumeId, cm.CostumeActiveSkillGroupId, costume.LimitBreakCount)
			return
		}

		skill := user.CostumeActiveSkills[req.UserCostumeUuid]
		currentLevel := skill.Level

		maxLevelFunc, ok := catalog.ActiveSkillMaxLevelByRarity[cm.RarityType]
		if !ok {
			log.Printf("[CostumeService] EnhanceActiveSkill: no max level func for rarity=%d", cm.RarityType)
			return
		}
		maxLevel := maxLevelFunc.Evaluate(1)

		addCount := req.AddLevelCount
		if currentLevel+addCount > maxLevel {
			addCount = maxLevel - currentLevel
		}
		if addCount <= 0 {
			log.Printf("[CostumeService] EnhanceActiveSkill: already at max level %d", currentLevel)
			return
		}

		for lvl := currentLevel; lvl < currentLevel+addCount; lvl++ {
			key := [2]int32{enhanceMatId, lvl}
			mats := catalog.ActiveSkillEnhanceMats[key]
			for _, mat := range mats {
				cur := user.Materials[mat.MaterialId]
				cost := mat.Count
				if cur < cost {
					log.Printf("[CostumeService] EnhanceActiveSkill: insufficient material id=%d have=%d need=%d", mat.MaterialId, cur, cost)
					cost = cur
				}
				user.Materials[mat.MaterialId] = cur - cost
			}

			if costFunc, ok := catalog.ActiveSkillCostByRarity[cm.RarityType]; ok {
				goldCost := costFunc.Evaluate(lvl + 1)
				user.ConsumableItems[config.ConsumableItemIdForGold] -= goldCost
			}
		}

		skill.UserCostumeUuid = req.UserCostumeUuid
		skill.Level = currentLevel + addCount
		skill.LatestVersion = nowMillis
		user.CostumeActiveSkills[req.UserCostumeUuid] = skill
		ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{ConditionType: 10, TargetId: costume.CostumeId, Delta: addCount}, nowMillis)
		ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{ConditionType: 41, TargetId: costume.CostumeId, CurrentValue: skill.Level}, nowMillis)
		log.Printf("[CostumeService] EnhanceActiveSkill: costumeId=%d level %d -> %d", costume.CostumeId, currentLevel, skill.Level)
	})
	if err != nil {
		return nil, fmt.Errorf("costume enhance active skill: %w", err)
	}

	return &pb.EnhanceActiveSkillResponse{}, nil
}

func (s *CostumeServiceServer) LimitBreak(ctx context.Context, req *pb.LimitBreakRequest) (*pb.LimitBreakResponse, error) {
	log.Printf("[CostumeService] LimitBreak: uuid=%s materials=%v", req.UserCostumeUuid, req.Materials)

	cat := s.holder.Get()
	catalog := cat.Costume
	config := cat.GameConfig
	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()

	_, err := s.users.UpdateUser(userId, func(user *store.UserState) {
		costume, ok := user.Costumes[req.UserCostumeUuid]
		if !ok {
			log.Printf("[CostumeService] LimitBreak: costume uuid=%s not found", req.UserCostumeUuid)
			return
		}

		if costume.LimitBreakCount >= config.CostumeLimitBreakAvailableCount {
			log.Printf("[CostumeService] LimitBreak: already at max limit break %d", costume.LimitBreakCount)
			return
		}

		cm, ok := catalog.Costumes[costume.CostumeId]
		if !ok {
			log.Printf("[CostumeService] LimitBreak: costume master id=%d not found", costume.CostumeId)
			return
		}

		totalMaterialCount := int32(0)
		for materialId, count := range req.Materials {
			cur := user.Materials[materialId]
			if cur < count {
				log.Printf("[CostumeService] LimitBreak: insufficient material id=%d have=%d need=%d", materialId, cur, count)
				count = cur
			}
			user.Materials[materialId] = cur - count
			totalMaterialCount += count
		}

		if costFunc, ok := catalog.LimitBreakCostByRarity[cm.RarityType]; ok && totalMaterialCount > 0 {
			goldCost := costFunc.Evaluate(totalMaterialCount)
			user.ConsumableItems[config.ConsumableItemIdForGold] -= goldCost
			log.Printf("[CostumeService] LimitBreak: gold cost=%d", goldCost)
		}

		costume.LimitBreakCount++
		costume.LatestVersion = nowMillis
		user.Costumes[req.UserCostumeUuid] = costume
		if totalMaterialCount > 0 {
			ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{ConditionType: 11, TargetId: costume.CostumeId, Delta: 1}, nowMillis)
		}
		log.Printf("[CostumeService] LimitBreak: costumeId=%d limitBreak -> %d", costume.CostumeId, costume.LimitBreakCount)
	})
	if err != nil {
		return nil, fmt.Errorf("costume limit break: %w", err)
	}

	return &pb.LimitBreakResponse{}, nil
}

func (s *CostumeServiceServer) UnlockLotteryEffectSlot(ctx context.Context, req *pb.UnlockLotteryEffectSlotRequest) (*pb.UnlockLotteryEffectSlotResponse, error) {
	log.Printf("[CostumeService] UnlockLotteryEffectSlot: uuid=%s slot=%d", req.UserCostumeUuid, req.SlotNumber)

	cat := s.holder.Get()
	catalog := cat.Costume
	config := cat.GameConfig
	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()

	_, err := s.users.UpdateUser(userId, func(user *store.UserState) {
		costume, ok := user.Costumes[req.UserCostumeUuid]
		if !ok {
			log.Printf("[CostumeService] UnlockLotteryEffectSlot: costume uuid=%s not found", req.UserCostumeUuid)
			return
		}

		effectRow, ok := catalog.LotteryEffects[[2]int32{costume.CostumeId, req.SlotNumber}]
		if !ok {
			log.Printf("[CostumeService] UnlockLotteryEffectSlot: no lottery effect for costumeId=%d slot=%d", costume.CostumeId, req.SlotNumber)
			return
		}

		user.ConsumableItems[config.ConsumableItemIdForGold] -= config.CostumeLotteryEffectUnlockSlotConsumeGold

		mats := catalog.LotteryEffectMats[effectRow.CostumeLotteryEffectUnlockMaterialGroupId]
		for _, mat := range mats {
			cur := user.Materials[mat.MaterialId]
			cost := mat.Count
			if cur < cost {
				log.Printf("[CostumeService] UnlockLotteryEffectSlot: insufficient material id=%d have=%d need=%d", mat.MaterialId, cur, cost)
				cost = cur
			}
			user.Materials[mat.MaterialId] = cur - cost
		}

		key := store.CostumeLotteryEffectKey{
			UserCostumeUuid: req.UserCostumeUuid,
			SlotNumber:      req.SlotNumber,
		}
		user.CostumeLotteryEffects[key] = store.CostumeLotteryEffectState{
			UserCostumeUuid: req.UserCostumeUuid,
			SlotNumber:      req.SlotNumber,
			OddsNumber:      0,
			LatestVersion:   nowMillis,
		}

		costume.CostumeLotteryEffectUnlockedSlotCount++
		costume.LatestVersion = nowMillis
		user.Costumes[req.UserCostumeUuid] = costume
		log.Printf("[CostumeService] UnlockLotteryEffectSlot: costumeId=%d slot=%d unlocked slotCount=%d", costume.CostumeId, req.SlotNumber, costume.CostumeLotteryEffectUnlockedSlotCount)
		// Karma mission (type 72): count each slot unlock as one Karma action.
		ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{ConditionType: missionConditionKarmaPanelType1, Delta: 1}, nowMillis)
	})
	if err != nil {
		return nil, fmt.Errorf("costume unlock lottery effect slot: %w", err)
	}

	return &pb.UnlockLotteryEffectSlotResponse{}, nil
}

func (s *CostumeServiceServer) DrawLotteryEffect(ctx context.Context, req *pb.DrawLotteryEffectRequest) (*pb.DrawLotteryEffectResponse, error) {
	log.Printf("[CostumeService] DrawLotteryEffect: uuid=%s slot=%d", req.UserCostumeUuid, req.SlotNumber)

	cat := s.holder.Get()
	catalog := cat.Costume
	config := cat.GameConfig
	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()

	_, err := s.users.UpdateUser(userId, func(user *store.UserState) {
		costume, ok := user.Costumes[req.UserCostumeUuid]
		if !ok {
			log.Printf("[CostumeService] DrawLotteryEffect: costume uuid=%s not found", req.UserCostumeUuid)
			return
		}

		effectRow, ok := catalog.LotteryEffects[[2]int32{costume.CostumeId, req.SlotNumber}]
		if !ok {
			log.Printf("[CostumeService] DrawLotteryEffect: no lottery effect for costumeId=%d slot=%d", costume.CostumeId, req.SlotNumber)
			return
		}

		oddsPool := catalog.LotteryEffectOdds[effectRow.CostumeLotteryEffectOddsGroupId]
		if len(oddsPool) == 0 {
			log.Printf("[CostumeService] DrawLotteryEffect: empty odds pool for groupId=%d", effectRow.CostumeLotteryEffectOddsGroupId)
			return
		}

		user.ConsumableItems[config.ConsumableItemIdForGold] -= config.CostumeLotteryEffectDrawSlotConsumeGold

		mats := catalog.LotteryEffectMats[effectRow.CostumeLotteryEffectDrawMaterialGroupId]
		for _, mat := range mats {
			cur := user.Materials[mat.MaterialId]
			cost := mat.Count
			if cur < cost {
				log.Printf("[CostumeService] DrawLotteryEffect: insufficient material id=%d have=%d need=%d", mat.MaterialId, cur, cost)
				cost = cur
			}
			user.Materials[mat.MaterialId] = cur - cost
		}

		totalWeight := int32(0)
		for _, row := range oddsPool {
			totalWeight += row.Weight
		}
		roll := rand.Int31n(totalWeight)
		var picked masterdata.EntityMCostumeLotteryEffectOddsGroup
		for _, row := range oddsPool {
			roll -= row.Weight
			if roll < 0 {
				picked = row
				break
			}
		}

		key := store.CostumeLotteryEffectKey{
			UserCostumeUuid: req.UserCostumeUuid,
			SlotNumber:      req.SlotNumber,
		}
		existing := user.CostumeLotteryEffects[key]
		firstDraw := existing.OddsNumber == 0
		if firstDraw {
			existing.UserCostumeUuid = req.UserCostumeUuid
			existing.SlotNumber = req.SlotNumber
			existing.OddsNumber = picked.OddsNumber
			existing.LatestVersion = nowMillis
			user.CostumeLotteryEffects[key] = existing
			// The first draw is accepted immediately. Materialize it now so the
			// deck screen receives the stat/ability rows in the same response;
			// previously this happened only after confirming a reroll.
			rebuildCostumeLotteryEffects(catalog, user, req.UserCostumeUuid, nowMillis)
		} else {
			user.CostumeLotteryEffectPending[req.UserCostumeUuid] = store.CostumeLotteryEffectPendingState{
				UserCostumeUuid: req.UserCostumeUuid,
				SlotNumber:      req.SlotNumber,
				OddsNumber:      picked.OddsNumber,
				LatestVersion:   nowMillis,
			}
		}

		log.Printf("[CostumeService] DrawLotteryEffect: costumeId=%d slot=%d drew oddsNumber=%d type=%d targetId=%d firstDraw=%v",
			costume.CostumeId, req.SlotNumber, picked.OddsNumber, picked.CostumeLotteryEffectType, picked.CostumeLotteryEffectTargetId, firstDraw)
		// Karma Flux mission (type 73): every draw counts, regardless of outcome.
		ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{ConditionType: missionConditionKarmaPanelType2, Delta: 1}, nowMillis)
	})
	if err != nil {
		return nil, fmt.Errorf("costume draw lottery effect: %w", err)
	}

	return &pb.DrawLotteryEffectResponse{}, nil
}

func (s *CostumeServiceServer) ConfirmLotteryEffect(ctx context.Context, req *pb.ConfirmLotteryEffectRequest) (*pb.ConfirmLotteryEffectResponse, error) {
	log.Printf("[CostumeService] ConfirmLotteryEffect: uuid=%s accept=%v", req.UserCostumeUuid, req.IsAccept)

	catalog := s.holder.Get().Costume
	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()

	_, err := s.users.UpdateUser(userId, func(user *store.UserState) {
		pending, ok := user.CostumeLotteryEffectPending[req.UserCostumeUuid]
		if !ok {
			log.Printf("[CostumeService] ConfirmLotteryEffect: no pending for uuid=%s", req.UserCostumeUuid)
			return
		}

		if req.IsAccept {
			key := store.CostumeLotteryEffectKey{
				UserCostumeUuid: pending.UserCostumeUuid,
				SlotNumber:      pending.SlotNumber,
			}
			effect := user.CostumeLotteryEffects[key]
			effect.UserCostumeUuid = pending.UserCostumeUuid
			effect.SlotNumber = pending.SlotNumber
			effect.OddsNumber = pending.OddsNumber
			effect.LatestVersion = nowMillis
			user.CostumeLotteryEffects[key] = effect
			rebuildCostumeLotteryEffects(catalog, user, req.UserCostumeUuid, nowMillis)
			log.Printf("[CostumeService] ConfirmLotteryEffect: accepted oddsNumber=%d for slot=%d", pending.OddsNumber, pending.SlotNumber)
		} else {
			log.Printf("[CostumeService] ConfirmLotteryEffect: rejected oddsNumber=%d for slot=%d", pending.OddsNumber, pending.SlotNumber)
		}

		delete(user.CostumeLotteryEffectPending, req.UserCostumeUuid)
	})
	if err != nil {
		return nil, fmt.Errorf("costume confirm lottery effect: %w", err)
	}

	return &pb.ConfirmLotteryEffectResponse{}, nil
}

// rebuildCostumeLotteryEffects materializes accepted lottery rolls into the
// user tables consumed by battle. Rebuilding the costume avoids keeping a
// bonus from a roll that the player later replaces.
func rebuildCostumeLotteryEffects(catalog *masterdata.CostumeCatalog, user *store.UserState, costumeUuid string, nowMillis int64) {
	// Older saves predate the two derived lottery-result maps. EnsureMaps makes
	// rebuilding them safe even when a player has never had those tables saved.
	user.EnsureMaps()
	costume, ok := user.Costumes[costumeUuid]
	if !ok {
		return
	}
	for k := range user.CostumeLotteryEffectAbilities {
		if k.UserCostumeUuid == costumeUuid {
			delete(user.CostumeLotteryEffectAbilities, k)
		}
	}
	for k := range user.CostumeLotteryEffectStatusUps {
		if k.UserCostumeUuid == costumeUuid {
			delete(user.CostumeLotteryEffectStatusUps, k)
		}
	}
	for key, effect := range user.CostumeLotteryEffects {
		if key.UserCostumeUuid != costumeUuid || effect.OddsNumber == 0 {
			continue
		}
		definition, ok := catalog.LotteryEffects[[2]int32{costume.CostumeId, key.SlotNumber}]
		if !ok {
			continue
		}
		// A single displayed roll can grant more than one effect. The master
		// table represents that by repeating OddsNumber for each target (for
		// example, Critical Rate and Critical Damage together). Apply every
		// matching row; taking only the first silently lost later bonuses.
		for _, roll := range catalog.LotteryEffectOdds[definition.CostumeLotteryEffectOddsGroupId] {
			if roll.OddsNumber != effect.OddsNumber {
				continue
			}
			switch model.CostumeLotteryEffectType(roll.CostumeLotteryEffectType) {
			case model.CostumeLotteryEffectTypeAbility:
				target, ok := catalog.LotteryEffectAbilities[roll.CostumeLotteryEffectTargetId]
				if !ok {
					continue
				}
				user.CostumeLotteryEffectAbilities[key] = store.UserCostumeLotteryEffectAbilityState{UserCostumeUuid: costumeUuid, SlotNumber: key.SlotNumber, AbilityId: target.AbilityId, AbilityLevel: target.AbilityLevel, LatestVersion: nowMillis}
			case model.CostumeLotteryEffectTypeStatusUp:
				targets, ok := catalog.LotteryEffectStatusUps[roll.CostumeLotteryEffectTargetId]
				if !ok {
					continue
				}
				for _, target := range targets {
					statusKey := store.CostumeLotteryEffectStatusUpKey{UserCostumeUuid: costumeUuid, StatusCalculationType: target.StatusCalculationType}
					state := user.CostumeLotteryEffectStatusUps[statusKey]
					state.UserCostumeUuid = costumeUuid
					state.StatusCalculationType = target.StatusCalculationType
					state.LatestVersion = nowMillis
					switch model.StatusKindType(target.StatusKindType) {
					case model.StatusKindTypeHp:
						state.Hp += target.EffectValue
					case model.StatusKindTypeAttack:
						state.Attack += target.EffectValue
					case model.StatusKindTypeVitality:
						state.Vitality += target.EffectValue
					case model.StatusKindTypeAgility:
						state.Agility += target.EffectValue
					case model.StatusKindTypeCriticalRatio:
						state.CriticalRatio += target.EffectValue
					case model.StatusKindTypeCriticalAttack:
						state.CriticalAttack += target.EffectValue
					}
					user.CostumeLotteryEffectStatusUps[statusKey] = state
				}
			}
		}
	}
}

func rebuildAllCostumeLotteryEffects(catalog *masterdata.CostumeCatalog, user *store.UserState) {
	nowMillis := gametime.NowMillis()
	for costumeUuid := range user.Costumes {
		rebuildCostumeLotteryEffects(catalog, user, costumeUuid, nowMillis)
	}
}
