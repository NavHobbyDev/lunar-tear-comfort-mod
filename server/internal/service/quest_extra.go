package service

import (
	"context"
	"log"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/questflow"
	"lunar-tear/server/internal/store"
)

func (s *QuestServiceServer) StartExtraQuest(ctx context.Context, req *pb.StartExtraQuestRequest) (*pb.StartExtraQuestResponse, error) {
	log.Printf("[QuestService] StartExtraQuest: questId=%d deckNumber=%d", req.QuestId, req.UserDeckNumber)

	engine := s.holder.Get().QuestHandler
	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()
	s.users.UpdateUser(userId, func(user *store.UserState) {
		engine.HandleExtraQuestStart(user, req.QuestId, req.UserDeckNumber, nowMillis)
		// Track stamina usage for missions (even though stamina is free on this server)
		staminaCost := engine.ExtraQuestStaminaCost(req.QuestId, nowMillis)
		if staminaCost > 0 {
			ApplyMissionProgressEvent(user, s.holder.Get().Mission, MissionProgressEvent{ConditionType: missionConditionStaminaUsed, Delta: staminaCost}, nowMillis)
		}
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

	return &pb.StartExtraQuestResponse{
		BattleDropReward: pbDrops,
	}, nil
}

func (s *QuestServiceServer) FinishExtraQuest(ctx context.Context, req *pb.FinishExtraQuestRequest) (*pb.FinishExtraQuestResponse, error) {
	log.Printf("[QuestService] FinishExtraQuest: questId=%d isRetired=%v isAnnihilated=%v", req.QuestId, req.IsRetired, req.IsAnnihilated)

	nowMillis := gametime.NowMillis()
	engine := s.holder.Get().QuestHandler
	userId := CurrentUserId(ctx, s.users, s.sessions)
	var outcome questflow.FinishOutcome
	s.users.UpdateUser(userId, func(user *store.UserState) {
		outcome = engine.HandleExtraQuestFinish(user, req.QuestId, req.IsRetired, req.IsAnnihilated, nowMillis)

		// Track quit/retire and party wipe for missions
		if req.IsRetired && !req.IsAnnihilated {
			ApplyMissionProgressEvent(user, s.holder.Get().Mission, MissionProgressEvent{ConditionType: missionConditionQuitBattle, Delta: 1}, nowMillis)
		}
		if req.IsAnnihilated {
			ApplyMissionProgressEvent(user, s.holder.Get().Mission, MissionProgressEvent{ConditionType: missionConditionPartyWipe, Delta: 1}, nowMillis)
		}

		if !req.IsRetired && !req.IsAnnihilated {
			log.Printf("[QuestService] FinishExtraQuest: before mission check - LastComboCount=%d, LastComboMaxDamage=%d", user.Battle.LastComboCount, user.Battle.LastComboMaxDamage)
			ApplyQuestClearMissionProgress(user, s.holder.Get().Mission, model.QuestTypeExtra, req.QuestId, 0, nowMillis)
			ApplyMissionProgressEvent(user, s.holder.Get().Mission, MissionProgressEvent{
				ConditionType: missionConditionPlayerLevel,
				CurrentValue:  user.Status.Level,
			}, nowMillis)

			// Update boss defeat missions (type 35 - for missions 220001-220022)
			if n := questBossCount(s.holder.Get().Quest, req.QuestId); n > 0 {
				log.Printf("[QuestService] ExtraQuest %d has %d boss(es) (BattleEnemyType=2)", req.QuestId, n)
				ApplyMissionProgressEvent(user, s.holder.Get().Mission, MissionProgressEvent{
					ConditionType: missionConditionBossDefeat,
					Delta:         n,
				}, nowMillis)
			}

			// Solo-clear missions (type 62): Dark Lair quests are extra quests.
			ApplySoloClearMissionProgress(user, s.holder.Get().Mission, s.holder.Get().Quest, req.QuestId, nowMillis)

			// Hidden-story quest-clear missions (category 7): "clear N quests
			// with X in your loadout" count extra quests too.
			ApplyHiddenStoryQuestClearMissionProgress(user, s.holder.Get().Mission, s.holder.Get().Quest, req.QuestId, nowMillis)
		}
	})

	return &pb.FinishExtraQuestResponse{
		DropReward:                      toProtoRewards(outcome.DropRewards),
		FirstClearReward:                toProtoRewards(outcome.FirstClearRewards),
		MissionClearReward:              toProtoRewards(outcome.MissionClearRewards),
		MissionClearCompleteReward:      toProtoRewards(outcome.MissionClearCompleteRewards),
		IsBigWin:                        outcome.IsBigWin,
		BigWinClearedQuestMissionIdList: outcome.BigWinClearedQuestMissionIds,
		UserStatusCampaignReward:        []*pb.QuestReward{},
	}, nil
}

func (s *QuestServiceServer) RestartExtraQuest(ctx context.Context, req *pb.RestartExtraQuestRequest) (*pb.RestartExtraQuestResponse, error) {
	log.Printf("[QuestService] RestartExtraQuest: questId=%d", req.QuestId)

	engine := s.holder.Get().QuestHandler
	userId := CurrentUserId(ctx, s.users, s.sessions)
	var deckNumber int32
	s.users.UpdateUser(userId, func(user *store.UserState) {
		engine.HandleExtraQuestRestart(user, req.QuestId, gametime.NowMillis())
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

	return &pb.RestartExtraQuestResponse{
		BattleDropReward: pbDrops,
		DeckNumber:       deckNumber,
	}, nil
}

func (s *QuestServiceServer) UpdateExtraQuestSceneProgress(ctx context.Context, req *pb.UpdateExtraQuestSceneProgressRequest) (*pb.UpdateExtraQuestSceneProgressResponse, error) {
	log.Printf("[QuestService] UpdateExtraQuestSceneProgress: questSceneId=%d", req.QuestSceneId)

	engine := s.holder.Get().QuestHandler
	userId := CurrentUserId(ctx, s.users, s.sessions)
	s.users.UpdateUser(userId, func(user *store.UserState) {
		engine.HandleExtraQuestSceneProgress(user, req.QuestSceneId, gametime.NowMillis())
	})

	return &pb.UpdateExtraQuestSceneProgressResponse{}, nil
}
