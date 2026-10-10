package service

import (
	"context"
	"log"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/runtime"
	"lunar-tear/server/internal/store"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

type FriendServiceServer struct {
	pb.UnimplementedFriendServiceServer
	users    store.UserRepository
	sessions store.SessionRepository
	dir      *PlayerDirectory
	holder   *runtime.Holder
}

func NewFriendServiceServer(users store.UserRepository, sessions store.SessionRepository, dir *PlayerDirectory, holder *runtime.Holder) *FriendServiceServer {
	return &FriendServiceServer{users: users, sessions: sessions, dir: dir, holder: holder}
}

func (s *FriendServiceServer) cardFor(playerId int64) (PlayerCard, bool) {
	if s.dir.IsBot(playerId) {
		return botCardFromId(s.dir.pools(), playerId), true
	}
	snap, err := s.dir.snaps.GetSnapshot(playerId)
	if err != nil {
		return PlayerCard{}, false
	}
	return cardFromSnapshot(snap), true
}

func (s *FriendServiceServer) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.GetUserResponse, error) {
	card, ok := s.cardFor(req.PlayerId)
	if !ok {
		return &pb.GetUserResponse{}, nil
	}
	return &pb.GetUserResponse{User: userProto(card)}, nil
}

func (s *FriendServiceServer) GetFriendList(ctx context.Context, req *pb.GetFriendListRequest) (*pb.GetFriendListResponse, error) {
	userId := CurrentUserId(ctx, s.users, s.sessions)
	// GetFriendList is one of the carrier RPCs for the client's Cage
	// accumulators (distance walked, Mama taps); fold them into missions.
	if cage := req.GetCageMeasurableValues(); cage != nil && (cage.RunningDistanceMeters > 0 || cage.MamaTappedCount > 0) {
		s.users.UpdateUser(userId, func(user *store.UserState) {
			applyCageMeasurableValues(user, s.holder.Get().Mission, cage, gametime.NowMillis())
		})
	}
	user, err := s.users.LoadUser(userId)
	if err != nil {
		return &pb.GetFriendListResponse{}, nil
	}
	maybeResetCheerDay(&user)
	var friends []*pb.FriendUser
	var sent, received int32
	for pid, edge := range user.Friends {
		card, ok := s.cardFor(pid)
		if !ok {
			continue
		}
		friends = append(friends, friendUserProto(card, edge))
		if edge.CheerSentToday {
			sent++
		}
		if edge.CheerReceivedPending {
			received++
		}
	}
	return &pb.GetFriendListResponse{
		FriendUser:         friends,
		SendCheerCount:     sent,
		ReceivedCheerCount: received,
	}, nil
}

func (s *FriendServiceServer) GetFriendRequestList(ctx context.Context, req *emptypb.Empty) (*pb.GetFriendRequestListResponse, error) {
	userId := CurrentUserId(ctx, s.users, s.sessions)
	user, err := s.users.LoadUser(userId)
	if err != nil {
		return &pb.GetFriendRequestListResponse{}, nil
	}
	var users []*pb.User
	for pid := range user.IncomingFriendRequests {
		if card, ok := s.cardFor(pid); ok {
			users = append(users, userProto(card))
		}
	}
	return &pb.GetFriendRequestListResponse{User: users}, nil
}

func (s *FriendServiceServer) SearchRecommendedUsers(ctx context.Context, req *emptypb.Empty) (*pb.SearchRecommendedUsersResponse, error) {
	userId := CurrentUserId(ctx, s.users, s.sessions)
	user, err := s.users.LoadUser(userId)
	if err != nil {
		return &pb.SearchRecommendedUsersResponse{}, nil
	}
	cards := s.dir.RealPlayersNear(user.PlayerId, user.Pvp.PvpPoint, 20)
	filtered := make([]PlayerCard, 0, len(cards))
	for _, c := range cards {
		if _, f := user.Friends[c.PlayerId]; f {
			continue
		}
		if _, q := user.OutgoingFriendRequests[c.PlayerId]; q {
			continue
		}
		filtered = append(filtered, c)
	}
	filtered = s.dir.FillWithBots(filtered, 10, user.PlayerId, dayBucket(), user.Pvp.PvpPoint)
	if len(filtered) > 10 {
		filtered = filtered[:10]
	}
	var out []*pb.User
	for _, c := range filtered {
		out = append(out, userProto(c))
	}
	if len(out) > 0 {
		f := out[0]
		log.Printf("[FriendService] SearchRecommendedUsers: returning %d users (real=%d) first: id=%d name=%q level=%d power=%d costume=%d login=%v",
			len(out), len(cards), f.PlayerId, f.UserName, f.Level, f.MaxDeckPower, f.FavoriteCostumeId, f.LastLoginDatetime != nil)
	} else {
		log.Printf("[FriendService] SearchRecommendedUsers: returning 0 users (real=%d)", len(cards))
	}
	return &pb.SearchRecommendedUsersResponse{Users: out}, nil
}

func (s *FriendServiceServer) SendFriendRequest(ctx context.Context, req *pb.SendFriendRequestRequest) (*pb.SendFriendRequestResponse, error) {
	userId := CurrentUserId(ctx, s.users, s.sessions)
	self, err := s.users.LoadUser(userId)
	if err != nil {
		return &pb.SendFriendRequestResponse{}, nil
	}
	target := req.PlayerId
	if target == self.PlayerId {
		return &pb.SendFriendRequestResponse{}, nil
	}
	now := gametime.NowMillis()

	if s.dir.IsBot(target) {
		s.users.UpdateUser(userId, func(u *store.UserState) {
			u.Friends[target] = store.FriendEdge{PlayerId: target, BecameFriendsAt: now,
				CheerReceivedPending: true, LastResetDay: dayBucket()}
			delete(u.OutgoingFriendRequests, target)
		})
		return &pb.SendFriendRequestResponse{}, nil
	}

	if _, already := self.Friends[target]; already {
		return &pb.SendFriendRequestResponse{}, nil
	}
	s.users.UpdateUser(userId, func(u *store.UserState) {
		u.OutgoingFriendRequests[target] = store.FriendRequest{PlayerId: target, RequestedAt: now}
	})
	targetUserId, err := s.userIdForPlayer(target)
	if err == nil {
		s.users.UpdateUser(targetUserId, func(u *store.UserState) {
			u.IncomingFriendRequests[self.PlayerId] = store.FriendRequest{PlayerId: self.PlayerId, RequestedAt: now}
			u.Notifications.FriendRequestReceiveCount = int32(len(u.IncomingFriendRequests))
		})
	}
	return &pb.SendFriendRequestResponse{}, nil
}

func (s *FriendServiceServer) AcceptFriendRequest(ctx context.Context, req *pb.AcceptFriendRequestRequest) (*pb.AcceptFriendRequestResponse, error) {
	userId := CurrentUserId(ctx, s.users, s.sessions)
	self, err := s.users.LoadUser(userId)
	if err != nil {
		return &pb.AcceptFriendRequestResponse{}, nil
	}
	other := req.PlayerId
	if _, pending := self.IncomingFriendRequests[other]; !pending {
		return &pb.AcceptFriendRequestResponse{}, nil
	}
	now := gametime.NowMillis()
	s.users.UpdateUser(userId, func(u *store.UserState) {
		delete(u.IncomingFriendRequests, other)
		u.Friends[other] = store.FriendEdge{PlayerId: other, BecameFriendsAt: now, LastResetDay: dayBucket()}
		u.Notifications.FriendRequestReceiveCount = int32(len(u.IncomingFriendRequests))
	})
	if otherUserId, err := s.userIdForPlayer(other); err == nil {
		s.users.UpdateUser(otherUserId, func(u *store.UserState) {
			delete(u.OutgoingFriendRequests, self.PlayerId)
			u.Friends[self.PlayerId] = store.FriendEdge{PlayerId: self.PlayerId, BecameFriendsAt: now, LastResetDay: dayBucket()}
		})
	}
	return &pb.AcceptFriendRequestResponse{}, nil
}

func (s *FriendServiceServer) DeclineFriendRequest(ctx context.Context, req *pb.DeclineFriendRequestRequest) (*pb.DeclineFriendRequestResponse, error) {
	userId := CurrentUserId(ctx, s.users, s.sessions)
	s.users.UpdateUser(userId, func(u *store.UserState) {
		for _, pid := range req.PlayerId {
			delete(u.IncomingFriendRequests, pid)
		}
		u.Notifications.FriendRequestReceiveCount = int32(len(u.IncomingFriendRequests))
	})
	return &pb.DeclineFriendRequestResponse{}, nil
}

func (s *FriendServiceServer) DeleteFriend(ctx context.Context, req *pb.DeleteFriendRequest) (*pb.DeleteFriendResponse, error) {
	userId := CurrentUserId(ctx, s.users, s.sessions)
	self, err := s.users.LoadUser(userId)
	if err != nil {
		return &pb.DeleteFriendResponse{}, nil
	}
	other := req.PlayerId
	s.users.UpdateUser(userId, func(u *store.UserState) { delete(u.Friends, other) })
	if !s.dir.IsBot(other) {
		if otherUserId, err := s.userIdForPlayer(other); err == nil {
			s.users.UpdateUser(otherUserId, func(u *store.UserState) { delete(u.Friends, self.PlayerId) })
		}
	}
	return &pb.DeleteFriendResponse{}, nil
}

// userIdForPlayer maps a real playerId to its userId. The server assigns player_id = user_id
// at creation, so this is identity for real players (and validates the user exists).
func (s *FriendServiceServer) userIdForPlayer(playerId int64) (int64, error) {
	if _, err := s.users.LoadUser(playerId); err != nil {
		return 0, err
	}
	return playerId, nil
}

// cheerStaminaMillis is ~30 minutes of natural stamina recovery: 1,800,000 ms / 180 divisor = 10,000
// milli-units = 10 stamina units, a sane and visible reward comparable to a small stamina item.
const cheerStaminaMillis int32 = 1_000// 10_000

// maxStaminaMillisFor returns the stamina cap for the user based on their level,
// using the same ShopCatalog.MaxStaminaMillis lookup as consumableitem.go.
// Falls back to a reasonable default if master data is unavailable (e.g. in tests).
func (s *FriendServiceServer) maxStaminaMillisFor(u *store.UserState) int32 {
	if s.holder != nil {
		if cat := s.holder.Get(); cat != nil && cat.Shop != nil {
			if max, ok := cat.Shop.MaxStaminaMillis[u.Status.Level]; ok {
				return max
			}
		}
	}
	return 120_000 // fallback: 120 stamina units, a typical mid-game cap
}

func (s *FriendServiceServer) CheerFriend(ctx context.Context, req *pb.CheerFriendRequest) (*pb.CheerFriendResponse, error) {
	userId := CurrentUserId(ctx, s.users, s.sessions)
	self, err := s.users.LoadUser(userId)
	if err != nil {
		return &pb.CheerFriendResponse{}, nil
	}
	target := req.PlayerId
	cat := s.holder.Get()
	now := gametime.NowMillis()
	s.users.UpdateUser(userId, func(u *store.UserState) {
		maybeResetCheerDay(u)
		if e, ok := u.Friends[target]; ok {
			sentAlready := e.CheerSentToday
			e.CheerSentToday = true
			u.Friends[target] = e
			if !sentAlready {
				// Friend support mission (type 50): one increment per
				// friend cheered today, matching the daily cap on this action.
				ApplyMissionProgressEvent(u, cat.Mission, MissionProgressEvent{ConditionType: missionConditionFriendSupport, Delta: 1}, now)
			}
		}
	})
	if !s.dir.IsBot(target) {
		if tid, err := s.userIdForPlayer(target); err == nil {
			s.users.UpdateUser(tid, func(u *store.UserState) {
				maybeResetCheerDay(u)
				if e, ok := u.Friends[self.PlayerId]; ok {
					e.CheerReceivedPending = true
					u.Friends[self.PlayerId] = e
				}
			})
		}
	}
	return &pb.CheerFriendResponse{}, nil
}

func (s *FriendServiceServer) BulkCheerFriend(ctx context.Context, _ *emptypb.Empty) (*pb.BulkCheerFriendResponse, error) {
	userId := CurrentUserId(ctx, s.users, s.sessions)
	self, err := s.users.LoadUser(userId)
	if err != nil {
		return &pb.BulkCheerFriendResponse{}, nil
	}
	var cheered []int64
	cat := s.holder.Get()
	now := gametime.NowMillis()
	s.users.UpdateUser(userId, func(u *store.UserState) {
		maybeResetCheerDay(u)
		for pid, e := range u.Friends {
			if !e.CheerSentToday {
				e.CheerSentToday = true
				u.Friends[pid] = e
				cheered = append(cheered, pid)
				ApplyMissionProgressEvent(u, cat.Mission, MissionProgressEvent{ConditionType: missionConditionFriendSupport, Delta: 1}, now)
			}
		}
	})
	for _, pid := range cheered {
		if s.dir.IsBot(pid) {
			continue
		}
		if tid, err := s.userIdForPlayer(pid); err == nil {
			s.users.UpdateUser(tid, func(u *store.UserState) {
				maybeResetCheerDay(u)
				if e, ok := u.Friends[self.PlayerId]; ok {
					e.CheerReceivedPending = true
					u.Friends[self.PlayerId] = e
				}
			})
		}
	}
	return &pb.BulkCheerFriendResponse{PlayerId: cheered}, nil
}

func (s *FriendServiceServer) ReceiveCheer(ctx context.Context, req *pb.ReceiveCheerRequest) (*pb.ReceiveCheerResponse, error) {
	userId := CurrentUserId(ctx, s.users, s.sessions)
	s.users.UpdateUser(userId, func(u *store.UserState) {
		maybeResetCheerDay(u)
		grantCheerReward(u, req.PlayerId, s.maxStaminaMillisFor(u))
	})
	return &pb.ReceiveCheerResponse{}, nil
}

func (s *FriendServiceServer) BulkReceiveCheer(ctx context.Context, _ *emptypb.Empty) (*pb.BulkReceiveCheerResponse, error) {
	userId := CurrentUserId(ctx, s.users, s.sessions)
	var got []int64
	s.users.UpdateUser(userId, func(u *store.UserState) {
		maybeResetCheerDay(u)
		maxMillis := s.maxStaminaMillisFor(u)
		for pid, e := range u.Friends {
			if e.CheerReceivedPending && !e.StaminaReceivedToday {
				grantCheerReward(u, pid, maxMillis)
				got = append(got, pid)
			}
		}
	})
	return &pb.BulkReceiveCheerResponse{PlayerId: got}, nil
}

// grantCheerReward collects one friend's pending cheer: grants stamina and marks it collected.
// maxStaminaMillis is the caller's level-based cap (pass s.maxStaminaMillisFor(u) in RPCs).
func grantCheerReward(u *store.UserState, friendPlayerId int64, maxStaminaMillis int32) {
	e, ok := u.Friends[friendPlayerId]
	if !ok || !e.CheerReceivedPending || e.StaminaReceivedToday {
		return
	}
	store.RecoverStamina(u, cheerStaminaMillis, maxStaminaMillis, gametime.NowMillis())
	e.CheerReceivedPending = false
	e.StaminaReceivedToday = true
	u.Friends[friendPlayerId] = e
}
