package service

import (
	"context"
	"log"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/runtime"
	"lunar-tear/server/internal/store"
)

type CharacterServiceServer struct {
	pb.UnimplementedCharacterServiceServer
	users    store.UserRepository
	sessions store.SessionRepository
	holder   *runtime.Holder
}

func NewCharacterServiceServer(users store.UserRepository, sessions store.SessionRepository, holder *runtime.Holder) *CharacterServiceServer {
	return &CharacterServiceServer{users: users, sessions: sessions, holder: holder}
}

func (s *CharacterServiceServer) Rebirth(ctx context.Context, req *pb.RebirthRequest) (*pb.RebirthResponse, error) {
	log.Printf("[CharacterService] Rebirth: characterId=%d rebirthCount=%d", req.CharacterId, req.RebirthCount)

	cat := s.holder.Get()
	catalog := cat.CharacterRebirth
	config := cat.GameConfig
	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()

	stepGroupId, ok := catalog.StepGroupByCharacterId[req.CharacterId]
	if !ok {
		log.Printf("[CharacterService] Rebirth: no step group for characterId=%d", req.CharacterId)
		return &pb.RebirthResponse{}, nil
	}

	_, err := s.users.UpdateUser(userId, func(user *store.UserState) {
		current := user.CharacterRebirths[req.CharacterId]
		currentCount := current.RebirthCount
		targetCount := currentCount + req.RebirthCount

		for count := currentCount; count < targetCount; count++ {
			step, ok := catalog.StepByGroupAndCount[masterdata.StepKey{GroupId: stepGroupId, BeforeRebirthCount: count}]
			if !ok {
				log.Printf("[CharacterService] Rebirth: no step row for groupId=%d beforeCount=%d", stepGroupId, count)
				return
			}

			goldId := config.ConsumableItemIdForGold
			user.ConsumableItems[goldId] = max(user.ConsumableItems[goldId]-config.CharacterRebirthConsumeGold, 0)
			log.Printf("[CharacterService] Rebirth: consumed gold=%d", config.CharacterRebirthConsumeGold)

			materials := catalog.MaterialsByGroupId[step.CharacterRebirthMaterialGroupId]
			for _, mat := range materials {
				user.Materials[mat.MaterialId] -= mat.Count
				if user.Materials[mat.MaterialId] <= 0 {
					delete(user.Materials, mat.MaterialId)
				}
				log.Printf("[CharacterService] Rebirth: consumed material=%d count=%d", mat.MaterialId, mat.Count)
			}
		}

		log.Printf("[CharacterService] Rebirth: characterId=%d count %d -> %d", req.CharacterId, currentCount, targetCount)
		user.CharacterRebirths[req.CharacterId] = store.CharacterRebirthState{
			CharacterId:   req.CharacterId,
			RebirthCount:  targetCount,
			LatestVersion: nowMillis,
		}

		// Character exalt mission (condition type 71). Type-71 missions use
		// per-character MissionClearConditionOptionGroupIds in the 60000 range
		// that do not equal CharacterId, so we look up the option group id via
		// the rebirth catalog. Missions with OptionGroupId=0 count any exalt.
		if diff := targetCount - currentCount; diff > 0 {
			optionGroupId := catalog.MissionOptionGroupByCharacterId[req.CharacterId]
			ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{
				ConditionType: missionConditionCharacterExalt,
				TargetId:      optionGroupId,
				Delta:         diff,
			}, nowMillis)
		}
	})
	if err != nil {
		log.Printf("[CharacterService] Rebirth error: %v", err)
		return nil, err
	}

	return &pb.RebirthResponse{}, nil
}
