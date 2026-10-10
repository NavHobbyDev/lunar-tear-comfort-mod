package service

import (
	"context"
	"log"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/questflow"
	"lunar-tear/server/internal/runtime"
	"lunar-tear/server/internal/store"
)

type TutorialServiceServer struct {
	pb.UnimplementedTutorialServiceServer
	users    store.UserRepository
	sessions store.SessionRepository
	snaps    store.SnapshotRepository
	holder   *runtime.Holder
}

func NewTutorialServiceServer(users store.UserRepository, sessions store.SessionRepository, snaps store.SnapshotRepository, holder *runtime.Holder) *TutorialServiceServer {
	return &TutorialServiceServer{users: users, sessions: sessions, snaps: snaps, holder: holder}
}

func (s *TutorialServiceServer) SetTutorialProgress(ctx context.Context, req *pb.SetTutorialProgressRequest) (*pb.SetTutorialProgressResponse, error) {
	log.Printf("[TutorialService] SetTutorialProgress: type=%d phase=%d choice=%d", req.TutorialType, req.ProgressPhase, req.ChoiceId)
	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()
	engine := s.holder.Get().QuestHandler
	var grants []questflow.RewardGrant
	s.users.UpdateUser(userId, func(user *store.UserState) {
		existing, exists := user.Tutorials[req.TutorialType]
		if !exists || req.ProgressPhase >= existing.ProgressPhase {
			user.Tutorials[req.TutorialType] = store.TutorialProgressState{
				TutorialType:  req.TutorialType,
				ProgressPhase: req.ProgressPhase,
				ChoiceId:      req.ChoiceId,
			}
		}
		grants = engine.ApplyTutorialReward(user, model.TutorialType(req.TutorialType), req.ChoiceId, nowMillis)
		if req.TutorialType == int32(model.TutorialTypeMenuFirst) && req.ProgressPhase == 20 {
			store.EnsureDefaultDeck(user, nowMillis)
		}
	})

	rewards := make([]*pb.TutorialChoiceReward, len(grants))
	for i, g := range grants {
		rewards[i] = &pb.TutorialChoiceReward{
			PossessionType: int32(g.PossessionType),
			PossessionId:   g.PossessionId,
			Count:          g.Count,
		}
	}
	s.kickPvpAutoProgressOnTutorial(req.TutorialType, userId)
	return &pb.SetTutorialProgressResponse{
		TutorialChoiceReward: rewards,
	}, nil
}

func (s *TutorialServiceServer) SetTutorialProgressAndReplaceDeck(ctx context.Context, req *pb.SetTutorialProgressAndReplaceDeckRequest) (*pb.SetTutorialProgressAndReplaceDeckResponse, error) {
	log.Printf("[TutorialService] SetTutorialProgressAndReplaceDeck: type=%d phase=%d deckType=%d deckNumber=%d", req.TutorialType, req.ProgressPhase, req.DeckType, req.UserDeckNumber)
	userId := CurrentUserId(ctx, s.users, s.sessions)
	s.users.UpdateUser(userId, func(user *store.UserState) {
		existing, exists := user.Tutorials[req.TutorialType]
		if !exists || req.ProgressPhase >= existing.ProgressPhase {
			user.Tutorials[req.TutorialType] = store.TutorialProgressState{
				TutorialType:  req.TutorialType,
				ProgressPhase: req.ProgressPhase,
			}
		}
		if req.Deck != nil {
			store.ApplyDeckReplacement(user, model.DeckType(req.DeckType), req.UserDeckNumber, deckSlotsFromProto(req.Deck), gametime.NowMillis())
		}
	})
	s.kickPvpAutoProgressOnTutorial(req.TutorialType, userId)
	return &pb.SetTutorialProgressAndReplaceDeckResponse{}, nil
}

// kickPvpAutoProgressOnTutorial fires the first simulated arena day the moment
// the arena tutorial (type 11) is reported — not at the next login bonus. The
// client reports the tutorial as soon as it plays, so the arena comes alive in
// the same session it opens. RunPvpAutoProgress is idempotent per day
// (PvpState.LastAutoProgressDay), so repeated tutorial-phase reports within the
// same day are no-ops, exactly like the login-bonus trigger.
func (s *TutorialServiceServer) kickPvpAutoProgressOnTutorial(tutorialType int32, userId int64) {
	if tutorialType != int32(model.TutorialTypePvp) {
		return
	}
	RunPvpAutoProgress(s.users, s.snaps, s.holder, userId)
}
