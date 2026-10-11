package service

import (
	"context"
	"log"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/questflow"
	"lunar-tear/server/internal/runtime"
	"lunar-tear/server/internal/store"
	"lunar-tear/server/internal/utils"
)

// Individual mission IDs to block (for excluding specific missions from categories 2 and 5)
var blockedMissionIds = map[int32]bool{
	/* 1010203: true, 1010204: true, 1010205: true, 1010208: true,
	1010209: true, 1010210: true, 1010213: true, 1010214: true,
	1010215: true, 1010403: true, 1010404: true, 1010405: true,
	1010406: true, 1010408: true, 1010409: true, 1010410: true,
	1010413: true, 1010414: true, 1010415: true,
	460002: true, 460003: true, 460004: true, 460005: true,
	460007: true, 460008: true, 460009: true, 460010: true, */
}

// hiddenMissionRange represents a range of mission IDs to hide
type hiddenMissionRange struct {
	start int32
	end   int32
}

// hiddenMissionIds contains specific mission IDs that should be hidden from the client
// (in addition to categories 1, 3, and 4 which are hidden entirely)
var hiddenMissionIds = []int32{
	// Add mission IDs here that should be hidden but don't belong to categories 1, 3, or 4
}

// hiddenMissionRanges contains ranges of mission IDs to hide
var hiddenMissionRanges = []hiddenMissionRange{
	// Example: {start: 100000, end: 100100},
	{start: 210001, end: 212241},
	{start: 470016, end: 470090},
}

// isMissionCategory1 checks if a mission belongs to category 1 (Daily)
func isMissionCategory1(mission masterdata.EntityMMission, cat *masterdata.MissionCatalog) bool {
	return cat.CategoryTypeOf(mission) == 1
}

// HasInitializedHiddenMissions checks if ALL hidden category 1 missions are properly initialized.
// This is a utility function for debugging/monitoring purposes - not used in main flow.
// Returns true only if ALL visible category 1 missions have Unknown(0) status.
func HasInitializedHiddenMissions(user *store.UserState, cat *masterdata.MissionCatalog, nowMillis int64) bool {
	if cat == nil || user == nil {
		return false
	}

	foundAny := false
	for _, m := range cat.ActiveMissionsAt(nowMillis) {
		if isMissionCategory1(m, cat) {
			foundAny = true
			progress, ok := user.Missions[m.MissionId]
			// If mission exists but is NOT Unknown(0), it's not properly initialized
			if !ok || progress.MissionProgressStatusType != 0 {
				return false
			}
		}
	}

	// Return true only if we found at least one category 1 mission and all had Unknown(0)
	return foundAny
}

// Pass mission ranges (monthly pass missions)
const (
	passMissionRange2Start int32 = 2000301
	passMissionRange2End   int32 = 2002414
)

// isBlockedPassMission checks if a mission ID is in the blocked pass range (2000301-2002414)
func isBlockedPassMission(missionId int32) bool {
	return missionId >= passMissionRange2Start && missionId <= passMissionRange2End
}

type MissionServiceServer struct {
	pb.UnimplementedMissionServiceServer
	users    store.UserRepository
	sessions store.SessionRepository
	holder   *runtime.Holder
}

func NewMissionServiceServer(users store.UserRepository, sessions store.SessionRepository, holder *runtime.Holder) *MissionServiceServer {
	return &MissionServiceServer{users: users, sessions: sessions, holder: holder}
}

// applyCageMeasurableValues folds the client-side Cage accumulators (running
// distance, Mama taps) into mission progress. The client piggybacks these
// values onto several RPCs — UpdateMissionProgress, StartMainQuest,
// GetMamaBanner and GetFriendList (field 50) — flushing into whichever fires
// first, so every one of those handlers must forward them here or the deltas
// are silently lost.
func applyCageMeasurableValues(user *store.UserState, cat *masterdata.MissionCatalog, cage *pb.CageMeasurableValues, nowMillis int64) {
	if cage == nil {
		return
	}
	if cage.RunningDistanceMeters > 0 {
		ApplyMissionProgressEvent(user, cat, MissionProgressEvent{
			ConditionType: missionConditionCageRunningDistance,
			Delta:         cage.RunningDistanceMeters,
		}, nowMillis)
	}
	if cage.MamaTappedCount > 0 {
		ApplyMissionProgressEvent(user, cat, MissionProgressEvent{
			ConditionType: missionConditionCageMamaTap,
			Delta:         cage.MamaTappedCount,
		}, nowMillis)
	}
}

func (s *MissionServiceServer) UpdateMissionProgress(ctx context.Context, req *pb.UpdateMissionProgressRequest) (*pb.UpdateMissionProgressResponse, error) {
	log.Printf("[MissionService] UpdateMissionProgress: cage=%v", req.CageMeasurableValues)

	userId := CurrentUserId(ctx, s.users, s.sessions)
	cat := s.holder.Get()
	nowMillis := gametime.NowMillis()

	s.users.UpdateUser(userId, func(user *store.UserState) {
		applyCageMeasurableValues(user, cat.Mission, req.GetCageMeasurableValues(), nowMillis)
		// pictureBookMeasurableValues carries the Cage picture-book accumulators:
		// crossed-out magick users (type 55) and rhythm-concert taps (type 61).
		if pbv := req.GetPictureBookMeasurableValues(); pbv != nil {
			if pbv.DefeatWizardCount > 0 {
				ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{
					ConditionType: missionConditionDefeatWizard,
					Delta:         pbv.DefeatWizardCount,
				}, nowMillis)
			}
			if ri := pbv.GetRhythmInteractionMeasurableValues(); ri != nil && ri.TapCount > 0 {
				log.Printf("[MissionService] rhythm interaction: liveTypeId=%d tapCount=%d", ri.LiveTypeId, ri.TapCount)
				ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{
					ConditionType: missionConditionRhythmConcertTap,
					TargetId:      ri.LiveTypeId,
					CurrentValue:  ri.TapCount,
				}, nowMillis)
			}
		}
	})

	return &pb.UpdateMissionProgressResponse{}, nil
}

func (s *MissionServiceServer) ReceiveMissionRewardsById(ctx context.Context, req *pb.ReceiveMissionRewardsByIdRequest) (*pb.ReceiveMissionRewardsResponse, error) {
	log.Printf("[MissionService] ReceiveMissionRewardsById: missionIds=%v", req.GetMissionId())

	userId := CurrentUserId(ctx, s.users, s.sessions)
	cat := s.holder.Get()
	granter := cat.QuestHandler.Granter
	missions := cat.Mission

	nowMillis := gametime.NowMillis()
	received := make([]*pb.MissionReward, 0)
	claimedCount := 0

	_, err := s.users.UpdateUser(userId, func(user *store.UserState) {
		for _, missionId := range req.GetMissionId() {
			m, ok := missions.Missions[missionId]
			if !ok {
				log.Printf("[MissionService] unknown missionId=%d", missionId)
				continue
			}
			state, exists := user.Missions[missionId]
			if !exists {
				log.Printf("[MissionService] user=%d has no row for mission=%d", userId, missionId)
				continue
			}
			if state.MissionProgressStatusType == int32(model.MissionProgressStatusTypeRewardReceived) {
				log.Printf("[MissionService] mission=%d already claimed by user=%d", missionId, userId)
				continue
			}
			if state.MissionProgressStatusType < int32(model.MissionProgressStatusTypeClear) {
				log.Printf("[MissionService] mission=%d not cleared (status=%d) for user=%d; skipping claim", missionId, state.MissionProgressStatusType, userId)
				continue
			}

			isChallenge := false
			if group, ok := missions.GroupById[m.MissionGroupId]; ok {
				isChallenge = group.MissionCategoryType == 2
			}

			for _, r := range missions.RewardsByGroupId[m.MissionRewardId] {
				count := r.Count
				// Apply x10 multiplier for free gems only in Challenge mission
				// rewards (MissionCategoryType 2); other categories pay as authored.
				if model.PossessionType(r.PossessionType) == model.PossessionTypeFreeGem && isChallenge {
					count = count * questflow.MissionRewardGemBonus
				}
				granter.GrantFull(user, model.PossessionType(r.PossessionType), r.PossessionId, count, nowMillis)
				received = append(received, &pb.MissionReward{
					PossessionType: r.PossessionType,
					PossessionId:   r.PossessionId,
					Count:          count,
				})
			}

			state.MissionProgressStatusType = int32(model.MissionProgressStatusTypeRewardReceived)
			state.LatestVersion = nowMillis
			user.Missions[missionId] = state
			// Only count non-type 25 missions and non-daily missions for progress updates
			if m.MissionClearConditionType != missionConditionMissionClearCount {
				// Check if this is a daily mission (MissionCategoryType 1)
				if group, ok := missions.GroupById[m.MissionGroupId]; ok {
					if group.MissionCategoryType != 1 {
						claimedCount++
					}
				} else {
					claimedCount++
				}
			}
		}

		// Update type 25 missions after claiming rewards
		// Only update if we claimed non-type 25 missions
		if claimedCount > 0 {
			for _, mission := range missions.ActiveMissionsAt(nowMillis) {
				if mission.MissionClearConditionType == missionConditionMissionClearCount {
					progress := user.Missions[mission.MissionId]
					// Skip if already cleared or reward received
					if progress.MissionProgressStatusType >= int32(model.MissionProgressStatusTypeClear) {
						continue
					}

					if progress.MissionId == 0 {
						progress.MissionId = mission.MissionId
						progress.StartDatetime = nowMillis
					}
					progress.ProgressValue += int32(claimedCount)
					progress.LatestVersion = nowMillis
					if progress.ProgressValue >= mission.ClearConditionValue {
						progress.ProgressValue = mission.ClearConditionValue
						progress.MissionProgressStatusType = int32(model.MissionProgressStatusTypeClear)
						progress.ClearDatetime = nowMillis
						log.Printf("[Mission] mission %d CLEARED by condition type 25 target %d", mission.MissionId, 0)
					} else {
						progress.MissionProgressStatusType = int32(model.MissionProgressStatusTypeInProgress)
					}
					user.Missions[mission.MissionId] = progress
				}
			}
		}
	})
	if err != nil {
		log.Printf("[MissionService] ReceiveMissionRewardsById: update user=%d failed: %v", userId, err)
		return &pb.ReceiveMissionRewardsResponse{}, nil
	}

	return &pb.ReceiveMissionRewardsResponse{
		ReceivedPossession: received,
		ExpiredPossession:  []*pb.MissionReward{},
		OverflowPossession: []*pb.MissionReward{},
	}, nil
}

func (s *MissionServiceServer) ReceiveMissionPassRewards(ctx context.Context, req *pb.ReceiveMissionPassRewardsRequest) (*pb.ReceiveMissionPassRewardsResponse, error) {
	log.Printf("[MissionService] ReceiveMissionPassRewards: missionPassId=%d", req.GetMissionPassId())

	userId := CurrentUserId(ctx, s.users, s.sessions)
	cat := s.holder.Get()
	granter := cat.QuestHandler.Granter
	missions := cat.Mission

	pass, ok := missions.PassById[req.GetMissionPassId()]
	if !ok {
		log.Printf("[MissionService] unknown missionPassId=%d", req.GetMissionPassId())
		return &pb.ReceiveMissionPassRewardsResponse{}, nil
	}

	nowMillis := gametime.NowMillis()
	received := make([]*pb.MissionPassReward, 0)

	_, err := s.users.UpdateUser(userId, func(user *store.UserState) {
		// The client offers this claim only once, but replays (relogs, retries)
		// must not hand out the pass rewards again.
		ps := user.MissionPassStates[pass.MissionPassId]
		if ps.RewardsClaimed {
			log.Printf("[MissionService] mission pass %d rewards already claimed by user=%d", pass.MissionPassId, userId)
			return
		}
		ps.MissionPassId = pass.MissionPassId
		ps.RewardsClaimed = true
		user.MissionPassStates[pass.MissionPassId] = ps

		for _, tier := range missions.PassRewardGroup[pass.MissionPassRewardGroupId] {
			granter.GrantFull(user, model.PossessionType(tier.PossessionType), tier.PossessionId, tier.Count, nowMillis)
			received = append(received, &pb.MissionPassReward{
				PossessionType: tier.PossessionType,
				PossessionId:   tier.PossessionId,
				Count:          tier.Count,
			})
		}
	})
	if err != nil {
		log.Printf("[MissionService] ReceiveMissionPassRewards: update user=%d failed: %v", userId, err)
		return &pb.ReceiveMissionPassRewardsResponse{}, nil
	}

	return &pb.ReceiveMissionPassRewardsResponse{
		ReceivedPossession: received,
		OverflowPossession: []*pb.MissionPassReward{},
	}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Quest-clear mission progress
// ─────────────────────────────────────────────────────────────────────────────

const (
	missionClearConditionTypeQuestClearCount = 1

	// Arena / PvP battle condition types. Data uses type 15 for "Play an
	// Arena match" (any outcome), type 16 for wins and type 17 for "win N
	// matches in a row"; type 69 tracks the current PvP rating.
	missionConditionArenaPlay      int32 = 15
	missionConditionArenaWin       int32 = 16
	missionConditionArenaWinStreak int32 = 17
	missionConditionArenaRank      int32 = 69

	// Cumulative / event-based condition types
	missionConditionSummons              int32 = 18
	missionConditionShopPurchase         int32 = 21
	missionConditionPlayerLevel          int32 = 22
	missionConditionLogin                int32 = 23
	missionConditionMissionClearCount    int32 = 25
	missionConditionExplorationClear     int32 = 26
	missionConditionCageMamaTap          int32 = 31
	missionConditionCageRunningDistance  int32 = 32
	missionConditionCostumeSetCollected  int32 = 49
	missionConditionFriendSupport        int32 = 50
	missionConditionSubjugationBattle    int32 = 51
	missionConditionBigHuntBattle        int32 = 52
	missionConditionArenaBattle          int32 = 53
	missionConditionCharacterBoardPanel  int32 = 54
	missionConditionDefeatWizard         int32 = 55 // Cage picture book: cross out N magick users
	missionConditionGimmickSequenceClear int32 = 60
	missionConditionRhythmConcertTap     int32 = 61 // Cage rhythm concert: tap count per live
	missionConditionSoloQuestClear       int32 = 62 // clear a concrete quest with only one specific character
	missionConditionStaminaUsed          int32 = 65
	missionConditionCostumeAwaken        int32 = 66
	missionConditionQuitBattle           int32 = 36 // Quit/retire from battle (for missions 440001-440002)
	missionConditionPartyWipe            int32 = 37 // Lose whole party in battle (for missions 450001)
	missionConditionLoginTotalDays       int32 = 67
	missionConditionWeaponAwaken         int32 = 70
	missionConditionCharacterExalt       int32 = 71
	missionConditionKarmaPanelType1      int32 = 72
	missionConditionKarmaPanelType2      int32 = 73
	missionConditionBossDefeat           int32 = 35 // Boss defeat count (for missions 220001-220022)
	missionConditionEncyclopediaEntries  int32 = 39 // Encyclopedia entry count (for missions 350001-350030)
	missionConditionRhythmHighScore      int32 = 29 // Shooting/FlyingMama high score (Condition 29)
	missionConditionTotalForce           int32 = 28 // Total force (sum of all deck powers)
)

// MissionProgressEvent describes a server-side action that can advance one or
// more missions. TargetId is the concrete equipment/character/quest involved;
// a zero target means that the action has no more specific target. Delta is
// used for cumulative conditions, while CurrentValue is used for thresholds
// such as "reach weapon level 50".
type MissionProgressEvent struct {
	ConditionType int32
	TargetId      int32
	Delta         int32
	CurrentValue  int32
}

const (
	missionLinkDomainAny        int32 = 0
	missionLinkDomainQuestClear int32 = 4 // main, event, extra all use domain=4
	missionLinkDomainEventQuest int32 = 7
	missionLinkDomainExtraQuest int32 = 13
)

// ApplyQuestClearMissionProgress advances every active mission whose clear
// condition counts quest clears matching this quest. Call once per successful
// quest finish.
//
// chapterId:
//   - main quests:  cat.QuestToMainChapter[questId]
//   - event quests: req.EventQuestChapterId
//   - extra quests: 0
func ApplyQuestClearMissionProgress(
	user *store.UserState,
	cat *masterdata.MissionCatalog,
	questType model.QuestType,
	questId, chapterId int32,
	nowMillis int64,
) {
	if cat == nil {
		return
	}
	// Some client flows send a display/parent chapter ID rather than the
	// concrete EventQuestChapterId. Prefer the request when it owns this quest,
	// otherwise recover the chapter from the event quest tables.
	if questType == model.QuestTypeEvent && !cat.EventChapterQuestIds[chapterId][questId] {
		if actualChapterID := cat.QuestToEventChapter[questId]; actualChapterID != 0 {
			chapterId = actualChapterID
		}
	}

	activeMissions := cat.ActiveMissionsAt(nowMillis)
	for _, m := range activeMissions {
		// Skip blocked individual mission IDs (for excluding specific missions from categories 2 and 5)
		if blockedMissionIds[m.MissionId] {
			continue
		}
		// Skip blocked pass missions (2000301-2002414)
		if isBlockedPassMission(m.MissionId) {
			continue
		}

		if m.MissionClearConditionType != missionClearConditionTypeQuestClearCount {
			continue
		}

		link, ok := cat.LinkById[m.MissionLinkId]
		if !ok {
			// If mission has no link (MissionLinkId=0), skip domain check and proceed to quest matching
			if m.MissionLinkId != 0 {
				continue
			}
		}
		if m.MissionLinkId != 0 && !missionDomainMatchesQuestType(link.DestinationDomainType, questType) {
			continue
		}

		// Subquest missions (MissionLinkId=1) should only count Event quests
		// Exclude Main, BigHunt, and Extra quests from subquest mission counting
		if m.MissionLinkId == 1 && questType != model.QuestTypeEvent {
			continue
		}

		// Missions 460002-460004 ("Clear Chapter 4 without playing the
		// tutorial that appears when you first fail a quest") share condition
		// group 460002, but their master-data row is just "clear any 1 quest"
		// (option group 0). Enforce the real hidden condition instead of the
		// generic matching, which would clear them on the very first quest.
		if m.MissionClearConditionGroupId == noDeathChapter4ConditionGroupId {
			if !noDeathChapter4MissionSatisfied(user, cat, questType, questId) {
				continue
			}
		} else if m.MissionClearConditionGroupId == exHardEventQuest3ConditionGroupId {
			// Mission 500013 ("Clear Quest 3 of an event quest on EX Hard") is
			// encoded as "clear any 1 quest"; enforce the real target here.
			pos, known := cat.EventQuestPosition[questId]
			if questType != model.QuestTypeEvent || !known ||
				pos.DifficultyType != eventDifficultyExHard || pos.QuestNumber != 3 {
				continue
			}
		} else if !questMissionMatches(cat, m, questType, questId, chapterId) {
			continue
		}

		progress, exists := user.Missions[m.MissionId]
		if exists && model.MissionProgressStatusType(progress.MissionProgressStatusType) >= model.MissionProgressStatusTypeClear {
			continue
		}
		if !exists {
			progress = store.UserMissionState{
				MissionId:     m.MissionId,
				StartDatetime: nowMillis,
			}
		}

		progress.ProgressValue++
		progress.LatestVersion = nowMillis

		if progress.ProgressValue >= m.ClearConditionValue {
			progress.ProgressValue = m.ClearConditionValue
			progress.MissionProgressStatusType = int32(model.MissionProgressStatusTypeClear)
			progress.ClearDatetime = nowMillis
		} else {
			progress.MissionProgressStatusType = int32(model.MissionProgressStatusTypeInProgress)
		}

		user.Missions[m.MissionId] = progress
	}

	// Type 25 missions are updated in ReceiveMissionRewardsById when rewards are claimed
}

// ApplyQuestClearMissionHooks runs the complete mission hook set that the
// FinishMainQuest / FinishEventQuest / FinishExtraQuest RPCs apply after a
// successful (not retired, not annihilated) quest clear. Offline clear paths
// — e.g. the lunar-base-grant admin shim, whose questflow handlers do not
// touch missions at all — must call this right after the finish handler so
// record counters (type 1), player level, boss defeats (type 35), solo-clear
// (type 62) and hidden-story (category 7) missions advance exactly like the
// live path.
//
// questType/chapterId follow ApplyQuestClearMissionProgress conventions:
// main uses cat.QuestToMainChapter[questId], event uses the event chapter id,
// extra uses chapterId=0.
func ApplyQuestClearMissionHooks(
	user *store.UserState,
	missionCat *masterdata.MissionCatalog,
	questCat *masterdata.QuestCatalog,
	questType model.QuestType,
	questId, chapterId int32,
	nowMillis int64,
) {
	ApplyQuestClearMissionProgress(user, missionCat, questType, questId, chapterId, nowMillis)
	ApplyMissionProgressEvent(user, missionCat, MissionProgressEvent{
		ConditionType: missionConditionPlayerLevel,
		CurrentValue:  user.Status.Level,
	}, nowMillis)

	// Boss defeat missions (type 35 - for missions 220001-220022)
	if n := questBossCount(questCat, questId); n > 0 {
		ApplyMissionProgressEvent(user, missionCat, MissionProgressEvent{
			ConditionType: missionConditionBossDefeat,
			Delta:         n,
		}, nowMillis)
	}

	// Solo-clear missions (type 62) are only applied by the main/extra
	// finish RPCs; the event finish deliberately skips them.
	if questType != model.QuestTypeEvent {
		ApplySoloClearMissionProgress(user, missionCat, questCat, questId, nowMillis)
	}

	// Hidden-story quest-clear missions (category 7) count every quest type.
	ApplyHiddenStoryQuestClearMissionProgress(user, missionCat, questCat, questId, nowMillis)
}

// noDeathChapter4ConditionGroupId is shared by missions 460002-460004: "Clear
// Chapter 4 without playing the tutorial that appears when you first fail a
// quest". Their master data only says "clear any 1 quest", so the real
// condition is enforced by noDeathChapter4MissionSatisfied.
const noDeathChapter4ConditionGroupId int32 = 460002

// exHardEventQuest3ConditionGroupId belongs to hidden-story mission 500013:
// "Clear Quest 3 of an event quest on EX Hard". Its master data is also just
// "clear any 1 quest" (option group 0); the real condition is enforced in
// ApplyQuestClearMissionProgress via the EventQuestPosition index.
const exHardEventQuest3ConditionGroupId int32 = 500013

// eventDifficultyExHard is DifficultyType 4 in m_event_quest_sequence_group
// (1=Normal 2=Hard 3=Very Hard 4=EX Hard).
const eventDifficultyExHard int32 = 4

// noDeathChapter4MissionSatisfied reports whether clearing the given quest
// completes the hidden no-death challenge: it must be the final Normal quest
// of game chapter 4, and the first-loss tutorials (LoseFirst=44,
// LoseFirstAfterChapter=57) must never have been triggered. The client only
// reports those tutorials after the player's first quest failure.
func noDeathChapter4MissionSatisfied(user *store.UserState, cat *masterdata.MissionCatalog, questType model.QuestType, questID int32) bool {
	if questType != model.QuestTypeMain {
		return false
	}
	chapter, ok := cat.GameToDataChapter[4]
	if !ok {
		return false
	}
	if cat.MainChapterFinalQuestByDifficulty[chapter][1] != questID {
		return false
	}
	if _, played := user.Tutorials[int32(model.TutorialTypeLoseFirst)]; played {
		return false
	}
	if _, played := user.Tutorials[int32(model.TutorialTypeLoseFirstAfterChapter)]; played {
		return false
	}
	return true
}

func questMissionMatches(cat *masterdata.MissionCatalog, mission masterdata.EntityMMission, questType model.QuestType, questID, chapterID int32) bool {
	// Missions registered against the main story (concrete Quest 1/4/7/10
	// templates and chapter-clear missions) can only be satisfied by main
	// quests. Checking membership before the quest type matters: their
	// OptionGroupId encodes a game chapter (e.g. 124 = chapter 24), which
	// collides with unrelated extra-quest IDs in the generic fallback below
	// (roaming-boss extra quest 124 must not clear "Clear The Sun VI").
	if cat.MainMissionConcreteTemplate[mission.MissionId] {
		// Do not fall through to OptionGroupId == QuestId. In the Sun/Moon
		// template OptionGroupId is a chapter/sequence reference (e.g. 18),
		// not the quest that must be cleared.
		return questType == model.QuestTypeMain && cat.MainMissionTargetQuest[mission.MissionId] == questID
	}
	if chapter, isChapterClear := cat.MainChapterClearMission[mission.MissionId]; isChapterClear {
		if questType != model.QuestTypeMain {
			return false
		}
		difficulty := cat.MainChapterClearDifficulty[mission.MissionId]
		if difficulty == 0 {
			difficulty = 1
		}
		return chapter == chapterID && cat.MainChapterFinalQuestByDifficulty[chapter][difficulty] == questID
	}
	if questType == model.QuestTypeEvent {
		if link, ok := cat.LinkById[mission.MissionLinkId]; ok && link.DestinationDomainId != 0 {
			// A link destination is an EventQuestChapterId. It isolates records
			// from each other even when their condition-option IDs coincide.
			// Rerun chapters missing from this data revision are aliased back
			// to the original chapter whose quests actually exist.
			dest := link.DestinationDomainId
			if alias, isRerun := cat.EventChapterRerunAlias[dest]; isRerun {
				dest = alias
			}
			if dest != chapterID {
				return false
			}
			if difficulty, isConcreteTarget := cat.EventMissionTargetDifficulty[mission.MissionId]; isConcreteTarget {
				// "Clear Record on Normal" and "Clear Quest 5 on EX Hard"
				// both target the last quest of the specified difficulty.
				return cat.EventChapterFinalQuestByDifficulty[chapterID][difficulty] == questID
			}
			return mission.ClearConditionValue > 1 && cat.EventChapterQuestIds[chapterID][questID]
		}
	}
	return questMissionOptionMatches(cat, mission.MissionClearConditionOptionGroupId, mission.MissionLinkId, questType, questID, chapterID)
}

func questMissionOptionMatches(cat *masterdata.MissionCatalog, option int32, missionLinkId int32, questType model.QuestType, questID, chapterID int32) bool {
	if option == 0 {
		return true
	}
	if option == 4 { // any main quest
		return questType == model.QuestTypeMain
	}
	// Main-story OptionGroupId is not a QuestId. For example, character
	// missions for Yurie use option 55 ("character quest 10"), while quest
	// 55 is an unrelated main-story quest. Main-story targets are resolved
	// above from their chapter/template relationships; an unresolved one must
	// remain pending rather than match an equal-looking quest ID.
	if questType == model.QuestTypeMain {
		return false
	}

	// Check for global subquest groups BEFORE checking for specific quest IDs
	// This allows subquest missions (MissionLinkId=1) with global groups to match any Event quest
	switch option {
	case 5, 70: // subquests
		if missionLinkId == 1 {
			return questType == model.QuestTypeEvent
		}
		return questType == model.QuestTypeExtra
	case 71: // Dark Memory quests (event chapters 99001+, quest ids 110000-110999)
		return questType == model.QuestTypeEvent && questID >= 110000 && questID < 111000
	case 30029: // "Clear a daily quest N times" - the Daily Quest chapter family.
		// Option 30029 is a daily-quest condition group, not a concrete quest:
		// the quests the player actually clears are the EventQuestType=4 "Daily
		// Quest: Easy/Normal/Hard/Master" entries (ids 100001-100024, 100073-100076,
		// surfaced through the daily event chapters). Match those event clears.
		return questType == model.QuestTypeEvent && isDailyQuestID(questID)
	case 30030: // "Clear guerrilla quests N times" - the Guerrilla chapter family.
		// Option 30030 groups the EventQuestType=5 weapon "Guerrilla" quests
		// (Gun/1H/2H/Fists/Staff/Spear x Easy..Master, ids 100025-100048).
		return questType == model.QuestTypeEvent && isGuerrillaQuestID(questID)
	}

	if cat.QuestToEventChapter[option] != 0 {
		return option == questID
	}
	for _, knownChapterID := range cat.QuestToEventChapter {
		if knownChapterID == option {
			return false
		}
	}

	// These groups are global quest families, not concrete IDs. Unknown groups
	// are deliberately rejected: awarding a daily/event/difficulty mission for
	// an unrelated quest is worse than leaving that specialized mission pending
	// until its group is implemented explicitly.
	switch option {
	case 5, 70: // subquests
		return questType == model.QuestTypeExtra
	default:
		// Remaining extra-quest missions store the concrete m_quest.QuestId in
		// their option group. The daily (30029) and guerrilla (30030) families are
		// resolved in the switch above and never reach here. This fallback is safe
		// after the concrete main/event templates have been handled.
		return questType == model.QuestTypeExtra && option == questID
	}
}

// isDailyQuestID reports whether questID is one of the "Daily Quest"
// material quests (EventQuestType=4: ids 100001-100024 and 100073-100076,
// named "Daily Quest: Easy/Normal/Hard/Master"). These satisfy the hidden
// story missions whose condition option group is 30029 ("Clear a daily quest
// N times"). The 100001-100076 id band is exclusive to these event quests,
// so a plain range check is unambiguous.
func isDailyQuestID(questID int32) bool {
	return (questID >= 100001 && questID <= 100024) ||
		(questID >= 100073 && questID <= 100076)
}

// isGuerrillaQuestID reports whether questID is one of the weapon "Guerrilla"
// quests (EventQuestType=5 chapter 7: ids 100025-100048, Gun/1H/2H/Fists/
// Staff/Spear x Easy/Normal/Hard/Master). These satisfy the hidden story
// missions whose condition option group is 30030 ("Clear guerrilla quests N
// times").
func isGuerrillaQuestID(questID int32) bool {
	return questID >= 100025 && questID <= 100048
}

// ApplyMissionProgressEvent advances missions attached to an equipment or
// progression action. The master data uses MissionClearConditionOptionGroupId
// as the specific target for these conditions (zero means any target).
//
// It intentionally ignores quest-clear conditions: those also need to match
// quest domain and chapter and are handled by ApplyQuestClearMissionProgress.
func ApplyMissionProgressEvent(user *store.UserState, cat *masterdata.MissionCatalog, event MissionProgressEvent, nowMillis int64) {
	if cat == nil || event.ConditionType == missionClearConditionTypeQuestClearCount {
		return
	}
	if event.Delta <= 0 && event.CurrentValue <= 0 {
		return
	}

	for _, mission := range cat.ActiveMissionsAt(nowMillis) {
		// Skip category 1 (Daily) missions - they are hidden and should not be updated here
		if isMissionCategory1(mission, cat) {
			continue
		}

		// Skip blocked individual mission IDs (for excluding specific missions from categories 2 and 5)
		if blockedMissionIds[mission.MissionId] {
			continue
		}
		// Skip blocked pass missions (2000301-2002414)
		if isBlockedPassMission(mission.MissionId) {
			continue
		}

		if mission.MissionClearConditionType != event.ConditionType {
			continue
		}
		// Hidden-story score-rank missions carry option group 0 but actually
		// require a specific boss and party; they are advanced only by
		// ApplyBigHuntPartyScoreMissionProgress, never by the generic matcher.
		if event.ConditionType == missionConditionArenaBattle {
			if _, hidden := bigHuntPartyScoreSpecs[mission.MissionId]; hidden {
				continue
			}
		}
		if mission.MissionClearConditionOptionGroupId != 0 {
			// For BigHunt missions, match against appropriate group ID
			if event.ConditionType == missionConditionSubjugationBattle ||
				event.ConditionType == missionConditionBigHuntBattle ||
				event.ConditionType == missionConditionRhythmConcertTap {
				// Types 51 and 52: no target matching needed, skip this check.
				// Type 61: the option group is a self-referential condition group
				// id, not the client-side liveTypeId, so it cannot be matched
				// against the event target either.
			} else if event.ConditionType == missionConditionArenaBattle {
				// For score rank missions, match against MissionClearConditionOptionGroupId (boss ID)
				if event.TargetId == 0 || mission.MissionClearConditionOptionGroupId != event.TargetId {
					continue
				}
			} else if event.ConditionType == missionConditionExplorationClear {
				// For Exploration missions, match against MissionClearConditionOptionGroupId
				// If option group is 0, match any exploration. Otherwise match specific explore stage.
				if mission.MissionClearConditionOptionGroupId != 0 && mission.MissionClearConditionOptionGroupId != event.TargetId {
					continue
				}
			} else if event.ConditionType == missionConditionRhythmHighScore {
				// For Shooting/FlyingMama score missions (condition 29):
				// MCCOGI=0 means any game type, otherwise match the specific ExploreId.
				if mission.MissionClearConditionOptionGroupId != 0 && mission.MissionClearConditionOptionGroupId != event.TargetId {
					continue
				}
			} else if event.ConditionType == missionConditionCharacterBoardPanel {
				// For Character Board Panel missions (condition 54):
				// MCCOGI is the board option group (310001-310042, one per
				// character and monument type); the event carries the same id.
				if event.TargetId == 0 || mission.MissionClearConditionOptionGroupId != event.TargetId {
					continue
				}
			} else if (event.ConditionType == 6 || event.ConditionType == 8 || event.ConditionType == 43) &&
				cat.WeaponIdsByOptionGroup[mission.MissionClearConditionOptionGroupId] != nil {
				// Weapon-specific missions (condition types 6, 8 and 43) carry an
				// option group id that is not the WeaponId itself; resolve it to
				// the targeted weapon family and require the event weapon to be
				// part of it.
				weaponIds := cat.WeaponIdsByOptionGroup[mission.MissionClearConditionOptionGroupId]
				if event.TargetId == 0 || !weaponIds[event.TargetId] {
					continue
				}
			} else {
				if event.TargetId == 0 || mission.MissionClearConditionOptionGroupId != event.TargetId {
					continue
				}
			}
		}

		progress := user.Missions[mission.MissionId]
		if model.MissionProgressStatusType(progress.MissionProgressStatusType) >= model.MissionProgressStatusTypeClear {
			continue
		}
		if progress.MissionId == 0 {
			progress.MissionId = mission.MissionId
			progress.StartDatetime = nowMillis
		}

		if event.ConditionType == missionConditionArenaRank {
			// Arena rank missions (269xx) are inverted: a LOWER rank number is
			// better, so progress is binary — 0/1 until the player actually sits
			// at ClearConditionValue rank or above it. Storing the raw rank as
			// ProgressValue would make the client bar render a false full 1/1
			// (rank 2000 vs target 1500 clamps to the target) while the button
			// still says Try.
			progress.LatestVersion = nowMillis
			if event.CurrentValue <= mission.ClearConditionValue {
				progress.ProgressValue = mission.ClearConditionValue
				progress.MissionProgressStatusType = int32(model.MissionProgressStatusTypeClear)
				progress.ClearDatetime = nowMillis
				log.Printf("[Mission] mission %d CLEARED by condition type %d target %d", mission.MissionId, event.ConditionType, event.TargetId)
			} else {
				progress.ProgressValue = 0
				progress.MissionProgressStatusType = int32(model.MissionProgressStatusTypeInProgress)
			}
			user.Missions[mission.MissionId] = progress
			continue
		}

		if event.CurrentValue > 0 {
			if event.CurrentValue > progress.ProgressValue {
				progress.ProgressValue = event.CurrentValue
			}
		} else {
			progress.ProgressValue += event.Delta
		}
		progress.LatestVersion = nowMillis
		if progress.ProgressValue >= mission.ClearConditionValue {
			progress.ProgressValue = mission.ClearConditionValue
			progress.MissionProgressStatusType = int32(model.MissionProgressStatusTypeClear)
			progress.ClearDatetime = nowMillis
			log.Printf("[Mission] mission %d CLEARED by condition type %d target %d", mission.MissionId, event.ConditionType, event.TargetId)
		} else {
			progress.MissionProgressStatusType = int32(model.MissionProgressStatusTypeInProgress)
		}
		user.Missions[mission.MissionId] = progress
	}
}

func missionDomainMatchesQuestType(domainType int32, questType model.QuestType) bool {
	switch domainType {
	case missionLinkDomainAny:
		return true
	case missionLinkDomainQuestClear:
		return questType == model.QuestTypeMain ||
			questType == model.QuestTypeEvent ||
			questType == model.QuestTypeExtra
	case missionLinkDomainEventQuest:
		return questType == model.QuestTypeEvent
	case missionLinkDomainExtraQuest:
		return questType == model.QuestTypeExtra
	default:
		return false
	}
}

// questBossCount counts the number of bosses (BattleEnemyType == 2) that appear
// in any battle in the quest. This is used for boss-defeat mission progress
// (type 35 - for missions 220001-220022). Uses the composite NpcDeckKey to
// ensure correct deck lookup by (BattleNpcId, DeckType, BattleNpcDeckNumber).
func questBossCount(cat *masterdata.QuestCatalog, questId int32) int32 {
	if cat == nil {
		return 0
	}
	var count int32
	for _, sceneId := range cat.SceneIdsByQuestId[questId] {
		battleGroupId, ok := cat.BattleGroupBySceneId[sceneId]
		if !ok {
			continue
		}
		for _, battleId := range cat.BattleIdsByGroupId[battleGroupId] {
			battle, ok := cat.BattleByIdMap[battleId]
			if !ok {
				continue
			}
			// Use the composite key (BattleNpcId, DeckType, BattleNpcDeckNumber)
			// to ensure we get the correct deck. The same BattleNpcDeckNumber
			// can be used by different BattleNpcIds with different DeckTypes.
			deck, ok := cat.BattleNpcDeckByKey[masterdata.NpcDeckKey{
				BattleNpcId:         battle.BattleNpcId,
				DeckType:            battle.DeckType,
				BattleNpcDeckNumber: battle.BattleNpcDeckNumber,
			}]
			if !ok {
				continue
			}
			// Check each character slot in the deck
			for _, uuid := range []string{
				deck.BattleNpcDeckCharacterUuid01,
				deck.BattleNpcDeckCharacterUuid02,
				deck.BattleNpcDeckCharacterUuid03,
			} {
				if uuid == "" {
					continue
				}
				// Count bosses (BattleEnemyType == 2) in this deck
				for _, ct := range cat.BattleNpcDeckCharacterTypeByNpcId[battle.BattleNpcId] {
					if ct.BattleNpcDeckCharacterUuid == uuid && ct.BattleEnemyType == 2 {
						count++
					}
				}
			}
		}
	}
	return count
}

// ─────────────────────────────────────────────────────────────────────────────
// Solo-clear missions (type 62)
// ─────────────────────────────────────────────────────────────────────────────

// soloClearSpec pins a type-62 mission ("clear quest X using only character Y")
// to its target quest. The master data option group for these missions is a
// self-referential condition-group id, so the quest/character pairing lives
// here instead. Main quests are located via chapter/difficulty/number; Dark
// Lair quests share a NameQuestTextId per difficulty and are matched by it.
type soloClearSpec struct {
	GameChapter    int32
	QuestNumber    int32
	Difficulty     int32 // 1=Normal 2=Hard 3=Very Hard 4=EX Hard
	LairNameTextId int32 // NameQuestTextId shared by Dark Lair quests of one difficulty
	CharacterId    int32
}

var soloClearSpecs = map[int32]soloClearSpec{
	500056: {GameChapter: 4, QuestNumber: 10, Difficulty: 3, CharacterId: 1015},  // Akeha, ch.4 Q10 Very Hard
	500057: {LairNameTextId: 110010, CharacterId: 1015},                          // Akeha, Dark Lair (Hard)
	500058: {GameChapter: 7, QuestNumber: 10, Difficulty: 2, CharacterId: 1011},  // 063y, ch.7 Q10 Hard
	500059: {GameChapter: 8, QuestNumber: 10, Difficulty: 2, CharacterId: 1012},  // F66x, ch.8 Q10 Hard
	500084: {GameChapter: 5, QuestNumber: 10, Difficulty: 3, CharacterId: 1006},  // Argo, ch.5 Q10 Very Hard
	500085: {LairNameTextId: 110010, CharacterId: 1006},                          // Argo, Dark Lair (Hard)
	500087: {GameChapter: 10, QuestNumber: 10, Difficulty: 2, CharacterId: 1014}, // Griff, ch.10 Q10 Hard
}

// ApplySoloClearMissionProgress advances type-62 missions after a successful
// quest clear. Called from FinishMainQuest and FinishExtraQuest (Dark Lairs
// are extra quests).
func ApplySoloClearMissionProgress(user *store.UserState, missions *masterdata.MissionCatalog, quests *masterdata.QuestCatalog, questId int32, nowMillis int64) {
	if missions == nil || quests == nil {
		return
	}
	for _, m := range missions.ActiveMissionsAt(nowMillis) {
		if m.MissionClearConditionType != missionConditionSoloQuestClear {
			continue
		}
		spec, ok := soloClearSpecs[m.MissionId]
		if !ok {
			continue
		}
		if spec.LairNameTextId != 0 {
			if quests.QuestById[questId].NameQuestTextId != spec.LairNameTextId {
				continue
			}
		} else {
			dataChapter := missions.GameToDataChapter[spec.GameChapter]
			pos := masterdata.MainQuestPosition{DifficultyType: spec.Difficulty, QuestNumber: spec.QuestNumber}
			if dataChapter == 0 || missions.MainQuestByPosition[dataChapter][pos] != questId {
				continue
			}
		}
		if !questDeckIsSoloCharacter(user, quests, questId, spec.CharacterId) {
			continue
		}

		progress := user.Missions[m.MissionId]
		if model.MissionProgressStatusType(progress.MissionProgressStatusType) >= model.MissionProgressStatusTypeClear {
			continue
		}
		if progress.MissionId == 0 {
			progress.MissionId = m.MissionId
			progress.StartDatetime = nowMillis
		}
		progress.ProgressValue++
		progress.LatestVersion = nowMillis
		if progress.ProgressValue >= m.ClearConditionValue {
			progress.ProgressValue = m.ClearConditionValue
			progress.MissionProgressStatusType = int32(model.MissionProgressStatusTypeClear)
			progress.ClearDatetime = nowMillis
			log.Printf("[Mission] mission %d CLEARED by solo clear (quest=%d character=%d)", m.MissionId, questId, spec.CharacterId)
		} else {
			progress.MissionProgressStatusType = int32(model.MissionProgressStatusTypeInProgress)
		}
		user.Missions[m.MissionId] = progress
	}
}

// questDeckIsSoloCharacter reports whether every filled slot of the deck the
// user ran questId with belongs to characterId (at least one slot filled).
func questDeckIsSoloCharacter(user *store.UserState, quests *masterdata.QuestCatalog, questId, characterId int32) bool {
	deckNumber := user.Quests[questId].UserDeckNumber
	if deckNumber == 0 {
		deckNumber = 1
	}
	deck, ok := user.Decks[store.DeckKey{DeckType: model.DeckTypeQuest, UserDeckNumber: deckNumber}]
	if !ok {
		return false
	}
	filled := 0
	uuids := [3]string{deck.UserDeckCharacterUuid01, deck.UserDeckCharacterUuid02, deck.UserDeckCharacterUuid03}
	for _, uuid := range uuids {
		if uuid == "" {
			continue
		}
		filled++
		dc, ok := user.DeckCharacters[uuid]
		if !ok {
			return false
		}
		costume, ok := user.Costumes[dc.UserCostumeUuid]
		if !ok {
			return false
		}
		if quests.CostumeById[costume.CostumeId].CharacterId != characterId {
			return false
		}
	}
	return filled > 0
}

// ─────────────────────────────────────────────────────────────────────────────
// Hidden-story quest-clear missions (category 7, type 1)
// ─────────────────────────────────────────────────────────────────────────────

// hiddenStoryQuestClearSpec pins a hidden-story type-1 mission whose option
// group cannot be resolved from master data (character-in-loadout groups,
// dungeon groups and per-character Dark Memory groups are self-referential
// ids). A zero QuestIdFirst means any quest counts; a zero CharacterId means
// no loadout requirement.
type hiddenStoryQuestClearSpec struct {
	QuestIdFirst int32 // inclusive range of counting quest ids
	QuestIdLast  int32
	CharacterId  int32 // character that must be in the quest deck
}

var hiddenStoryQuestClearSpecs = map[int32]hiddenStoryQuestClearSpec{
	// "Clear N quests with <character> in your loadout"
	500001: {CharacterId: 1013}, // Lars
	500002: {CharacterId: 1013}, // Lars
	500006: {CharacterId: 1008}, // Rion
	500008: {CharacterId: 1022}, // Saryu
	500015: {CharacterId: 1009}, // Gayle
	500026: {CharacterId: 1012}, // F66x
	500032: {CharacterId: 1023}, // Priyet
	500038: {CharacterId: 1015}, // Akeha
	500041: {CharacterId: 1015}, // Akeha
	500077: {CharacterId: 1006}, // Argo
	500082: {CharacterId: 1014}, // Griff
	500095: {CharacterId: 1004}, // Levania
	500099: {CharacterId: 1019}, // Fio
	500113: {CharacterId: 1024}, // Marie
	500124: {CharacterId: 1007}, // Dimos
	500130: {CharacterId: 1007}, // Dimos
	500133: {CharacterId: 1014}, // Griff
	500141: {CharacterId: 1024}, // Marie
	500146: {CharacterId: 1010}, // Noelle

	// "Clear a dungeon 30 times": dungeon chapters 220001-220007 own event
	// quests 210001-210070 (7 dungeons x 10 floors).
	500072: {QuestIdFirst: 210001, QuestIdLast: 210070},
	500125: {QuestIdFirst: 210001, QuestIdLast: 210070},
	500138: {QuestIdFirst: 210001, QuestIdLast: 210070},

	// "Complete Floor 10 of Dungeon: The Dynast's Memories with Saryu in your
	// loadout": chapter 220001, floor 10 = quest 210010. Skip tickets never
	// reach FinishEventQuest, so the no-skip rule holds by construction.
	500007: {QuestIdFirst: 210010, QuestIdLast: 210010, CharacterId: 1022},

	// "Clear 10 Dark Memory quests for <character>": per-character Dark
	// Memory chapters (m_event_quest_chapter_character).
	500144: {QuestIdFirst: 110186, QuestIdLast: 110197}, // Marie   (chapter 99016)
	500159: {QuestIdFirst: 110234, QuestIdLast: 110245}, // Hina    (chapter 99020)
}

// ApplyHiddenStoryQuestClearMissionProgress advances the hidden-story type-1
// missions listed in hiddenStoryQuestClearSpecs after a successful quest
// clear. The generic quest-clear matcher never resolves their option groups
// (they collide with unrelated chapter ids and are rejected), so this is
// their only progress source. Called from FinishMainQuest, FinishEventQuest
// and FinishExtraQuest.
func ApplyHiddenStoryQuestClearMissionProgress(user *store.UserState, missions *masterdata.MissionCatalog, quests *masterdata.QuestCatalog, questId int32, nowMillis int64) {
	if missions == nil || quests == nil {
		return
	}
	for _, m := range missions.ActiveMissionsAt(nowMillis) {
		if m.MissionClearConditionType != missionClearConditionTypeQuestClearCount {
			continue
		}
		spec, ok := hiddenStoryQuestClearSpecs[m.MissionId]
		if !ok {
			continue
		}
		if spec.QuestIdFirst != 0 && (questId < spec.QuestIdFirst || questId > spec.QuestIdLast) {
			continue
		}
		if spec.CharacterId != 0 && !questDeckContainsCharacter(user, quests, questId, spec.CharacterId) {
			continue
		}

		progress := user.Missions[m.MissionId]
		if model.MissionProgressStatusType(progress.MissionProgressStatusType) >= model.MissionProgressStatusTypeClear {
			continue
		}
		if progress.MissionId == 0 {
			progress.MissionId = m.MissionId
			progress.StartDatetime = nowMillis
		}
		progress.ProgressValue++
		progress.LatestVersion = nowMillis
		if progress.ProgressValue >= m.ClearConditionValue {
			progress.ProgressValue = m.ClearConditionValue
			progress.MissionProgressStatusType = int32(model.MissionProgressStatusTypeClear)
			progress.ClearDatetime = nowMillis
			log.Printf("[Mission] hidden-story mission %d CLEARED by quest clear (quest=%d character=%d)", m.MissionId, questId, spec.CharacterId)
		} else {
			progress.MissionProgressStatusType = int32(model.MissionProgressStatusTypeInProgress)
		}
		user.Missions[m.MissionId] = progress
	}
}

// questDeckContainsCharacter reports whether any filled slot of the deck the
// user ran questId with belongs to characterId.
func questDeckContainsCharacter(user *store.UserState, quests *masterdata.QuestCatalog, questId, characterId int32) bool {
	deckNumber := user.Quests[questId].UserDeckNumber
	if deckNumber == 0 {
		deckNumber = 1
	}
	deck, ok := user.Decks[store.DeckKey{DeckType: model.DeckTypeQuest, UserDeckNumber: deckNumber}]
	if !ok {
		return false
	}
	uuids := [3]string{deck.UserDeckCharacterUuid01, deck.UserDeckCharacterUuid02, deck.UserDeckCharacterUuid03}
	for _, uuid := range uuids {
		if uuid == "" {
			continue
		}
		dc, ok := user.DeckCharacters[uuid]
		if !ok {
			continue
		}
		costume, ok := user.Costumes[dc.UserCostumeUuid]
		if !ok {
			continue
		}
		if quests.CostumeById[costume.CostumeId].CharacterId == characterId {
			return true
		}
	}
	return false
}

// ─────────────────────────────────────────────────────────────────────────────
// Hidden-story Manifestation score missions (category 7, type 53, option 0)
// ─────────────────────────────────────────────────────────────────────────────

// bigHuntPartyScoreSpec pins a type-53 mission ("Achieve a score rank of S1
// in Manifestation: <boss> with <characters> in your loadout") to its boss
// and required party. The option group is 0 in master data, so without this
// table any boss score would satisfy them. Boss ids follow the S5 missions'
// encoding (option 300000+bossId): 1=Fiery 2=Soggy 3=Windy 4=Bright.
type bigHuntPartyScoreSpec struct {
	BigHuntBossId int32
	CharacterIds  []int32
}

var bigHuntPartyScoreSpecs = map[int32]bigHuntPartyScoreSpec{
	500045: {BigHuntBossId: 3, CharacterIds: []int32{1013, 1014}}, // Windy, Lars & Griff
	500067: {BigHuntBossId: 1, CharacterIds: []int32{1011, 1012}}, // Fiery, 063y & F66x
	500068: {BigHuntBossId: 2, CharacterIds: []int32{1011, 1012}}, // Soggy, 063y & F66x
	500078: {BigHuntBossId: 1, CharacterIds: []int32{1022}},       // Fiery, Saryu
	500089: {BigHuntBossId: 1, CharacterIds: []int32{1013, 1014}}, // Fiery, Lars & Griff
	500102: {BigHuntBossId: 4, CharacterIds: []int32{1004}},       // Bright, Levania
	500108: {BigHuntBossId: 2, CharacterIds: []int32{1020}},       // Soggy, Hina
	500109: {BigHuntBossId: 4, CharacterIds: []int32{1021}},       // Bright, Yuzuki
}

// ApplyBigHuntPartyScoreMissionProgress advances the hidden-story type-53
// missions after a scored BigHunt run. The party is taken from the run's
// CostumeBattleInfo (the costumes that actually fought), so deck edits after
// the battle cannot spoof it. Score uses max semantics like the generic
// score-rank handler. Called from FinishBigHuntQuest.
func ApplyBigHuntPartyScoreMissionProgress(user *store.UserState, missions *masterdata.MissionCatalog, quests *masterdata.QuestCatalog, bigHuntBossId int32, score int64, nowMillis int64) {
	if missions == nil || quests == nil || score <= 0 {
		return
	}
	partyCharacters := map[int32]bool{}
	for _, ci := range user.BigHuntBattleDetail.CostumeBattleInfo {
		if characterId := quests.CostumeById[ci.CostumeId].CharacterId; characterId != 0 {
			partyCharacters[characterId] = true
		}
	}
	for _, m := range missions.ActiveMissionsAt(nowMillis) {
		if m.MissionClearConditionType != missionConditionArenaBattle {
			continue
		}
		spec, ok := bigHuntPartyScoreSpecs[m.MissionId]
		if !ok || spec.BigHuntBossId != bigHuntBossId {
			continue
		}
		requiredPresent := true
		for _, characterId := range spec.CharacterIds {
			if !partyCharacters[characterId] {
				requiredPresent = false
				break
			}
		}
		if !requiredPresent {
			continue
		}

		progress := user.Missions[m.MissionId]
		if model.MissionProgressStatusType(progress.MissionProgressStatusType) >= model.MissionProgressStatusTypeClear {
			continue
		}
		if progress.MissionId == 0 {
			progress.MissionId = m.MissionId
			progress.StartDatetime = nowMillis
		}
		if int32(score) > progress.ProgressValue {
			progress.ProgressValue = int32(score)
		}
		progress.LatestVersion = nowMillis
		if progress.ProgressValue >= m.ClearConditionValue {
			progress.ProgressValue = m.ClearConditionValue
			progress.MissionProgressStatusType = int32(model.MissionProgressStatusTypeClear)
			progress.ClearDatetime = nowMillis
			log.Printf("[Mission] hidden-story mission %d CLEARED by BigHunt score (boss=%d score=%d)", m.MissionId, bigHuntBossId, score)
		} else {
			progress.MissionProgressStatusType = int32(model.MissionProgressStatusTypeInProgress)
		}
		user.Missions[m.MissionId] = progress
	}
}

// initializeHiddenCategoryMissions initializes missions from categories 1, 3, and 4
// directly from master data. These categories are not tracked by the normal
// mission progression system, so they must be seeded once at game start with
// status Unknown (0) to hide them on the client while keeping them in the
// user's mission list. Additionally, specific mission IDs listed in hiddenMissionIds
// will also be hidden.
func InitializeHiddenCategoryMissions(user *store.UserState, cat *masterdata.MissionCatalog, nowMillis int64) {
	if cat == nil || user == nil {
		return
	}

	// Read all missions from master data (not from cat.Missions which is filtered)
	rows, err := utils.ReadTable[masterdata.EntityMMission]("m_mission")
	if err != nil {
		log.Printf("[Mission] InitializeHiddenCategoryMissions: read m_mission failed: %v", err)
		return
	}

	initialized := 0
	reset := 0
	for _, mission := range rows {
		// Check if mission is in the explicit hidden list or range
		isHidden := false

		// Check explicit IDs
		for _, hiddenId := range hiddenMissionIds {
			if mission.MissionId == hiddenId {
				isHidden = true
				break
			}
		}

		// Check ranges
		if !isHidden {
			for _, r := range hiddenMissionRanges {
				if mission.MissionId >= r.start && mission.MissionId <= r.end {
					isHidden = true
					break
				}
			}
		}

		if !isHidden {
			// Process categories 1, 3, and 4
			group, ok := cat.GroupById[mission.MissionGroupId]
			if !ok || (group.MissionCategoryType != 1 && group.MissionCategoryType != 3 && group.MissionCategoryType != 4) {
				continue
			}
		}

		// Check if this mission had non-hidden status before reset
		if existing, ok := user.Missions[mission.MissionId]; ok && existing.MissionProgressStatusType != 0 {
			reset++
		}

		// ALWAYS overwrite with Unknown status (0) to hide on client, even if already exists
		// This ensures that any progress or status changes are reset to hidden
		progress := store.UserMissionState{
			MissionId:                 mission.MissionId,
			StartDatetime:             nowMillis,
			MissionProgressStatusType: 0, // Unknown - hidden on client
			ProgressValue:             0,
			ClearDatetime:             0,
			LatestVersion:             nowMillis,
		}
		user.Missions[mission.MissionId] = progress
		initialized++
	}

	if initialized > 0 {
		if reset > 0 {
			log.Printf("[Mission] Initialized %d hidden category missions (1,3,4) for user %d (reset %d from visible status)", initialized, user.UserId, reset)
		} else {
			log.Printf("[Mission] Initialized %d hidden category missions (1,3,4) for user %d (all new)", initialized, user.UserId)
		}
	}
}
