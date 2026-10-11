package service

import (
	"context"

	"lunar-tear/server/internal/store"

	"google.golang.org/grpc/metadata"
)

func CurrentUserId(ctx context.Context, users store.UserRepository, sessions store.SessionRepository) int64 {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if vals := md.Get("x-apb-session-key"); len(vals) > 0 {
			if userId, err := sessions.ResolveUserId(vals[0]); err == nil {
				return userId
			}
		}
	}

	defaultId, _ := users.DefaultUserId()
	return defaultId
}
