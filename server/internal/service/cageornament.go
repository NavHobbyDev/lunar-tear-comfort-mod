package service

import (
	"context"
	"log"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/runtime"
	"lunar-tear/server/internal/store"
)

type CageOrnamentServiceServer struct {
	pb.UnimplementedCageOrnamentServiceServer
	users    store.UserRepository
	sessions store.SessionRepository
	holder   *runtime.Holder
}

func NewCageOrnamentServiceServer(users store.UserRepository, sessions store.SessionRepository, holder *runtime.Holder) *CageOrnamentServiceServer {
	return &CageOrnamentServiceServer{users: users, sessions: sessions, holder: holder}
}

func (s *CageOrnamentServiceServer) ReceiveReward(ctx context.Context, req *pb.ReceiveRewardRequest) (*pb.ReceiveRewardResponse, error) {
	log.Printf("[CageOrnamentService] ReceiveReward: cageOrnamentId=%d", req.CageOrnamentId)

	cat := s.holder.Get()
	reward, ok := cat.CageOrnament.LookupReward(req.CageOrnamentId)

	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()
	s.users.UpdateUser(userId, func(user *store.UserState) {
		user.CageOrnamentRewards[req.CageOrnamentId] = store.CageOrnamentRewardState{
			CageOrnamentId:      req.CageOrnamentId,
			AcquisitionDatetime: nowMillis,
			LatestVersion:       nowMillis,
		}
		if ok {
			cat.QuestHandler.Granter.GrantFull(user, model.PossessionType(reward.PossessionType), reward.PossessionId, reward.Count, nowMillis)
		}
	})

	if !ok {
		// "Fickle Black Birds" (type-1 gimmicks) tap into this RPC with CageOrnamentIds
		// not present in m_cage_ornament_reward (their GimmickOrnamentViewIds are 101/103,
		// not the 1002xxx-style ids the table uses). Record the access and return an empty
		// reward so the client doesn't hang and the server doesn't crash.
		log.Printf("[CageOrnamentService] ReceiveReward: no reward mapping for cageOrnamentId=%d, returning empty",
			req.CageOrnamentId)
		return &pb.ReceiveRewardResponse{}, nil
	}

	return &pb.ReceiveRewardResponse{
		CageOrnamentReward: []*pb.CageOrnamentReward{
			{
				PossessionType: reward.PossessionType,
				PossessionId:   reward.PossessionId,
				Count:          reward.Count,
			},
		},
	}, nil
}

func (s *CageOrnamentServiceServer) RecordAccess(ctx context.Context, req *pb.RecordAccessRequest) (*pb.RecordAccessResponse, error) {
	log.Printf("[CageOrnamentService] RecordAccess: cageOrnamentId=%d", req.CageOrnamentId)

	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()
	s.users.UpdateUser(userId, func(user *store.UserState) {
		access := user.CageOrnamentAccesses[req.CageOrnamentId]
		access.CageOrnamentId = req.CageOrnamentId
		if access.FirstAccessDatetime == 0 {
			access.FirstAccessDatetime = nowMillis
		}
		access.LatestAccessDatetime = nowMillis
		access.LatestVersion = nowMillis
		user.CageOrnamentAccesses[req.CageOrnamentId] = access

		// S2 photo spots: gallery reads rewards
		if _, exists := user.CageOrnamentRewards[req.CageOrnamentId]; !exists {
			user.CageOrnamentRewards[req.CageOrnamentId] = store.CageOrnamentRewardState{
				CageOrnamentId:      req.CageOrnamentId,
				AcquisitionDatetime: nowMillis,
				LatestVersion:       nowMillis,
			}
		}
	})

	return &pb.RecordAccessResponse{}, nil
}
