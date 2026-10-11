package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/runtime"
	"lunar-tear/server/internal/store"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

type LoginBonusServiceServer struct {
	pb.UnimplementedLoginBonusServiceServer
	users    store.UserRepository
	sessions store.SessionRepository
	snaps    store.SnapshotRepository
	holder   *runtime.Holder
}

func NewLoginBonusServiceServer(users store.UserRepository, sessions store.SessionRepository, snaps store.SnapshotRepository, holder *runtime.Holder) *LoginBonusServiceServer {
	return &LoginBonusServiceServer{users: users, sessions: sessions, snaps: snaps, holder: holder}
}

func (s *LoginBonusServiceServer) ReceiveStamp(ctx context.Context, req *emptypb.Empty) (*pb.ReceiveStampResponse, error) {
	log.Printf("[LoginBonusService] ReceiveStamp")

	userId := CurrentUserId(ctx, s.users, s.sessions)

	if user, err := s.users.LoadUser(userId); err == nil {
		if user.LoginBonus.LatestRewardReceiveDatetime >= gametime.StartOfDayMillis() {
			log.Printf("[LoginBonusService] already claimed today, skipping (no write)")
			return &pb.ReceiveStampResponse{}, nil
		}
	}

	catalog := s.holder.Get().LoginBonus

	var alreadyClaimed bool
	_, err := s.users.UpdateUser(userId, func(u *store.UserState) {
		// Convert any existing stamina to gold on first login bonus claim
		store.ConvertStaminaToGold(u)
		// Convert any leftover summon tickets to free gems on first login bonus claim
		store.ConvertSummonTicketsToGems(u)

		// Same-day protection INSIDE the update to prevent race conditions.
		// The client auto-calls ReceiveStamp after certain quest completions
		// (e.g. tutorial quests), so without this guard each call would
		// incorrectly advance the login-day counter.
		if u.LoginBonus.LatestRewardReceiveDatetime >= gametime.StartOfDayMillis() {
			alreadyClaimed = true
			return
		}

		nextPage, nextStamp, reward, err := resolveNextStamp(catalog, u.LoginBonus)
		if err != nil {
			log.Printf("[LoginBonusService] resolveNextStamp error: %v", err)
			alreadyClaimed = true
			return
		}

		log.Printf("[LoginBonusService] bonusId=%d page %d->%d stamp %d->%d",
			u.LoginBonus.LoginBonusId, u.LoginBonus.CurrentPageNumber, nextPage,
			u.LoginBonus.CurrentStampNumber, nextStamp)

		now := gametime.NowMillis()

		u.AddGift(store.NotReceivedGiftState{
			GiftCommon: store.GiftCommonState{
				PossessionType: reward.PossessionType,
				PossessionId:   reward.PossessionId,
				Count:          reward.Count,
				GrantDatetime:  now,
			},
			ExpirationDatetime: now + int64(30*24*time.Hour/time.Millisecond),
			UserGiftUuid:       uuid.New().String(),
		})

		u.LoginBonus.CurrentPageNumber = nextPage
		u.LoginBonus.CurrentStampNumber = nextStamp
		u.LoginBonus.LatestRewardReceiveDatetime = now
		u.LoginBonus.LatestVersion = now

		ApplyMissionProgressEvent(u, s.holder.Get().Mission, MissionProgressEvent{ConditionType: missionConditionLogin, Delta: 1}, now)
		ApplyMissionProgressEvent(u, s.holder.Get().Mission, MissionProgressEvent{ConditionType: missionConditionLoginTotalDays, Delta: 1}, now)

		// Claim BigHunt daily and weekly rewards on login
		ApplyBigHuntRewards(u, s.holder.Get(), now)
		// Labyrinth season tier: daily mail for current best cleared milestone
		ApplyLabyrinthSeasonRewards(u, s.holder.Get(), now)

	})
	if err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}

	if alreadyClaimed {
		log.Printf("[LoginBonusService] already claimed today, skipping")
	} else {
		// Every claimed login-bonus day doubles as a simulated arena day:
		// 20 honest battles (outcomes by deck power, ±25% contested band),
		// arena missions and per-match/weekly rewards advance together with
		// the stamp. Guarded to run at most once per day and only once the
		// arena is open for the player (checked inside).
		RunPvpAutoProgress(s.users, s.snaps, s.holder, userId)
	}

	return &pb.ReceiveStampResponse{}, nil
}

func resolveNextStamp(catalog *masterdata.LoginBonusCatalog, lb store.UserLoginBonusState) (nextPage, nextStamp int32, reward masterdata.LoginBonusReward, err error) {
	bonusId := lb.LoginBonusId
	curPage := lb.CurrentPageNumber
	curStamp := lb.CurrentStampNumber

	nextPage = curPage
	nextStamp = curStamp + 1
	var ok bool
	reward, ok = catalog.LookupStampReward(bonusId, nextPage, nextStamp)
	if !ok {
		nextPage = curPage + 1
		nextStamp = 1
		total := catalog.TotalPageCount(bonusId)
		if total > 0 && nextPage > total {
			err = status.Errorf(codes.FailedPrecondition,
				"login bonus %d exhausted (page %d stamp %d is the last)",
				bonusId, curPage, curStamp)
			return
		}
		reward, ok = catalog.LookupStampReward(bonusId, nextPage, nextStamp)
		if !ok {
			err = status.Errorf(codes.FailedPrecondition,
				"no reward found for login bonus %d page %d stamp %d",
				bonusId, nextPage, nextStamp)
			return
		}
	}
	return
}
