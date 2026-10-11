package service

import (
	"context"
	"log"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/questflow"
	"lunar-tear/server/internal/runtime"
	"lunar-tear/server/internal/store"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

type QuestServiceServer struct {
	pb.UnimplementedQuestServiceServer
	users    store.UserRepository
	sessions store.SessionRepository
	holder   *runtime.Holder
}

func NewQuestServiceServer(users store.UserRepository, sessions store.SessionRepository, holder *runtime.Holder) *QuestServiceServer {
	if holder == nil {
		panic("runtime holder is required")
	}
	return &QuestServiceServer{users: users, sessions: sessions, holder: holder}
}

func (s *QuestServiceServer) UpdateMainFlowSceneProgress(ctx context.Context, req *pb.UpdateMainFlowSceneProgressRequest) (*pb.UpdateMainFlowSceneProgressResponse, error) {
	log.Printf("[QuestService] UpdateMainFlowSceneProgress: questSceneId=%d", req.QuestSceneId)

	engine := s.holder.Get().QuestHandler
	userId := CurrentUserId(ctx, s.users, s.sessions)
	s.users.UpdateUser(userId, func(user *store.UserState) {
		engine.HandleMainFlowSceneProgress(user, req.QuestSceneId, gametime.NowMillis())
	})

	return &pb.UpdateMainFlowSceneProgressResponse{}, nil
}

func (s *QuestServiceServer) UpdateReplayFlowSceneProgress(ctx context.Context, req *pb.UpdateReplayFlowSceneProgressRequest) (*pb.UpdateReplayFlowSceneProgressResponse, error) {
	log.Printf("[QuestService] UpdateReplayFlowSceneProgress: questSceneId=%d", req.QuestSceneId)

	engine := s.holder.Get().QuestHandler
	userId := CurrentUserId(ctx, s.users, s.sessions)
	s.users.UpdateUser(userId, func(user *store.UserState) {
		engine.HandleReplayFlowSceneProgress(user, req.QuestSceneId, gametime.NowMillis())
	})

	return &pb.UpdateReplayFlowSceneProgressResponse{}, nil
}

func (s *QuestServiceServer) UpdateMainQuestSceneProgress(ctx context.Context, req *pb.UpdateMainQuestSceneProgressRequest) (*pb.UpdateMainQuestSceneProgressResponse, error) {
	log.Printf("[QuestService] UpdateMainQuestSceneProgress: questSceneId=%d", req.QuestSceneId)

	engine := s.holder.Get().QuestHandler
	userId := CurrentUserId(ctx, s.users, s.sessions)
	s.users.UpdateUser(userId, func(user *store.UserState) {
		engine.HandleMainQuestSceneProgress(user, req.QuestSceneId)
		
		// Clear suppress flag when player moves on main map
		// This allows them to progress normally on the new scene
		if user.MainQuest.SuppressReplayFlow {
			user.MainQuest.SuppressReplayFlow = false
			log.Printf("[UpdateMainQuestSceneProgress] cleared SuppressReplayFlow on scene move to %d", req.QuestSceneId)
		}
	})

	return &pb.UpdateMainQuestSceneProgressResponse{}, nil
}

func (s *QuestServiceServer) StartMainQuest(ctx context.Context, req *pb.StartMainQuestRequest) (*pb.StartMainQuestResponse, error) {
	log.Printf("[QuestService] StartMainQuest: questId=%d isMainFlow=%v isReplayFlow=%v isBattleOnly=%v maxAutoOrbitCount=%d",
		req.QuestId, req.IsMainFlow, req.IsReplayFlow, req.IsBattleOnly, req.MaxAutoOrbitCount)

	engine := s.holder.Get().QuestHandler
	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()
	
	if _, err := s.users.UpdateUser(userId, func(user *store.UserState) {
		if req.IsReplayFlow {
			engine.HandleQuestStartReplay(user, req.QuestId, req.IsBattleOnly, req.UserDeckNumber, nowMillis)
		} else {
			engine.HandleQuestStart(user, req.QuestId, req.IsBattleOnly, req.IsMainFlow, req.UserDeckNumber, nowMillis)
		}

		// Normal quest start: reset suppress flag so replay-flow updates are allowed again
		user.MainQuest.SuppressReplayFlow = false

		user.Battle.LastStartedQuestId = req.QuestId
		user.Battle.LastStartedQuestAt = nowMillis

		startAutoOrbit(user, model.QuestTypeMain, 0, req.QuestId, req.MaxAutoOrbitCount, nowMillis)
		// The client flushes its Cage accumulators (distance walked, Mama taps)
		// into whichever RPC fires first; StartMainQuest is one of the carriers.
		applyCageMeasurableValues(user, s.holder.Get().Mission, req.GetCageMeasurableValues(), nowMillis)
		// Track stamina usage for missions (even though stamina is free on this server)
		staminaCost := engine.MainQuestStaminaCost(req.QuestId, nowMillis)
		if staminaCost > 0 {
			ApplyMissionProgressEvent(user, s.holder.Get().Mission, MissionProgressEvent{ConditionType: missionConditionStaminaUsed, Delta: staminaCost}, nowMillis)
		}
	}); err != nil {
		log.Printf("[StartMainQuest] UpdateUser FAILED user=%d: %v", userId, err)
		return nil, status.Errorf(codes.Internal, "save user: %v", err)
	}

	drops := engine.BattleDropRewards(req.QuestId)
	log.Printf("[StartMainQuest] quest=%d drops=%d", req.QuestId, len(drops))
	pbDrops := make([]*pb.BattleDropReward, len(drops))
	for i, d := range drops {
		pbDrops[i] = &pb.BattleDropReward{
			QuestSceneId:         d.QuestSceneId,
			BattleDropCategoryId: d.BattleDropCategoryId,
			BattleDropEffectId:   1,
		}
	}

	return &pb.StartMainQuestResponse{
		BattleDropReward: pbDrops,
	}, nil
}

func emptyAutoOrbitReward() *pb.QuestAutoOrbitResult {
	return &pb.QuestAutoOrbitResult{
		DropReward:               []*pb.QuestReward{},
		UserStatusCampaignReward: []*pb.QuestReward{},
	}
}

func autoOrbitDropsToProto(drops []store.AutoOrbitDropEntry) []*pb.QuestReward {
	out := make([]*pb.QuestReward, len(drops))
	for i, d := range drops {
		out[i] = &pb.QuestReward{
			PossessionType: d.PossessionType,
			PossessionId:   canonicalRewardId(d.PossessionType, d.PossessionId),
			Count:          d.Count,
			IsAutoSale:     d.IsAutoSale,
		}
	}
	return out
}

// canonicalRewardId remaps a reward's possession id for display so quest reward
// popups show the spendable base medal -- matching what is actually granted --
// without altering the masterdata catalog. The transform is purely server-side.
func canonicalRewardId(possType, possId int32) int32 {
	if possType == int32(model.PossessionTypeConsumableItem) {
		return store.CanonicalConsumableMedalId(possId)
	}
	return possId
}

func toProtoRewards(grants []questflow.RewardGrant) []*pb.QuestReward {
	if len(grants) == 0 {
		return []*pb.QuestReward{}
	}
	out := make([]*pb.QuestReward, len(grants))
	for i, g := range grants {
		out[i] = &pb.QuestReward{
			PossessionType: int32(g.PossessionType),
			PossessionId:   canonicalRewardId(int32(g.PossessionType), g.PossessionId),
			Count:          g.Count,
			IsAutoSale:     g.IsAutoSale,
		}
	}
	return out
}

func (s *QuestServiceServer) FinishMainQuest(ctx context.Context, req *pb.FinishMainQuestRequest) (*pb.FinishMainQuestResponse, error) {
	log.Printf("[QuestService] FinishMainQuest: questId=%d isMainFlow=%v isRetired=%v isAnnihilated=%v isAutoOrbit=%v storySkipType=%d",
		req.QuestId, req.IsMainFlow, req.IsRetired, req.IsAnnihilated, req.IsAutoOrbit, req.StorySkipType)

	nowMillis := gametime.NowMillis()
	engine := s.holder.Get().QuestHandler
	userId := CurrentUserId(ctx, s.users, s.sessions)
	var outcome questflow.FinishOutcome
	var endedDrops []store.AutoOrbitDropEntry
	var loopEnded bool
	s.users.UpdateUser(userId, func(user *store.UserState) {
		finishQuestId := req.QuestId

		ao := user.QuestAutoOrbit
		if ao.MaxAutoOrbitCount > 0 &&
			ao.QuestId != 0 &&
			finishQuestId != ao.QuestId &&
			!req.IsRetired && !req.IsAnnihilated {
			log.Printf("[FinishMainQuest] REMAP mismatched finish during autoOrbit: got=%d -> tracked=%d",
				finishQuestId, ao.QuestId)
			finishQuestId = ao.QuestId
		}

		if user.Battle.LastStartedQuestId != 0 &&
			finishQuestId != user.Battle.LastStartedQuestId &&
			!req.IsRetired && !req.IsAnnihilated {
			log.Printf("[FinishMainQuest] REMAP mismatched finish: got=%d -> lastStarted=%d",
				finishQuestId, user.Battle.LastStartedQuestId)
			finishQuestId = user.Battle.LastStartedQuestId
		}

		outcome = engine.HandleQuestFinish(user, finishQuestId, req.IsRetired, req.IsAnnihilated, nowMillis)

		user.Battle.LastStartedQuestId = 0
		user.Battle.LastStartedQuestAt = 0

		endedDrops, loopEnded = finishAutoOrbit(user, req.IsAutoOrbit, req.IsRetired, req.IsAnnihilated, model.QuestTypeMain, 0, finishQuestId, nowMillis, outcome.DropRewards)

		if req.IsRetired || req.IsAnnihilated {
			// Log quest states around the retired quest for debugging
			for _, delta := range []int32{-1, 0, 1, 2} {
				checkId := finishQuestId + delta
				if q, ok := user.Quests[checkId]; ok {
					log.Printf("[FinishMainQuest] retire debug: quest=%d state=%d clearCount=%d", checkId, q.QuestStateType, q.ClearCount)
				}
			}
			log.Printf("[FinishMainQuest] retire debug: scene=%d head=%d route=%d savedActive=%v",
				user.MainQuest.CurrentQuestSceneId, user.MainQuest.HeadQuestSceneId,
				user.MainQuest.CurrentMainQuestRouteId, user.MainQuest.SavedContext.Active)
		}

		// Track quit/retire and party wipe for missions.
		// IsRetired counts as "quit" only if it is NOT also an annihilation —
		// a party wipe sets both flags on some clients; we don't want to
		// double-count it as both a quit and a wipe.
		if req.IsRetired && !req.IsAnnihilated {
			log.Printf("[Mission] Quit battle counted (questId=%d)", finishQuestId)
			ApplyMissionProgressEvent(user, s.holder.Get().Mission, MissionProgressEvent{ConditionType: missionConditionQuitBattle, Delta: 1}, nowMillis)
		}
		if req.IsAnnihilated {
			log.Printf("[Mission] Party wipe counted (questId=%d)", finishQuestId)
			ApplyMissionProgressEvent(user, s.holder.Get().Mission, MissionProgressEvent{ConditionType: missionConditionPartyWipe, Delta: 1}, nowMillis)
		}

		if !req.IsRetired && !req.IsAnnihilated {
			log.Printf("[QuestService] FinishMainQuest: before mission check - LastComboCount=%d, LastComboMaxDamage=%d", user.Battle.LastComboCount, user.Battle.LastComboMaxDamage)
			chapterId := s.holder.Get().Mission.QuestToMainChapter[finishQuestId]
			ApplyQuestClearMissionProgress(user, s.holder.Get().Mission, model.QuestTypeMain, finishQuestId, chapterId, nowMillis)
			ApplyMissionProgressEvent(user, s.holder.Get().Mission, MissionProgressEvent{
				ConditionType: missionConditionPlayerLevel,
				CurrentValue:  user.Status.Level,
			}, nowMillis)

			// Update boss defeat missions (type 35 - for missions 220001-220022)
			if n := questBossCount(s.holder.Get().Quest, finishQuestId); n > 0 {
				log.Printf("[QuestService] Quest %d has %d boss(es) (BattleEnemyType=2)", finishQuestId, n)
				ApplyMissionProgressEvent(user, s.holder.Get().Mission, MissionProgressEvent{
					ConditionType: missionConditionBossDefeat,
					Delta:         n,
				}, nowMillis)
			}

			// Solo-clear missions (type 62): "clear quest X with only character Y".
			ApplySoloClearMissionProgress(user, s.holder.Get().Mission, s.holder.Get().Quest, finishQuestId, nowMillis)

			// Hidden-story quest-clear missions (category 7): "clear N quests
			// with X in your loadout" count main quests too.
			ApplyHiddenStoryQuestClearMissionProgress(user, s.holder.Get().Mission, s.holder.Get().Quest, finishQuestId, nowMillis)
		}

		// NOTE: cage running distance is processed in UpdateMissionProgress (mission.go),
		// not here. FinishMainQuest only handles battle-specific missions (boss defeat, quits, wipes).
		// Distance tracking happens continuously via UpdateMissionProgress during gameplay.
	})

	autoOrbitReward := emptyAutoOrbitReward()
	if loopEnded {
		autoOrbitReward.DropReward = autoOrbitDropsToProto(endedDrops)
	}

	return &pb.FinishMainQuestResponse{
		DropReward:                      toProtoRewards(outcome.DropRewards),
		FirstClearReward:                toProtoRewards(outcome.FirstClearRewards),
		MissionClearReward:              toProtoRewards(outcome.MissionClearRewards),
		MissionClearCompleteReward:      toProtoRewards(outcome.MissionClearCompleteRewards),
		AutoOrbitResult:                 []*pb.QuestReward{},
		IsBigWin:                        outcome.IsBigWin,
		BigWinClearedQuestMissionIdList: outcome.BigWinClearedQuestMissionIds,
		ReplayFlowFirstClearReward:      toProtoRewards(outcome.ReplayFlowFirstClearRewards),
		UserStatusCampaignReward:        []*pb.QuestReward{},
		AutoOrbitReward:                 autoOrbitReward,
	}, nil
}

func (s *QuestServiceServer) RestartMainQuest(ctx context.Context, req *pb.RestartMainQuestRequest) (*pb.RestartMainQuestResponse, error) {
	log.Printf("[QuestService] RestartMainQuest: questId=%d isMainFlow=%v", req.QuestId, req.IsMainFlow)

	engine := s.holder.Get().QuestHandler
	userId := CurrentUserId(ctx, s.users, s.sessions)
	var deckNumber int32
	s.users.UpdateUser(userId, func(user *store.UserState) {
		engine.HandleQuestRestart(user, req.QuestId, gametime.NowMillis())
		deckNumber = user.Quests[req.QuestId].UserDeckNumber
	})

	drops := engine.BattleDropRewards(req.QuestId)
	pbDrops := make([]*pb.BattleDropReward, len(drops))
	for i, d := range drops {
		pbDrops[i] = &pb.BattleDropReward{
			QuestSceneId:         d.QuestSceneId,
			BattleDropCategoryId: d.BattleDropCategoryId,
			BattleDropEffectId:   1,
		}
	}

	return &pb.RestartMainQuestResponse{
		BattleDropReward: pbDrops,
		DeckNumber:       deckNumber,
	}, nil
}

func (s *QuestServiceServer) FinishAutoOrbit(ctx context.Context, req *emptypb.Empty) (*pb.FinishAutoOrbitResponse, error) {
	log.Printf("[QuestService] FinishAutoOrbit")
	userId := CurrentUserId(ctx, s.users, s.sessions)
	var drops []store.AutoOrbitDropEntry
	s.users.UpdateUser(userId, func(user *store.UserState) {
		drops = consumeAutoOrbitRewards(user)
	})
	pbDrops := make([]*pb.QuestReward, len(drops))
	for i, d := range drops {
		pbDrops[i] = &pb.QuestReward{
			PossessionType: d.PossessionType,
			PossessionId:   canonicalRewardId(d.PossessionType, d.PossessionId),
			Count:          d.Count,
		}
	}
	return &pb.FinishAutoOrbitResponse{
		AutoOrbitResult: []*pb.QuestReward{},
		AutoOrbitReward: &pb.QuestAutoOrbitResult{
			DropReward:               pbDrops,
			UserStatusCampaignReward: []*pb.QuestReward{},
		},
	}, nil
}

func (s *QuestServiceServer) SkipQuest(ctx context.Context, req *pb.SkipQuestRequest) (*pb.SkipQuestResponse, error) {
	log.Printf("[QuestService] SkipQuest: questId=%d skipCount=%d useEffectItems=%d", req.QuestId, req.SkipCount, len(req.UseEffectItem))

	nowMillis := gametime.NowMillis()
	engine := s.holder.Get().QuestHandler
	userId := CurrentUserId(ctx, s.users, s.sessions)
	var outcome questflow.FinishOutcome
	s.users.UpdateUser(userId, func(user *store.UserState) {
		// req.UseEffectItem is not consumed here: the engine deducts skip
		// tickets itself, and stamina is free on this server.
		outcome, _ = engine.HandleQuestSkip(user, req.QuestId, req.SkipCount, nowMillis)
	})

	return &pb.SkipQuestResponse{
		DropReward:               toProtoRewards(outcome.DropRewards),
		UserStatusCampaignReward: []*pb.QuestReward{},
	}, nil
}

func (s *QuestServiceServer) SkipQuestBulk(ctx context.Context, req *pb.SkipQuestBulkRequest) (*pb.SkipQuestBulkResponse, error) {
	log.Printf("[QuestService] SkipQuestBulk: quests=%d useEffectItems=%d", len(req.SkipQuestInfo), len(req.UseEffectItem))

	nowMillis := gametime.NowMillis()
	engine := s.holder.Get().QuestHandler
	userId := CurrentUserId(ctx, s.users, s.sessions)
	var drops []questflow.RewardGrant
	s.users.UpdateUser(userId, func(user *store.UserState) {
		// Validate the whole batch up front so a bad request never consumes
		// tickets partially.
		total := int32(0)
		for _, info := range req.SkipQuestInfo {
			if _, ok := engine.QuestById[info.QuestId]; !ok {
				log.Printf("[QuestService] SkipQuestBulk: unknown questId=%d, batch rejected", info.QuestId)
				return
			}
			total += info.SkipCount
		}
		ticketId := engine.Config.ConsumableItemIdForQuestSkipTicket
		if user.ConsumableItems[ticketId] < total {
			log.Printf("[QuestService] SkipQuestBulk: not enough skip tickets: have=%d need=%d",
				user.ConsumableItems[ticketId], total)
			return
		}
		for _, info := range req.SkipQuestInfo {
			outcome, ok := engine.HandleQuestSkip(user, info.QuestId, info.SkipCount, nowMillis)
			if !ok {
				break
			}
			drops = append(drops, outcome.DropRewards...)
		}
	})

	return &pb.SkipQuestBulkResponse{
		DropReward:               toProtoRewards(drops),
		UserStatusCampaignReward: []*pb.QuestReward{},
	}, nil
}

func (s *QuestServiceServer) SetRoute(ctx context.Context, req *pb.SetRouteRequest) (*pb.SetRouteResponse, error) {
	log.Printf("[QuestService] SetRoute: mainQuestRouteId=%d", req.MainQuestRouteId)

	engine := s.holder.Get().QuestHandler
	userId := CurrentUserId(ctx, s.users, s.sessions)
	s.users.UpdateUser(userId, func(user *store.UserState) {
		user.MainQuest.CurrentMainQuestRouteId = req.MainQuestRouteId
		if seasonId, ok := engine.SeasonIdByRouteId[req.MainQuestRouteId]; ok {
			user.MainQuest.MainQuestSeasonId = seasonId
		}
		now := gametime.NowMillis()
		user.PortalCageStatus.IsCurrentProgress = false
		user.PortalCageStatus.LatestVersion = now
		if user.SideStoryActiveProgress.CurrentSideStoryQuestId != 0 {
			user.SideStoryActiveProgress = store.SideStoryActiveProgress{
				LatestVersion: now,
			}
		}
	})

	return &pb.SetRouteResponse{}, nil
}

func (s *QuestServiceServer) SetQuestSceneChoice(ctx context.Context, req *pb.SetQuestSceneChoiceRequest) (*pb.SetQuestSceneChoiceResponse, error) {
	log.Printf("[QuestService] SetQuestSceneChoice: questSceneId=%d choiceNumber=%d flow=%d",
		req.QuestSceneId, req.ChoiceNumber, req.QuestFlowType)

	catalog := s.holder.Get().Quest

	// Resolve choice by (questSceneId, questFlowType, choiceNumber)
	choiceKey := masterdata.SceneChoiceKey{
		QuestSceneId:  req.QuestSceneId,
		QuestFlowType: req.QuestFlowType,
		ChoiceNumber:  req.ChoiceNumber,
	}
	choice, ok := catalog.SceneChoiceByKey[choiceKey]
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "invalid quest scene choice")
	}

	// Get effect and grouping info
	effect, ok := catalog.SceneChoiceEffectById[choice.QuestSceneChoiceEffectId]
	if !ok {
		return nil, status.Error(codes.FailedPrecondition, "quest scene choice effect unavailable")
	}

	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()
	s.users.UpdateUser(userId, func(user *store.UserState) {
		groupingId := effect.QuestSceneChoiceGroupingId
		effectId := effect.QuestSceneChoiceEffectId

		// Update current choice for this grouping
		user.QuestSceneChoices[groupingId] = store.QuestSceneChoiceState{
			QuestSceneChoiceGroupingId: groupingId,
			QuestSceneChoiceEffectId:   effectId,
			LatestVersion:              nowMillis,
		}

		// Add to history if not already there
		if _, exists := user.QuestSceneChoiceHistory[effectId]; !exists {
			user.QuestSceneChoiceHistory[effectId] = store.QuestSceneChoiceHistoryState{
				QuestSceneChoiceEffectId: effectId,
				ChoiceDatetime:           nowMillis,
				LatestVersion:            nowMillis,
			}
		}
	})

	return &pb.SetQuestSceneChoiceResponse{}, nil
}

func (s *QuestServiceServer) ResetLimitContentQuestProgress(ctx context.Context, req *pb.ResetLimitContentQuestProgressRequest) (*pb.ResetLimitContentQuestProgressResponse, error) {
	log.Printf("[QuestService] ResetLimitContentQuestProgress: eventQuestChapterId=%d questId=%d",
		req.EventQuestChapterId, req.QuestId)

	cat := s.holder.Get()
	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()
	s.users.UpdateUser(userId, func(user *store.UserState) {
		// Legacy side-story path (kept for compatibility).
		if _, exists := user.SideStoryQuests[req.QuestId]; exists {
			user.SideStoryQuests[req.QuestId] = store.SideStoryQuestProgress{
				HeadSideStoryQuestSceneId: 0,
				SideStoryQuestStateType:   model.SideStoryQuestStateUnknown,
				LatestVersion:             nowMillis,
			}
		}
		if user.SideStoryActiveProgress.CurrentSideStoryQuestId == req.QuestId {
			user.SideStoryActiveProgress = store.SideStoryActiveProgress{
				LatestVersion: nowMillis,
			}
		}

		// Recollections of Dusk: reset the whole difficulty (floor) containing questId.
		questIds := []int32{req.QuestId}
		if cat != nil && cat.Quest != nil {
			for _, ids := range cat.Quest.EventQuestIdsByChapterDifficulty[req.EventQuestChapterId] {
				for _, id := range ids {
					if id == req.QuestId {
						questIds = append([]int32(nil), ids...)
						goto foundDifficulty
					}
				}
			}
		}
	foundDifficulty:

		for _, id := range questIds {
			if q, ok := user.Quests[id]; ok {
				q.QuestStateType = model.UserQuestStateTypeUnknown
				q.UserDeckNumber = 0
				user.Quests[id] = q
			} else {
				user.Quests[id] = store.UserQuestState{
					QuestId:        id,
					QuestStateType: model.UserQuestStateTypeUnknown,
				}
			}
			delete(user.QuestLimitContentStatus, id)
		}

		if user.DeckLimitContentRestricted != nil {
			for id, restricted := range user.DeckLimitContentRestricted {
				if restricted.EventQuestChapterId != req.EventQuestChapterId {
					continue
				}
				for _, qid := range questIds {
					if restricted.QuestId == qid {
						delete(user.DeckLimitContentRestricted, id)
						break
					}
				}
			}
		}
	})

	return &pb.ResetLimitContentQuestProgressResponse{}, nil
}

func (s *QuestServiceServer) SetAutoSaleSetting(ctx context.Context, req *pb.SetAutoSaleSettingRequest) (*pb.SetAutoSaleSettingResponse, error) {
	log.Printf("[QuestService] SetAutoSaleSetting: items=%d", len(req.AutoSaleSettingItem))

	userId := CurrentUserId(ctx, s.users, s.sessions)
	s.users.UpdateUser(userId, func(user *store.UserState) {
		user.AutoSaleSettings = make(map[int32]store.AutoSaleSettingState, len(req.AutoSaleSettingItem))
		for itemType, itemValue := range req.AutoSaleSettingItem {
			user.AutoSaleSettings[itemType] = store.AutoSaleSettingState{
				PossessionAutoSaleItemType:  itemType,
				PossessionAutoSaleItemValue: itemValue,
			}
		}
	})

	return &pb.SetAutoSaleSettingResponse{}, nil
}
