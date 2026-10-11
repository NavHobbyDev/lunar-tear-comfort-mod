package interceptor

import (
	"context"
	"fmt"

	"lunar-tear/server/internal/gametime"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TimeSync(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	resp, err := handler(ctx, req)
	switch info.FullMethod {
	case "/apb.api.user.UserService/Auth",
		"/apb.api.user.UserService/RegisterUser",
		"/apb.api.user.UserService/TransferUser":
	default:
		grpc.SetTrailer(ctx, metadata.Pairs(
			"x-apb-response-datetime", fmt.Sprintf("%d", gametime.NowMillis()),
		))
	}
	return resp, err
}
