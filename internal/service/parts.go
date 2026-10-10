package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/campaign"
	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/runtime"
	"lunar-tear/server/internal/store"
	"lunar-tear/server/internal/userdata"
)

const partsMaxLevel = int32(15)

type PartsServiceServer struct {
	pb.UnimplementedPartsServiceServer
	users    store.UserRepository
	sessions store.SessionRepository
	holder   *runtime.Holder
}

func NewPartsServiceServer(users store.UserRepository, sessions store.SessionRepository, holder *runtime.Holder) *PartsServiceServer {
	return &PartsServiceServer{users: users, sessions: sessions, holder: holder}
}

func (s *PartsServiceServer) Protect(ctx context.Context, req *pb.PartsProtectRequest) (*pb.PartsProtectResponse, error) {
	log.Printf("[PartsService] Protect: uuids=%v", req.UserPartsUuid)

	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()

	s.users.UpdateUser(userId, func(user *store.UserState) {
		for _, uuid := range req.UserPartsUuid {
			part, ok := user.Parts[uuid]
			if !ok {
				log.Printf("[PartsService] Protect: part uuid=%s not found", uuid)
				continue
			}
			part.IsProtected = true
			part.LatestVersion = nowMillis
			user.Parts[uuid] = part
		}
	})

	return &pb.PartsProtectResponse{}, nil
}

func (s *PartsServiceServer) Unprotect(ctx context.Context, req *pb.PartsUnprotectRequest) (*pb.PartsUnprotectResponse, error) {
	log.Printf("[PartsService] Unprotect: uuids=%v", req.UserPartsUuid)

	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()

	s.users.UpdateUser(userId, func(user *store.UserState) {
		for _, uuid := range req.UserPartsUuid {
			part, ok := user.Parts[uuid]
			if !ok {
				log.Printf("[PartsService] Unprotect: part uuid=%s not found", uuid)
				continue
			}
			part.IsProtected = false
			part.LatestVersion = nowMillis
			user.Parts[uuid] = part
		}
	})

	return &pb.PartsUnprotectResponse{}, nil
}

func (s *PartsServiceServer) Sell(ctx context.Context, req *pb.PartsSellRequest) (*pb.PartsSellResponse, error) {
	log.Printf("[PartsService] Sell: %d part(s)", len(req.UserPartsUuid))

	cat := s.holder.Get()
	catalog := cat.Parts
	config := cat.GameConfig
	userId := CurrentUserId(ctx, s.users, s.sessions)

	before, err := s.users.LoadUser(userId)
	if err != nil {
		return nil, fmt.Errorf("parts sell: load user: %w", err)
	}

	// Uuids the client sent but the server no longer has: the client missed the
	// diff of an earlier successful sell and kept ghost inventory rows. They are
	// answered with explicit delete keys below so its cache converges.
	var staleUuids []string
	after, err := s.users.UpdateUser(userId, func(user *store.UserState) {
		staleUuids = staleUuids[:0]
		totalGold := int32(0)
		for _, uuid := range req.UserPartsUuid {
			part, ok := user.Parts[uuid]
			if !ok {
				log.Printf("[PartsService] Sell: uuid=%s not found, sending delete key to client", uuid)
				staleUuids = append(staleUuids, uuid)
				continue
			}
			if part.IsProtected {
				log.Printf("[PartsService] Sell: uuid=%s is protected, skipping", uuid)
				continue
			}

			partDef, ok := catalog.PartsById[part.PartsId]
			if !ok {
				log.Printf("[PartsService] Sell: partsId=%d not in catalog, skipping", part.PartsId)
				continue
			}

			sellFunc, ok := catalog.SellPriceByRarity[partDef.RarityType]
			if !ok {
				log.Printf("[PartsService] Sell: no sell price func for rarity=%d, skipping", partDef.RarityType)
				continue
			}

			gold := sellFunc.Evaluate(part.Level)
			totalGold += gold
			delete(user.Parts, uuid)
			for k := range user.PartsStatusSubs {
				if k.UserPartsUuid == uuid {
					delete(user.PartsStatusSubs, k)
				}
			}
			log.Printf("[PartsService] Sell: uuid=%s partsId=%d level=%d -> %d gold", uuid, part.PartsId, part.Level, gold)
		}

		if totalGold > 0 {
			user.ConsumableItems[config.ConsumableItemIdForGold] += totalGold
			log.Printf("[PartsService] Sell: total gold +%d", totalGold)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("parts sell: %w", err)
	}

	resp := &pb.PartsSellResponse{}
	if len(staleUuids) > 0 {
		resp.DiffUserData = partsStaleDeleteDiff(&before, &after, staleUuids)
	}
	return resp, nil
}

// partsStaleDeleteDiff builds the complete user-data diff for a sell request
// that referenced stale part uuids. Pre-filling DiffUserData makes the diff
// interceptor skip its own computation, so the genuine changes of this request
// (sold rows, gold) are computed here and merged with forced delete keys for
// the stale uuids the client still caches.
func partsStaleDeleteDiff(before, after *store.UserState, staleUuids []string) map[string]*pb.DiffData {
	diff := userdata.ComputeDelta(before, after, userdata.ChangedTables(before, after))
	if diff == nil {
		diff = map[string]*pb.DiffData{}
	}
	partsKeys := make([]map[string]any, 0, len(staleUuids))
	subKeys := make([]map[string]any, 0, len(staleUuids)*4)
	for _, uuid := range staleUuids {
		partsKeys = append(partsKeys, map[string]any{"userId": after.UserId, "userPartsUuid": uuid})
		// Ghost parts may carry cached sub-status rows (indices 1..4); deleting
		// rows the client does not have is a no-op.
		for i := 1; i <= 4; i++ {
			subKeys = append(subKeys, map[string]any{"userId": after.UserId, "userPartsUuid": uuid, "statusIndex": i})
		}
	}
	mergeDeleteKeys(diff, "IUserParts", partsKeys)
	mergeDeleteKeys(diff, "IUserPartsStatusSub", subKeys)
	return diff
}

func mergeDeleteKeys(diff map[string]*pb.DiffData, table string, keys []map[string]any) {
	entry := diff[table]
	if entry == nil {
		entry = &pb.DiffData{UpdateRecordsJson: "[]", DeleteKeysJson: "[]"}
		diff[table] = entry
	}
	var records []map[string]any
	if entry.DeleteKeysJson != "" && entry.DeleteKeysJson != "[]" {
		if err := json.Unmarshal([]byte(entry.DeleteKeysJson), &records); err != nil {
			log.Printf("[PartsService] mergeDeleteKeys: bad existing keys for %s: %v", table, err)
			records = nil
		}
	}
	records = append(records, keys...)
	if b, err := json.Marshal(records); err == nil {
		entry.DeleteKeysJson = string(b)
	}
}

func (s *PartsServiceServer) Enhance(ctx context.Context, req *pb.PartsEnhanceRequest) (*pb.PartsEnhanceResponse, error) {
	log.Printf("[PartsService] Enhance: uuid=%s", req.UserPartsUuid)

	cat := s.holder.Get()
	catalog := cat.Parts
	config := cat.GameConfig
	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()

	isSuccess := false

	_, err := s.users.UpdateUser(userId, func(user *store.UserState) {
		part, ok := user.Parts[req.UserPartsUuid]
		if !ok {
			log.Printf("[PartsService] Enhance: part uuid=%s not found", req.UserPartsUuid)
			return
		}

		if part.Level >= partsMaxLevel {
			log.Printf("[PartsService] Enhance: part uuid=%s already at max level %d", req.UserPartsUuid, part.Level)
			return
		}

		partDef, ok := catalog.PartsById[part.PartsId]
		if !ok {
			log.Printf("[PartsService] Enhance: part master id=%d not found", part.PartsId)
			return
		}

		rarity, ok := catalog.RarityByRarityType[partDef.RarityType]
		if !ok {
			log.Printf("[PartsService] Enhance: rarity type=%d not found", partDef.RarityType)
			return
		}

		goldCost := int32(0)
		if prices, ok := catalog.PriceByGroupAndLevel[rarity.PartsLevelUpPriceGroupId]; ok {
			goldCost = prices[part.Level]
		}

		currentGold := user.ConsumableItems[config.ConsumableItemIdForGold]
		if currentGold < goldCost {
			log.Printf("[PartsService] Enhance: insufficient gold have=%d need=%d", currentGold, goldCost)
			return
		}

		user.ConsumableItems[config.ConsumableItemIdForGold] -= goldCost

		successRate := int32(1000)
		if rates, ok := catalog.RateByGroupAndLevel[rarity.PartsLevelUpRateGroupId]; ok {
			if r, ok := rates[part.Level]; ok {
				successRate = r
			}
		}
		baseRate := successRate
		successRate = cat.Campaign.PartsRateBonus(campaign.PartsTarget{
			PartsId:      part.PartsId,
			PartsGroupId: partDef.PartsGroupId,
			Rarity:       model.RarityType(partDef.RarityType),
		}, campaign.Filter{NowMillis: nowMillis, UserStatus: campaign.TargetUserStatusAll}).Apply(baseRate)

		if rand.Intn(1000) < int(successRate) {
			part.Level++
			isSuccess = true
			log.Printf("[PartsService] Enhance: SUCCESS partsId=%d level %d -> %d (rate=%d‰ base=%d‰, cost=%d gold)",
				part.PartsId, part.Level-1, part.Level, successRate, baseRate, goldCost)

			grantPartsSubStatuses(catalog, user, req.UserPartsUuid, part, partDef, nowMillis)
		} else {
			log.Printf("[PartsService] Enhance: FAIL partsId=%d stays level %d (rate=%d‰ base=%d‰, cost=%d gold)",
				part.PartsId, part.Level, successRate, baseRate, goldCost)
		}

		part.LatestVersion = nowMillis
		user.Parts[req.UserPartsUuid] = part
		if isSuccess {
			ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{ConditionType: 13, TargetId: part.PartsId, Delta: 1}, nowMillis)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("parts enhance: %w", err)
	}

	return &pb.PartsEnhanceResponse{
		IsSuccess: isSuccess,
	}, nil
}

func grantPartsSubStatuses(catalog *masterdata.PartsCatalog, user *store.UserState, uuid string, part store.PartsState, partDef masterdata.EntityMParts, nowMillis int64) {
	unlockLevels := catalog.SubStatusUnlockLvls[partDef.RarityType]
	pool := catalog.SubStatusPool[partDef.PartsStatusSubLotteryGroupId]
	if len(pool) == 0 {
		return
	}

	// First, handle new slot unlocks
	for slotIdx, lvl := range unlockLevels {
		if part.Level != lvl {
			continue
		}
		statusIndex := int32(slotIdx + 1)
		key := store.PartsStatusSubKey{UserPartsUuid: uuid, StatusIndex: statusIndex}
		if _, exists := user.PartsStatusSubs[key]; exists {
			continue
		}

		pick := pool[rand.Intn(len(pool))]
		def, ok := catalog.PartsStatusMainById[pick]
		if !ok {
			continue
		}

		statusValue := def.StatusChangeInitialValue
		if f, ok := catalog.FuncResolver.Resolve(def.StatusNumericalFunctionId); ok {
			statusValue = f.Evaluate(part.Level)
		}

		user.PartsStatusSubs[key] = store.PartsStatusSubState{
			UserPartsUuid:           uuid,
			StatusIndex:             statusIndex,
			PartsStatusSubLotteryId: pick,
			Level:                   part.Level,
			StatusKindType:          def.StatusKindType,
			StatusCalculationType:   def.StatusCalculationType,
			StatusChangeValue:       statusValue,
			LatestVersion:           nowMillis,
		}
		log.Printf("[PartsService] Enhance: granted sub-status slot=%d lotteryId=%d kind=%d calc=%d val=%d",
			statusIndex, pick, def.StatusKindType, def.StatusCalculationType, statusValue)
	}

	// Then, handle random enhancement of existing stats
	enhanceExistingStats(catalog, user, uuid, part, partDef, nowMillis)
}

// enhanceExistingStats randomly enhances existing stats based on probability table
func enhanceExistingStats(catalog *masterdata.PartsCatalog, user *store.UserState, uuid string, part store.PartsState, partDef masterdata.EntityMParts, nowMillis int64) {
	if len(catalog.EnhanceProbability) == 0 {
		return
	}

	// Select enhancement pattern based on probabilities
	totalProb := int32(0)
	for _, prob := range catalog.EnhanceProbability {
		totalProb += prob.ProbabilityPermil
	}
	roll := rand.Int31n(totalProb)
	cumulative := int32(0)
	var selectedPattern *masterdata.EntityMPartsEnhanceProbability
	for _, prob := range catalog.EnhanceProbability {
		cumulative += prob.ProbabilityPermil
		if roll < cumulative {
			selectedPattern = &prob
			break
		}
	}
	if selectedPattern == nil {
		return
	}

	log.Printf("[PartsService] Enhance: selected pattern main=%d subCount=%d (prob=%d‰)",
		selectedPattern.EnhanceMain, selectedPattern.EnhanceSubCount, selectedPattern.ProbabilityPermil)

	// Collect existing sub-stats
	var existingSubStats []store.PartsStatusSubState
	for key, subStat := range user.PartsStatusSubs {
		if key.UserPartsUuid == uuid {
			existingSubStats = append(existingSubStats, subStat)
		}
	}

	// Enhance main stat if selected
	if selectedPattern.EnhanceMain == 1 {
		mainStatId := catalog.DefaultPartsStatusMainByLotteryGroup[partDef.PartsStatusMainLotteryGroupId]
		if def, ok := catalog.PartsStatusMainById[mainStatId]; ok {
			if f, ok := catalog.FuncResolver.Resolve(def.StatusNumericalFunctionId); ok {
				newValue := f.Evaluate(part.Level)
				part.PartsStatusMainValue = newValue
				log.Printf("[PartsService] Enhance: main stat enhanced to value=%d", newValue)
			}
		}
	}

	// Randomly select and enhance sub-stats
	if selectedPattern.EnhanceSubCount > 0 && len(existingSubStats) > 0 {
		// Shuffle existing sub-stats
		rand.Shuffle(len(existingSubStats), func(i, j int) {
			existingSubStats[i], existingSubStats[j] = existingSubStats[j], existingSubStats[i]
		})

		// Enhance up to EnhanceSubCount sub-stats
		countToEnhance := int(selectedPattern.EnhanceSubCount)
		if countToEnhance > len(existingSubStats) {
			countToEnhance = len(existingSubStats)
		}

		for i := 0; i < countToEnhance; i++ {
			subStat := existingSubStats[i]
			def, ok := catalog.PartsStatusMainById[subStat.PartsStatusSubLotteryId]
			if !ok {
				continue
			}

			oldValue := subStat.StatusChangeValue
			var newValue int32
			if f, ok := catalog.FuncResolver.Resolve(def.StatusNumericalFunctionId); ok {
				newValue = f.Evaluate(part.Level)
			} else {
				newValue = def.StatusChangeInitialValue
			}

			// Update the sub-stat
			key := store.PartsStatusSubKey{UserPartsUuid: uuid, StatusIndex: subStat.StatusIndex}
			subStat.StatusChangeValue = newValue
			subStat.Level = part.Level
			subStat.LatestVersion = nowMillis
			user.PartsStatusSubs[key] = subStat

			log.Printf("[PartsService] Enhance: sub-stat slot=%d enhanced %d->%d",
				subStat.StatusIndex, oldValue, newValue)
		}
	}
}

func (s *PartsServiceServer) ReplacePreset(ctx context.Context, req *pb.PartsReplacePresetRequest) (*pb.PartsReplacePresetResponse, error) {
	log.Printf("[PartsService] ReplacePreset: preset=%d uuids=[%s, %s, %s]",
		req.UserPartsPresetNumber, req.UserPartsUuid01, req.UserPartsUuid02, req.UserPartsUuid03)

	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()

	_, err := s.users.UpdateUser(userId, func(user *store.UserState) {
		preset := user.PartsPresets[req.UserPartsPresetNumber]
		preset.UserPartsPresetNumber = req.UserPartsPresetNumber
		preset.UserPartsUuid01 = req.UserPartsUuid01
		preset.UserPartsUuid02 = req.UserPartsUuid02
		preset.UserPartsUuid03 = req.UserPartsUuid03
		preset.LatestVersion = nowMillis
		user.PartsPresets[req.UserPartsPresetNumber] = preset
	})
	if err != nil {
		return nil, fmt.Errorf("parts replace preset: %w", err)
	}

	return &pb.PartsReplacePresetResponse{}, nil
}

func (s *PartsServiceServer) UpdatePresetName(ctx context.Context, req *pb.PartsUpdatePresetNameRequest) (*pb.PartsUpdatePresetNameResponse, error) {
	log.Printf("[PartsService] UpdatePresetName: preset=%d name=%q", req.UserPartsPresetNumber, req.Name)

	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()

	_, err := s.users.UpdateUser(userId, func(user *store.UserState) {
		preset := user.PartsPresets[req.UserPartsPresetNumber]
		preset.UserPartsPresetNumber = req.UserPartsPresetNumber
		preset.Name = req.Name
		preset.LatestVersion = nowMillis
		user.PartsPresets[req.UserPartsPresetNumber] = preset
	})
	if err != nil {
		return nil, fmt.Errorf("parts update preset name: %w", err)
	}

	return &pb.PartsUpdatePresetNameResponse{}, nil
}

func (s *PartsServiceServer) UpdatePresetTagNumber(ctx context.Context, req *pb.PartsUpdatePresetTagNumberRequest) (*pb.PartsUpdatePresetTagNumberResponse, error) {
	log.Printf("[PartsService] UpdatePresetTagNumber: preset=%d tag=%d", req.UserPartsPresetNumber, req.UserPartsPresetTagNumber)

	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()

	_, err := s.users.UpdateUser(userId, func(user *store.UserState) {
		preset := user.PartsPresets[req.UserPartsPresetNumber]
		preset.UserPartsPresetNumber = req.UserPartsPresetNumber
		preset.UserPartsPresetTagNumber = req.UserPartsPresetTagNumber
		preset.LatestVersion = nowMillis
		user.PartsPresets[req.UserPartsPresetNumber] = preset
	})
	if err != nil {
		return nil, fmt.Errorf("parts update preset tag number: %w", err)
	}

	return &pb.PartsUpdatePresetTagNumberResponse{}, nil
}

func (s *PartsServiceServer) UpdatePresetTagName(ctx context.Context, req *pb.PartsUpdatePresetTagNameRequest) (*pb.PartsUpdatePresetTagNameResponse, error) {
	log.Printf("[PartsService] UpdatePresetTagName: tag=%d name=%q", req.UserPartsPresetTagNumber, req.Name)

	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()

	_, err := s.users.UpdateUser(userId, func(user *store.UserState) {
		tag := user.PartsPresetTags[req.UserPartsPresetTagNumber]
		tag.UserPartsPresetTagNumber = req.UserPartsPresetTagNumber
		tag.Name = req.Name
		tag.LatestVersion = nowMillis
		user.PartsPresetTags[req.UserPartsPresetTagNumber] = tag
	})
	if err != nil {
		return nil, fmt.Errorf("parts update preset tag name: %w", err)
	}

	return &pb.PartsUpdatePresetTagNameResponse{}, nil
}

func (s *PartsServiceServer) CopyPreset(ctx context.Context, req *pb.PartsCopyPresetRequest) (*pb.PartsCopyPresetResponse, error) {
	log.Printf("[PartsService] CopyPreset: from=%d to=%d", req.FromUserPartsPresetNumber, req.ToUserPartsPresetNumber)

	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()

	_, err := s.users.UpdateUser(userId, func(user *store.UserState) {
		from, ok := user.PartsPresets[req.FromUserPartsPresetNumber]
		if !ok {
			log.Printf("[PartsService] CopyPreset: source preset=%d not found, skipping", req.FromUserPartsPresetNumber)
			return
		}
		to := from
		to.UserPartsPresetNumber = req.ToUserPartsPresetNumber
		to.LatestVersion = nowMillis
		user.PartsPresets[req.ToUserPartsPresetNumber] = to
	})
	if err != nil {
		return nil, fmt.Errorf("parts copy preset: %w", err)
	}

	return &pb.PartsCopyPresetResponse{}, nil
}

func (s *PartsServiceServer) RemovePreset(ctx context.Context, req *pb.PartsRemovePresetRequest) (*pb.PartsRemovePresetResponse, error) {
	log.Printf("[PartsService] RemovePreset: preset=%d", req.UserPartsPresetNumber)

	userId := CurrentUserId(ctx, s.users, s.sessions)

	_, err := s.users.UpdateUser(userId, func(user *store.UserState) {
		delete(user.PartsPresets, req.UserPartsPresetNumber)
	})
	if err != nil {
		return nil, fmt.Errorf("parts remove preset: %w", err)
	}

	return &pb.PartsRemovePresetResponse{}, nil
}
