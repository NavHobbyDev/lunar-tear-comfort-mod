package service

import (
	"context"
	"fmt"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/runtime"
	"lunar-tear/server/internal/store"
)

type BannerServiceServer struct {
	pb.UnimplementedBannerServiceServer
	holder   *runtime.Holder
	users    store.UserRepository
	sessions store.SessionRepository
}

func NewBannerServiceServer(holder *runtime.Holder, users store.UserRepository, sessions store.SessionRepository) *BannerServiceServer {
	return &BannerServiceServer{holder: holder, users: users, sessions: sessions}
}

func (s *BannerServiceServer) GetMamaBanner(ctx context.Context, req *pb.GetMamaBannerRequest) (*pb.GetMamaBannerResponse, error) {
	catalog := s.holder.Get().GachaEntries
	nowMillis := gametime.NowMillis()

	userId := CurrentUserId(ctx, s.users, s.sessions)
	// GetMamaBanner fires every time the player returns to the Cage, and the
	// client uses it as a carrier for the Cage accumulators (distance walked,
	// Mama taps). Fold them into mission progress before serving banners.
	if cage := req.GetCageMeasurableValues(); cage != nil && (cage.RunningDistanceMeters > 0 || cage.MamaTappedCount > 0) {
		s.users.UpdateUser(userId, func(user *store.UserState) {
			applyCageMeasurableValues(user, s.holder.Get().Mission, cage, nowMillis)
		})
	}
	user, err := s.users.LoadUser(userId)
	if err != nil {
		return nil, fmt.Errorf("load user: %w", err)
	}

	var termLimited []*pb.GachaBanner
	var latestChapter *pb.GachaBanner
	for _, entry := range catalog {
		if !gachaActiveAt(entry, nowMillis) {
			continue
		}
		if entry.GachaLabelType == model.GachaLabelPortalCage || entry.GachaLabelType == model.GachaLabelRecycle {
			continue
		}
		if !isGachaVisible(entry.GachaId, user) {
			continue
		}
		b := &pb.GachaBanner{
			GachaLabelType: entry.GachaLabelType,
			GachaAssetName: entry.BannerAssetName,
			GachaId:        entry.GachaId,
		}
		switch entry.GachaLabelType {
		case model.GachaLabelEvent, model.GachaLabelPremium:
			termLimited = append(termLimited, b)
		case model.GachaLabelChapter:
			if latestChapter == nil || entry.GachaId > latestChapter.GachaId {
				latestChapter = b
			}
		}
	}
	return &pb.GetMamaBannerResponse{
		TermLimitedGacha:   termLimited,
		LatestChapterGacha: latestChapter,
		IsExistUnreadPop:   false,
	}, nil
}
