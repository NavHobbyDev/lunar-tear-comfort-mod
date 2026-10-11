package service

import (
	"context"
	"fmt"
	"log"
	"slices"
	"sort"
	"time"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/runtime"
	"lunar-tear/server/internal/store"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type GiftServiceServer struct {
	pb.UnimplementedGiftServiceServer
	users    store.UserRepository
	sessions store.SessionRepository
	holder   *runtime.Holder
}

func NewGiftServiceServer(users store.UserRepository, sessions store.SessionRepository, holder *runtime.Holder) *GiftServiceServer {
	return &GiftServiceServer{users: users, sessions: sessions, holder: holder}
}

func (s *GiftServiceServer) ReceiveGift(ctx context.Context, req *pb.ReceiveGiftRequest) (*pb.ReceiveGiftResponse, error) {
	log.Printf("[GiftService] ReceiveGift: giftUuids=%d", len(req.UserGiftUuid))

	userId := CurrentUserId(ctx, s.users, s.sessions)
	received := make([]string, 0, len(req.UserGiftUuid))
	overflow := make([]string, 0)

	cat := s.holder.Get()
	granter := cat.QuestHandler.Granter

	_, err := s.users.UpdateUser(userId, func(user *store.UserState) {
		nowMillis := gametime.NowMillis()
		remaining := make([]store.NotReceivedGiftState, 0, len(user.Gifts.NotReceived))

		for _, gift := range user.Gifts.NotReceived {
			requested := slices.Contains(req.UserGiftUuid, gift.UserGiftUuid)
			if requested && claimGift(user, granter, gift, nowMillis) {
				received = append(received, gift.UserGiftUuid)
				continue
			}
			if requested {
				// Requested but not claimable (weapon inventory full): report
				// it as overflow so it stays visible in the mailbox.
				overflow = append(overflow, gift.UserGiftUuid)
			}
			remaining = append(remaining, gift)
		}

		user.Gifts.NotReceived = remaining
		user.Notifications.GiftNotReceiveCount = int32(len(user.Gifts.NotReceived))
	})

	if err != nil {
		return &pb.ReceiveGiftResponse{
			ReceivedGiftUuid: []string{},
			ExpiredGiftUuid:  []string{},
			OverflowGiftUuid: []string{},
		}, nil
	}

	return &pb.ReceiveGiftResponse{
		ReceivedGiftUuid: received,
		ExpiredGiftUuid:  []string{},
		OverflowGiftUuid: overflow,
	}, nil
}

// claimGift grants one requested gift to the user and records it as
// received, returning false when the gift must stay in the mailbox. Weapon
// and memoir gifts occupy capped inventory slots, so at the cap they are
// kept in the mailbox instead of granted: "receive all" must not smuggle
// them past the cap just because other items in the batch are receivable.
func claimGift(user *store.UserState, granter *store.PossessionGranter, gift store.NotReceivedGiftState, nowMillis int64) bool {
	g := gift.GiftCommon
	if isWeaponPossession(g.PossessionType) && len(user.Weapons) >= int(model.WeaponInventoryCap) {
		log.Printf("[GiftService] ReceiveGift: weapon gift %s (id=%d) kept in mailbox, weapon inventory full (%d/%d)",
			gift.UserGiftUuid, g.PossessionId, len(user.Weapons), model.WeaponInventoryCap)
		return false
	}
	if isPartsPossession(g.PossessionType) && len(user.Parts) >= int(model.PartsInventoryCap) {
		log.Printf("[GiftService] ReceiveGift: parts gift %s (id=%d) kept in mailbox, parts inventory full (%d/%d)",
			gift.UserGiftUuid, g.PossessionId, len(user.Parts), model.PartsInventoryCap)
		return false
	}

	if isPartsPossession(g.PossessionType) {
		// Mailed overflow memoirs keep their exact rolled variant (rarity
		// and rank): grant without re-rolling a sibling.
		granter.GrantPartsExact(user, g.PossessionId, nowMillis)
	} else {
		granter.GrantFull(user, model.PossessionType(g.PossessionType), g.PossessionId, g.Count, nowMillis)
	}
	user.Gifts.Received = append(user.Gifts.Received, store.ReceivedGiftState{
		GiftCommon:       gift.GiftCommon,
		ReceivedDatetime: nowMillis,
	})
	return true
}

// isWeaponPossession reports whether the possession type grants a weapon
// inventory slot (both plain and enhanced weapon gifts do).
func isWeaponPossession(possessionType int32) bool {
	return possessionType == int32(model.PossessionTypeWeapon) ||
		possessionType == int32(model.PossessionTypeWeaponEnhanced)
}

// isPartsPossession reports whether the possession type grants a memoir
// (parts) inventory slot.
func isPartsPossession(possessionType int32) bool {
	return possessionType == int32(model.PossessionTypeParts) ||
		possessionType == int32(model.PossessionTypePartsEnhanced)
}

// Client gift filter enums. RewardKindType is the checkbox list of reward
// categories; NONE means "no filtering". ExpirationType selects mails by
// expiry state.
const (
	giftRewardKindNone      int32 = 1
	giftRewardKindGem       int32 = 2
	giftRewardKindGold      int32 = 3
	giftRewardKindWeapon    int32 = 4
	giftRewardKindCompanion int32 = 5
	giftRewardKindParts     int32 = 6
	giftRewardKindMaterial  int32 = 7
	giftRewardKindOther     int32 = 8
	giftRewardKindCostume   int32 = 9
)

const (
	giftExpirationNone        int32 = 1
	giftExpirationOnlyExpired int32 = 2
	giftExpirationOnlyValid   int32 = 3
)

// giftKindPossessionTypes maps a filter checkbox to the possession types
// it covers. Gold is one specific consumable id (resolved at call time)
// and OTHER is the catch-all for whatever no checkbox claims.
var giftKindPossessionTypes = map[int32][]model.PossessionType{
	giftRewardKindGem:       {model.PossessionTypePaidGem, model.PossessionTypeFreeGem},
	giftRewardKindWeapon:    {model.PossessionTypeWeapon, model.PossessionTypeWeaponEnhanced},
	giftRewardKindCompanion: {model.PossessionTypeCompanion, model.PossessionTypeCompanionEnhanced},
	giftRewardKindParts:     {model.PossessionTypeParts, model.PossessionTypePartsEnhanced},
	giftRewardKindMaterial:  {model.PossessionTypeMaterial},
	giftRewardKindCostume:   {model.PossessionTypeCostume, model.PossessionTypeCostumeEnhanced},
}

// giftMatchesKinds reports whether a mail passes the checked reward-kind
// checkboxes. An empty set (or the NONE checkbox) matches everything.
// Consumables split into GOLD (the gold item) and OTHER (tickets etc.).
func giftMatchesKinds(g store.GiftCommonState, kinds map[int32]bool, goldItemId int32) bool {
	if len(kinds) == 0 {
		return true
	}
	if g.PossessionType == int32(model.PossessionTypeConsumableItem) {
		if goldItemId != 0 && g.PossessionId == goldItemId {
			return kinds[giftRewardKindGold]
		}
		return kinds[giftRewardKindOther]
	}
	for kind, types := range giftKindPossessionTypes {
		if !kinds[kind] {
			continue
		}
		for _, pt := range types {
			if g.PossessionType == int32(pt) {
				return true
			}
		}
	}
	return kinds[giftRewardKindOther]
}

// giftMatchesExpiration reports whether a mail passes the expiry filter.
// Mails without an expiration never count as expired.
func giftMatchesExpiration(expirationDatetime int64, expirationType int32, nowMillis int64) bool {
	switch expirationType {
	case giftExpirationOnlyExpired:
		return expirationDatetime > 0 && expirationDatetime < nowMillis
	case giftExpirationOnlyValid:
		return expirationDatetime == 0 || expirationDatetime >= nowMillis
	default:
		return true
	}
}

func (s *GiftServiceServer) GetGiftList(ctx context.Context, req *pb.GetGiftListRequest) (*pb.GetGiftListResponse, error) {
	log.Printf("[GiftService] GetGiftList: rewardKinds=%v expirationType=%d ascending=%v nextCursor=%d previousCursor=%d getCount=%d",
		req.RewardKindType, req.ExpirationType, req.IsAscendingSort, req.NextCursor, req.PreviousCursor, req.GetCount)

	userId := CurrentUserId(ctx, s.users, s.sessions)
	user, err := s.users.LoadUser(userId)
	if err != nil {
		return nil, fmt.Errorf("snapshot user: %w", err)
	}

	kinds := make(map[int32]bool, len(req.RewardKindType))
	for _, kind := range req.RewardKindType {
		if kind != giftRewardKindNone {
			kinds[kind] = true
		}
	}
	goldItemId := s.holder.Get().QuestHandler.Granter.GoldConsumableItemId
	nowMillis := gametime.NowMillis()

	gifts := make([]store.NotReceivedGiftState, 0, len(user.Gifts.NotReceived))
	for _, gift := range user.Gifts.NotReceived {
		if !giftMatchesKinds(gift.GiftCommon, kinds, goldItemId) {
			continue
		}
		if !giftMatchesExpiration(gift.ExpirationDatetime, req.ExpirationType, nowMillis) {
			continue
		}
		gifts = append(gifts, gift)
	}
	// Stable sort keeps insertion order for equal expirations, so a gift
	// cannot jump between pages across requests.
	sort.SliceStable(gifts, func(i, j int) bool {
		if req.IsAscendingSort {
			return gifts[i].ExpirationDatetime < gifts[j].ExpirationDatetime
		}
		return gifts[i].ExpirationDatetime > gifts[j].ExpirationDatetime
	})

	// The client echoes the cursors from the previous response: nextCursor
	// when paging forward, previousCursor when paging back, both 0 for the
	// first page. Cursors are item offsets into the sorted list.
	start, nextCursor, previousCursor := giftPageBounds(len(gifts), int(req.GetCount), req.NextCursor, req.PreviousCursor)
	page := gifts[start:]
	if req.GetCount > 0 && len(page) > int(req.GetCount) {
		page = page[:req.GetCount]
	}

	items := make([]*pb.NotReceivedGift, 0, len(page))
	for _, gift := range page {
		items = append(items, &pb.NotReceivedGift{
			GiftCommon:         toProtoGiftCommon(gift.GiftCommon),
			ExpirationDatetime: timestampOrNilGift(gift.ExpirationDatetime),
			UserGiftUuid:       gift.UserGiftUuid,
		})
	}

	return &pb.GetGiftListResponse{
		Gift:           items,
		TotalPageCount: pageCount(len(gifts), int(req.GetCount)),
		NextCursor:     nextCursor,
		PreviousCursor: previousCursor,
	}, nil
}

// giftPageBounds resolves the paging request into the offset of the first
// item to return and the cursors for the neighbouring pages. A cursor of 0
// in the response means there is no page in that direction (the client
// greys out the arrow). A stale cursor past the end of the list — mails
// claimed or evicted since the last page — falls back to the first page.
func giftPageBounds(total, pageSize int, nextCursor, previousCursor int64) (start, next, previous int64) {
	start = 0
	if previousCursor > 0 {
		start = previousCursor
	} else if nextCursor > 0 {
		start = nextCursor
	}
	if start < 0 || (pageSize > 0 && int(start) >= total) {
		start = 0
	}
	next = 0
	if pageSize > 0 && int(start)+pageSize < total {
		next = start + int64(pageSize)
	}
	previous = 0
	if start > 0 {
		previous = start - int64(pageSize)
		if previous < 0 {
			previous = 0
		}
	}
	return start, next, previous
}

func (s *GiftServiceServer) GetGiftReceiveHistoryList(ctx context.Context, req *emptypb.Empty) (*pb.GetGiftReceiveHistoryListResponse, error) {
	log.Printf("[GiftService] GetGiftReceiveHistoryList")
	userId := CurrentUserId(ctx, s.users, s.sessions)
	user, err := s.users.LoadUser(userId)
	if err != nil {
		return nil, fmt.Errorf("snapshot user: %w", err)
	}

	items := make([]*pb.ReceivedGift, 0, len(user.Gifts.Received))
	// Newest first; the stable sort keeps the claim order inside one
	// receive batch (all items share the same timestamp).
	history := append([]store.ReceivedGiftState(nil), user.Gifts.Received...)
	sort.SliceStable(history, func(i, j int) bool {
		return history[i].ReceivedDatetime > history[j].ReceivedDatetime
	})
	for _, gift := range history {
		items = append(items, &pb.ReceivedGift{
			GiftCommon:       toProtoGiftCommon(gift.GiftCommon),
			ReceivedDatetime: timestampOrNilGift(gift.ReceivedDatetime),
		})
	}
	return &pb.GetGiftReceiveHistoryListResponse{
		Gift: items,
	}, nil
}

func toProtoGiftCommon(gift store.GiftCommonState) *pb.GiftCommon {
	return &pb.GiftCommon{
		PossessionType:        gift.PossessionType,
		PossessionId:          gift.PossessionId,
		Count:                 gift.Count,
		GrantDatetime:         timestampOrNilGift(gift.GrantDatetime),
		DescriptionGiftTextId: gift.DescriptionGiftTextId,
		EquipmentData:         gift.EquipmentData,
	}
}

func timestampOrNilGift(unixMillis int64) *timestamppb.Timestamp {
	if unixMillis == 0 {
		return nil
	}
	return timestamppb.New(time.UnixMilli(unixMillis))
}

func pageCount(total, pageSize int) int32 {
	if total == 0 {
		return 0
	}
	if pageSize <= 0 {
		return 1
	}
	pages := total / pageSize
	if total%pageSize != 0 {
		pages++
	}
	return int32(pages)
}
