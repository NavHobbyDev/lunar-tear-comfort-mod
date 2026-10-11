package service

import (
	"context"
	"fmt"
	"log"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/store"
)

type DokanServiceServer struct {
	pb.UnimplementedDokanServiceServer
	users    store.UserRepository
	sessions store.SessionRepository
}

func NewDokanServiceServer(users store.UserRepository, sessions store.SessionRepository) *DokanServiceServer {
	return &DokanServiceServer{users: users, sessions: sessions}
}

func (s *DokanServiceServer) RegisterDokanConfirmed(ctx context.Context, req *pb.RegisterDokanConfirmedRequest) (*pb.RegisterDokanConfirmedResponse, error) {
	log.Printf("[DokanService] RegisterDokanConfirmed: dokanIds=%v", req.DokanId)

	userId := CurrentUserId(ctx, s.users, s.sessions)
	_, err := s.users.UpdateUser(userId, func(user *store.UserState) {
		for _, id := range req.DokanId {
			user.DokanConfirmed[id] = true
		}
	})
	if err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}

	return &pb.RegisterDokanConfirmedResponse{}, nil
}
