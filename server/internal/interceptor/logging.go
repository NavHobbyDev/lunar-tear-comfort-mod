package interceptor

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// quietMethods are high-frequency RPCs fired in bursts (e.g. one UpdateSequence
// per gimmick on map load -- hundreds to ~1000 calls). The per-call log lines
// dominate map-load time (3 synchronous stdout writes per call, far more than
// the ~0s handler), so these methods are not logged.
var quietMethods = map[string]bool{
	"/apb.api.gimmick.GimmickService/UpdateSequence": true,
}

func Logging(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	if quietMethods[info.FullMethod] {
		return handler(ctx, req)
	}
	log.Printf(">>> %s", info.FullMethod)
	start := time.Now()
	resp, err := handler(ctx, req)
	elapsed := time.Since(start)
	if err != nil {
		log.Printf("<<< %s ERROR (%s): %v", info.FullMethod, elapsed, err)
	} else {
		size := 0
		if msg, ok := resp.(proto.Message); ok {
			size = proto.Size(msg)
		}
		log.Printf("<<< %s OK (%s, resp %d bytes)", info.FullMethod, elapsed, size)
	}
	return resp, err
}

func UnknownService(_ any, stream grpc.ServerStream) error {
	fullMethod, ok := grpc.MethodFromServerStream(stream)
	if !ok {
		fullMethod = "<unknown>"
	}
	log.Printf(">>> %s", fullMethod)
	err := status.Errorf(codes.Unimplemented, "unknown service or method %s", fullMethod)
	log.Printf("<<< %s ERROR: %v", fullMethod, err)
	return err
}
