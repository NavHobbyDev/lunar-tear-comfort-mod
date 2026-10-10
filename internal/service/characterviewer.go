package service

import (
	"context"
	"log"
	"time"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/runtime"
	"lunar-tear/server/internal/store"

	"google.golang.org/protobuf/types/known/emptypb"
)

type CharacterViewerServiceServer struct {
	pb.UnimplementedCharacterViewerServiceServer
	users    store.UserRepository
	sessions store.SessionRepository
	holder   *runtime.Holder
	viewer   *ViewerSessions
}

func NewCharacterViewerServiceServer(users store.UserRepository, sessions store.SessionRepository, holder *runtime.Holder, viewer *ViewerSessions) *CharacterViewerServiceServer {
	return &CharacterViewerServiceServer{users: users, sessions: sessions, holder: holder, viewer: viewer}
}

func (s *CharacterViewerServiceServer) CharacterViewerTop(ctx context.Context, _ *emptypb.Empty) (*pb.CharacterViewerTopResponse, error) {
	log.Printf("[CharacterViewerService] CharacterViewerTop")

	userId := CurrentUserId(ctx, s.users, s.sessions)

	// Opening the viewer top screen marks the session so the preview's StartWave
	// (which has no active quest) is accepted instead of ejecting the player.
	s.viewer.Mark(userId)

	// Get newly released locations and mark them as viewed
	var newlyReleased []int32
	_, err := s.users.UpdateUser(userId, func(user *store.UserState) {
		newlyReleased = s.holder.Get().CharacterViewer.NewlyReleasedFieldIds(*user)

		if len(newlyReleased) > 0 {
			if user.CharacterViewerFields == nil {
				user.CharacterViewerFields = make(map[int32]store.CharacterViewerFieldState)
			}
			nowMillis := time.Now().UnixMilli()
			for _, fieldId := range newlyReleased {
				field := user.CharacterViewerFields[fieldId]
				field.CharacterViewerFieldId = fieldId
				field.IsViewed = true
				field.ViewedDatetime = nowMillis
				field.LatestVersion = 0
				user.CharacterViewerFields[fieldId] = field
			}
			log.Printf("[CharacterViewerService] Marked %d fields as viewed", len(newlyReleased))
		}
	})
	if err != nil {
		log.Printf("[CharacterViewerService] Error updating user: %v", err)
	}

	log.Printf("[CharacterViewerService] Returning %d new fields for user %d", len(newlyReleased), userId)

	return &pb.CharacterViewerTopResponse{
		ReleaseCharacterViewerFieldId: newlyReleased,
	}, nil
}
