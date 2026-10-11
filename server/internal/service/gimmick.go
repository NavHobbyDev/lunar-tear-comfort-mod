package service

import (
	"context"
	"log"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/runtime"
	"lunar-tear/server/internal/store"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

type GimmickServiceServer struct {
	pb.UnimplementedGimmickServiceServer
	users    store.UserRepository
	sessions store.SessionRepository
	holder   *runtime.Holder
}

func NewGimmickServiceServer(users store.UserRepository, sessions store.SessionRepository, holder *runtime.Holder) *GimmickServiceServer {
	return &GimmickServiceServer{users: users, sessions: sessions, holder: holder}
}

func (s *GimmickServiceServer) UpdateSequence(ctx context.Context, req *pb.UpdateSequenceRequest) (*pb.UpdateSequenceResponse, error) {
	// No per-call logging: the client fires this once per gimmick sequence in a
	// burst on map load, and the per-call log I/O dominated the load time.
	userId := CurrentUserId(ctx, s.users, s.sessions)
	// Targeted single-row upsert. The client fires this once per sequence in a
	// burst on map load; the previous full UpdateUser cycle (LoadUser + clone +
	// diff) per call made map loads take many seconds. UpdateSequence is also
	// listed in the diff interceptor's skip set so it no longer triggers the
	// surrounding before/after full-state loads either.
	if err := s.users.EnsureGimmickSequence(userId, req.GimmickSequenceScheduleId, req.GimmickSequenceId); err != nil {
		return nil, err
	}
	return &pb.UpdateSequenceResponse{}, nil
}

func (s *GimmickServiceServer) UpdateGimmickProgress(ctx context.Context, req *pb.UpdateGimmickProgressRequest) (*pb.UpdateGimmickProgressResponse, error) {
	log.Printf("[GimmickService] UpdateGimmickProgress: scheduleId=%d sequenceId=%d gimmickId=%d ornamentIndex=%d progressValueBit=%d flowType=%d",
		req.GimmickSequenceScheduleId, req.GimmickSequenceId, req.GimmickId, req.GimmickOrnamentIndex, req.ProgressValueBit, req.FlowType)
	userId := CurrentUserId(ctx, s.users, s.sessions)
	cat := s.holder.Get()

	var ornamentRewards []*pb.GimmickReward
	var sequenceCleared bool
	s.users.UpdateUser(userId, func(user *store.UserState) {
		nowMillis := gametime.NowMillis()
		progressKey := store.GimmickKey{
			GimmickSequenceScheduleId: req.GimmickSequenceScheduleId,
			GimmickSequenceId:         req.GimmickSequenceId,
			GimmickId:                 req.GimmickId,
		}
		progress := user.Gimmick.Progress[progressKey]
		progress.Key = progressKey
		progress.StartDatetime = nowMillis

		ornamentKey := store.GimmickOrnamentKey{
			GimmickSequenceScheduleId: req.GimmickSequenceScheduleId,
			GimmickSequenceId:         req.GimmickSequenceId,
			GimmickId:                 req.GimmickId,
			GimmickOrnamentIndex:      req.GimmickOrnamentIndex,
		}
		ornament := user.Gimmick.OrnamentProgress[ornamentKey]
		ornament.Key = ornamentKey
		ornament.ProgressValueBit = req.ProgressValueBit
		ornament.BaseDatetime = nowMillis
		user.Gimmick.OrnamentProgress[ornamentKey] = ornament

		// Per-type branches:
		//   * Report (type 9, "Hidden Stories")            — mark gimmick + sequence
		//     cleared, grant SequenceRewards (ImportantItem type 3, library reads it).
		//   * MapOnlyCageTreasureHunt (type 7, "Hidden Black Birds") — same as Report
		//     but the per-tap reward also comes back from m_cage_ornament_reward via
		//     GimmickOrnamentViewId.
		//   * CageMemory (type 10, "Lost Archives")        — resolve an ImportantItem
		//     (type 4) from the gimmick's monitor texture and grant it. IsGimmickCleared
		//     stays false (matches original userdata; only ornament progress flips).
		//   * CageTreasureHunt / CageIntervalDropItem*     — stub per-tap material so
		//     the client's reward popup fires; real reward source still unmapped.
		switch cat.Gimmick.GimmickType(req.GimmickId) {
		case model.GimmickTypeReport:
			progress.IsGimmickCleared = true
			sequenceCleared = markSequenceClearedOnce(user, cat, req.GimmickSequenceScheduleId, req.GimmickSequenceId, nowMillis)

		case model.GimmickTypeMapOnlyCageTreasureHunt:
			r, ok := cat.Gimmick.HiddenBirdReward(req.GimmickId, req.GimmickOrnamentIndex)
			if !ok {
				log.Printf("[GimmickService] UpdateGimmickProgress: hidden-bird %d ornament %d has no reward mapping, skipping",
					req.GimmickId, req.GimmickOrnamentIndex)
				break
			}
			cat.QuestHandler.Granter.GrantFull(user, model.PossessionType(r.PossessionType), r.PossessionId, r.Count, nowMillis)
			ornamentRewards = append(ornamentRewards, &pb.GimmickReward{
				PossessionType: r.PossessionType,
				PossessionId:   r.PossessionId,
				Count:          r.Count,
			})
			progress.IsGimmickCleared = true
			sequenceCleared = markSequenceClearedOnce(user, cat, req.GimmickSequenceScheduleId, req.GimmickSequenceId, nowMillis)

		case model.GimmickTypeCageMemory:
			itemId, ok := cat.Gimmick.CageMemoryImportantItem(req.GimmickId)
			if !ok {
				log.Printf("[GimmickService] UpdateGimmickProgress: cage memory %d has no important-item mapping, skipping grant",
					req.GimmickId)
				break
			}
			if _, owned := user.ImportantItems[itemId]; owned {
				break
			}
			cat.QuestHandler.Granter.GrantFull(user, model.PossessionTypeImportantItem, itemId, 1, nowMillis)
			ornamentRewards = append(ornamentRewards, &pb.GimmickReward{
				PossessionType: int32(model.PossessionTypeImportantItem),
				PossessionId:   itemId,
				Count:          1,
			})

		case model.GimmickTypeCageTreasureHunt:
			// Fickle Black Birds (type 1). The reward tier is chosen from the bird's
			// difficulty (primary); if that slot is absent in this chapter, retry the
			// view-only id (fallback) so we never regress a reward that used to resolve.
			primaryId, fallbackId, ok := cat.Gimmick.FickleBirdCageOrnamentId(req.GimmickId, req.GimmickOrnamentIndex)
			if !ok {
				log.Printf("[GimmickService] UpdateGimmickProgress: fickle-bird %d ornament %d has no ornament mapping, using fallback",
					req.GimmickId, req.GimmickOrnamentIndex)
				const fallbackMaterialId = int32(100004)
				const fallbackMaterialCount = int32(1)
				cat.QuestHandler.Granter.GrantFull(user, model.PossessionTypeMaterial, fallbackMaterialId, fallbackMaterialCount, nowMillis)
				ornamentRewards = append(ornamentRewards, &pb.GimmickReward{
					PossessionType: int32(model.PossessionTypeMaterial),
					PossessionId:   fallbackMaterialId,
					Count:          fallbackMaterialCount,
				})
				break
			}
			cageOrnamentId := primaryId
			reward, ok := cat.CageOrnament.LookupReward(primaryId)
			if !ok && fallbackId != primaryId {
				cageOrnamentId = fallbackId
				reward, ok = cat.CageOrnament.LookupReward(fallbackId)
			}
			if !ok {
				log.Printf("[GimmickService] UpdateGimmickProgress: fickle-bird %d cageOrnamentId %d has no reward mapping, using fallback",
					req.GimmickId, cageOrnamentId)
				const fallbackMaterialId = int32(100004)
				const fallbackMaterialCount = int32(1)
				cat.QuestHandler.Granter.GrantFull(user, model.PossessionTypeMaterial, fallbackMaterialId, fallbackMaterialCount, nowMillis)
				ornamentRewards = append(ornamentRewards, &pb.GimmickReward{
					PossessionType: int32(model.PossessionTypeMaterial),
					PossessionId:   fallbackMaterialId,
					Count:          fallbackMaterialCount,
				})
				break
			}
			cat.QuestHandler.Granter.GrantFull(user, model.PossessionType(reward.PossessionType), reward.PossessionId, reward.Count, nowMillis)
			ornamentRewards = append(ornamentRewards, &pb.GimmickReward{
				PossessionType: reward.PossessionType,
				PossessionId:   reward.PossessionId,
				Count:          reward.Count,
			})

		case model.GimmickTypeCageIntervalDropItem,
			model.GimmickTypeMapOnlyCageIntervalDrop:
			reward := cat.LostItems.RandomRewardWithBonus(req.GimmickId, req.GimmickOrnamentIndex)
			if reward.PossessionType == 0 {
				const fallbackMaterialId = int32(100004)
				const fallbackMaterialCount = int32(1)
				cat.QuestHandler.Granter.GrantFull(user, model.PossessionTypeMaterial, fallbackMaterialId, fallbackMaterialCount, nowMillis)
				ornamentRewards = append(ornamentRewards, &pb.GimmickReward{
					PossessionType: int32(model.PossessionTypeMaterial),
					PossessionId:   fallbackMaterialId,
					Count:          fallbackMaterialCount,
				})
			} else {
				cat.QuestHandler.Granter.GrantFull(user, model.PossessionType(reward.PossessionType), reward.PossessionId, reward.Count, nowMillis)
				ornamentRewards = append(ornamentRewards, &pb.GimmickReward{
					PossessionType: reward.PossessionType,
					PossessionId:   reward.PossessionId,
					Count:          reward.Count,
				})
			}
		}
		user.Gimmick.Progress[progressKey] = progress
	})

	var clearReward []*pb.GimmickReward
	if sequenceCleared {
		for _, r := range cat.Gimmick.SequenceRewards(req.GimmickSequenceId) {
			clearReward = append(clearReward, &pb.GimmickReward{
				PossessionType: r.PossessionType,
				PossessionId:   r.PossessionId,
				Count:          r.Count,
			})
		}
	}
	return &pb.UpdateGimmickProgressResponse{
		GimmickOrnamentReward:      ornamentRewards,
		IsSequenceCleared:          sequenceCleared,
		GimmickSequenceClearReward: clearReward,
	}, nil
}

func markSequenceClearedOnce(user *store.UserState, cat *runtime.Catalogs, scheduleId, sequenceId int32, nowMillis int64) bool {
	seqKey := store.GimmickSequenceKey{
		GimmickSequenceScheduleId: scheduleId,
		GimmickSequenceId:         sequenceId,
	}
	sequence := user.Gimmick.Sequences[seqKey]
	sequence.Key = seqKey
	defer func() { user.Gimmick.Sequences[seqKey] = sequence }()

	if sequence.IsGimmickSequenceCleared {
		return false
	}
	sequence.IsGimmickSequenceCleared = true
	sequence.ClearDatetime = nowMillis
	for _, r := range cat.Gimmick.SequenceRewards(sequenceId) {
		cat.QuestHandler.Granter.GrantFull(user, model.PossessionType(r.PossessionType), r.PossessionId, r.Count, nowMillis)
	}
	ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{ConditionType: missionConditionGimmickSequenceClear, Delta: 1}, nowMillis)
	return true
}

func (s *GimmickServiceServer) InitSequenceSchedule(ctx context.Context, _ *emptypb.Empty) (*pb.InitSequenceScheduleResponse, error) {
	log.Printf("[GimmickService] InitSequenceSchedule")
	userId := CurrentUserId(ctx, s.users, s.sessions)
	now := gametime.NowMillis()
	s.users.UpdateUser(userId, func(user *store.UserState) {
		eligible := s.holder.Get().Gimmick.ActiveScheduleKeys(*user, now)
		eligibleSet := make(map[store.GimmickSequenceKey]struct{}, len(eligible))
		for _, key := range eligible {
			eligibleSet[key] = struct{}{}
		}
		pruned := 0
		for key, entry := range user.Gimmick.Sequences {
			if _, ok := eligibleSet[key]; ok {
				continue
			}
			if entry.IsGimmickSequenceCleared {
				continue
			}
			delete(user.Gimmick.Sequences, key)
			pruned++
		}
		// Also drop Unlock / Progress rows tied to (scheduleId, sequenceId) keys
		// that were just pruned and are not cleared — stale rows from earlier
		// server versions that let stories be unlocked out of chain order would
		// otherwise keep the icon visible via the projection layer.
		for k, row := range user.Gimmick.Unlocks {
			if !row.IsUnlocked {
				continue
			}
			seqKey := store.GimmickSequenceKey{
				GimmickSequenceScheduleId: k.GimmickSequenceScheduleId,
				GimmickSequenceId:         k.GimmickSequenceId,
			}
			if _, ok := eligibleSet[seqKey]; ok {
				continue
			}
			if seq, exists := user.Gimmick.Sequences[seqKey]; exists && seq.IsGimmickSequenceCleared {
				continue
			}
			if user.Gimmick.Progress[k].IsGimmickCleared {
				continue
			}
			delete(user.Gimmick.Unlocks, k)
		}

		added := 0
		for _, key := range eligible {
			if len(user.Gimmick.Sequences) >= masterdata.MaxUserGimmickRows {
				break
			}
			if _, exists := user.Gimmick.Sequences[key]; !exists {
				user.Gimmick.Sequences[key] = store.GimmickSequenceState{Key: key}
				added++
			}
		}
		if pruned > 0 || added > 0 {
			log.Printf("[GimmickService] InitSequenceSchedule: pruned %d stale, added %d sequences (total %d, eligible %d, cap %d)",
				pruned, added, len(user.Gimmick.Sequences), len(eligible), masterdata.MaxUserGimmickRows)
		}
	})
	return &pb.InitSequenceScheduleResponse{}, nil
}

func (s *GimmickServiceServer) Unlock(ctx context.Context, req *pb.UnlockRequest) (*pb.UnlockResponse, error) {
	log.Printf("[GimmickService] Unlock: gimmickKeys=%d", len(req.GimmickKey))
	userId := CurrentUserId(ctx, s.users, s.sessions)
	s.users.UpdateUser(userId, func(user *store.UserState) {
		for _, item := range req.GimmickKey {
			key := store.GimmickKey{
				GimmickSequenceScheduleId: item.GimmickSequenceScheduleId,
				GimmickSequenceId:         item.GimmickSequenceId,
				GimmickId:                 item.GimmickId,
			}
			unlock := user.Gimmick.Unlocks[key]
			unlock.Key = key
			unlock.IsUnlocked = true
			user.Gimmick.Unlocks[key] = unlock
		}
	})
	return &pb.UnlockResponse{}, nil
}
