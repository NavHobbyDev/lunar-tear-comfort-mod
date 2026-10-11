package service

import (
	"context"
	"fmt"
	"log"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/runtime"
	"lunar-tear/server/internal/store"
)

const companionMaxLevel = int32(50)

type CompanionServiceServer struct {
	pb.UnimplementedCompanionServiceServer
	users    store.UserRepository
	sessions store.SessionRepository
	holder   *runtime.Holder
}

func NewCompanionServiceServer(users store.UserRepository, sessions store.SessionRepository, holder *runtime.Holder) *CompanionServiceServer {
	return &CompanionServiceServer{users: users, sessions: sessions, holder: holder}
}

func (s *CompanionServiceServer) Enhance(ctx context.Context, req *pb.CompanionEnhanceRequest) (*pb.CompanionEnhanceResponse, error) {
	log.Printf("[CompanionService] Enhance: uuid=%s addLevel=%d", req.UserCompanionUuid, req.AddLevelCount)

	cat := s.holder.Get()
	catalog := cat.Companion
	config := cat.GameConfig
	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()

	_, err := s.users.UpdateUser(userId, func(user *store.UserState) {
		companion, ok := user.Companions[req.UserCompanionUuid]
		if !ok {
			log.Printf("[CompanionService] Enhance: companion uuid=%s not found", req.UserCompanionUuid)
			return
		}

		compDef, ok := catalog.CompanionById[companion.CompanionId]
		if !ok {
			log.Printf("[CompanionService] Enhance: companion master id=%d not found", companion.CompanionId)
			return
		}

		previousLevel := companion.Level
		targetLevel := companion.Level + req.AddLevelCount
		if targetLevel > companionMaxLevel {
			targetLevel = companionMaxLevel
		}

		for lvl := companion.Level; lvl < targetLevel; lvl++ {
			if costFunc, ok := catalog.GoldCostByCategory[compDef.CompanionCategoryType]; ok {
				goldCost := costFunc.Evaluate(lvl)
				user.ConsumableItems[config.ConsumableItemIdForGold] -= goldCost
			}

			matKey := masterdata.CompanionLevelKey{CategoryType: compDef.CompanionCategoryType, Level: lvl}
			if mat, ok := catalog.MaterialsByKey[matKey]; ok {
				user.Materials[mat.MaterialId] -= mat.Count
			}
		}

		companion.Level = targetLevel
		companion.LatestVersion = nowMillis
		user.Companions[req.UserCompanionUuid] = companion
		if targetLevel > previousLevel {
			// "Enhance companions N times" counts every successful level-up,
			// including a multi-level enhancement request.
			ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{ConditionType: 12, TargetId: companion.CompanionId, Delta: targetLevel - previousLevel}, nowMillis)
		}
		log.Printf("[CompanionService] Enhance: companionId=%d level -> %d", companion.CompanionId, targetLevel)
	})
	if err != nil {
		return nil, fmt.Errorf("companion enhance: %w", err)
	}

	return &pb.CompanionEnhanceResponse{}, nil
}
