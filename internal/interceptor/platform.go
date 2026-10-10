package interceptor

import (
	"context"

	"lunar-tear/server/internal/model"

	"google.golang.org/grpc"
)

func Platform(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	p := model.ClientPlatformFromHeaders(ctx)
	ctx = model.NewContextWithPlatform(ctx, p)
	return handler(ctx, req)
}
