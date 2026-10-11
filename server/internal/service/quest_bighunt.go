package service

import (
	"context"
	"log"
	"sort"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/runtime"
	"lunar-tear/server/internal/store"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

type BigHuntServiceServer struct {
	pb.UnimplementedBigHuntServiceServer
	users    store.UserRepository
	sessions store.SessionRepository
	holder   *runtime.Holder
}

func NewBigHuntServiceServer(
	users store.UserRepository,
	sessions store.SessionRepository,
	holder *runtime.Holder,
) *BigHuntServiceServer {
	return &BigHuntServiceServer{users: users, sessions: sessions, holder: holder}
}

func (s *BigHuntServiceServer) StartBigHuntQuest(ctx context.Context, req *pb.StartBigHuntQuestRequest) (*pb.StartBigHuntQuestResponse, error) {
	log.Printf("[BigHuntService] StartBigHuntQuest: bossQuestId=%d questId=%d deckNumber=%d isDryRun=%v",
		req.BigHuntBossQuestId, req.BigHuntQuestId, req.UserDeckNumber, req.IsDryRun)

	cat := s.holder.Get()
	catalog := cat.BigHunt
	engine := cat.QuestHandler
	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()

	bhQuest, ok := catalog.QuestById[req.BigHuntQuestId]
	if !ok {
		log.Printf("[BigHuntService] StartBigHuntQuest: unknown bigHuntQuestId=%d", req.BigHuntQuestId)
	}

	today := gametime.StartOfDayMillis()

	s.users.UpdateUser(userId, func(user *store.UserState) {
		// Always clear any leftover battle state from a previous run that ended
		// abnormally (e.g. client crash before FinishBigHuntQuest was called).
		// Without this, SaveBigHuntBattleInfo would inherit the old CostumeBattleInfo
		// slice and assign the first wave of the new run a wave index of 3+, causing
		// the old damage to be double-counted in TotalDamage.
		user.BigHuntBattleDetail = store.BigHuntBattleDetail{}
		user.BigHuntBattleBinary = nil

		if ok {
			engine.HandleBigHuntQuestStart(user, bhQuest.QuestId, req.UserDeckNumber, nowMillis)
		}

		user.BigHuntProgress = store.BigHuntProgress{
			CurrentBigHuntBossQuestId: req.BigHuntBossQuestId,
			CurrentBigHuntQuestId:     req.BigHuntQuestId,
			CurrentQuestSceneId:       0,
			IsDryRun:                  req.IsDryRun,
			LatestVersion:             nowMillis,
		}

		user.BigHuntDeckNumber = req.UserDeckNumber

		st := user.BigHuntStatuses[req.BigHuntBossQuestId]
		if st.LatestChallengeDatetime < today {
			st.DailyChallengeCount = 0
		}
		st.DailyChallengeCount++
		st.LatestChallengeDatetime = nowMillis
		st.LatestVersion = nowMillis
		user.BigHuntStatuses[req.BigHuntBossQuestId] = st

		// Track stamina usage for missions (even though stamina is free on this server)
		if ok {
			staminaCost := engine.BigHuntQuestStaminaCost(bhQuest.QuestId, nowMillis)
			if staminaCost > 0 {
				ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{ConditionType: missionConditionStaminaUsed, Delta: staminaCost}, nowMillis)
			}
		}
	})

	return &pb.StartBigHuntQuestResponse{}, nil
}

func (s *BigHuntServiceServer) UpdateBigHuntQuestSceneProgress(ctx context.Context, req *pb.UpdateBigHuntQuestSceneProgressRequest) (*pb.UpdateBigHuntQuestSceneProgressResponse, error) {
	log.Printf("[BigHuntService] UpdateBigHuntQuestSceneProgress: questSceneId=%d", req.QuestSceneId)

	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()
	s.users.UpdateUser(userId, func(user *store.UserState) {
		user.BigHuntProgress.CurrentQuestSceneId = req.QuestSceneId
		user.BigHuntProgress.LatestVersion = nowMillis
	})

	return &pb.UpdateBigHuntQuestSceneProgressResponse{}, nil
}

func (s *BigHuntServiceServer) FinishBigHuntQuest(ctx context.Context, req *pb.FinishBigHuntQuestRequest) (*pb.FinishBigHuntQuestResponse, error) {
	log.Printf("[BigHuntService] FinishBigHuntQuest: bossQuestId=%d questId=%d isRetired=%v",
		req.BigHuntBossQuestId, req.BigHuntQuestId, req.IsRetired)

	cat := s.holder.Get()
	catalog := cat.BigHunt
	engine := cat.QuestHandler
	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()
	today := gametime.StartOfDayMillis()

	bhQuest := catalog.QuestById[req.BigHuntQuestId]
	bossQuest := catalog.BossQuestById[req.BigHuntBossQuestId]
	boss := catalog.BossByBossId[bossQuest.BigHuntBossId]

	var scoreInfo *pb.BigHuntScoreInfo
	var scoreRewards []*pb.BigHuntReward
	var battleReportWaves []*pb.BigHuntBattleReportWave

	s.users.UpdateUser(userId, func(user *store.UserState) {
		engine.HandleBigHuntQuestFinish(user, bhQuest.QuestId, req.IsRetired, false, nowMillis)

		// Track quit/retire and party wipe for missions
		if req.IsRetired {
			ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{ConditionType: missionConditionQuitBattle, Delta: 1}, nowMillis)
		}

		if req.IsRetired || user.BigHuntProgress.IsDryRun {
			user.BigHuntProgress = store.BigHuntProgress{LatestVersion: nowMillis}
			user.BigHuntBattleBinary = nil
			user.BigHuntBattleDetail = store.BigHuntBattleDetail{}
			return
		}

		detail := user.BigHuntBattleDetail
		totalDamage := detail.TotalDamage
		// Base score is the integer floor of total damage / 100, before any
		// bonus multipliers are applied.
		baseScore := totalDamage / 100

		difficultyBonusPermil := int32(0)
		if coeff, ok := catalog.ScoreCoefficients[bhQuest.BigHuntQuestScoreCoefficientId]; ok {
			difficultyBonusPermil = coeff
		}

		// Survival bonus scales with how many characters died over the run.
		deathCount := bigHuntDeathCount(detail.CostumeBattleInfo)
		aliveBonusPermil := bigHuntSurvivalBonusPermil(deathCount)

		// Combo bonus is keyed on the run's max combo (accumulated across waves).
		maxComboBonusPermil := bigHuntComboBonusPermil(detail.MaxComboCount)

		userScore := baseScore * int64(1000+difficultyBonusPermil+aliveBonusPermil+maxComboBonusPermil) / 1000

		// Apply mission progress for BigHunt battles (type 51 - no specific target)
		ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{
			ConditionType: missionConditionSubjugationBattle,
			Delta:         1,
		}, nowMillis)
		// Apply mission progress for BigHunt boss knockdowns (type 52 - no specific target)
		ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{
			ConditionType: missionConditionBigHuntBattle,
			Delta:         int32(detail.BossKnockDownCount),
		}, nowMillis)
		// Apply mission progress for BigHunt score ranks (type 53 - specific boss)
		ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{
			ConditionType: missionConditionArenaBattle,
			TargetId:      300000 + bossQuest.BigHuntBossId,
			CurrentValue:  int32(userScore),
		}, nowMillis)
		// Hidden-story score missions (type 53, option group 0) additionally
		// require a specific boss and party composition.
		ApplyBigHuntPartyScoreMissionProgress(user, cat.Mission, cat.Quest, bossQuest.BigHuntBossId, userScore, nowMillis)

		if userScore > user.BigHuntMaxScores[bossQuest.BigHuntBossId].MaxScore {
			user.BigHuntMaxScores[bossQuest.BigHuntBossId] = store.BigHuntMaxScore{
				MaxScore:               userScore,
				MaxScoreUpdateDatetime: nowMillis,
				LatestVersion:          nowMillis,
			}
		}

		schedKey := store.BigHuntScheduleScoreKey{
			BigHuntScheduleId: catalog.ActiveScheduleId,
			BigHuntBossId:     bossQuest.BigHuntBossId,
		}
		oldSchedMax := user.BigHuntScheduleMaxScores[schedKey].MaxScore
		isHighScore := userScore > oldSchedMax
		if isHighScore {
			user.BigHuntScheduleMaxScores[schedKey] = store.BigHuntScheduleMaxScore{
				MaxScore:               userScore,
				MaxScoreUpdateDatetime: nowMillis,
				LatestVersion:          nowMillis,
			}
		}

		weeklyVersion := gametime.WeeklyVersion(nowMillis)
		weekKey := store.BigHuntWeeklyScoreKey{
			BigHuntWeeklyVersion: weeklyVersion,
			AttributeType:        boss.AttributeType,
		}
		oldWeeklyMax := user.BigHuntWeeklyMaxScores[weekKey].MaxScore
		if userScore > oldWeeklyMax {
			user.BigHuntWeeklyMaxScores[weekKey] = store.BigHuntWeeklyMaxScore{
				MaxScore:      userScore,
				LatestVersion: nowMillis,
			}
		}

		assetGradeIconId := catalog.ResolveGradeIconId(bossQuest.BigHuntBossId, userScore)

		scoreInfo = &pb.BigHuntScoreInfo{
			UserScore:             userScore,
			IsHighScore:           isHighScore,
			TotalDamage:           totalDamage,
			BaseScore:             baseScore,
			DifficultyBonusPermil: difficultyBonusPermil,
			AliveBonusPermil:      aliveBonusPermil,
			MaxComboBonusPermil:   maxComboBonusPermil,
			AssetGradeIconId:      assetGradeIconId,
		}

		// Daily rank reward (battle path): at most one reward per day per
		// boss. If the no-battle claim button was already used today there is
		// nothing more to grant. Otherwise the payout rank is the better of
		// this run's score and the season best reached before this run
		// (oldSchedMax): a lower-scoring run still pays at the maximum
		// achieved rank, while a new record pays at the new rank.
		st := user.BigHuntStatuses[req.BigHuntBossQuestId]
		if st.LastDailyRewardReceivedDayVersion < today {
			effectiveScore := userScore
			if oldSchedMax > effectiveScore {
				effectiveScore = oldSchedMax
			}
			rewardGroupId := catalog.ResolveActiveScoreRewardGroupId(
				bossQuest.BigHuntScoreRewardGroupScheduleId, nowMillis)
			if rewardGroupId > 0 && effectiveScore > 0 {
				for _, item := range catalog.CollectHighestReward(rewardGroupId, effectiveScore) {
					engine.Granter.GrantFull(user, model.PossessionType(item.PossessionType), item.PossessionId, item.Count, nowMillis)
					scoreRewards = append(scoreRewards, &pb.BigHuntReward{
						PossessionType: item.PossessionType,
						PossessionId:   item.PossessionId,
						Count:          item.Count,
					})
				}
				st.LastDailyRewardReceivedDayVersion = today
				st.LatestVersion = nowMillis
				user.BigHuntStatuses[req.BigHuntBossQuestId] = st
			}
		}

		if len(detail.CostumeBattleInfo) > 0 {
			wavesByIndex := map[int32]*pb.BigHuntBattleReportWave{}
			var waveOrder []int32
			for _, ci := range detail.CostumeBattleInfo {
				wave, ok := wavesByIndex[ci.WaveIndex]
				if !ok {
					wave = &pb.BigHuntBattleReportWave{}
					wavesByIndex[ci.WaveIndex] = wave
					waveOrder = append(waveOrder, ci.WaveIndex)
				}
				wave.BattleReportCostume = append(wave.BattleReportCostume, &pb.BigHuntBattleReportCostume{
					CostumeId:   ci.CostumeId,
					TotalDamage: ci.TotalDamage,
					HitCount:    ci.HitCount,
					BattleReportRandomDisplay: &pb.BattleReportRandomDisplay{
						RandomDisplayValueType: ci.RandomDisplayValueType,
						RandomDisplayValue:     ci.RandomDisplayValue,
					},
				})
			}
			for _, idx := range waveOrder {
				battleReportWaves = append(battleReportWaves, wavesByIndex[idx])
			}
		}

		user.BigHuntProgress = store.BigHuntProgress{LatestVersion: nowMillis}
		user.BigHuntBattleBinary = nil
		user.BigHuntBattleDetail = store.BigHuntBattleDetail{}
	})

	if scoreInfo == nil {
		scoreInfo = &pb.BigHuntScoreInfo{}
	}
	if scoreRewards == nil {
		scoreRewards = []*pb.BigHuntReward{}
	}

	if battleReportWaves == nil {
		battleReportWaves = []*pb.BigHuntBattleReportWave{}
	}
	battleReport := &pb.BigHuntBattleReport{
		BattleReportWave: battleReportWaves,
	}

	return &pb.FinishBigHuntQuestResponse{
		ScoreInfo:    scoreInfo,
		ScoreReward:  scoreRewards,
		BattleReport: battleReport,
	}, nil
}

func (s *BigHuntServiceServer) RestartBigHuntQuest(ctx context.Context, req *pb.RestartBigHuntQuestRequest) (*pb.RestartBigHuntQuestResponse, error) {
	log.Printf("[BigHuntService] RestartBigHuntQuest: bossQuestId=%d questId=%d", req.BigHuntBossQuestId, req.BigHuntQuestId)

	cat := s.holder.Get()
	catalog := cat.BigHunt
	engine := cat.QuestHandler
	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()

	bhQuest := catalog.QuestById[req.BigHuntQuestId]

	var battleBinary []byte
	var deckNumber int32

	today := gametime.StartOfDayMillis()

	s.users.UpdateUser(userId, func(user *store.UserState) {
		engine.HandleBigHuntQuestStart(user, bhQuest.QuestId, user.BigHuntDeckNumber, nowMillis)

		user.BigHuntProgress.CurrentQuestSceneId = 0
		user.BigHuntProgress.LatestVersion = nowMillis

		st := user.BigHuntStatuses[req.BigHuntBossQuestId]
		if st.LatestChallengeDatetime < today {
			st.DailyChallengeCount = 0
		}
		st.DailyChallengeCount++
		st.LatestChallengeDatetime = nowMillis
		st.LatestVersion = nowMillis
		user.BigHuntStatuses[req.BigHuntBossQuestId] = st

		battleBinary = user.BigHuntBattleBinary
		deckNumber = user.BigHuntDeckNumber
	})

	return &pb.RestartBigHuntQuestResponse{
		BattleBinary: battleBinary,
		DeckNumber:   deckNumber,
	}, nil
}

func (s *BigHuntServiceServer) SkipBigHuntQuest(ctx context.Context, req *pb.SkipBigHuntQuestRequest) (*pb.SkipBigHuntQuestResponse, error) {
	log.Printf("[BigHuntService] SkipBigHuntQuest: bossQuestId=%d skipCount=%d", req.BigHuntBossQuestId, req.SkipCount)

	cat := s.holder.Get()
	catalog := cat.BigHunt
	granter := cat.QuestHandler.Granter
	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()
	today := gametime.StartOfDayMillis()

	bossQuest, hasBossQuest := catalog.BossQuestById[req.BigHuntBossQuestId]
	var scoreRewards []*pb.BigHuntReward

	s.users.UpdateUser(userId, func(user *store.UserState) {
		st := user.BigHuntStatuses[req.BigHuntBossQuestId]
		if st.LatestChallengeDatetime < today {
			st.DailyChallengeCount = 0
		}
		st.DailyChallengeCount += req.SkipCount
		st.LatestChallengeDatetime = nowMillis
		st.LatestVersion = nowMillis
		defer func() { user.BigHuntStatuses[req.BigHuntBossQuestId] = st }()

		if !hasBossQuest || req.SkipCount <= 0 {
			return
		}
		// The no-battle claim button: pays once per day at the maximum rank
		// achieved this season for this boss (the stored schedule max score)
		// — only the reward of that rank, never the sum of every lower tier.
		// Claiming it consumes today's reward, so battles afterwards no longer
		// pay until the next day.
		if st.LastDailyRewardReceivedDayVersion >= today {
			return
		}
		rewardGroupId := catalog.ResolveActiveScoreRewardGroupId(bossQuest.BigHuntScoreRewardGroupScheduleId, nowMillis)
		if rewardGroupId == 0 {
			return
		}
		maxScore := user.BigHuntScheduleMaxScores[store.BigHuntScheduleScoreKey{
			BigHuntScheduleId: catalog.ActiveScheduleId,
			BigHuntBossId:     bossQuest.BigHuntBossId,
		}].MaxScore
		if maxScore <= 0 {
			return
		}
		items := catalog.CollectHighestReward(rewardGroupId, maxScore)
		for _, item := range items {
			granter.GrantFull(user, model.PossessionType(item.PossessionType), item.PossessionId, item.Count, nowMillis)
			scoreRewards = append(scoreRewards, &pb.BigHuntReward{
				PossessionType: item.PossessionType,
				PossessionId:   item.PossessionId,
				Count:          item.Count,
			})
		}
		st.LastDailyRewardReceivedDayVersion = today
	})

	if scoreRewards == nil {
		scoreRewards = []*pb.BigHuntReward{}
	}
	return &pb.SkipBigHuntQuestResponse{
		ScoreReward: scoreRewards,
	}, nil
}

func (s *BigHuntServiceServer) SaveBigHuntBattleInfo(ctx context.Context, req *pb.SaveBigHuntBattleInfoRequest) (*pb.SaveBigHuntBattleInfoResponse, error) {
	log.Printf("[BigHuntService] SaveBigHuntBattleInfo: elapsedFrames=%d", req.ElapsedFrameCount)

	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()

	s.users.UpdateUser(userId, func(user *store.UserState) {
		user.BigHuntBattleBinary = req.BattleBinary

		if req.BigHuntBattleDetail != nil {
			existing := user.BigHuntBattleDetail
			existingCostumes := existing.CostumeBattleInfo
			nextWaveIndex := int32(bigHuntWaveCount(existingCostumes))
			newCostumes := make([]store.BigHuntCostumeBattleInfo, 0, len(req.BigHuntBattleDetail.CostumeBattleInfo))
			for _, ci := range req.BigHuntBattleDetail.CostumeBattleInfo {
				if ci == nil {
					continue
				}
				var rdType int32
				var rdValue int64
				if rd := ci.BattleReportRandomDisplay; rd != nil {
					rdType = rd.RandomDisplayValueType
					rdValue = rd.RandomDisplayValue
				}
				newCostumes = append(newCostumes, store.BigHuntCostumeBattleInfo{
					WaveIndex:              nextWaveIndex,
					CostumeId:              resolveBigHuntCostumeId(user, ci.UserDeckNumber, ci.DeckCharacterNumber),
					TotalDamage:            ci.TotalDamage,
					HitCount:               ci.HitCount,
					RandomDisplayValueType: rdType,
					RandomDisplayValue:     rdValue,
					IsAlive:                ci.IsAlive,
				})
			}

			// Accumulate aggregates across every wave, not just the current request.
			// CostumeBattleInfo is the single source of truth for total damage.
			allCostumes := append(existingCostumes, newCostumes...)
			var accumulatedDamage int64
			for _, ci := range allCostumes {
				accumulatedDamage += ci.TotalDamage
			}
			maxCombo := existing.MaxComboCount
			if req.BigHuntBattleDetail.MaxComboCount > maxCombo {
				maxCombo = req.BigHuntBattleDetail.MaxComboCount
			}

			user.BigHuntBattleDetail = store.BigHuntBattleDetail{
				DeckType:             req.BigHuntBattleDetail.DeckType,
				UserTripleDeckNumber: req.BigHuntBattleDetail.UserTripleDeckNumber,
				BossKnockDownCount:   existing.BossKnockDownCount + req.BigHuntBattleDetail.BossKnockDownCount,
				MaxComboCount:        maxCombo,
				TotalDamage:          accumulatedDamage,
				CostumeBattleInfo:    allCostumes,
			}
		}

		user.BigHuntProgress.LatestVersion = nowMillis
	})

	return &pb.SaveBigHuntBattleInfoResponse{}, nil
}

func (s *BigHuntServiceServer) GetBigHuntTopData(ctx context.Context, _ *emptypb.Empty) (*pb.GetBigHuntTopDataResponse, error) {
	log.Printf("[BigHuntService] GetBigHuntTopData")

	catalog := s.holder.Get().BigHunt
	userId := CurrentUserId(ctx, s.users, s.sessions)
	user, _ := s.users.LoadUser(userId)

	nowMillis := gametime.NowMillis()
	weeklyVersion := gametime.WeeklyVersion(nowMillis)

	var weeklyScoreResults []*pb.WeeklyScoreResult
	for _, boss := range bigHuntBossesSorted(catalog) {
		key := store.BigHuntWeeklyScoreKey{
			BigHuntWeeklyVersion: weeklyVersion,
			AttributeType:        boss.AttributeType,
		}
		ws := user.BigHuntWeeklyMaxScores[key]
		gradeIconId := catalog.ResolveGradeIconId(boss.BigHuntBossId, ws.MaxScore)

		weeklyScoreResults = append(weeklyScoreResults, &pb.WeeklyScoreResult{
			AttributeType:           boss.AttributeType,
			BeforeMaxScore:          ws.MaxScore,
			CurrentMaxScore:         ws.MaxScore,
			BeforeAssetGradeIconId:  gradeIconId,
			CurrentAssetGradeIconId: gradeIconId,
			AfterMaxScore:           ws.MaxScore,
			AfterAssetGradeIconId:   gradeIconId,
		})
	}

	ws := user.BigHuntWeeklyStatuses[weeklyVersion]

	// The season reward is the sum of each boss's rank reward at the player's
	// season-best score; it is not bound to a single week, so both popup fields
	// show the same total.
	seasonRewards := resolveBigHuntSeasonRewards(catalog, user, nowMillis)

	return &pb.GetBigHuntTopDataResponse{
		WeeklyScoreResult:           weeklyScoreResults,
		WeeklyScoreReward:           seasonRewards,
		IsReceivedWeeklyScoreReward: ws.IsReceivedWeeklyReward,
		LastWeekWeeklyScoreReward:   seasonRewards,
	}, nil
}

func bigHuntWaveCount(infos []store.BigHuntCostumeBattleInfo) int {
	if len(infos) == 0 {
		return 0
	}
	return int(infos[len(infos)-1].WaveIndex) + 1
}

// bigHuntDeathCount counts how many distinct player characters ended the run
// dead. A character can appear across several waves; its final state is taken
// from the highest wave index in which it was reported. Entries that do not
// resolve to one of the player's deck characters (CostumeId 0 -- e.g. the boss,
// which is also reported in CostumeBattleInfo and is "not alive" once defeated)
// are skipped so they don't inflate the death count.
func bigHuntDeathCount(infos []store.BigHuntCostumeBattleInfo) int32 {
	latestWave := map[int32]int32{}
	latestAlive := map[int32]bool{}
	for _, ci := range infos {
		if ci.CostumeId == 0 {
			continue
		}
		if w, ok := latestWave[ci.CostumeId]; !ok || ci.WaveIndex >= w {
			latestWave[ci.CostumeId] = ci.WaveIndex
			latestAlive[ci.CostumeId] = ci.IsAlive
		}
	}
	var deaths int32
	for _, alive := range latestAlive {
		if !alive {
			deaths++
		}
	}
	return deaths
}

// bigHuntSurvivalBonusPermil maps the run's death count to the survival score
// bonus: 0 deaths=+200%, 1=+150%, 2=+100%, 3=+50%, 4 or more=0%.
func bigHuntSurvivalBonusPermil(deaths int32) int32 {
	switch deaths {
	case 0:
		return 2000
	case 1:
		return 1500
	case 2:
		return 1000
	case 3:
		return 500
	default:
		return 0
	}
}

// bigHuntComboBonusPermil maps the run's max combo to the combo score bonus.
func bigHuntComboBonusPermil(maxCombo int32) int32 {
	switch {
	case maxCombo >= 46:
		return 1800
	case maxCombo >= 42:
		return 1600
	case maxCombo >= 36:
		return 1400
	case maxCombo >= 31:
		return 1200
	case maxCombo >= 26:
		return 1000
	case maxCombo >= 21:
		return 800
	case maxCombo >= 16:
		return 600
	case maxCombo >= 11:
		return 400
	case maxCombo >= 6:
		return 200
	default:
		return 0
	}
}

func resolveBigHuntCostumeId(user *store.UserState, userDeckNumber, deckCharacterNumber int32) int32 {
	if userDeckNumber == 0 {
		userDeckNumber = user.BigHuntDeckNumber
	}
	for _, dt := range []model.DeckType{model.DeckTypeBigHunt, model.DeckTypeQuest} {
		deck, ok := user.Decks[store.DeckKey{DeckType: dt, UserDeckNumber: userDeckNumber}]
		if !ok {
			continue
		}
		var dcUuid string
		switch deckCharacterNumber {
		case 1:
			dcUuid = deck.UserDeckCharacterUuid01
		case 2:
			dcUuid = deck.UserDeckCharacterUuid02
		case 3:
			dcUuid = deck.UserDeckCharacterUuid03
		}
		if dcUuid == "" {
			continue
		}
		dc, ok := user.DeckCharacters[dcUuid]
		if !ok || dc.UserCostumeUuid == "" {
			continue
		}
		if costume, ok := user.Costumes[dc.UserCostumeUuid]; ok {
			return costume.CostumeId
		}
	}
	return 0
}

// bigHuntBossSeasonScore returns the player's season-best score for a boss:
// the persisted schedule max score. Unlike the weekly max scores it never
// ages out of a one- or two-week lookup window, so a best set weeks ago still
// counts towards the season reward.
func bigHuntBossSeasonScore(user *store.UserState, catalog *masterdata.BigHuntCatalog, bossId int32) int64 {
	return user.BigHuntScheduleMaxScores[store.BigHuntScheduleScoreKey{
		BigHuntScheduleId: catalog.ActiveScheduleId,
		BigHuntBossId:     bossId,
	}].MaxScore
}

// resolveBigHuntSeasonRewards sums the season (weekly attribute ranking)
// reward of every boss at the player's season-best score — the total season
// reward shown by the client: one top-tier reward per boss, never the sum of
// every lower tier.
func resolveBigHuntSeasonRewards(catalog *masterdata.BigHuntCatalog, user store.UserState, nowMillis int64) []*pb.BigHuntReward {
	var rewards []*pb.BigHuntReward
	for _, boss := range bigHuntBossesSorted(catalog) {
		rewardGroupId := catalog.ResolveActiveWeeklyRewardGroupIdByAttr(boss.AttributeType, nowMillis)
		if rewardGroupId == 0 {
			continue
		}
		score := bigHuntBossSeasonScore(&user, catalog, boss.BigHuntBossId)
		if score <= 0 {
			continue // never fought this boss: not even the score-0 tier applies
		}
		for _, item := range catalog.CollectHighestReward(rewardGroupId, score) {
			rewards = append(rewards, &pb.BigHuntReward{
				PossessionType: item.PossessionType,
				PossessionId:   item.PossessionId,
				Count:          item.Count,
			})
		}
	}
	if rewards == nil {
		rewards = []*pb.BigHuntReward{}
	}
	return rewards
}

// bigHuntBossesSorted returns the catalog's bosses ordered by id so response
// fields and mailed gift rows come out in a stable order.
func bigHuntBossesSorted(catalog *masterdata.BigHuntCatalog) []masterdata.BigHuntBossRow {
	bosses := make([]masterdata.BigHuntBossRow, 0, len(catalog.BossByBossId))
	for _, boss := range catalog.BossByBossId {
		bosses = append(bosses, boss)
	}
	sort.Slice(bosses, func(i, j int) bool { return bosses[i].BigHuntBossId < bosses[j].BigHuntBossId })
	return bosses
}
