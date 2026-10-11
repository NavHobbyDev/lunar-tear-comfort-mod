package service

import (
	"context"
	"fmt"
	"log"
	"slices"

	"github.com/google/uuid"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/questflow"
	"lunar-tear/server/internal/store"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

func (s *QuestServiceServer) StartEventQuest(ctx context.Context, req *pb.StartEventQuestRequest) (*pb.StartEventQuestResponse, error) {
	log.Printf("[QuestService] StartEventQuest: chapterId=%d questId=%d isBattleOnly=%v maxAutoOrbitCount=%d",
		req.EventQuestChapterId, req.QuestId, req.IsBattleOnly, req.MaxAutoOrbitCount)

	cat := s.holder.Get()
	engine := cat.QuestHandler
	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()
	var validationErr error
	s.users.UpdateUser(userId, func(user *store.UserState) {
		if err := validateLimitContentDeck(user, cat.LimitContent, cat.Quest, req.EventQuestChapterId, req.QuestId, req.UserDeckNumber, nowMillis); err != nil {
			validationErr = err
			return
		}
		if err := validateQuestDeckRestrictions(user, cat.Quest, req.QuestId, req.UserDeckNumber); err != nil {
			validationErr = err
			return
		}
		engine.HandleEventQuestStart(user, req.EventQuestChapterId, req.QuestId, req.IsBattleOnly, req.UserDeckNumber, nowMillis)
		startAutoOrbit(user, model.QuestTypeEvent, req.EventQuestChapterId, req.QuestId, req.MaxAutoOrbitCount, nowMillis)
		staminaCost := engine.EventQuestStaminaCost(req.EventQuestChapterId, req.QuestId, nowMillis)
		if staminaCost > 0 {
			ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{ConditionType: missionConditionStaminaUsed, Delta: staminaCost}, nowMillis)
		}
	})
	if validationErr != nil {
		return nil, validationErr
	}

	drops := engine.BattleDropRewards(req.QuestId)
	pbDrops := make([]*pb.BattleDropReward, len(drops))
	for i, d := range drops {
		pbDrops[i] = &pb.BattleDropReward{
			QuestSceneId:         d.QuestSceneId,
			BattleDropCategoryId: d.BattleDropCategoryId,
			BattleDropEffectId:   1,
		}
	}

	return &pb.StartEventQuestResponse{
		BattleDropReward: pbDrops,
	}, nil
}

func (s *QuestServiceServer) FinishEventQuest(ctx context.Context, req *pb.FinishEventQuestRequest) (*pb.FinishEventQuestResponse, error) {
	log.Printf("[QuestService] FinishEventQuest: chapterId=%d questId=%d isRetired=%v isAnnihilated=%v isAutoOrbit=%v",
		req.EventQuestChapterId, req.QuestId, req.IsRetired, req.IsAnnihilated, req.IsAutoOrbit)

	nowMillis := gametime.NowMillis()
	cat := s.holder.Get()
	engine := cat.QuestHandler
	userId := CurrentUserId(ctx, s.users, s.sessions)
	var outcome questflow.FinishOutcome
	var endedDrops []store.AutoOrbitDropEntry
	var loopEnded bool
	var validationErr error
	s.users.UpdateUser(userId, func(user *store.UserState) {
		deckNumber := user.Quests[req.QuestId].UserDeckNumber
		if !req.IsRetired && !req.IsAnnihilated {
			if err := validateLimitContentDeck(user, cat.LimitContent, cat.Quest, req.EventQuestChapterId, req.QuestId, deckNumber, nowMillis); err != nil {
				validationErr = err
				return
			}
		}

		outcome = engine.HandleEventQuestFinish(user, req.EventQuestChapterId, req.QuestId, req.IsRetired, req.IsAnnihilated, nowMillis)

		if !req.IsRetired && !req.IsAnnihilated {
			if err := recordLimitContentDeck(user, cat.LimitContent, cat.Quest, req.EventQuestChapterId, req.QuestId, deckNumber, nowMillis); err != nil {
				validationErr = err
				return
			}
			releaseClearedLimitContentDifficultyDecks(user, cat.Quest, req.EventQuestChapterId, req.QuestId)
		}

		endedDrops, loopEnded = finishAutoOrbit(user, req.IsAutoOrbit, req.IsRetired, req.IsAnnihilated, model.QuestTypeEvent, req.EventQuestChapterId, req.QuestId, nowMillis, outcome.DropRewards)

		if req.IsRetired && !req.IsAnnihilated {
			ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{ConditionType: missionConditionQuitBattle, Delta: 1}, nowMillis)
		}
		if req.IsAnnihilated {
			ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{ConditionType: missionConditionPartyWipe, Delta: 1}, nowMillis)
		}

		if !req.IsRetired && !req.IsAnnihilated {
			ApplyQuestClearMissionProgress(user, cat.Mission, model.QuestTypeEvent, req.QuestId, req.EventQuestChapterId, nowMillis)
			ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{
				ConditionType: missionConditionPlayerLevel,
				CurrentValue:  user.Status.Level,
			}, nowMillis)
			if n := questBossCount(cat.Quest, req.QuestId); n > 0 {
				ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{
					ConditionType: missionConditionBossDefeat,
					Delta:         n,
				}, nowMillis)
			}
			ApplyHiddenStoryQuestClearMissionProgress(user, cat.Mission, cat.Quest, req.QuestId, nowMillis)
		}
	})
	if validationErr != nil {
		return nil, validationErr
	}

	autoOrbitReward := emptyAutoOrbitReward()
	if loopEnded {
		autoOrbitReward.DropReward = autoOrbitDropsToProto(endedDrops)
	}

	return &pb.FinishEventQuestResponse{
		DropReward:                      toProtoRewards(outcome.DropRewards),
		FirstClearReward:                toProtoRewards(outcome.FirstClearRewards),
		MissionClearReward:              toProtoRewards(outcome.MissionClearRewards),
		MissionClearCompleteReward:      toProtoRewards(outcome.MissionClearCompleteRewards),
		AutoOrbitResult:                 []*pb.QuestReward{},
		IsBigWin:                        outcome.IsBigWin,
		BigWinClearedQuestMissionIdList: outcome.BigWinClearedQuestMissionIds,
		UserStatusCampaignReward:        []*pb.QuestReward{},
		AutoOrbitReward:                 autoOrbitReward,
	}, nil
}

// releaseClearedLimitContentDifficultyDecks clears costume/weapon usage locks
// for a difficulty once every quest in that difficulty is cleared.
func releaseClearedLimitContentDifficultyDecks(user *store.UserState, catalog *masterdata.QuestCatalog, chapterId, questId int32) {
	if catalog == nil || !catalog.LimitContentQuestIds[questId] {
		return
	}
	for _, questIds := range catalog.EventQuestIdsByChapterDifficulty[chapterId] {
		if !slices.Contains(questIds, questId) {
			continue
		}
		for _, id := range questIds {
			if user.Quests[id].QuestStateType != model.UserQuestStateTypeCleared {
				return
			}
		}
		for id, restricted := range user.DeckLimitContentRestricted {
			if restricted.EventQuestChapterId == chapterId && slices.Contains(questIds, restricted.QuestId) {
				delete(user.DeckLimitContentRestricted, id)
			}
		}
		return
	}
}

type limitContentDeckTarget struct {
	possessionType int32
	uuid           string
}

func limitContentDeckTargets(user *store.UserState, deckNumber int32) ([]limitContentDeckTarget, error) {
	deck, ok := user.Decks[store.DeckKey{DeckType: model.DeckTypeRestrictedLimitContentQuest, UserDeckNumber: deckNumber}]
	if !ok {
		return nil, status.Error(codes.FailedPrecondition, "limit-content deck does not exist")
	}
	deckCharacterUuids := []string{deck.UserDeckCharacterUuid01, deck.UserDeckCharacterUuid02, deck.UserDeckCharacterUuid03}
	var targets []limitContentDeckTarget
	seen := make(map[string]bool)
	add := func(possessionType int32, targetUuid string) {
		key := fmt.Sprintf("%d:%s", possessionType, targetUuid)
		if targetUuid != "" && !seen[key] {
			seen[key] = true
			targets = append(targets, limitContentDeckTarget{possessionType, targetUuid})
		}
	}
	for _, deckCharacterUuid := range deckCharacterUuids {
		if deckCharacterUuid == "" {
			continue
		}
		character, ok := user.DeckCharacters[deckCharacterUuid]
		if !ok {
			return nil, status.Error(codes.FailedPrecondition, "limit-content deck contains an unknown character")
		}
		if character.UserCostumeUuid == "" {
			return nil, status.Error(codes.FailedPrecondition, "limit-content deck character has no costume")
		}
		if _, ok := user.Costumes[character.UserCostumeUuid]; !ok {
			return nil, status.Error(codes.FailedPrecondition, "limit-content deck contains an unknown costume")
		}
		if character.MainUserWeaponUuid == "" {
			return nil, status.Error(codes.FailedPrecondition, "limit-content deck character has no main weapon")
		}
		if _, ok := user.Weapons[character.MainUserWeaponUuid]; !ok {
			return nil, status.Error(codes.FailedPrecondition, "limit-content deck contains an unknown main weapon")
		}
		add(int32(model.PossessionTypeCostume), character.UserCostumeUuid)
		add(int32(model.PossessionTypeWeapon), character.MainUserWeaponUuid)
		for _, weaponUuid := range user.DeckSubWeapons[deckCharacterUuid] {
			if _, ok := user.Weapons[weaponUuid]; !ok {
				return nil, status.Error(codes.FailedPrecondition, "limit-content deck contains an unknown sub weapon")
			}
			add(int32(model.PossessionTypeWeapon), weaponUuid)
		}
	}
	if len(targets) == 0 {
		return nil, status.Error(codes.FailedPrecondition, "limit-content deck is empty")
	}
	return targets, nil
}

func restrictionPossessionType(restrictionType int32) int32 {
	if restrictionType == masterdata.LimitContentDeckRestrictionTypeCostume {
		return int32(model.PossessionTypeCostume)
	}
	if restrictionType == masterdata.LimitContentDeckRestrictionTypeWeapon {
		return int32(model.PossessionTypeWeapon)
	}
	return 0
}

// shouldLockWeapons: official rule — on floors/quests with required affinities,
// weapons alone may be reused within the floor. Skip weapon lock/validate then.
func shouldLockWeapons(questCatalog *masterdata.QuestCatalog, questId int32) bool {
	if questCatalog == nil {
		return true
	}
	// Affinity-required quest => weapons are reusable.
	if questCatalog.QuestHasAffinityRestriction(questId) {
		return false
	}
	return true
}

func validateLimitContentDeck(user *store.UserState, limitCat *masterdata.LimitContentCatalog, questCat *masterdata.QuestCatalog, chapterId, questId, deckNumber int32, nowMillis int64) error {
	if limitCat == nil {
		return nil
	}
	restrictedTypes := limitCat.ActiveRestrictionTypes(chapterId, nowMillis)
	if len(restrictedTypes) == 0 {
		return nil
	}
	if _, ok := user.Decks[store.DeckKey{DeckType: model.DeckTypeRestrictedLimitContentQuest, UserDeckNumber: deckNumber}]; !ok {
		return nil
	}
	targets, err := limitContentDeckTargets(user, deckNumber)
	if err != nil {
		return err
	}
	lockWeapons := shouldLockWeapons(questCat, questId)
	for _, restrictionType := range restrictedTypes {
		if restrictionType == masterdata.LimitContentDeckRestrictionTypeWeapon && !lockWeapons {
			continue
		}
		possessionType := restrictionPossessionType(restrictionType)
		for _, target := range targets {
			if target.possessionType != possessionType {
				continue
			}
			for _, used := range user.DeckLimitContentRestricted {
				if used.EventQuestChapterId == chapterId && used.PossessionType == possessionType && used.TargetUuid == target.uuid {
					return status.Error(codes.FailedPrecondition, "deck contains content already used in this limit quest")
				}
			}
		}
	}
	return nil
}

func recordLimitContentDeck(user *store.UserState, limitCat *masterdata.LimitContentCatalog, questCat *masterdata.QuestCatalog, chapterId, questId, deckNumber int32, nowMillis int64) error {
	if limitCat == nil {
		return nil
	}
	restrictionTypes := limitCat.ActiveRestrictionTypes(chapterId, nowMillis)
	if len(restrictionTypes) == 0 {
		return nil
	}
	if _, ok := user.Decks[store.DeckKey{DeckType: model.DeckTypeRestrictedLimitContentQuest, UserDeckNumber: deckNumber}]; !ok {
		return nil
	}
	targets, err := limitContentDeckTargets(user, deckNumber)
	if err != nil {
		return err
	}
	if user.DeckLimitContentRestricted == nil {
		user.DeckLimitContentRestricted = make(map[string]store.DeckLimitContentRestrictedState)
	}
	lockWeapons := shouldLockWeapons(questCat, questId)
	for _, restrictionType := range restrictionTypes {
		if restrictionType == masterdata.LimitContentDeckRestrictionTypeWeapon && !lockWeapons {
			continue
		}
		possessionType := restrictionPossessionType(restrictionType)
		for _, target := range targets {
			if target.possessionType != possessionType {
				continue
			}
			alreadyRecorded := false
			for _, used := range user.DeckLimitContentRestricted {
				if used.EventQuestChapterId == chapterId && used.PossessionType == possessionType && used.TargetUuid == target.uuid {
					alreadyRecorded = true
					break
				}
			}
			if alreadyRecorded {
				continue
			}
			id := uuid.NewString()
			user.DeckLimitContentRestricted[id] = store.DeckLimitContentRestrictedState{
				DeckRestrictedUuid:  id,
				EventQuestChapterId: chapterId,
				QuestId:             questId,
				PossessionType:      possessionType,
				TargetUuid:          target.uuid,
				LatestVersion:       nowMillis,
			}
		}
	}
	return nil
}

// validateQuestDeckRestrictions enforces required characters / costumes / affinities
// from m_quest_deck_restriction_group (QuestDeckRestrictionType).
func validateQuestDeckRestrictions(user *store.UserState, catalog *masterdata.QuestCatalog, questId, deckNumber int32) error {
	if catalog == nil {
		return nil
	}
	restrictions := catalog.DeckRestrictionsForQuest(questId)
	if len(restrictions) == 0 {
		return nil
	}
	deck, ok := user.Decks[store.DeckKey{DeckType: model.DeckTypeRestrictedLimitContentQuest, UserDeckNumber: deckNumber}]
	if !ok {
		// Also accept normal quest decks if client used them.
		deck, ok = user.Decks[store.DeckKey{DeckType: model.DeckTypeQuest, UserDeckNumber: deckNumber}]
		if !ok {
			return status.Error(codes.FailedPrecondition, "deck does not exist for restriction check")
		}
	}
	slots := []string{deck.UserDeckCharacterUuid01, deck.UserDeckCharacterUuid02, deck.UserDeckCharacterUuid03}

	for _, row := range restrictions {
		if row.QuestDeckRestrictionType == masterdata.QuestDeckRestrictionTypeForbidden {
			continue
		}
		// SlotNumber is 1-based in master data.
		slotIdx := int(row.SlotNumber - 1)
		if slotIdx < 0 || slotIdx >= len(slots) {
			continue
		}
		dcUuid := slots[slotIdx]
		if dcUuid == "" {
			return status.Errorf(codes.FailedPrecondition, "deck slot %d is empty but required", row.SlotNumber)
		}
		dc, ok := user.DeckCharacters[dcUuid]
		if !ok {
			return status.Errorf(codes.FailedPrecondition, "deck slot %d has unknown character", row.SlotNumber)
		}
		costume, ok := user.Costumes[dc.UserCostumeUuid]
		if !ok {
			return status.Errorf(codes.FailedPrecondition, "deck slot %d has unknown costume", row.SlotNumber)
		}
		masterCostume, ok := catalog.CostumeById[costume.CostumeId]
		if !ok {
			return status.Errorf(codes.FailedPrecondition, "deck slot %d costume not in master data", row.SlotNumber)
		}

		switch row.QuestDeckRestrictionType {
		case masterdata.QuestDeckRestrictionTypeCharacterId:
			if masterCostume.CharacterId != row.RestrictionValue {
				return status.Errorf(codes.FailedPrecondition,
					"deck slot %d requires character %d", row.SlotNumber, row.RestrictionValue)
			}
		case masterdata.QuestDeckRestrictionTypeCostumeId:
			if costume.CostumeId != row.RestrictionValue {
				return status.Errorf(codes.FailedPrecondition,
					"deck slot %d requires costume %d", row.SlotNumber, row.RestrictionValue)
			}
		case masterdata.QuestDeckRestrictionTypeProperAttributeType:
			attr, ok := catalog.CostumeProperAttributeByCostumeId[costume.CostumeId]
			if !ok || attr != row.RestrictionValue {
				return status.Errorf(codes.FailedPrecondition,
					"deck slot %d requires affinity %d", row.SlotNumber, row.RestrictionValue)
			}
		}
	}
	return nil
}

func (s *QuestServiceServer) RestartEventQuest(ctx context.Context, req *pb.RestartEventQuestRequest) (*pb.RestartEventQuestResponse, error) {
	log.Printf("[QuestService] RestartEventQuest: chapterId=%d questId=%d", req.EventQuestChapterId, req.QuestId)

	engine := s.holder.Get().QuestHandler
	userId := CurrentUserId(ctx, s.users, s.sessions)
	s.users.UpdateUser(userId, func(user *store.UserState) {
		engine.HandleEventQuestRestart(user, req.EventQuestChapterId, req.QuestId, gametime.NowMillis())
	})

	return &pb.RestartEventQuestResponse{
		BattleDropReward: []*pb.BattleDropReward{},
	}, nil
}

func (s *QuestServiceServer) UpdateEventQuestSceneProgress(ctx context.Context, req *pb.UpdateEventQuestSceneProgressRequest) (*pb.UpdateEventQuestSceneProgressResponse, error) {
	log.Printf("[QuestService] UpdateEventQuestSceneProgress: questSceneId=%d", req.QuestSceneId)

	engine := s.holder.Get().QuestHandler
	userId := CurrentUserId(ctx, s.users, s.sessions)
	s.users.UpdateUser(userId, func(user *store.UserState) {
		engine.HandleEventQuestSceneProgress(user, req.QuestSceneId, gametime.NowMillis())
	})

	return &pb.UpdateEventQuestSceneProgressResponse{}, nil
}

const defaultGuerrillaFreeOpenMinutes = int32(60)

func (s *QuestServiceServer) StartGuerrillaFreeOpen(ctx context.Context, req *emptypb.Empty) (*pb.StartGuerrillaFreeOpenResponse, error) {
	log.Printf("[QuestService] StartGuerrillaFreeOpen")

	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()
	s.users.UpdateUser(userId, func(user *store.UserState) {
		user.GuerrillaFreeOpen.StartDatetime = nowMillis
		user.GuerrillaFreeOpen.OpenMinutes = defaultGuerrillaFreeOpenMinutes
		user.GuerrillaFreeOpen.DailyOpenedCount++
		user.GuerrillaFreeOpen.LatestVersion = nowMillis
	})

	return &pb.StartGuerrillaFreeOpenResponse{}, nil
}
