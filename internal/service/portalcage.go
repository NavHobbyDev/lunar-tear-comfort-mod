package service

import (
	"context"
	"log"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
)

type PortalCageServiceServer struct {
	pb.UnimplementedPortalCageServiceServer
	users    store.UserRepository
	sessions store.SessionRepository
}

func NewPortalCageServiceServer(users store.UserRepository, sessions store.SessionRepository) *PortalCageServiceServer {
	return &PortalCageServiceServer{users: users, sessions: sessions}
}

func (s *PortalCageServiceServer) UpdatePortalCageSceneProgress(ctx context.Context, req *pb.UpdatePortalCageSceneProgressRequest) (*pb.UpdatePortalCageSceneProgressResponse, error) {
	log.Printf("[PortalCageService] UpdatePortalCageSceneProgress: portalCageSceneId=%d", req.PortalCageSceneId)

	userId := CurrentUserId(ctx, s.users, s.sessions)
	s.users.UpdateUser(userId, func(user *store.UserState) {
		now := gametime.NowMillis()
		user.PortalCageStatus.IsCurrentProgress = true
		user.PortalCageStatus.LatestVersion = now
		// Mama's Room ends any active replay — flip flow type to MainFlow;
		// ReplayFlow* stay sticky (matches original userdata snapshot).
		if user.MainQuest.CurrentQuestFlowType != int32(model.QuestFlowTypeMainFlow) {
			user.MainQuest.CurrentQuestFlowType = int32(model.QuestFlowTypeMainFlow)
			user.MainQuest.LatestVersion = now
		}
		// Returning to Mama's Room also ends any active side story.
		if user.SideStoryActiveProgress.CurrentSideStoryQuestId != 0 {
			user.SideStoryActiveProgress = store.SideStoryActiveProgress{
				LatestVersion: now,
			}
		}
	})
	return &pb.UpdatePortalCageSceneProgressResponse{}, nil
}
