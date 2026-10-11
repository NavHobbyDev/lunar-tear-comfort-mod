package service

import (
	"context"
	"log"

	"github.com/google/uuid"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/runtime"
	"lunar-tear/server/internal/store"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// ApplyBigHuntRewards mails the BigHunt season (weekly attribute ranking)
// reward on login.
//
// Each of the five bosses belongs to one of the five season attribute
// rankings. The master-data seasons never rotate on this server (their end
// datetimes were pushed decades into the future), so instead of one payout
// per season the season reward is paid out once per day: for every boss the
// reward of the player's current season rank is posted to the gift box. The
// score is the player's season best for that boss (the persisted schedule
// max score) — it never ages out of a weekly window, and the total mailed is
// exactly the "total season reward" the client displays: one top-tier reward
// per boss, never the sum of every lower tier.
func ApplyBigHuntRewards(user *store.UserState, cat *runtime.Catalogs, nowMillis int64) {
	bhCatalog := cat.BigHunt
	weeklyVersion := gametime.WeeklyVersion(nowMillis)
	dayVersion := gametime.StartOfDayMillisAt(nowMillis)

	ws := user.BigHuntWeeklyStatuses[weeklyVersion]
	if ws.LatestVersion >= dayVersion {
		return // today's season rewards were already mailed
	}

	var mailed []masterdata.RewardItem
	for _, boss := range bigHuntBossesSorted(bhCatalog) {
		rewardGroupId := bhCatalog.ResolveActiveWeeklyRewardGroupIdByAttr(boss.AttributeType, nowMillis)
		if rewardGroupId == 0 {
			continue
		}
		score := bigHuntBossSeasonScore(user, bhCatalog, boss.BigHuntBossId)
		if score <= 0 {
			continue
		}
		mailed = append(mailed, bhCatalog.CollectHighestReward(rewardGroupId, score)...)
	}

	if len(mailed) > 0 {
		mailBigHuntRewardItems(user, mailed, nowMillis)
		log.Printf("[ApplyBigHuntRewards] mailed %d season rank reward row(s) for weekly version %d", len(mailed), weeklyVersion)
	} else {
		log.Printf("[ApplyBigHuntRewards] no season rank reward to mail for weekly version %d (no score yet)", weeklyVersion)
	}

	ws.IsReceivedWeeklyReward = true
	ws.LatestVersion = nowMillis
	user.BigHuntWeeklyStatuses[weeklyVersion] = ws
}

// mailBigHuntRewardItems posts reward items to the gift box (the in-game
// mail), merging counts per (possession type, id) first. Existing unreceived
// mails carrying the same items are consolidated (removed and summed into the
// new ones) to prevent mailbox clutter.
func mailBigHuntRewardItems(user *store.UserState, items []masterdata.RewardItem, nowMillis int64) {
	merged := mergeRewardItems(items)
	if len(merged) == 0 {
		return
	}

	// Build key set for consolidation
	keys := make(map[[2]int32]bool, len(merged))
	for _, item := range merged {
		keys[[2]int32{item.PossessionType, item.PossessionId}] = true
	}
	pending := user.ConsolidateGifts(keys)

	expiry := nowMillis + pvpGiftExpiryMillis
	gifts := make([]store.NotReceivedGiftState, 0, len(merged))
	for _, item := range merged {
		key := [2]int32{item.PossessionType, item.PossessionId}
		gifts = append(gifts, store.NotReceivedGiftState{
			GiftCommon: store.GiftCommonState{
				PossessionType: item.PossessionType,
				PossessionId:   item.PossessionId,
				Count:          item.Count + pending[key],
				GrantDatetime:  nowMillis,
			},
			ExpirationDatetime: expiry,
			UserGiftUuid:       uuid.New().String(),
		})
	}
	user.AddGiftsOrdered(gifts)
}

// ApplyLabyrinthSeasonRewards mails the labyrinth season-tier reward on login,
// once per chapter per day — same pattern as ApplyBigHuntRewards.
//
// For each labyrinth chapter the player's highest cleared season milestone
// (by HeadStageOrder / cleared HeadQuestId) is resolved and that tier's
// rewards are posted to the gift box. Lower tiers are not summed; only the
// best reached position is mailed, matching the BigHunt "highest reward"
// behaviour. Master-data seasons on this server typically do not rotate, so
// the active chapter milestone ladder is used rather than an ended-season
// settlement.
func ApplyLabyrinthSeasonRewards(user *store.UserState, cat *runtime.Catalogs, nowMillis int64) {
	laby := cat.Labyrinth
	if laby == nil || len(laby.ChaptersByOrder) == 0 {
		return
	}
	dayVersion := gametime.StartOfDayMillisAt(nowMillis)

	var mailed []masterdata.RewardItem
	chaptersMailed := 0
	for _, chapter := range laby.ChaptersByOrder {
		chapterId := chapter.EventQuestChapterId
		state := user.LabyrinthSeasons[chapterId]
		if state.LatestVersion >= dayVersion {
			continue // already mailed today's reward for this chapter
		}

		milestones := laby.SeasonMilestones(chapterId)
		var earned masterdata.LabyrinthSeasonMilestone
		hasEarned := false
		for _, m := range milestones {
			quest, ok := user.Quests[m.HeadQuestId]
			if ok && quest.QuestStateType == model.UserQuestStateTypeCleared {
				if !hasEarned || m.HeadStageOrder > earned.HeadStageOrder {
					earned = m
					hasEarned = true
				}
			}
		}

		state.EventQuestChapterId = chapterId
		if state.LastJoinSeasonNumber == 0 {
			state.LastJoinSeasonNumber = chapter.LatestSeasonNumber
		}
		state.LatestVersion = nowMillis
		user.LabyrinthSeasons[chapterId] = state

		if hasEarned {
			mailed = append(mailed, earned.Rewards...)
			chaptersMailed++
		}
	}

	if len(mailed) > 0 {
		mailBigHuntRewardItems(user, mailed, nowMillis)
		log.Printf("[ApplyLabyrinthSeasonRewards] mailed %d reward row(s) across %d chapter(s)", len(mailed), chaptersMailed)
	} else {
		log.Printf("[ApplyLabyrinthSeasonRewards] no labyrinth season reward to mail (no cleared milestones or already claimed today)")
	}
}

type RewardServiceServer struct {
	pb.UnimplementedRewardServiceServer
	users    store.UserRepository
	sessions store.SessionRepository
	holder   *runtime.Holder
}

func NewRewardServiceServer(
	users store.UserRepository,
	sessions store.SessionRepository,
	holder *runtime.Holder,
) *RewardServiceServer {
	return &RewardServiceServer{users: users, sessions: sessions, holder: holder}
}

func (s *RewardServiceServer) ReceiveBigHuntReward(ctx context.Context, _ *emptypb.Empty) (*pb.ReceiveBigHuntRewardResponse, error) {
	log.Printf("[RewardService] ReceiveBigHuntReward")

	cat := s.holder.Get()
	bhCatalog := cat.BigHunt
	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()
	weeklyVersion := gametime.WeeklyVersion(nowMillis)

	var weeklyScoreResults []*pb.WeeklyScoreResult
	var weeklyRewards []*pb.BigHuntReward
	var lastWeekRewards []*pb.BigHuntReward
	var isReceived bool

	s.users.UpdateUser(userId, func(user *store.UserState) {
		// The season rank rewards themselves are mailed automatically on
		// login (ApplyBigHuntRewards); this RPC only builds the client popup
		// data and never grants items again — that would duplicate the mailed
		// reward.
		for _, boss := range bigHuntBossesSorted(bhCatalog) {
			key := store.BigHuntWeeklyScoreKey{
				BigHuntWeeklyVersion: weeklyVersion,
				AttributeType:        boss.AttributeType,
			}
			wms := user.BigHuntWeeklyMaxScores[key]
			gradeIcon := bhCatalog.ResolveGradeIconId(boss.BigHuntBossId, wms.MaxScore)
			weeklyScoreResults = append(weeklyScoreResults, &pb.WeeklyScoreResult{
				AttributeType:           boss.AttributeType,
				BeforeMaxScore:          wms.MaxScore,
				CurrentMaxScore:         wms.MaxScore,
				BeforeAssetGradeIconId:  gradeIcon,
				CurrentAssetGradeIconId: gradeIcon,
				AfterMaxScore:           wms.MaxScore,
				AfterAssetGradeIconId:   gradeIcon,
			})

			rewardGroupId := bhCatalog.ResolveActiveWeeklyRewardGroupIdByAttr(boss.AttributeType, nowMillis)
			if rewardGroupId == 0 {
				continue
			}
			score := bigHuntBossSeasonScore(user, bhCatalog, boss.BigHuntBossId)
			for _, item := range bhCatalog.CollectHighestReward(rewardGroupId, score) {
				weeklyRewards = append(weeklyRewards, &pb.BigHuntReward{
					PossessionType: item.PossessionType,
					PossessionId:   item.PossessionId,
					Count:          item.Count,
				})
			}
		}
		lastWeekRewards = resolveBigHuntSeasonRewards(bhCatalog, *user, nowMillis)

		ws := user.BigHuntWeeklyStatuses[weeklyVersion]
		isReceived = ws.IsReceivedWeeklyReward
	})

	if weeklyRewards == nil {
		weeklyRewards = []*pb.BigHuntReward{}
	}
	if weeklyScoreResults == nil {
		weeklyScoreResults = []*pb.WeeklyScoreResult{}
	}

	return &pb.ReceiveBigHuntRewardResponse{
		WeeklyScoreResult:           weeklyScoreResults,
		WeeklyScoreReward:           weeklyRewards,
		IsReceivedWeeklyScoreReward: isReceived,
		LastWeekWeeklyScoreReward:   lastWeekRewards,
	}, nil
}

func (s *RewardServiceServer) ReceivePvpReward(ctx context.Context, _ *emptypb.Empty) (*pb.ReceivePvpRewardResponse, error) {
	log.Printf("[RewardService] ReceivePvpReward")

	userId := CurrentUserId(ctx, s.users, s.sessions)

	// The grade weekly reward popup data is snapshotted once per week by
	// advanceWeeklyCycle; this RPC is only an optional client ack. It clears
	// a still-pending announcement and echoes its result for the popup, but
	// never grants items — the arena reward tabs are mailed daily by
	// RunPvpAutoProgress.
	var weeklyResult *pb.WeeklyGradeResult
	s.users.UpdateUser(userId, func(user *store.UserState) {
		groupId := user.Pvp.PendingWeeklyRewardGroupId
		if groupId == 0 {
			return
		}
		weeklyResult = &pb.WeeklyGradeResult{
			TargetSeasonId:              user.Pvp.PendingWeeklyRewardSeasonId,
			PvpPoint:                    user.Pvp.PendingWeeklyRewardPoint,
			PvpGradeWeeklyRewardGroupId: groupId,
		}
		user.Pvp.PendingWeeklyRewardGroupId = 0
		user.Pvp.PendingWeeklyRewardPoint = 0
		user.Pvp.PendingWeeklyRewardSeasonId = 0
	})

	return &pb.ReceivePvpRewardResponse{
		WeeklyGradeResult: weeklyResult,
		DiffUserData:      map[string]*pb.DiffData{},
	}, nil
}

func (s *RewardServiceServer) ReceiveLabyrinthSeasonReward(ctx context.Context, _ *emptypb.Empty) (*pb.ReceiveLabyrinthSeasonRewardResponse, error) {
	log.Printf("[RewardService] ReceiveLabyrinthSeasonReward (stub)")
	return &pb.ReceiveLabyrinthSeasonRewardResponse{
		DiffUserData: map[string]*pb.DiffData{},
	}, nil
}

func (s *RewardServiceServer) ReceiveMissionPassRemainingReward(ctx context.Context, _ *emptypb.Empty) (*pb.ReceiveMissionPassRemainingRewardResponse, error) {
	log.Printf("[RewardService] ReceiveMissionPassRemainingReward (stub)")
	return &pb.ReceiveMissionPassRemainingRewardResponse{
		DiffUserData: map[string]*pb.DiffData{},
	}, nil
}
