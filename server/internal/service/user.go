package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/runtime"
	"lunar-tear/server/internal/store"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type UserServiceServer struct {
	pb.UnimplementedUserServiceServer
	users      store.UserRepository
	sessions   store.SessionRepository
	holder     *runtime.Holder
	authURL    string
	noRegister bool
	snaps      store.SnapshotRepository
	dir        *PlayerDirectory
}

func NewUserServiceServer(users store.UserRepository, sessions store.SessionRepository, holder *runtime.Holder, authURL string, noRegister bool, snaps store.SnapshotRepository, dir *PlayerDirectory) *UserServiceServer {
	if authURL != "" && !strings.Contains(authURL, "://") {
		authURL = "http://" + authURL
	}
	return &UserServiceServer{users: users, sessions: sessions, holder: holder, authURL: authURL, noRegister: noRegister, snaps: snaps, dir: dir}
}

func (s *UserServiceServer) RegisterUser(ctx context.Context, req *pb.RegisterUserRequest) (*pb.RegisterUserResponse, error) {
	if s.noRegister {
		ip := "invalid"

		if p, ok := peer.FromContext(ctx); ok {
			ip = p.Addr.String()
		}

		return nil, fmt.Errorf("Denied user registration: ip=%s uuid=%s", ip, req.Uuid)
	}

	platform := model.ClientPlatformFromContext(ctx)
	userId, err := s.users.CreateUser(req.Uuid, platform)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	log.Printf("[UserService] RegisterUser: uuid=%s terminalId=%s platform=%s -> userId=%d", req.Uuid, req.TerminalId, platform, userId)

	return &pb.RegisterUserResponse{
		UserId:    userId,
		Signature: fmt.Sprintf("sig_%d_%d", userId, gametime.Now().Unix()),
	}, nil
}

func (s *UserServiceServer) Auth(ctx context.Context, req *pb.AuthUserRequest) (*pb.AuthUserResponse, error) {
	platform := model.ClientPlatformFromContext(ctx)
	log.Printf("[UserService] Auth: uuid=%s platform=%s", req.Uuid, platform)

	// The TTL is checked against the GAME clock, and players routinely skip
	// days ahead to trigger daily/weekly resets — a short TTL would log them
	// out mid-skip (the client treats an expired session as fatal). A year
	// of game time keeps clock-skipping painless while still expiring
	// eventually. Note: re-Auth always mints a fresh key, and a request with
	// an unknown/expired key is rejected by the session guard instead of
	// silently falling back to another account.
	session, err := s.sessions.CreateSession(req.Uuid, 365*24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	user, err := s.users.LoadUser(session.UserId)
	if err != nil {
		return nil, fmt.Errorf("load user: %w", err)
	}

	return &pb.AuthUserResponse{
		SessionKey:     session.SessionKey,
		ExpireDatetime: timestamppb.New(session.ExpireAt),
		Signature:      req.Signature,
		UserId:         user.UserId,
	}, nil
}

func (s *UserServiceServer) GameStart(ctx context.Context, _ *emptypb.Empty) (*pb.GameStartResponse, error) {
	log.Printf("[UserService] GameStart")

	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if vals := md.Get("x-apb-session-key"); len(vals) > 0 {
			log.Printf("[UserService] GameStart session: %s", vals[0])
		}
	}

	userId := CurrentUserId(ctx, s.users, s.sessions)
	nowMillis := gametime.NowMillis()
	after, _ := s.users.UpdateUser(userId, func(user *store.UserState) {
		// Ensure all maps are initialized
		user.EnsureMaps()
		// Convert existing stamina consumables to gold on game start
		store.ConvertStaminaToGold(user)
		// Convert leftover summon tickets to free gems on game start
		store.ConvertSummonTicketsToGems(user)
		RefreshDailyLoginState(user, nowMillis)
		// Initialize hidden category missions (1, 3, 4) once at game start
		InitializeHiddenCategoryMissions(user, s.holder.Get().Mission, nowMillis)
		user.GameStartDatetime = nowMillis
	})
	if s.snaps != nil {
		if err := RefreshSnapshot(s.snaps, &after); err != nil {
			log.Printf("[UserService] GameStart snapshot refresh failed: %v", err)
		}
	}

	return &pb.GameStartResponse{}, nil
}

func RefreshDailyLoginState(user *store.UserState, nowMillis int64) bool {
	todayStart := gametime.StartOfDayMillisAt(nowMillis)
	if user.Login.LastLoginDatetime >= todayStart {
		return false
	}

	lastLoginStart := int64(0)
	if user.Login.LastLoginDatetime > 0 {
		lastLoginStart = gametime.StartOfDayMillisAt(user.Login.LastLoginDatetime)
	}
	yesterdayStart := gametime.StartOfDayMillisAt(todayStart - 1)

	user.Login.TotalLoginCount++
	if lastLoginStart == yesterdayStart {
		user.Login.ContinualLoginCount++
	} else {
		user.Login.ContinualLoginCount = 1
	}
	if user.Login.ContinualLoginCount > user.Login.MaxContinualLoginCount {
		user.Login.MaxContinualLoginCount = user.Login.ContinualLoginCount
	}
	user.Login.LastLoginDatetime = nowMillis
	user.Login.LatestVersion = nowMillis
	user.LoginBonus.LatestVersion = nowMillis

	return true
}

func (s *UserServiceServer) TransferUser(ctx context.Context, req *pb.TransferUserRequest) (*pb.TransferUserResponse, error) {
	platform := model.ClientPlatformFromContext(ctx)

	log.Printf("[UserService] TransferUser: platform=%s", platform)

	userId, err := s.users.GetUserByUUID(req.Uuid)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	return &pb.TransferUserResponse{
		UserId:    userId,
		Signature: "transferred-sig",
	}, nil
}

func (s *UserServiceServer) SetUserName(ctx context.Context, req *pb.SetUserNameRequest) (*pb.SetUserNameResponse, error) {
	log.Printf("[UserService] SetUserName: %s", req.Name)
	userId := CurrentUserId(ctx, s.users, s.sessions)
	s.users.UpdateUser(userId, func(user *store.UserState) {
		nowMillis := gametime.NowMillis()
		user.Profile.Name = req.Name
		user.Profile.NameUpdateDatetime = nowMillis
	})
	return &pb.SetUserNameResponse{}, nil
}

func (s *UserServiceServer) SetUserMessage(ctx context.Context, req *pb.SetUserMessageRequest) (*pb.SetUserMessageResponse, error) {
	log.Printf("[UserService] SetUserMessage: %s", req.Message)
	userId := CurrentUserId(ctx, s.users, s.sessions)
	s.users.UpdateUser(userId, func(user *store.UserState) {
		nowMillis := gametime.NowMillis()
		user.Profile.Message = req.Message
		user.Profile.MessageUpdateDatetime = nowMillis
	})
	return &pb.SetUserMessageResponse{}, nil
}

func (s *UserServiceServer) SetUserFavoriteCostumeId(ctx context.Context, req *pb.SetUserFavoriteCostumeIdRequest) (*pb.SetUserFavoriteCostumeIdResponse, error) {
	log.Printf("[UserService] SetUserFavoriteCostumeId: %d", req.FavoriteCostumeId)
	userId := CurrentUserId(ctx, s.users, s.sessions)
	cat := s.holder.Get()
	nowMillis := gametime.NowMillis()
	s.users.UpdateUser(userId, func(user *store.UserState) {
		user.Profile.FavoriteCostumeId = req.FavoriteCostumeId
		user.Profile.FavoriteCostumeIdUpdateDatetime = nowMillis

		// Mission 11: "Set a favorite character" (condition type 27)
		ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{
			ConditionType: 27, // missionConditionSetFavoriteCharacter
			Delta:         1,
		}, nowMillis)
	})
	return &pb.SetUserFavoriteCostumeIdResponse{}, nil
}

func (s *UserServiceServer) GetUserProfile(ctx context.Context, req *pb.GetUserProfileRequest) (*pb.GetUserProfileResponse, error) {
	log.Printf("[UserService] GetUserProfile: playerId=%d", req.PlayerId)
	userId := req.PlayerId
	if userId == 0 {
		userId = CurrentUserId(ctx, s.users, s.sessions)
	}

	// Bots have no user rows at all: synthesize the profile from the same
	// deterministic generators that build the bot card and its defense deck,
	// so the history screen gets a fully populated card instead of an empty
	// one (an empty profile hangs the client's card rendering).
	if IsBotId(userId) {
		return s.botProfile(userId), nil
	}

	user, err := s.users.LoadUser(userId)
	if err != nil {
		return &pb.GetUserProfileResponse{}, nil
	}

	// The player's arena defense deck is what the profile is opened from
	// (battle history); show it with one entry per occupied slot. Resolve it
	// exactly like battles do (PickDefenseDeck) so the card always displays
	// the party opponents actually fight, even when DefenseDeckNumber is 0.
	deckCharacters := []*pb.ProfileDeckCharacter{}
	deckType, deckNumber := PickDefenseDeck(&user)
	if pvp, ok := user.Decks[store.DeckKey{DeckType: deckType, UserDeckNumber: deckNumber}]; ok {
		for _, uuid := range []string{pvp.UserDeckCharacterUuid01, pvp.UserDeckCharacterUuid02, pvp.UserDeckCharacterUuid03} {
			if uuid == "" {
				continue
			}
			deckCharacter, ok := user.DeckCharacters[uuid]
			if !ok {
				continue
			}
			costumeId := int32(0)
			if costume, ok := user.Costumes[deckCharacter.UserCostumeUuid]; ok {
				costumeId = costume.CostumeId
			}
			mainWeaponId := int32(0)
			mainWeaponLevel := int32(0)
			if weapon, ok := user.Weapons[deckCharacter.MainUserWeaponUuid]; ok {
				mainWeaponId = weapon.WeaponId
				mainWeaponLevel = weapon.Level
			}
			deckCharacters = append(deckCharacters, &pb.ProfileDeckCharacter{
				CostumeId:       costumeId,
				MainWeaponId:    mainWeaponId,
				MainWeaponLevel: mainWeaponLevel,
			})
		}
	}

	// The player card's arena block: the current rank is always derived live
	// from the ladder snapshot, so it is fresh after every battle without any
	// extra refresh step; the grade follows the current rating; the best rank
	// only counts when it was recorded in the still-active season.
	pvpInfo := &pb.ProfilePvpInfo{}
	if rank, err := s.snaps.RankOfPlayer(userId); err == nil && rank > 0 {
		pvpInfo.CurrentRank = int32(rank)
	}
	if cat := s.holder.Get(); cat.Pvp != nil {
		nowMillis := gametime.NowMillis()
		if season, ok := cat.Pvp.CurrentSeason(nowMillis); ok {
			if grade, ok := cat.Pvp.GradeForPoint(season.GradeGroupId, user.Pvp.PvpPoint); ok {
				pvpInfo.CurrentGradeId = grade.GradeId
			}
			if user.Pvp.MaxSeasonRankSeasonId == season.SeasonId {
				pvpInfo.MaxSeasonRank = user.Pvp.MaxSeasonRank
			}
		}
	}

	return &pb.GetUserProfileResponse{
		Level:             user.Status.Level,
		Name:              user.Profile.Name,
		FavoriteCostumeId: user.Profile.FavoriteCostumeId,
		Message:           user.Profile.Message,
		IsFriend:          false,
		LatestUsedDeck: &pb.ProfileDeck{
			Power:         maxDeckPower(&user),
			DeckCharacter: deckCharacters,
		},
		PvpInfo: pvpInfo,
		GamePlayHistory: &pb.GamePlayHistory{
			HistoryItem:              []*pb.PlayHistoryItem{},
			HistoryCategoryGraphItem: []*pb.PlayHistoryCategoryGraphItem{},
		},
	}, nil
}

// botProfile builds a complete profile card for a bot: name, level and
// favorite costume from the deterministic bot card, and the bot's synthesized
// defense deck (the same one battles use). Every field the client renders is
// populated — an empty response makes the profile screen hang.
func (s *UserServiceServer) botProfile(playerId int64) *pb.GetUserProfileResponse {
	deckCharacters := []*pb.ProfileDeckCharacter{}
	power := int32(0)
	if s.dir != nil {
		if card, ok := s.dir.CardFor(playerId); ok {
			for _, ch := range s.dir.DefenseDeckOf(card) {
				entry := &pb.ProfileDeckCharacter{}
				if ch.Costume != nil {
					entry.CostumeId = ch.Costume.CostumeId
				}
				if ch.MainWeapon != nil {
					entry.MainWeaponId = ch.MainWeapon.WeaponId
					entry.MainWeaponLevel = ch.MainWeapon.Level
				}
				deckCharacters = append(deckCharacters, entry)
			}
			power = card.MaxDeckPower
			// Bots live only on the ladder: their current position doubles as
			// the season best, same figures the ranking table shows.
			botPvpInfo := &pb.ProfilePvpInfo{}
			if rank, err := s.snaps.RankOfPlayer(playerId); err == nil && rank > 0 {
				botPvpInfo.CurrentRank = int32(rank)
				botPvpInfo.MaxSeasonRank = int32(rank)
			}
			return &pb.GetUserProfileResponse{
				Level:             card.Level,
				Name:              card.Name,
				FavoriteCostumeId: card.FavoriteCostumeId,
				IsFriend:          false,
				LatestUsedDeck: &pb.ProfileDeck{
					Power:         power,
					DeckCharacter: deckCharacters,
				},
				PvpInfo: botPvpInfo,
				GamePlayHistory: &pb.GamePlayHistory{
					HistoryItem:              []*pb.PlayHistoryItem{},
					HistoryCategoryGraphItem: []*pb.PlayHistoryCategoryGraphItem{},
				},
			}
		}
	}
	return &pb.GetUserProfileResponse{
		Name:              "???",
		Level:             1,
		FavoriteCostumeId: 11001,
		IsFriend:          false,
		LatestUsedDeck: &pb.ProfileDeck{
			DeckCharacter: deckCharacters,
		},
		PvpInfo: &pb.ProfilePvpInfo{},
		GamePlayHistory: &pb.GamePlayHistory{
			HistoryItem:              []*pb.PlayHistoryItem{},
			HistoryCategoryGraphItem: []*pb.PlayHistoryCategoryGraphItem{},
		},
	}
}

func (s *UserServiceServer) SetBirthYearMonth(ctx context.Context, req *pb.SetBirthYearMonthRequest) (*pb.SetBirthYearMonthResponse, error) {
	log.Printf("[UserService] SetBirthYearMonth: %d/%d", req.BirthYear, req.BirthMonth)
	userId := CurrentUserId(ctx, s.users, s.sessions)
	if _, err := s.users.UpdateUser(userId, func(user *store.UserState) {
		user.BirthYear = req.BirthYear
		user.BirthMonth = req.BirthMonth
	}); err != nil {
		// Surface it: otherwise the client believes the age was persisted and
		// keeps re-asking for it on every launch.
		log.Printf("[UserService] SetBirthYearMonth: save for user %d failed: %v", userId, err)
		return nil, fmt.Errorf("save birth year/month: %w", err)
	}
	return &pb.SetBirthYearMonthResponse{}, nil
}

func (s *UserServiceServer) GetBirthYearMonth(ctx context.Context, _ *emptypb.Empty) (*pb.GetBirthYearMonthResponse, error) {
	userId := CurrentUserId(ctx, s.users, s.sessions)
	user, err := s.users.LoadUser(userId)
	if err != nil {
		return &pb.GetBirthYearMonthResponse{BirthYear: 2000, BirthMonth: 1}, nil
	}
	return &pb.GetBirthYearMonthResponse{BirthYear: user.BirthYear, BirthMonth: user.BirthMonth}, nil
}

func (s *UserServiceServer) GetChargeMoney(ctx context.Context, _ *emptypb.Empty) (*pb.GetChargeMoneyResponse, error) {
	userId := CurrentUserId(ctx, s.users, s.sessions)
	user, err := s.users.LoadUser(userId)
	if err != nil {
		return &pb.GetChargeMoneyResponse{ChargeMoneyThisMonth: 0}, nil
	}
	return &pb.GetChargeMoneyResponse{ChargeMoneyThisMonth: user.ChargeMoneyThisMonth}, nil
}

func (s *UserServiceServer) SetUserSetting(ctx context.Context, req *pb.SetUserSettingRequest) (*pb.SetUserSettingResponse, error) {
	log.Printf("[UserService] SetUserSetting: isNotifyPurchaseAlert=%v", req.IsNotifyPurchaseAlert)
	userId := CurrentUserId(ctx, s.users, s.sessions)
	s.users.UpdateUser(userId, func(user *store.UserState) {
		user.Setting.IsNotifyPurchaseAlert = req.IsNotifyPurchaseAlert
	})
	return &pb.SetUserSettingResponse{}, nil
}

func (s *UserServiceServer) GetAndroidArgs(ctx context.Context, req *pb.GetAndroidArgsRequest) (*pb.GetAndroidArgsResponse, error) {
	return &pb.GetAndroidArgsResponse{Nonce: "Mama", ApiKey: "1234567890"}, nil
}

func (s *UserServiceServer) GetBackupToken(ctx context.Context, req *pb.GetBackupTokenRequest) (*pb.GetBackupTokenResponse, error) {
	userId := CurrentUserId(ctx, s.users, s.sessions)
	user, err := s.users.LoadUser(userId)
	if err != nil {
		return &pb.GetBackupTokenResponse{BackupToken: "mock-backup-token"}, nil
	}
	return &pb.GetBackupTokenResponse{BackupToken: user.BackupToken}, nil
}

func (s *UserServiceServer) CheckTransferSetting(ctx context.Context, _ *emptypb.Empty) (*pb.CheckTransferSettingResponse, error) {
	return &pb.CheckTransferSettingResponse{}, nil
}

func (s *UserServiceServer) GetUserGamePlayNote(ctx context.Context, req *pb.GetUserGamePlayNoteRequest) (*pb.GetUserGamePlayNoteResponse, error) {
	return &pb.GetUserGamePlayNoteResponse{}, nil
}

func (s *UserServiceServer) resolveAuthToken(token string) (facebookId int64, err error) {
	if s.authURL == "" {
		return 0, status.Error(codes.FailedPrecondition, "auth server not configured (--auth-url)")
	}

	resp, err := http.Get(s.authURL + "/me?access_token=" + token)
	if err != nil {
		return 0, status.Errorf(codes.Internal, "auth server unreachable: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, status.Error(codes.Unauthenticated, "invalid or expired token")
	}

	var body struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, status.Errorf(codes.Internal, "decode auth response: %v", err)
	}
	if body.ID == "" {
		return 0, status.Error(codes.Unauthenticated, "auth server returned empty id")
	}

	id, err := strconv.ParseInt(body.ID, 10, 64)
	if err != nil {
		return 0, status.Errorf(codes.Internal, "invalid auth id %q: %v", body.ID, err)
	}
	return id, nil
}

func (s *UserServiceServer) SetFacebookAccount(ctx context.Context, req *pb.SetFacebookAccountRequest) (*pb.SetFacebookAccountResponse, error) {
	log.Printf("[UserService] SetFacebookAccount")

	fbId, err := s.resolveAuthToken(req.Token)
	if err != nil {
		return nil, err
	}

	userId := CurrentUserId(ctx, s.users, s.sessions)
	if err := s.users.SetFacebookId(userId, fbId); err != nil {
		return nil, fmt.Errorf("set facebook id: %w", err)
	}
	log.Printf("[UserService] linked facebook_id=%d to user_id=%d", fbId, userId)

	// Mission 12: "Use Data Transfer to back up your game data" (condition type 38)
	// Triggered when the player links their account (Data Transfer / backup).
	cat := s.holder.Get()
	nowMillis := gametime.NowMillis()
	s.users.UpdateUser(userId, func(user *store.UserState) {
		ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{
			ConditionType: 38,
			Delta:         1,
		}, nowMillis)
	})

	return &pb.SetFacebookAccountResponse{}, nil
}

func (s *UserServiceServer) UnsetFacebookAccount(ctx context.Context, _ *emptypb.Empty) (*pb.UnsetFacebookAccountResponse, error) {
	log.Printf("[UserService] UnsetFacebookAccount")

	userId := CurrentUserId(ctx, s.users, s.sessions)
	if err := s.users.ClearFacebookId(userId); err != nil {
		return nil, fmt.Errorf("clear facebook id: %w", err)
	}
	log.Printf("[UserService] unlinked facebook from user_id=%d", userId)
	return &pb.UnsetFacebookAccountResponse{}, nil
}

func (s *UserServiceServer) TransferUserByFacebook(ctx context.Context, req *pb.TransferUserByFacebookRequest) (*pb.TransferUserByFacebookResponse, error) {
	log.Printf("[UserService] TransferUserByFacebook: uuid=%s", req.Uuid)

	fbId, err := s.resolveAuthToken(req.Token)
	if err != nil {
		return nil, err
	}

	userId, err := s.users.GetUserByFacebookId(fbId)
	if err != nil {
		return nil, status.Error(codes.NotFound, "no account linked to this login")
	}

	if err := s.users.UpdateUUID(userId, req.Uuid); err != nil {
		return nil, fmt.Errorf("update uuid: %w", err)
	}

	log.Printf("[UserService] transferred facebook_id=%d -> user_id=%d with new uuid=%s", fbId, userId, req.Uuid)

	return &pb.TransferUserByFacebookResponse{
		UserId:    userId,
		Signature: fmt.Sprintf("fb_transfer_%d_%d", userId, gametime.Now().Unix()),
	}, nil
}
