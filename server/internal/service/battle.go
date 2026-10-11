package service

import (
	"context"
	"log"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/runtime"
	"lunar-tear/server/internal/store"
)

type BattleServiceServer struct {
	pb.UnimplementedBattleServiceServer
	users    store.UserRepository
	sessions store.SessionRepository
	holder   *runtime.Holder
	viewer   *ViewerSessions
}

func NewBattleServiceServer(users store.UserRepository, sessions store.SessionRepository, holder *runtime.Holder, viewer *ViewerSessions) *BattleServiceServer {
	if holder == nil {
		panic("runtime holder is required")
	}
	return &BattleServiceServer{users: users, sessions: sessions, holder: holder, viewer: viewer}
}

func (s *BattleServiceServer) StartWave(ctx context.Context, req *pb.StartWaveRequest) (*pb.StartWaveResponse, error) {
	userId := CurrentUserId(ctx, s.users, s.sessions)

	var suppress bool
	var hasActiveMainQuest bool
	var hasActiveEventQuest bool
	s.users.UpdateUser(userId, func(user *store.UserState) {
		suppress = user.MainQuest.SuppressReplayFlow

		// Check for active main quest (tracked by LastStartedQuestId or autoOrbit)
		if user.Battle.LastStartedQuestId != 0 {
			hasActiveMainQuest = true
		}
		if user.QuestAutoOrbit.MaxAutoOrbitCount > 0 {
			hasActiveMainQuest = true
		}
		for _, q := range user.Quests {
			if q.QuestStateType == model.UserQuestStateTypeActive {
				hasActiveMainQuest = true
				break
			}
		}

		// Check for active event quest (separate from main quest flow)
		if user.EventQuest.CurrentQuestId != 0 {
			hasActiveEventQuest = true
		}
	})

	// Suppress flag blocks orphan waves (no active main or event quest)
	// But allow event quest waves to proceed (they use separate quest state)
	if suppress && !hasActiveEventQuest {
		log.Printf("[StartWave] REJECT: suppressReplayFlow + no event quest active = orphan wave blocked (userId=%d)", userId)
		return nil, status.Error(codes.FailedPrecondition, "quest flow suppressed")
	}

	if !hasActiveMainQuest && !hasActiveEventQuest {
		// The Character Viewer (3D team preview) enters a location scene by firing
		// StartWave with no quest attempt. That is a legitimate preview, not an
		// orphan wave, so allow it while the viewer session is fresh. Every other
		// quest-less StartWave stays blocked (the orphan-wave guard).
		if s.viewer.InViewer(userId) {
			log.Printf("[StartWave] ALLOW: character viewer preview wave with no active quest (userId=%d)", userId)
			return &pb.StartWaveResponse{}, nil
		}
		log.Printf("[StartWave] REJECT: no active quest attempt (userId=%d)", userId)
		return nil, status.Error(codes.FailedPrecondition, "no active quest attempt")
	}

	return &pb.StartWaveResponse{}, nil
}

func (s *BattleServiceServer) FinishWave(ctx context.Context, req *pb.FinishWaveRequest) (*pb.FinishWaveResponse, error) {
	userId := CurrentUserId(ctx, s.users, s.sessions)
	s.users.UpdateUser(userId, func(user *store.UserState) {
		user.Battle.IsActive = false
		user.Battle.FinishCount++
		user.Battle.LastFinishedAt = gametime.NowMillis()
		user.Battle.LastUserPartyCount = int32(len(req.UserPartyResultInfoList))
		user.Battle.LastNpcPartyCount = int32(len(req.NpcPartyResultInfoList))
		user.Battle.LastBattleBinarySize = int32(len(req.BattleBinary))
		user.Battle.LastElapsedFrameCount = req.ElapsedFrameCount

		// Count costume skill usages from SkillUseInfo. The client does not always populate
		// BattleDetail.playerCostumeActiveSkillUsedCount, so this per-skill breakdown is the
		// reliable source for Type 6/54 missions.
		costumeSkillDetailIds := s.holder.Get().Quest.CostumeSkillDetailIds
		costumeSkillUsesFromInfo := int32(0)
		for _, party := range req.UserPartyResultInfoList {
			for _, su := range party.SkillUseInfo {
				if costumeSkillDetailIds[su.SkillDetailId] {
					costumeSkillUsesFromInfo += su.UseCount
				}
			}
		}

		if d := req.GetBattleDetail(); d != nil {
			user.Battle.LastCharacterDeathCount = d.CharacterDeathCount
			// ACCUMULATE skill counts across waves (don't overwrite)
			// For costume skills prefer BattleDetail, but fall back to the SkillUseInfo
			// breakdown when the client sends 0 there despite skills being used.
			costumeSkillDelta := d.PlayerCostumeActiveSkillUsedCount
			if costumeSkillDelta == 0 && costumeSkillUsesFromInfo > 0 {
				costumeSkillDelta = costumeSkillUsesFromInfo
			}
			user.Battle.LastCostumeSkillUsedCount += costumeSkillDelta
			user.Battle.LastWeaponSkillUsedCount += d.PlayerWeaponActiveSkillUsedCount
			user.Battle.LastCompanionSkillUsedCount += d.PlayerCompanionSkillUsedCount
			user.Battle.LastCriticalCount += d.CriticalCount
			// For combo count, take the maximum value received from client
			// This is the max combo chain achieved during this wave
			if d.ComboCount > user.Battle.LastComboCount {
				user.Battle.LastComboCount = d.ComboCount
			}
			// ACCUMULATE recovery points
			user.Battle.LastTotalRecoverPoint += d.TotalRecoverPoint
			// For max damage, keep the maximum value seen
			if d.ComboMaxDamage > user.Battle.LastComboMaxDamage {
				user.Battle.LastComboMaxDamage = d.ComboMaxDamage
			}

			// Initialize all slots to alive (default), then overwrite with battle data
			aliveCount := 0
			partySize := int32(len(d.CostumeBattleInfo))

			// Resolve which quest is actually being fought right now, and take its
			// deck. Each quest flow keeps its own "current" pointer that is non-zero
			// only while that flow is active:
			//   - EventQuest.CurrentQuestId / ExtraQuest.CurrentQuestId for event /
			//     extra battles (these NEVER set Battle.LastStartedQuestId), and
			//   - Battle.LastStartedQuestId for the main flow.
			// Using the correct quest id is what makes deck-derived missions
			// (element / weapon type 5/19) evaluate against the deck the player really
			// used. The previous code only knew about LastStartedQuestId and otherwise
			// scanned user.Quests for the first "Active" entry, which is nondeterministic
			// (Go randomizes map order) and, for event/extra quests, picked an unrelated
			// main quest's deck - so a correct dark-weapon event deck was ignored and a
			// "clear with N-affinity main weapon" mission failed to count even though
			// every character met it.
			activeQuestId := int32(0)
			switch {
			case user.EventQuest.CurrentQuestId != 0:
				activeQuestId = user.EventQuest.CurrentQuestId
			case user.ExtraQuest.CurrentQuestId != 0:
				activeQuestId = user.ExtraQuest.CurrentQuestId
			case user.Battle.LastStartedQuestId != 0:
				activeQuestId = user.Battle.LastStartedQuestId
			}
			deckNumber := int32(0)
			if activeQuestId != 0 {
				if qstate, ok := user.Quests[activeQuestId]; ok {
					deckNumber = qstate.UserDeckNumber
				}
			}
			if deckNumber == 0 {
				// Last resort for flows with no current-quest pointer (e.g. big hunt):
				// pick the lowest-numbered active quest's deck so behaviour is at least
				// deterministic rather than map-order dependent.
				var bestId int32 = -1
				for qid, qstate := range user.Quests {
					if qstate.QuestStateType == model.UserQuestStateTypeActive && (bestId < 0 || qid < bestId) {
						bestId = qid
					}
				}
				if bestId >= 0 {
					deckNumber = user.Quests[bestId].UserDeckNumber
				}
			}
			if deckNumber == 0 {
				deckNumber = 1
			}

			deckKey := store.DeckKey{DeckType: model.DeckTypeQuest, UserDeckNumber: deckNumber}
			deck, _ := user.Decks[deckKey]
			deckUuids := [3]string{deck.UserDeckCharacterUuid01, deck.UserDeckCharacterUuid02, deck.UserDeckCharacterUuid03}

			for i, ci := range d.CostumeBattleInfo {
				if i >= 3 {
					break
				}
				if ci.IsAlive {
					aliveCount++
				}
				if ci.MaxHp > 0 {
					user.Battle.LastCostumeHpPercent[i] = int32(ci.GetRemainingHp() * 100 / ci.MaxHp)
				} else {
					user.Battle.LastCostumeHpPercent[i] = 0
				}
				// Save character IDs from the battle
				user.Battle.LastPartyCharacterIds[i] = ci.DeckCharacterNumber

				// Save main weapon ID - map DeckCharacterNumber to UUID and get weapon
				if ci.DeckCharacterNumber >= 1 && ci.DeckCharacterNumber <= 3 {
					dcUuid := deckUuids[ci.DeckCharacterNumber-1]
					if dcUuid != "" {
						if dc, ok := user.DeckCharacters[dcUuid]; ok {
							if w, ok := user.Weapons[dc.MainUserWeaponUuid]; ok {
								user.Battle.LastMainWeaponIds[i] = w.WeaponId
							}
						}
					}
				}
			}
			user.Battle.LastCostumeAliveCount = int32(aliveCount)
			user.Battle.LastCostumePartySize = partySize
		} else {
			// No BattleDetail at all: still credit costume skill uses from SkillUseInfo
			if costumeSkillUsesFromInfo > 0 {
				user.Battle.LastCostumeSkillUsedCount += costumeSkillUsesFromInfo
			}
		}
	})
	return &pb.FinishWaveResponse{}, nil
}
