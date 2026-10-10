package interceptor

import (
	"context"

	"lunar-tear/server/internal/store"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// NewSessionGuard rejects requests whose session key no longer resolves
// (expired or unknown). Without this guard the handlers silently fell back
// to the default account whenever a session expired, serving one player's
// data to another player's client (e.g. after the game clock advanced past
// the session TTL). Requests without a session key pass through untouched,
// and so do the methods that create sessions in the first place.
func NewSessionGuard(sessions store.SessionRepository) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if sessionExempt(info.FullMethod) {
			return handler(ctx, req)
		}
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return handler(ctx, req)
		}
		keys := md.Get("x-apb-session-key")
		if len(keys) == 0 || keys[0] == "" {
			return handler(ctx, req)
		}
		if _, err := sessions.ResolveUserId(keys[0]); err != nil {
			return nil, status.Error(codes.Unauthenticated, "session expired or unknown")
		}
		return handler(ctx, req)
	}
}

func sessionExempt(method string) bool {
	switch method {
	case "/apb.api.user.UserService/Auth",
		"/apb.api.user.UserService/RegisterUser",
		"/apb.api.user.UserService/TransferUser",
		"/apb.api.user.UserService/TransferUserByFacebook":
		return true
	}
	return false
}
