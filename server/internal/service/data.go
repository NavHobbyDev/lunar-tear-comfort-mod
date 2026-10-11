package service

import (
	"context"
	"fmt"
	"log"
	"os"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/runtime"
	"lunar-tear/server/internal/store"
	"lunar-tear/server/internal/userdata"

	"google.golang.org/protobuf/types/known/emptypb"
)

// masterDataBinPath is the canonical location of the encrypted master data
// file. The mtime of this file is folded into the version string so the
// client invalidates its cache as soon as an admin reload swaps it in.
const masterDataBinPath = "assets/release/20240404193219.bin.e"

// masterDataBaseVersion preserves the historical "yyyymmddHHMMSS" value the
// client has always seen; we suffix it with the file mtime to force a
// re-download when content changes.
const masterDataBaseVersion = "20240404193219"

type DataServiceServer struct {
	pb.UnimplementedDataServiceServer
	users    store.UserRepository
	sessions store.SessionRepository
	holder   *runtime.Holder
}

func NewDataServiceServer(users store.UserRepository, sessions store.SessionRepository, holder *runtime.Holder) *DataServiceServer {
	return &DataServiceServer{users: users, sessions: sessions, holder: holder}
}

func (s *DataServiceServer) GetLatestMasterDataVersion(ctx context.Context, _ *emptypb.Empty) (*pb.MasterDataGetLatestVersionResponse, error) {
	version := masterDataBaseVersion
	if info, err := os.Stat(masterDataBinPath); err == nil {
		version = fmt.Sprintf("%s_%d", masterDataBaseVersion, info.ModTime().UnixMilli())
	} else {
		log.Printf("[DataService] stat %s: %v (falling back to base version)", masterDataBinPath, err)
	}

	// Resolve the user's master-data variant and other settings from the
	// persisted user_options row (managed on the octo web page).
	userId := CurrentUserId(ctx, s.users, s.sessions)
	if user, err := s.users.LoadUser(userId); err == nil {
		settings := userdata.ResolveOptions(user.Options)
		if settings.MasterDataVariant != "" {
			version = fmt.Sprintf("%s_%s", version, settings.MasterDataVariant)
		}
	}

	log.Printf("[DataService] GetLatestMasterDataVersion -> %s", version)
	return &pb.MasterDataGetLatestVersionResponse{
		LatestMasterDataVersion: version,
	}, nil
}

func (s *DataServiceServer) GetUserDataNameV2(ctx context.Context, _ *emptypb.Empty) (*pb.UserDataGetNameResponseV2, error) {
	log.Printf("[DataService] GetUserDataNameV2")
	return &pb.UserDataGetNameResponseV2{
		TableNameList: []*pb.TableNameList{
			{TableName: defaultTableNames()},
		},
	}, nil
}

func (s *DataServiceServer) GetUserData(ctx context.Context, req *pb.UserDataGetRequest) (*pb.UserDataGetResponse, error) {
	log.Printf("[DataService] GetUserData: tables=%v", req.TableName)

	userId := CurrentUserId(ctx, s.users, s.sessions)
	user, err := s.users.LoadUser(userId)
	if err != nil {
		return nil, fmt.Errorf("snapshot user: %w", err)
	}
	nowMillis := gametime.NowMillis()
	needsDailyRefresh := user.Login.LastLoginDatetime < gametime.StartOfDayMillisAt(nowMillis)

	// Force stamina back to the level-based maximum on load: the client still
	// validates stamina for skips even though the server never consumes it.
	var maxStaminaMillis int32
	if questHandler := s.holder.Get().QuestHandler; questHandler != nil {
		if maxStamina, ok := questHandler.MaxStaminaByLevel[user.Status.Level]; ok {
			maxStaminaMillis = maxStamina * 1000
		}
	}
	needsStaminaSet := maxStaminaMillis > 0 && user.Status.StaminaMilliValue != maxStaminaMillis

	// Keep the two derived lottery-result tables in sync with accepted rolls.
	// A previous recovery only checked whether both tables were completely
	// empty. It therefore missed a partial state, such as a first-slot bonus
	// absent while another slot already had a result row.
	if needsDailyRefresh || needsStaminaSet || costumeLotteryEffectsNeedRebuild(s.holder.Get().Costume, &user) {
		log.Printf("[DataService] Updating user - needsDailyRefresh=%v, needsStaminaSet=%v (stamina=%d)",
			needsDailyRefresh, needsStaminaSet, user.Status.StaminaMilliValue)
		user, err = s.users.UpdateUser(userId, func(current *store.UserState) {
			RefreshDailyLoginState(current, nowMillis)
			// Reinitialize hidden category missions on daily refresh to keep them hidden
			if needsDailyRefresh {
				InitializeHiddenCategoryMissions(current, s.holder.Get().Mission, nowMillis)
			}
			if maxStaminaMillis > 0 {
				// Set stamina directly, bypassing SettleStamina so the value is
				// exactly the level-based maximum regardless of regen state.
				current.Status.StaminaMilliValue = maxStaminaMillis
				current.Status.StaminaUpdateDatetime = nowMillis
			}
			if costumeLotteryEffectsNeedRebuild(s.holder.Get().Costume, current) {
				rebuildAllCostumeLotteryEffects(s.holder.Get().Costume, current)
			}
		})
		if err != nil {
			return nil, fmt.Errorf("refresh user data state: %w", err)
		}
	}

	// Project only the tables the client actually requested. Building the full
	// ~100-table map and discarding all but a few (the old FullClientTableMap +
	// SelectTables path) re-serialized the entire save on every fetch, so map
	// loads got slower as the player accumulated cleared content.
	result := userdata.ProjectTables(user, req.TableName)
	return &pb.UserDataGetResponse{
		UserDataJson: result,
	}, nil
}

func costumeLotteryEffectsNeedRebuild(catalog *masterdata.CostumeCatalog, user *store.UserState) bool {
	if len(user.CostumeLotteryEffects) == 0 {
		return false
	}

	// Calculate the expected results in a copy. Timestamps are deliberately
	// ignored below: only the actual bonus values determine whether repair is
	// necessary.
	expected := store.CloneUserState(*user)
	expected.CostumeLotteryEffectAbilities = make(map[store.CostumeLotteryEffectKey]store.UserCostumeLotteryEffectAbilityState)
	expected.CostumeLotteryEffectStatusUps = make(map[store.CostumeLotteryEffectStatusUpKey]store.UserCostumeLotteryEffectStatusUpState)
	rebuildAllCostumeLotteryEffects(catalog, &expected)

	if len(user.CostumeLotteryEffectAbilities) != len(expected.CostumeLotteryEffectAbilities) ||
		len(user.CostumeLotteryEffectStatusUps) != len(expected.CostumeLotteryEffectStatusUps) {
		return true
	}
	for key, want := range expected.CostumeLotteryEffectAbilities {
		got, ok := user.CostumeLotteryEffectAbilities[key]
		if !ok || got.UserCostumeUuid != want.UserCostumeUuid || got.SlotNumber != want.SlotNumber ||
			got.AbilityId != want.AbilityId || got.AbilityLevel != want.AbilityLevel {
			return true
		}
	}
	for key, want := range expected.CostumeLotteryEffectStatusUps {
		got, ok := user.CostumeLotteryEffectStatusUps[key]
		if !ok || got.UserCostumeUuid != want.UserCostumeUuid ||
			got.StatusCalculationType != want.StatusCalculationType || got.Hp != want.Hp ||
			got.Attack != want.Attack || got.Vitality != want.Vitality || got.Agility != want.Agility ||
			got.CriticalRatio != want.CriticalRatio || got.CriticalAttack != want.CriticalAttack {
			return true
		}
	}
	return false
}

func defaultTableNames() []string {
	return []string{
		"IUser",
		"IUserApple",
		"IUserAutoSaleSettingDetail",
		"IUserBeginnerCampaign",
		"IUserBigHuntMaxScore",
		"IUserBigHuntProgressStatus",
		"IUserBigHuntScheduleMaxScore",
		"IUserBigHuntStatus",
		"IUserBigHuntWeeklyMaxScore",
		"IUserBigHuntWeeklyStatus",
		"IUserCageOrnamentReward",
		"IUserCharacter",
		"IUserCharacterBoard",
		"IUserCharacterBoardAbility",
		"IUserCharacterBoardCompleteReward",
		"IUserCharacterBoardStatusUp",
		"IUserCharacterCostumeLevelBonus",
		"IUserCharacterRebirth",
		"IUserCharacterViewerField",
		"IUserComebackCampaign",
		"IUserCompanion",
		"IUserConsumableItem",
		"IUserContentsStory",
		"IUserCostume",
		"IUserCostumeActiveSkill",
		"IUserCostumeAwakenStatusUp",
		"IUserCostumeLevelBonusReleaseStatus",
		"IUserCostumeLotteryEffect",
		"IUserCostumeLotteryEffectAbility",
		"IUserCostumeLotteryEffectPending",
		"IUserCostumeLotteryEffectStatusUp",
		"IUserDeck",
		"IUserDeckCharacter",
		"IUserDeckCharacterDressupCostume",
		"IUserDeckLimitContentRestricted",
		"IUserDeckPartsGroup",
		"IUserDeckSubWeaponGroup",
		"IUserDeckTypeNote",
		"IUserDokan",
		"IUserEventQuestDailyGroupCompleteReward",
		"IUserEventQuestGuerrillaFreeOpen",
		"IUserEventQuestLabyrinthSeason",
		"IUserEventQuestLabyrinthStage",
		"IUserEventQuestProgressStatus",
		"IUserEventQuestTowerAccumulationReward",
		"IUserExplore",
		"IUserExploreScore",
		"IUserExtraQuestProgressStatus",
		"IUserFacebook",
		"IUserGem",
		"IUserGimmick",
		"IUserGimmickOrnamentProgress",
		"IUserGimmickSequence",
		"IUserGimmickUnlock",
		"IUserImportantItem",
		"IUserLimitedOpen",
		// "IUserLogin",
		"IUserLoginBonus",
		"IUserMainQuestFlowStatus",
		"IUserMainQuestMainFlowStatus",
		"IUserMainQuestProgressStatus",
		"IUserMainQuestReplayFlowStatus",
		"IUserMainQuestSeasonRoute",
		"IUserMaterial",
		"IUserMission",
		"IUserMissionCompletionProgress",
		"IUserMissionPassPoint",
		"IUserMovie",
		"IUserNaviCutIn",
		"IUserOmikuji",
		"IUserParts",
		"IUserPartsGroupNote",
		"IUserPartsPreset",
		"IUserPartsPresetTag",
		"IUserPartsStatusSub",
		"IUserPortalCageStatus",
		"IUserPossessionAutoConvert",
		"IUserPremiumItem",
		"IUserProfile",
		"IUserPvpDefenseDeck",
		"IUserPvpStatus",
		"IUserPvpWeeklyResult",
		"IUserQuest",
		"IUserQuestAutoOrbit",
		"IUserQuestLimitContentStatus",
		"IUserQuestMission",
		"IUserQuestReplayFlowRewardGroup",
		"IUserQuestSceneChoice",
		"IUserQuestSceneChoiceHistory",
		// "IUserSetting",
		"IUserShopItem",
		"IUserShopReplaceable",
		"IUserShopReplaceableLineup",
		"IUserSideStoryQuest",
		"IUserSideStoryQuestSceneProgressStatus",
		"IUserStatus",
		"IUserThought",
		"IUserTripleDeck",
		"IUserTutorialProgress",
		"IUserWeapon",
		"IUserWeaponAbility",
		"IUserWeaponAwaken",
		"IUserWeaponNote",
		"IUserWeaponSkill",
		"IUserWeaponStory",
		"IUserWebviewPanelMission",
	}
}
