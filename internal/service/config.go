package service

import (
	"context"
	"log"

	pb "lunar-tear/server/gen/proto"

	"google.golang.org/protobuf/types/known/emptypb"
)

type ConfigServiceServer struct {
	pb.UnimplementedConfigServiceServer
	GrpcHost string
	GrpcPort int32
	OctoURL  string
}

func NewConfigServiceServer(host string, port int32, octoURL string) *ConfigServiceServer {
	return &ConfigServiceServer{GrpcHost: host, GrpcPort: port, OctoURL: octoURL}
}

func (s *ConfigServiceServer) GetReviewServerConfig(ctx context.Context, _ *emptypb.Empty) (*pb.GetReviewServerConfigResponse, error) {
	log.Printf("[ConfigService] GetReviewServerConfig -> %s:%d", s.GrpcHost, s.GrpcPort)

	return &pb.GetReviewServerConfigResponse{
		Api: &pb.ApiConfig{
			Hostname: s.GrpcHost,
			Port:     s.GrpcPort,
		},
		Octo: &pb.OctoConfig{
			Version:         1,
			AppId:           1,
			ClientSecretKey: "secret",
			AesKey:          "aeskey",
			Url:             s.OctoURL,
		},
		WebView: &pb.WebViewConfig{
			BaseUrl: s.OctoURL,
		},
		MasterData: &pb.MasterDataConfig{
			UrlFormat: s.OctoURL + "/master-data/%s",
		},
	}, nil
}
