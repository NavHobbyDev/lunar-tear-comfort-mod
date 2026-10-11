package service

import (
	"context"
	"fmt"
	"log"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/store"
)

type NaviCutInServiceServer struct {
	pb.UnimplementedNaviCutInServiceServer
	users    store.UserRepository
	sessions store.SessionRepository
}

func NewNaviCutInServiceServer(users store.UserRepository, sessions store.SessionRepository) *NaviCutInServiceServer {
	return &NaviCutInServiceServer{users: users, sessions: sessions}
}

func (s *NaviCutInServiceServer) RegisterPlayed(ctx context.Context, req *pb.RegisterPlayedRequest) (*pb.RegisterPlayedResponse, error) {
	log.Printf("[NaviCutInService] RegisterPlayed: naviCutId=%d", req.NaviCutId)

	userId := CurrentUserId(ctx, s.users, s.sessions)
	_, err := s.users.UpdateUser(userId, func(user *store.UserState) {
		user.NaviCutInPlayed[req.NaviCutId] = true
	})
	if err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}

	return &pb.RegisterPlayedResponse{}, nil
}
