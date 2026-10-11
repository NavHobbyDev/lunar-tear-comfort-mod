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
)

type SideStoryQuestServiceServer struct {
	pb.UnimplementedSideStoryQuestServiceServer
	users    store.UserRepository
	sessions store.SessionRepository
	holder   *runtime.Holder
}

func NewSideStoryQuestServiceServer(users store.UserRepository, sessions store.SessionRepository, holder *runtime.Holder) *SideStoryQuestServiceServer {
	return &SideStoryQuestServiceServer{users: users, sessions: sessions, holder: holder}
}

func sideStoryClearedCount(info *masterdata.SideStoryQuestInfo, user *store.UserState) int {
	cleared := 0
	for _, questId := range info.Quests {
		if user.QuestLimitContentStatus[questId].LimitContentQuestStatusType == 1 {
			cleared++
		}
	}
	return cleared
}

func sideStoryQuestCleared(info *masterdata.SideStoryQuestInfo, user *store.UserState) bool {
	return info != nil && len(info.Quests) > 0 && sideStoryClearedCount(info, user) == len(info.Quests)
}

func sideStoryNextSceneAfterBattle(info *masterdata.SideStoryQuestInfo, user *store.UserState) (int32, bool) {
	cleared := sideStoryClearedCount(info, user)
	if cleared == 0 {
		return 0, false
	}
	total := len(info.Quests)
	var sceneType model.SideStorySceneIdType
	switch {
	case cleared >= total:
		sceneType = model.SideStorySceneOutroduction
	case cleared == total-1:
		sceneType = model.SideStorySceneUnlockLastQuest
	default:
		sceneType = model.SideStoryScenePlayLastQuest
	}
	return info.SceneIdByType(sceneType)
}

func applySideStoryProgressState(progress *store.SideStoryQuestProgress, info *masterdata.SideStoryQuestInfo, user *store.UserState) {
	if sideStoryQuestCleared(info, user) {
		progress.SideStoryQuestStateType = model.SideStoryQuestStateCleared
	} else if progress.SideStoryQuestStateType == model.SideStoryQuestStateUnknown {
		progress.SideStoryQuestStateType = model.SideStoryQuestStateActive
	}
}

func setSideStoryActive(user *store.UserState, questId, sceneId int32, nowMillis int64) {
	user.SideStoryActiveProgress = store.SideStoryActiveProgress{
		CurrentSideStoryQuestId:      questId,
		CurrentSideStoryQuestSceneId: sceneId,
		LatestVersion:                nowMillis,
	}
}

func setSideStoryScene(user *store.UserState, info *masterdata.SideStoryQuestInfo, questId, sceneId int32, nowMillis int64) {
	progress := user.SideStoryQuests[questId]
	progress.HeadSideStoryQuestSceneId = sceneId
	applySideStoryProgressState(&progress, info, user)
	progress.LatestVersion = nowMillis
	user.SideStoryQuests[questId] = progress
	setSideStoryActive(user, questId, sceneId, nowMillis)
}

func (s *SideStoryQuestServiceServer) MoveSideStoryQuestProgress(ctx context.Context, req *pb.MoveSideStoryQuestRequest) (*pb.MoveSideStoryQuestResponse, error) {
	log.Printf("[SideStoryQuestService] MoveSideStoryQuestProgress: sideStoryQuestId=%d", req.SideStoryQuestId)

	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()
	info := s.holder.Get().SideStory.QuestById[req.SideStoryQuestId]

	s.users.UpdateUser(userId, func(user *store.UserState) {
		if info == nil || len(info.Quests) == 0 {
			log.Printf("[SideStoryQuestService] unknown sideStoryQuestId=%d, skipping", req.SideStoryQuestId)
			return
		}

		existing, exists := user.SideStoryQuests[req.SideStoryQuestId]

		var scene int32
		var ok bool
		if !exists || existing.HeadSideStoryQuestSceneId == 0 {
			scene, ok = info.SceneIdByType(model.SideStorySceneIntroduction)
		} else {
			scene, ok = sideStoryNextSceneAfterBattle(info, user)
			if !ok {
				scene, ok = existing.HeadSideStoryQuestSceneId, true
			}
		}
		if !ok {
			return
		}
		setSideStoryScene(user, info, req.SideStoryQuestId, scene, nowMillis)
	})

	return &pb.MoveSideStoryQuestResponse{}, nil
}

func (s *SideStoryQuestServiceServer) UpdateSideStoryQuestSceneProgress(ctx context.Context, req *pb.UpdateSideStoryQuestSceneProgressRequest) (*pb.UpdateSideStoryQuestSceneProgressResponse, error) {
	log.Printf("[SideStoryQuestService] UpdateSideStoryQuestSceneProgress: sideStoryQuestId=%d sceneId=%d",
		req.SideStoryQuestId, req.SideStoryQuestSceneId)

	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()
	info := s.holder.Get().SideStory.QuestById[req.SideStoryQuestId]

	s.users.UpdateUser(userId, func(user *store.UserState) {
		setSideStoryScene(user, info, req.SideStoryQuestId, req.SideStoryQuestSceneId, nowMillis)
	})

	return &pb.UpdateSideStoryQuestSceneProgressResponse{}, nil
}
