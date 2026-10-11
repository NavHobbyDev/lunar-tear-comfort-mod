package service

import (
	"context"
	"fmt"
	"log"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/runtime"
	"lunar-tear/server/internal/store"
)

type DeckServiceServer struct {
	pb.UnimplementedDeckServiceServer
	users    store.UserRepository
	sessions store.SessionRepository
	snaps    store.SnapshotRepository
	holder   *runtime.Holder
}

func NewDeckServiceServer(users store.UserRepository, sessions store.SessionRepository, snaps store.SnapshotRepository, holder *runtime.Holder) *DeckServiceServer {
	return &DeckServiceServer{users: users, sessions: sessions, snaps: snaps, holder: holder}
}

func (s *DeckServiceServer) UpdateName(ctx context.Context, req *pb.UpdateNameRequest) (*pb.UpdateNameResponse, error) {
	log.Printf("[DeckService] UpdateName: deckType=%d deckNumber=%d name=%q", req.DeckType, req.UserDeckNumber, req.Name)
	userId := CurrentUserId(ctx, s.users, s.sessions)

	s.users.UpdateUser(userId, func(user *store.UserState) {
		deckKey := store.DeckKey{DeckType: model.DeckType(req.DeckType), UserDeckNumber: req.UserDeckNumber}
		deck := user.Decks[deckKey]
		deck.Name = req.Name
		user.Decks[deckKey] = deck
	})

	return &pb.UpdateNameResponse{}, nil
}

// deckPowerEntries flattens the three positional DeckCharacterPower fields
// into slot-ordered report entries (nil slots keep their position).
func deckPowerEntries(dp *pb.DeckPower) []store.DeckPowerReportEntry {
	if dp == nil {
		return nil
	}
	out := make([]store.DeckPowerReportEntry, 0, 3)
	for _, cp := range []*pb.DeckCharacterPower{dp.DeckCharacterPower01, dp.DeckCharacterPower02, dp.DeckCharacterPower03} {
		if cp == nil {
			out = append(out, store.DeckPowerReportEntry{})
			continue
		}
		out = append(out, store.DeckPowerReportEntry{UserDeckCharacterUuid: cp.UserDeckCharacterUuid, Power: cp.Power})
	}
	return out
}

func (s *DeckServiceServer) RefreshDeckPower(ctx context.Context, req *pb.RefreshDeckPowerRequest) (*pb.RefreshDeckPowerResponse, error) {
	log.Printf("[DeckService] RefreshDeckPower: deckType=%d deckNumber=%d power=%d", req.DeckType, req.UserDeckNumber, req.GetDeckPower().GetPower())
	userId := CurrentUserId(ctx, s.users, s.sessions)

	after, _ := s.users.UpdateUser(userId, func(user *store.UserState) {
		if req.DeckPower == nil {
			log.Printf("[DeckService] RefreshDeckPower: deckPower is nil")
			return
		}

		dt := model.DeckType(req.DeckType)
		deckKey := store.DeckKey{DeckType: dt, UserDeckNumber: req.UserDeckNumber}
		deck, ok := user.Decks[deckKey]
		if !ok {
			// The client reported power for a slot the server has no row for
			// (it can report before the replacement lands). Never attribute the
			// power to a different slot — create the missing skeleton instead.
			log.Printf("[DeckService] RefreshDeckPower: deck not found deckType=%d deckNumber=%d, creating skeleton", req.DeckType, req.UserDeckNumber)
			deck = store.DeckState{
				DeckType:       dt,
				UserDeckNumber: req.UserDeckNumber,
				Name:           fmt.Sprintf("Deck %d", req.UserDeckNumber),
				LatestVersion:  gametime.NowMillis(),
			}
		}

		deck.Power = req.DeckPower.Power
		user.Decks[deckKey] = deck

		store.ApplyDeckPowerReport(user, dt, req.UserDeckNumber, deckPowerEntries(req.DeckPower))

		// Reconcile the stored total with the per-character powers: the
		// reported total can be a stale echo of an earlier save, the character
		// powers are what the client actually computed.
		store.RecalcDeckPower(user, dt, req.UserDeckNumber)

		note := user.DeckTypeNotes[dt]
		if req.DeckPower.Power > note.MaxDeckPower {
			note.DeckType = dt
			note.MaxDeckPower = req.DeckPower.Power
			user.DeckTypeNotes[dt] = note
		}

		// Calculate total force (only quest deck, DeckTypeQuest = 1)
		totalForce := int32(0)
		if dt == model.DeckTypeQuest {
			totalForce = user.DeckTypeNotes[dt].MaxDeckPower
		}

		// Update mission progress for total force missions (400001-400120)
		if s.holder != nil {
			cat := s.holder.Get()
			nowMillis := gametime.NowMillis()
			ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{
				ConditionType: missionConditionTotalForce,
				CurrentValue:  totalForce,
			}, nowMillis)
		}

		// A fresh power report is the only deck the server may trust right
		// now: promote exactly this deck into the arena when it beats the
		// current PvP slot #1 (other quest decks may be stale).
		if dt == model.DeckTypeQuest {
			maybePromoteQuestDeckToPvp(user, req.UserDeckNumber, gametime.NowMillis())
		}
	})

	// Any deck power report can change what opponents see: the defense
	// deck's strength derives from the quest deck it mirrors, so refresh the
	// snapshot right away instead of waiting for the next arena action.
	if s.snaps != nil {
		if err := RefreshSnapshot(s.snaps, &after); err != nil {
			log.Printf("[DeckService] RefreshDeckPower snapshot refresh failed: %v", err)
		}
	}

	return &pb.RefreshDeckPowerResponse{}, nil
}

func (s *DeckServiceServer) RefreshMultiDeckPower(ctx context.Context, req *pb.RefreshMultiDeckPowerRequest) (*pb.RefreshMultiDeckPowerResponse, error) {
	log.Printf("[DeckService] RefreshMultiDeckPower: %d entries", len(req.DeckPowerInfo))
	userId := CurrentUserId(ctx, s.users, s.sessions)

	after, _ := s.users.UpdateUser(userId, func(user *store.UserState) {
		for _, info := range req.DeckPowerInfo {
			if info.DeckPower == nil {
				continue
			}

			dt := model.DeckType(info.DeckType)
			deckKey := store.DeckKey{DeckType: dt, UserDeckNumber: info.UserDeckNumber}
			deck, ok := user.Decks[deckKey]
			if !ok {
				log.Printf("[DeckService] RefreshMultiDeckPower: deck not found deckType=%d deckNumber=%d", info.DeckType, info.UserDeckNumber)
				continue
			}

			deck.Power = info.DeckPower.Power
			user.Decks[deckKey] = deck

			store.ApplyDeckPowerReport(user, dt, info.UserDeckNumber, deckPowerEntries(info.DeckPower))

			// Same reconciliation as RefreshDeckPower: derive the stored total
			// from the freshly reported character powers.
			store.RecalcDeckPower(user, dt, info.UserDeckNumber)

			note := user.DeckTypeNotes[dt]
			if info.DeckPower.Power > note.MaxDeckPower {
				note.DeckType = dt
				note.MaxDeckPower = info.DeckPower.Power
				user.DeckTypeNotes[dt] = note
			}
		}

		// Calculate total force (only quest deck, DeckTypeQuest = 1)
		totalForce := int32(0)
		for _, info := range req.DeckPowerInfo {
			dt := model.DeckType(info.DeckType)
			if dt == model.DeckTypeQuest {
				totalForce += user.DeckTypeNotes[dt].MaxDeckPower
			}
		}

		// Update mission progress for total force missions (400001-400120)
		if s.holder != nil {
			cat := s.holder.Get()
			nowMillis := gametime.NowMillis()
			ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{
				ConditionType: missionConditionTotalForce,
				CurrentValue:  totalForce,
			}, nowMillis)
		}

		// Same as RefreshDeckPower: only the decks that just reported are
		// trusted, and each may promote itself into the arena when stronger
		// than the current PvP slot #1.
		for _, info := range req.DeckPowerInfo {
			if model.DeckType(info.DeckType) == model.DeckTypeQuest {
				maybePromoteQuestDeckToPvp(user, info.UserDeckNumber, gametime.NowMillis())
			}
		}
	})

	// Same as RefreshDeckPower: the defense deck's strength can change with
	// any report, so push the fresh state into the snapshot immediately.
	if s.snaps != nil {
		if err := RefreshSnapshot(s.snaps, &after); err != nil {
			log.Printf("[DeckService] RefreshMultiDeckPower snapshot refresh failed: %v", err)
		}
	}

	return &pb.RefreshMultiDeckPowerResponse{}, nil
}

func deckSlotsFromProto(deck *pb.Deck) []store.DeckCharacterInput {
	slots := make([]store.DeckCharacterInput, 3)
	for i, ch := range []*pb.DeckCharacter{deck.Character01, deck.Character02, deck.Character03} {
		if ch == nil {
			continue
		}
		slots[i] = store.DeckCharacterInput{
			UserCostumeUuid:    ch.UserCostumeUuid,
			MainUserWeaponUuid: ch.MainUserWeaponUuid,
			SubWeaponUuids:     ch.SubUserWeaponUuid,
			PartsUuids:         ch.UserPartsUuid,
			UserCompanionUuid:  ch.UserCompanionUuid,
			UserThoughtUuid:    ch.UserThoughtUuid,
			DressupCostumeId:   ch.DressupCostumeId,
		}
	}
	return slots
}

func (s *DeckServiceServer) ReplaceDeck(ctx context.Context, req *pb.ReplaceDeckRequest) (*pb.ReplaceDeckResponse, error) {
	log.Printf("[DeckService] ReplaceDeck: deckType=%d deckNumber=%d", req.DeckType, req.UserDeckNumber)
	if req.Deck != nil {
		for i, ch := range []*pb.DeckCharacter{req.Deck.Character01, req.Deck.Character02, req.Deck.Character03} {
			if ch == nil {
				continue
			}
			log.Printf("[DeckService] ReplaceDeck slot %d: costume=%s mainWeapon=%s subWeapons=%v companion=%s thought=%s",
				i+1, ch.UserCostumeUuid, ch.MainUserWeaponUuid, ch.SubUserWeaponUuid, ch.UserCompanionUuid, ch.UserThoughtUuid)
		}
	}
	userId := CurrentUserId(ctx, s.users, s.sessions)

	after, _ := s.users.UpdateUser(userId, func(user *store.UserState) {
		if req.Deck == nil {
			return
		}
		dt := model.DeckType(req.DeckType)
		store.ApplyDeckReplacement(user, dt, req.UserDeckNumber, deckSlotsFromProto(req.Deck), gametime.NowMillis())

		// Calculate total force (only quest deck, DeckTypeQuest = 1)
		totalForce := int32(0)
		if dt == model.DeckTypeQuest {
			totalForce = user.DeckTypeNotes[dt].MaxDeckPower
		}

		// Update mission progress for total force missions (400001-400120)
		if s.holder != nil {
			cat := s.holder.Get()
			nowMillis := gametime.NowMillis()
			ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{
				ConditionType: missionConditionTotalForce,
				CurrentValue:  totalForce,
			}, nowMillis)
		}
	})
	if s.snaps != nil && model.DeckType(req.DeckType) == model.DeckTypePvp {
		if err := RefreshSnapshot(s.snaps, &after); err != nil {
			log.Printf("[DeckService] ReplaceDeck snapshot refresh failed: %v", err)
		}
	}

	return &pb.ReplaceDeckResponse{}, nil
}

func (s *DeckServiceServer) ReplaceTripleDeck(ctx context.Context, req *pb.ReplaceTripleDeckRequest) (*pb.ReplaceTripleDeckResponse, error) {
	log.Printf("[DeckService] ReplaceTripleDeck: deckType=%d deckNumber=%d", req.DeckType, req.UserDeckNumber)
	userId := CurrentUserId(ctx, s.users, s.sessions)

	after, _ := s.users.UpdateUser(userId, func(user *store.UserState) {
		nowMillis := gametime.NowMillis()
		for idx, detail := range []*pb.DeckDetail{req.DeckDetail01, req.DeckDetail02, req.DeckDetail03} {
			if detail == nil || detail.Deck == nil {
				continue
			}
			log.Printf("[DeckService] ReplaceTripleDeck detail %d: deckType=%d deckNumber=%d", idx+1, detail.DeckType, detail.UserDeckNumber)
			if detail.Deck != nil {
				for i, ch := range []*pb.DeckCharacter{detail.Deck.Character01, detail.Deck.Character02, detail.Deck.Character03} {
					if ch == nil {
						continue
					}
					log.Printf("[DeckService] ReplaceTripleDeck detail %d slot %d: costume=%s mainWeapon=%s subWeapons=%v companion=%s thought=%s",
						idx+1, i+1, ch.UserCostumeUuid, ch.MainUserWeaponUuid, ch.SubUserWeaponUuid, ch.UserCompanionUuid, ch.UserThoughtUuid)
				}
			}
			store.ApplyDeckReplacement(user, model.DeckType(detail.DeckType), detail.UserDeckNumber, deckSlotsFromProto(detail.Deck), nowMillis)
		}

		key := store.DeckKey{DeckType: model.DeckType(req.DeckType), UserDeckNumber: req.UserDeckNumber}
		td := user.TripleDecks[key]
		td.DeckType = model.DeckType(req.DeckType)
		td.UserDeckNumber = req.UserDeckNumber
		td.DeckNumber01 = innerDeckNumber(req.DeckDetail01)
		td.DeckNumber02 = innerDeckNumber(req.DeckDetail02)
		td.DeckNumber03 = innerDeckNumber(req.DeckDetail03)
		td.LatestVersion = nowMillis
		user.TripleDecks[key] = td

		// Calculate total force (only quest deck, DeckTypeQuest = 1)
		totalForce := int32(0)
		for _, detail := range []*pb.DeckDetail{req.DeckDetail01, req.DeckDetail02, req.DeckDetail03} {
			if detail == nil {
				continue
			}
			dt := model.DeckType(detail.DeckType)
			if dt == model.DeckTypeQuest {
				totalForce += user.DeckTypeNotes[dt].MaxDeckPower
			}
		}

		// Update mission progress for total force missions (400001-400120)
		if s.holder != nil {
			cat := s.holder.Get()
			ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{
				ConditionType: missionConditionTotalForce,
				CurrentValue:  totalForce,
			}, nowMillis)
		}
	})
	if s.snaps != nil {
		for _, detail := range []*pb.DeckDetail{req.DeckDetail01, req.DeckDetail02, req.DeckDetail03} {
			if detail != nil && model.DeckType(detail.DeckType) == model.DeckTypePvp {
				if err := RefreshSnapshot(s.snaps, &after); err != nil {
					log.Printf("[DeckService] ReplaceTripleDeck snapshot refresh failed: %v", err)
				}
				break
			}
		}
	}

	return &pb.ReplaceTripleDeckResponse{}, nil
}

func innerDeckNumber(d *pb.DeckDetail) int32 {
	if d == nil {
		return 0
	}
	return d.UserDeckNumber
}

func (s *DeckServiceServer) UpdateTripleDeckName(ctx context.Context, req *pb.UpdateTripleDeckNameRequest) (*pb.UpdateTripleDeckNameResponse, error) {
	log.Printf("[DeckService] UpdateTripleDeckName: deckType=%d deckNumber=%d name=%q", req.DeckType, req.UserDeckNumber, req.Name)
	userId := CurrentUserId(ctx, s.users, s.sessions)

	s.users.UpdateUser(userId, func(user *store.UserState) {
		key := store.DeckKey{DeckType: model.DeckType(req.DeckType), UserDeckNumber: req.UserDeckNumber}
		td := user.TripleDecks[key]
		td.DeckType = model.DeckType(req.DeckType)
		td.UserDeckNumber = req.UserDeckNumber
		td.Name = req.Name
		td.LatestVersion = gametime.NowMillis()
		user.TripleDecks[key] = td
	})

	return &pb.UpdateTripleDeckNameResponse{}, nil
}

func (s *DeckServiceServer) ReplaceMultiDeck(ctx context.Context, req *pb.ReplaceMultiDeckRequest) (*pb.ReplaceMultiDeckResponse, error) {
	log.Printf("[DeckService] ReplaceMultiDeck: %d entries", len(req.DeckDetail))
	userId := CurrentUserId(ctx, s.users, s.sessions)

	after, _ := s.users.UpdateUser(userId, func(user *store.UserState) {
		nowMillis := gametime.NowMillis()
		for idx, detail := range req.DeckDetail {
			if detail == nil || detail.Deck == nil {
				continue
			}
			log.Printf("[DeckService] ReplaceMultiDeck detail %d: deckType=%d deckNumber=%d", idx+1, detail.DeckType, detail.UserDeckNumber)
			store.ApplyDeckReplacement(user, model.DeckType(detail.DeckType), detail.UserDeckNumber, deckSlotsFromProto(detail.Deck), nowMillis)
		}

		// Calculate total force (only quest deck, DeckTypeQuest = 1)
		totalForce := int32(0)
		for _, detail := range req.DeckDetail {
			if detail == nil {
				continue
			}
			dt := model.DeckType(detail.DeckType)
			if dt == model.DeckTypeQuest {
				totalForce += user.DeckTypeNotes[dt].MaxDeckPower
			}
		}

		// Update mission progress for total force missions (400001-400120)
		if s.holder != nil {
			cat := s.holder.Get()
			ApplyMissionProgressEvent(user, cat.Mission, MissionProgressEvent{
				ConditionType: missionConditionTotalForce,
				CurrentValue:  totalForce,
			}, nowMillis)
		}
	})
	if s.snaps != nil {
		for _, detail := range req.DeckDetail {
			if detail != nil && model.DeckType(detail.DeckType) == model.DeckTypePvp {
				if err := RefreshSnapshot(s.snaps, &after); err != nil {
					log.Printf("[DeckService] ReplaceMultiDeck snapshot refresh failed: %v", err)
				}
				break
			}
		}
	}

	return &pb.ReplaceMultiDeckResponse{}, nil
}

// SetPvpDefenseDeck records which PvP deck slot the player exposes to attackers.
// Arena decks are a separate set (DeckTypePvp) from quest and BigHunt decks, so
// this only touches the player's PvP slots. The chosen slot's snapshot is
// refreshed immediately so opponents see the new defense team.
func (s *DeckServiceServer) SetPvpDefenseDeck(ctx context.Context, req *pb.SetPvpDefenseDeckRequest) (*pb.SetPvpDefenseDeckResponse, error) {
	reportedPower := int32(-1)
	if req.DeckPower != nil {
		reportedPower = req.DeckPower.Power
	}
	log.Printf("[DeckService] SetPvpDefenseDeck: deckNumber=%d reportedPower=%d", req.UserDeckNumber, reportedPower)
	userId := CurrentUserId(ctx, s.users, s.sessions)

	after, _ := s.users.UpdateUser(userId, func(user *store.UserState) {
		user.Pvp.DefenseDeckNumber = req.UserDeckNumber

		// Persist the reported deck power for the chosen PvP slot so ranking /
		// matching surfaces the right strength, mirroring RefreshDeckPower.
		if req.DeckPower != nil {
			deckKey := store.DeckKey{DeckType: model.DeckTypePvp, UserDeckNumber: req.UserDeckNumber}
			if deck, ok := user.Decks[deckKey]; ok {
				deck.Power = req.DeckPower.Power
				user.Decks[deckKey] = deck
			}
			store.ApplyDeckPowerReport(user, model.DeckTypePvp, req.UserDeckNumber, deckPowerEntries(req.DeckPower))
			// Reconcile the stored total with the character powers (the
			// reported total can be a stale echo of an earlier save).
			store.RecalcDeckPower(user, model.DeckTypePvp, req.UserDeckNumber)
			note := user.DeckTypeNotes[model.DeckTypePvp]
			if req.DeckPower.Power > note.MaxDeckPower {
				note.DeckType = model.DeckTypePvp
				note.MaxDeckPower = req.DeckPower.Power
				user.DeckTypeNotes[model.DeckTypePvp] = note
			}
		}
	})

	if s.snaps != nil {
		if err := RefreshSnapshot(s.snaps, &after); err != nil {
			log.Printf("[DeckService] SetPvpDefenseDeck snapshot refresh failed: %v", err)
		}
	}

	return &pb.SetPvpDefenseDeckResponse{DiffUserData: map[string]*pb.DiffData{}}, nil
}

func (s *DeckServiceServer) RemoveDeck(ctx context.Context, req *pb.RemoveDeckRequest) (*pb.RemoveDeckResponse, error) {
	log.Printf("[DeckService] RemoveDeck: deckType=%d deckNumber=%d", req.DeckType, req.UserDeckNumber)
	userId := CurrentUserId(ctx, s.users, s.sessions)

	after, _ := s.users.UpdateUser(userId, func(user *store.UserState) {
		store.RemoveDeckData(user, model.DeckType(req.DeckType), req.UserDeckNumber)
		// Deleting the slot currently exposed on defense leaves the selection
		// dangling (the client would keep highlighting a deck that no longer
		// exists); clear it so PickDefenseDeck's fallback is authoritative.
		if model.DeckType(req.DeckType) == model.DeckTypePvp && user.Pvp.DefenseDeckNumber == req.UserDeckNumber {
			user.Pvp.DefenseDeckNumber = 0
		}
	})
	if s.snaps != nil && model.DeckType(req.DeckType) == model.DeckTypePvp {
		if err := RefreshSnapshot(s.snaps, &after); err != nil {
			log.Printf("[DeckService] RemoveDeck snapshot refresh failed: %v", err)
		}
	}

	return &pb.RemoveDeckResponse{}, nil
}

func (s *DeckServiceServer) CopyDeck(ctx context.Context, req *pb.CopyDeckRequest) (*pb.CopyDeckResponse, error) {
	log.Printf("[DeckService] CopyDeck: from deckType=%d deckNumber=%d -> to deckType=%d deckNumber=%d",
		req.FromDeckType, req.FromUserDeckNumber, req.ToDeckType, req.ToUserDeckNumber)
	userId := CurrentUserId(ctx, s.users, s.sessions)

	var resultType int32
	after, _ := s.users.UpdateUser(userId, func(user *store.UserState) {
		slots := store.ReadDeckSlots(user, model.DeckType(req.FromDeckType), req.FromUserDeckNumber)
		if slots == nil {
			return
		}

		nowMillis := gametime.NowMillis()
		fromKey := store.DeckKey{DeckType: model.DeckType(req.FromDeckType), UserDeckNumber: req.FromUserDeckNumber}
		srcName := user.Decks[fromKey].Name

		store.ApplyDeckReplacement(user, model.DeckType(req.ToDeckType), req.ToUserDeckNumber, slots, nowMillis)

		toKey := store.DeckKey{DeckType: model.DeckType(req.ToDeckType), UserDeckNumber: req.ToUserDeckNumber}
		deck := user.Decks[toKey]
		deck.Name = srcName
		user.Decks[toKey] = deck

		resultType = 1
	})
	if s.snaps != nil && model.DeckType(req.ToDeckType) == model.DeckTypePvp {
		if err := RefreshSnapshot(s.snaps, &after); err != nil {
			log.Printf("[DeckService] CopyDeck snapshot refresh failed: %v", err)
		}
	}

	return &pb.CopyDeckResponse{ResultType: resultType}, nil
}
