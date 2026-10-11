package service

import (
	"context"
	"log"
	"os"
	"sort"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/runtime"
	"lunar-tear/server/internal/store"

	"google.golang.org/protobuf/encoding/protojson"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

type PvpServiceServer struct {
	pb.UnimplementedPvpServiceServer
	users    store.UserRepository
	sessions store.SessionRepository
	snaps    store.SnapshotRepository
	dir      *PlayerDirectory
	holder   *runtime.Holder
}

func NewPvpServiceServer(users store.UserRepository, sessions store.SessionRepository, snaps store.SnapshotRepository, dir *PlayerDirectory, holder *runtime.Holder) *PvpServiceServer {
	return &PvpServiceServer{users: users, sessions: sessions, snaps: snaps, dir: dir, holder: holder}
}

// currentSeasonId is the fallback season id used only when the PvP catalog is
// unavailable (e.g. in tests that build the service without masterdata).
const currentSeasonId int32 = 1

// seasonIdFor resolves the season id shown to the client: the newest
// masterdata season (a future placeholder whose date window has not started).
// The client looks this id up in its own masterdata, sees the season is not
// active, and disables the battle button — which is what we want: manual
// arena battles are unusable (the battle screen freezes), so the entry point
// is intentionally closed. Server-side reward logic stays on the active
// season, see seasonFor.
func (s *PvpServiceServer) seasonIdFor(user *store.UserState) int32 {
	if cat := s.holder.Get(); cat.Pvp != nil {
		if season, ok := cat.Pvp.LatestSeason(); ok {
			return season.SeasonId
		}
	}
	return currentSeasonId
}

// seasonFor resolves the full season record (grade group etc.) that selects
// the arena reward tables: the season active at the current date.
func (s *PvpServiceServer) seasonFor(user *store.UserState) (masterdata.PvpSeason, bool) {
	if cat := s.holder.Get(); cat.Pvp != nil {
		return cat.Pvp.CurrentSeason(gametime.NowMillis())
	}
	return masterdata.PvpSeason{}, false
}

// recordPvpBestRank keeps the player card's "best rank this season" figure
// current: rank is a ladder position (lower = better), so a new record is a
// strictly lower number. A stored record from a different (past) season is
// discarded before comparing, so each season starts clean.
func recordPvpBestRank(u *store.UserState, rank int, seasonId int32, nowMillis int64) {
	if rank <= 0 || seasonId == 0 {
		return
	}
	if u.Pvp.MaxSeasonRankSeasonId != seasonId {
		u.Pvp.MaxSeasonRank = 0
	}
	if u.Pvp.MaxSeasonRank == 0 || int32(rank) < u.Pvp.MaxSeasonRank {
		u.Pvp.MaxSeasonRank = int32(rank)
		u.Pvp.MaxSeasonRankSeasonId = seasonId
		u.Pvp.LatestVersion = nowMillis
	}
}

func (s *PvpServiceServer) GetTopData(ctx context.Context, _ *emptypb.Empty) (*pb.GetTopDataResponse, error) {
	userId := CurrentUserId(ctx, s.users, s.sessions)
	user, _ := s.users.LoadUser(userId)
	resp := &pb.GetTopDataResponse{
		CurrentSeasonId: s.seasonIdFor(&user),
		PvpPoint:        user.Pvp.PvpPoint,
	}
	// Surface a pending weekly grade reward announcement exactly once: the items
	// were already mailed at the week boundary, so here we only build the popup
	// payload and clear the flag so re-entering the arena doesn't re-announce.
	if user.Pvp.PendingWeeklyRewardGroupId != 0 {
		after, err := s.users.UpdateUser(userId, func(u *store.UserState) {
			if u.Pvp.PendingWeeklyRewardGroupId == 0 {
				return
			}
			resp.WeeklyGradeResult = &pb.WeeklyGradeResult{
				TargetSeasonId:              u.Pvp.PendingWeeklyRewardSeasonId,
				PvpPoint:                    u.Pvp.PendingWeeklyRewardPoint,
				PvpGradeWeeklyRewardGroupId: u.Pvp.PendingWeeklyRewardGroupId,
			}
			u.Pvp.PendingWeeklyRewardGroupId = 0
			u.Pvp.PendingWeeklyRewardPoint = 0
			u.Pvp.PendingWeeklyRewardSeasonId = 0
		})
		if err == nil {
			user = after
		} else {
			// The flag was not persisted: keep the announcement for the next
			// entry instead of consuming it here.
			resp.WeeklyGradeResult = nil
		}
	}
	rank, _ := s.snaps.RankOfPlayer(user.PlayerId)
	if rank <= 0 {
		// No ladder row yet (arena just opened, snapshot not written): the
		// player stands at the very bottom of the board — never rank 1.
		rank = 1
		if count, err := s.snaps.CountSnapshots(); err == nil {
			rank = count + 1
		}
	}
	resp.Rank = int32(rank)
	return resp, nil
}

// pointDelta is the AP change for one match, reverse-engineered from
// recorded retail arena results: every observed fluctuation is a multiple of
// 12 (the retail unit), and both outcomes follow the same rating-gap curve
// with gap = oppPoint - myPoint (positive when the opponent is stronger):
//
//	win:  +192 + gap/12, clamped to [+12, +300]
//	loss: -144 + gap/12, clamped to [-288, -144]
//
// Beating a stronger opponent pays +1 per 12 points of their lead; beating a
// weaker one pays less. Losing to a weaker opponent (negative gap) costs more
// than losing to a stronger one. Caller floors the total at 0.
const (
	pvpPointUnit = 12
	pvpWinBase   = 192
	pvpWinMin    = 12
	pvpWinMax    = 300
	pvpLoseBase  = 144
	pvpLoseMax   = 288
)

func pointDelta(myPoint, oppPoint int32, victory bool) int32 {
	gap := (oppPoint - myPoint) / pvpPointUnit
	if victory {
		d := pvpWinBase + gap
		if d < pvpWinMin {
			d = pvpWinMin
		}
		if d > pvpWinMax {
			d = pvpWinMax
		}
		return d
	}
	d := gap - pvpLoseBase
	if d > -pvpLoseBase {
		d = -pvpLoseBase
	}
	if d < -pvpLoseMax {
		d = -pvpLoseMax
	}
	return d
}

func applyPointDelta(cur, delta int32) int32 {
	v := cur + delta
	if v < 0 {
		return 0
	}
	return v
}

// matchingCount is the number of opponent slots the arena battle-target
// screen displays.
const matchingCount = 3

// pvpMatchingDailyRefreshLimit caps how many times the opponent list may be
// rerolled per arena day (like the original). Once exhausted, UpdateMatchingList
// keeps returning the current list unchanged until the day rolls over.
const pvpMatchingDailyRefreshLimit = 5

// matchingRankTargets picks one ladder position per opponent slot from a
// window around the player's own rank, spreading the slots into one clearly
// above (risky, big gain on a win), one close, one below (safe, small gain).
// Window size follows recorded retail matchmaking: near the top of the
// ladder (rank <= 1000) opponents come from ~±20 ranks, further down the
// window widens to ~±150 ranks.
const (
	matchingTopRankWindow   = 20
	matchingLowRankWindow   = 150
	matchingWindowThreshold = 1000
	// matchingScanWindow is how far around a target rank the slot scan walks
	// to find a rated opponent — rating-0 rows (players who never opened the
	// arena, clustered at the ladder tail) are skipped, so the scan steps to
	// the nearest entry that actually stands on the board.
	matchingScanWindow = 10
)

func matchingRankTargets(rank int) []int {
	window := matchingLowRankWindow
	if rank <= matchingWindowThreshold {
		window = matchingTopRankWindow
	}
	targets := []int{rank - window/2, rank + window/4, rank + window}
	for i, t := range targets {
		if t < 1 {
			targets[i] = 1
		}
	}
	return targets
}

// ladderCardsInRankWindow returns up to limit ladder entries (real players
// and roster bots alike) whose rank falls in [fromRank, toRank], excluding
// excludeId, best rank first. This is how arena opponents are chosen: by
// ladder position, not rating proximity.
func ladderCardsInRankWindow(snaps store.SnapshotRepository, excludeId int64, fromRank, toRank, limit int) []PlayerCard {
	if fromRank < 1 {
		fromRank = 1
	}
	if toRank < fromRank {
		return nil
	}
	rows, err := snaps.ListSnapshotsByPointDesc(fromRank-1, toRank-fromRank+1)
	if err != nil {
		return nil
	}
	out := make([]PlayerCard, 0, len(rows))
	for _, sn := range rows {
		if sn.PlayerId == excludeId {
			continue
		}
		out = append(out, cardFromSnapshot(sn))
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (s *PvpServiceServer) buildMatching(user *store.UserState) []store.MatchingEntry {
	// The screen has exactly three opponent slots. Each is drawn from the
	// live ladder at one target rank around the player (see
	// matchingRankTargets); only rated opponents qualify — a rating-0
	// snapshot is a player who has not opened the arena yet and must never
	// be offered. An account without a ladder rank yet falls back to rating
	// proximity instead; any slot still empty is filled with a bot.
	var cards []PlayerCard
	seen := map[int64]bool{user.PlayerId: true}
	if rank, _ := s.snaps.RankOfPlayer(user.PlayerId); rank > 0 {
		for _, target := range matchingRankTargets(rank) {
			from := target - matchingScanWindow
			if from < 1 {
				from = 1
			}
			window := ladderCardsInRankWindow(s.snaps, user.PlayerId, from, target+matchingScanWindow, 2*matchingScanWindow+1)
			// The rated card nearest the target rank wins the slot.
			best, bestDist := PlayerCard{}, -1
			for i, c := range window {
				if c.PvpPoint <= 0 || seen[c.PlayerId] {
					continue
				}
				dist := from + i - target
				if dist < 0 {
					dist = -dist
				}
				if bestDist < 0 || dist < bestDist {
					best, bestDist = c, dist
				}
			}
			if bestDist >= 0 {
				cards = append(cards, best)
				seen[best.PlayerId] = true
			}
		}
	} else {
		for _, c := range s.dir.RealPlayersNear(user.PlayerId, user.Pvp.PvpPoint, matchingCount*4) {
			if c.PvpPoint <= 0 || seen[c.PlayerId] {
				continue
			}
			cards = append(cards, c)
			seen[c.PlayerId] = true
			if len(cards) >= matchingCount {
				break
			}
		}
	}
	// Any slot the rank windows missed is topped up from the table itself —
	// real bots and players with their live rating, power and deck — before
	// synthesized fill bots are even considered.
	if len(cards) < matchingCount {
		for _, c := range s.dir.RealPlayersNear(user.PlayerId, user.Pvp.PvpPoint, matchingCount*8) {
			if c.PvpPoint <= 0 || seen[c.PlayerId] {
				continue
			}
			cards = append(cards, c)
			seen[c.PlayerId] = true
			if len(cards) >= matchingCount {
				break
			}
		}
	}
	cards = s.dir.FillWithBots(cards, matchingCount, user.PlayerId, gametime.NowMillis(), user.Pvp.PvpPoint)
	out := make([]store.MatchingEntry, 0, len(cards))
	for _, c := range cards {
		rank, _ := s.snaps.RankOfPlayer(c.PlayerId)
		if rank == 0 {
			// Ephemeral fill bots have no snapshot row; never surface rank 0
			// to the client (the battle-confirm screen chokes on it).
			rank = 1
		}
		out = append(out, store.MatchingEntry{
			PlayerId: c.PlayerId, Name: c.Name, PvpPoint: c.PvpPoint, Rank: int32(rank),
			DeckPower: c.MaxDeckPower, IsBot: c.IsBot, MostPowerfulCostumeId: c.FavoriteCostumeId,
			DeckMainWeaponAttributeTypes: s.deckMainWeaponAttributes(c),
		})
	}
	return out
}

// deckMainWeaponAttributes resolves the element of each main weapon in the
// opponent's defense deck. The client's battle-target screen draws one element
// icon per deck slot and indexes those slots directly, so this must never be
// empty for a listed opponent.
func (s *PvpServiceServer) deckMainWeaponAttributes(card PlayerCard) []int32 {
	deck := s.dir.DefenseDeckOf(card)
	if len(deck) == 0 {
		return nil
	}
	var attrByWeapon map[int32]int32
	if cat := s.holder.Get(); cat.Quest != nil {
		attrByWeapon = cat.Quest.WeaponAttributeById
	}
	out := make([]int32, 0, len(deck))
	for _, ch := range deck {
		var attr int32
		if ch != nil && ch.MainWeapon != nil {
			attr = attrByWeapon[ch.MainWeapon.WeaponId]
		}
		out = append(out, attr)
	}
	return out
}

func matchingToProto(entries []store.MatchingEntry) []*pb.MatchingOpponent {
	var out []*pb.MatchingOpponent
	for _, e := range entries {
		out = append(out, &pb.MatchingOpponent{
			PlayerId: e.PlayerId, Name: e.Name, PvpPoint: e.PvpPoint, Rank: e.Rank,
			DeckPower: e.DeckPower, MostPowerfulCostumeId: e.MostPowerfulCostumeId,
			DeckMainWeaponAttributeType: e.DeckMainWeaponAttributeTypes,
		})
	}
	return out
}

func (s *PvpServiceServer) GetMatchingList(ctx context.Context, _ *emptypb.Empty) (*pb.GetMatchingListResponse, error) {
	userId := CurrentUserId(ctx, s.users, s.sessions)
	after, _ := s.users.UpdateUser(userId, func(u *store.UserState) {
		if needsMatchingRebuild(u.PvpMatching) {
			u.PvpMatching = s.buildMatching(u)
		}
	})
	return &pb.GetMatchingListResponse{Matching: matchingToProto(after.PvpMatching)}, nil
}

// needsMatchingRebuild reports whether the cached matching list must be rebuilt:
// when it is empty, when its size doesn't match the slot count (older caches
// hold 5 entries), or when it predates the deck-element data the battle-target
// screen requires (older cached rows lack DeckMainWeaponAttributeTypes).
func needsMatchingRebuild(entries []store.MatchingEntry) bool {
	if len(entries) != matchingCount {
		return true
	}
	for _, e := range entries {
		if len(e.DeckMainWeaponAttributeTypes) == 0 {
			return true
		}
	}
	return false
}

// UpdateMatchingList rerolls the opponent list, but only while the player has
// refreshes left today (pvpMatchingDailyRefreshLimit, reset each server day).
// Once exhausted the call is a no-op returning the current list, so the player
// fights whoever is listed (and can lose to them).
func (s *PvpServiceServer) UpdateMatchingList(ctx context.Context, _ *emptypb.Empty) (*pb.UpdateMatchingListResponse, error) {
	userId := CurrentUserId(ctx, s.users, s.sessions)
	after, _ := s.users.UpdateUser(userId, func(u *store.UserState) {
		day := gametime.StartOfDayMillis()
		if u.Pvp.MatchingRefreshDay != day {
			u.Pvp.MatchingRefreshDay = day
			u.Pvp.MatchingRefreshCount = 0
		}
		if u.Pvp.MatchingRefreshCount >= pvpMatchingDailyRefreshLimit {
			return // daily limit spent: keep the list as is
		}
		u.Pvp.MatchingRefreshCount++
		u.PvpMatching = s.buildMatching(u)
	})
	return &pb.UpdateMatchingListResponse{Matching: matchingToProto(after.PvpMatching)}, nil
}

func (s *PvpServiceServer) StartBattle(ctx context.Context, req *pb.StartBattleRequest) (*pb.StartBattleResponse, error) {
	userId := CurrentUserId(ctx, s.users, s.sessions)
	user, _ := s.users.LoadUser(userId)
	card, ok := s.dir.CardFor(req.OpponentPlayerId)
	if !ok {
		// Real opponent with no snapshot (rare): degrade to a fresh bot rather than erroring the screen.
		card = synthBot(s.dir.pools(), user.PlayerId, 0, gametime.NowMillis(), user.Pvp.PvpPoint)
	}
	deck := s.dir.DefenseDeckOf(card)
	// Diagnostic mirror mode: when LUNAR_PVP_MIRROR is set, the opponent gets a
	// copy of the viewer's own arena deck. That deck's costumes/weapons are
	// proven to load on this exact device, so if the battle still hangs the
	// cause is outside the deck payload. Temporary; remove once resolved.
	if os.Getenv("LUNAR_PVP_MIRROR") != "" {
		if mirror := BuildPvpDeckCharacters(&user, model.DeckTypePvp, req.UseDeckNumber); len(mirror) > 0 {
			s.dir.resolveWeaponSlotIds(mirror)
			log.Printf("[PvpDebug] MIRROR MODE: opponent deck replaced with viewer's own arena deck #%d", req.UseDeckNumber)
			deck = mirror
		}
	}
	debugDumpStartBattle(user, req, card, deck)
	return &pb.StartBattleResponse{OpponentDeckCharacter: deck}, nil
}

// debugDumpStartBattle logs, on the server side, the exact opponent deck the
// client receives plus the player's own chosen arena deck, so a client-side
// battle-load hang can be diagnosed without access to the device log (the
// player runs the client on iPad). Temporary diagnostic; remove once resolved.
func debugDumpStartBattle(user store.UserState, req *pb.StartBattleRequest, card PlayerCard, oppDeck []*pb.PvpDeckCharacter) {
	log.Printf("[PvpDebug] StartBattle: viewer=%d opponent=%d isBot=%v useDeckNumber=%d oppDeckChars=%d",
		user.PlayerId, req.OpponentPlayerId, card.IsBot, req.UseDeckNumber, len(oppDeck))
	marshal := protojson.MarshalOptions{EmitUnpopulated: false}
	for i, ch := range oppDeck {
		b, _ := marshal.Marshal(ch)
		log.Printf("[PvpDebug]   opponent char[%d]=%s", i, string(b))
	}
	ownDeck := BuildPvpDeckCharacters(&user, model.DeckTypePvp, req.UseDeckNumber)
	if len(ownDeck) == 0 {
		ownDeck = BuildPvpDeckCharacters(&user, model.DeckTypeQuest, req.UseDeckNumber)
		log.Printf("[PvpDebug]   (arena deck #%d empty; fell back to quest deck, chars=%d)", req.UseDeckNumber, len(ownDeck))
	}
	log.Printf("[PvpDebug] StartBattle: own arena deck #%d chars=%d", req.UseDeckNumber, len(ownDeck))
	for i, ch := range ownDeck {
		b, _ := marshal.Marshal(ch)
		log.Printf("[PvpDebug]   own char[%d]=%s", i, string(b))
	}
}

const maxLogEntries = 30

func capLog(entries []store.BattleLogEntry) []store.BattleLogEntry {
	if len(entries) <= maxLogEntries {
		return entries
	}
	return entries[len(entries)-maxLogEntries:]
}

func (s *PvpServiceServer) FinishBattle(ctx context.Context, req *pb.FinishBattleRequest) (*pb.FinishBattleResponse, error) {
	userId := CurrentUserId(ctx, s.users, s.sessions)
	now := gametime.NowMillis()

	self, _ := s.users.LoadUser(userId)
	beforePoint := self.Pvp.PvpPoint
	beforeRank, _ := s.snaps.RankOfPlayer(self.PlayerId)

	opp := store.MatchingEntry{PlayerId: req.OpponentPlayerId}
	if card, ok := s.dir.CardFor(req.OpponentPlayerId); ok {
		opp.Name = card.Name
		opp.PvpPoint = card.PvpPoint
		opp.DeckPower = card.MaxDeckPower
		opp.IsBot = card.IsBot
	}
	delta := pointDelta(beforePoint, opp.PvpPoint, req.IsVictory)

	cat := s.holder.Get()

	// Power ladder cap: on a win the player may never climb above the
	// strongest bot that outranks them in deck power (equal rating = equal
	// rank, never higher). The compared power is the better of the exposed
	// defense deck (snapshot) and the player's strongest reported deck — the
	// defense deck alone is often unreported (power 0/100) and would pin a
	// strong player to the ladder bottom. Losses are unaffected.
	newPoint := applyPointDelta(beforePoint, delta)
	if delta > 0 {
		if snap, err := s.snaps.GetSnapshot(self.PlayerId); err == nil {
			myPower := snap.MaxDeckPower
			if sp := strongestDeckPower(&self); sp > myPower {
				myPower = sp
			}
			if capPoint, ok, capErr := s.snaps.MaxPointAbovePower(self.PlayerId, myPower); capErr == nil && ok && newPoint > capPoint {
				newPoint = capPoint
				delta = newPoint - beforePoint
			}
		}
	}

	// Resolve the per-match reward from the player's post-battle grade. Only a
	// victory pays out: a win yields its grade's one-match reward (arena coins
	// etc.), while a loss earns nothing. The grade ladder comes from the latest
	// masterdata season's tables.
	var oneMatchRewardId, gradeGroupId int32
	var rewardItems []masterdata.RewardItem
	if season, ok := s.seasonFor(&self); ok {
		gradeGroupId = season.GradeGroupId
		if req.IsVictory {
			if grade, ok := cat.Pvp.GradeForPoint(season.GradeGroupId, newPoint); ok {
				oneMatchRewardId, rewardItems = cat.Pvp.RollOneMatchReward(grade.OneMatchRewardGroupId, pvpRoll)
			}
		}
	}

	after, _ := s.users.UpdateUser(userId, func(u *store.UserState) {
		u.Pvp.PvpPoint = applyPointDelta(u.Pvp.PvpPoint, delta)
		if req.IsVictory {
			u.Pvp.AttackWinCount++
			u.Pvp.AttackWinStreak++
		} else {
			u.Pvp.AttackLoseCount++
			u.Pvp.AttackWinStreak = 0
		}
		u.Pvp.LastFinishDay = gametime.StartOfDayMillis()
		// The history screen displays this rank next to the opponent, so it
		// is the opponent's own place on the board — never the viewer's.
		oppRank, _ := s.snaps.RankOfPlayer(req.OpponentPlayerId)
		if oppRank == 0 {
			oppRank = 1
		}
		entry := store.BattleLogEntry{
			Seq: now, OpponentPlayerId: opp.PlayerId, OpponentName: opp.Name,
			OpponentPvpPoint: opp.PvpPoint, OpponentDeckPower: opp.DeckPower,
			IsVictory: req.IsVictory, BattleDatetime: now, FluctuatedPoint: delta, Rank: int32(oppRank),
		}
		u.PvpAttackLog = capLog(append(u.PvpAttackLog, entry))

		// Hand the per-match reward straight into the inventory, and advance
		// the lazy weekly cycle (legacy counters + popup snapshot at the
		// Monday boundary; the mailed reward tabs are daily now).
		grantPvpRewardItems(u, cat.QuestHandler.Granter, rewardItems, now)
		advanceWeeklyCycle(cat.Pvp, u, now)

		// Arena missions: type 15 counts every finished match, type 16 only
		// wins, type 17 the current win streak (max semantics keep the best
		// run). Type 69 (rank threshold) fires below once the snapshot table
		// reflects the new rating and the fresh rank is known.
		ApplyMissionProgressEvent(u, cat.Mission, MissionProgressEvent{ConditionType: missionConditionArenaPlay, Delta: 1}, now)
		if req.IsVictory {
			ApplyMissionProgressEvent(u, cat.Mission, MissionProgressEvent{ConditionType: missionConditionArenaWin, Delta: 1}, now)
			ApplyMissionProgressEvent(u, cat.Mission, MissionProgressEvent{ConditionType: missionConditionArenaWinStreak, CurrentValue: u.Pvp.AttackWinStreak}, now)
		}
	})

	if err := RefreshSnapshot(s.snaps, &after); err != nil {
		log.Printf("[PvpService] FinishBattle snapshot refresh failed: %v", err)
	}

	if !s.dir.IsBot(opp.PlayerId) && opp.PlayerId != 0 {
		if oid, err := s.userIdForPlayer(opp.PlayerId); err == nil {
			s.users.UpdateUser(oid, func(u *store.UserState) {
				defWin := !req.IsVictory
				if defWin {
					u.Pvp.DefenseWinCount++
				} else {
					u.Pvp.DefenseLoseCount++
				}
				entry := store.BattleLogEntry{
					Seq: now, OpponentPlayerId: self.PlayerId, OpponentName: self.Profile.Name,
					OpponentPvpPoint: beforePoint, IsVictory: defWin, BattleDatetime: now, Rank: int32(beforeRank),
				}
				u.PvpDefenseLog = capLog(append(u.PvpDefenseLog, entry))
			})
			// The defender's ladder position may have moved too — keep their
			// player-card best rank in step, exactly like the attacker's below.
			if rank, err := s.snaps.RankOfPlayer(opp.PlayerId); err == nil && rank > 0 {
				s.users.UpdateUser(oid, func(u *store.UserState) {
					if cat.Pvp != nil {
						if season, ok := cat.Pvp.CurrentSeason(now); ok {
							recordPvpBestRank(u, rank, season.SeasonId, now)
						}
					}
				})
			}
		}
	}

	afterRank, _ := s.snaps.RankOfPlayer(self.PlayerId)
	if afterRank == 0 {
		afterRank = 1
	}
	// Type-69 arena missions clear when the player's rank reaches at most the
	// target (lower rank = better), so feed the engine the actual ladder rank.
	// The player card's best-rank figure is refreshed in the same pass, right
	// after the snapshot refresh above, so it never lags behind the ladder.
	s.users.UpdateUser(userId, func(u *store.UserState) {
		if cat.Mission != nil {
			ApplyMissionProgressEvent(u, cat.Mission, MissionProgressEvent{ConditionType: missionConditionArenaRank, CurrentValue: int32(afterRank)}, now)
		}
		if cat.Pvp != nil {
			if season, ok := cat.Pvp.CurrentSeason(now); ok {
				recordPvpBestRank(u, afterRank, season.SeasonId, now)
			}
		}
	})
	return &pb.FinishBattleResponse{
		BeforePvpPoint: beforePoint, BeforeRank: int32(beforeRank),
		AfterPvpPoint: after.Pvp.PvpPoint, AfterRank: int32(afterRank),
		PvpGradeOneMatchRewardId: oneMatchRewardId, PvpGradeGroupId: gradeGroupId,
	}, nil
}

// userIdForPlayer maps a real playerId to userId (player_id == user_id), validating existence.
func (s *PvpServiceServer) userIdForPlayer(playerId int64) (int64, error) {
	if _, err := s.users.LoadUser(playerId); err != nil {
		return 0, err
	}
	return playerId, nil
}

func (s *PvpServiceServer) GetRanking(ctx context.Context, req *pb.GetRankingRequest) (*pb.GetRankingResponse, error) {
	userId := CurrentUserId(ctx, s.users, s.sessions)
	user, _ := s.users.LoadUser(userId)
	const pageSize = 50
	// RankFrom is 1-indexed (rank 1, 101, etc.), but database offset is 0-indexed
	offset := int(req.RankFrom) - 1
	if offset < 0 {
		offset = 0
	}
	snaps, _ := s.snaps.ListSnapshotsByPointDesc(offset, pageSize)
	var rows []*pb.RankingUser
	for i, sn := range snaps {
		rows = append(rows, &pb.RankingUser{
			Rank: int32(offset + i + 1), PlayerId: sn.PlayerId, Name: sn.UserName,
			PvpPoint: sn.PvpPoint, DeckPower: sn.MaxDeckPower, FavoriteCostumeId: sn.FavoriteCostumeId,
		})
	}
	count, _ := s.snaps.CountSnapshots()
	myRank, _ := s.snaps.RankOfPlayer(user.PlayerId)
	return &pb.GetRankingResponse{
		RankingUser: rows, UserCount: int32(count), RankingPosition: int32(myRank),
	}, nil
}

func (s *PvpServiceServer) GetSeasonResult(ctx context.Context, _ *emptypb.Empty) (*pb.GetSeasonResultResponse, error) {
	userId := CurrentUserId(ctx, s.users, s.sessions)
	user, _ := s.users.LoadUser(userId)
	p := user.Pvp
	var defRate int32
	if total := p.DefenseWinCount + p.DefenseLoseCount; total > 0 {
		defRate = (p.DefenseWinCount * 1000) / total
	}
	return &pb.GetSeasonResultResponse{
		AttackWinCount: p.AttackWinCount, AttackLoseCount: p.AttackLoseCount,
		AttackPvpPoint: p.PvpPoint, DefenseWinRatePermil: defRate, DefensePvpPoint: p.PvpPoint,
	}, nil
}

// logToProto maps stored history rows to client BattleLogs. Both the battle
// timestamp and one costume id per opponent deck slot must be present: the
// history screen renders both per entry, and a row without them leaves the
// screen unresponsive (same pattern as the element icons on the battle-target
// screen).
func (s *PvpServiceServer) logToProto(entries []store.BattleLogEntry) []*pb.BattleLog {
	sorted := append([]store.BattleLogEntry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Seq > sorted[j].Seq })
	var out []*pb.BattleLog
	for _, e := range sorted {
		out = append(out, &pb.BattleLog{
			PlayerId: e.OpponentPlayerId, Name: e.OpponentName, PvpPoint: e.OpponentPvpPoint,
			DeckPower: e.OpponentDeckPower, DeckCostumeId: s.deckCostumeIds(e.OpponentPlayerId),
			IsVictory: e.IsVictory, BattleDatetime: safeTimestamp(e.BattleDatetime),
			FluctuatedPvpPoint: e.FluctuatedPoint, Rank: e.Rank,
		})
	}
	return out
}

// deckCostumeIds resolves the costume ids shown on a battle-log row. Real
// players are represented by their profile avatar only (the favorite costume
// the snapshot table carries, like the ranking table) — the two deck slots
// stay empty on purpose, marking the row as a real player in BOTH the attack
// and the defense history; tapping it opens the player card with the actual
// deck. A real player without a favorite costume yet shows the lead costume
// of their defense deck as the avatar — still a single costume, never the
// full deck. Bots show their synthesized defense deck costumes.
func (s *PvpServiceServer) deckCostumeIds(playerId int64) []int32 {
	card, ok := s.dir.CardFor(playerId)
	if !ok {
		return nil
	}
	if !IsBotId(playerId) {
		if card.FavoriteCostumeId != 0 {
			return []int32{card.FavoriteCostumeId}
		}
		for _, ch := range s.dir.DefenseDeckOf(card) {
			if ch != nil && ch.Costume != nil && ch.Costume.CostumeId != 0 {
				return []int32{ch.Costume.CostumeId}
			}
		}
		return nil
	}
	out := make([]int32, 0, 3)
	for _, ch := range s.dir.DefenseDeckOf(card) {
		id := card.FavoriteCostumeId
		if ch != nil && ch.Costume != nil && ch.Costume.CostumeId != 0 {
			id = ch.Costume.CostumeId
		}
		out = append(out, id)
	}
	if len(out) == 0 && card.FavoriteCostumeId != 0 {
		out = append(out, card.FavoriteCostumeId)
	}
	return out
}

func (s *PvpServiceServer) GetAttackLogList(ctx context.Context, _ *emptypb.Empty) (*pb.GetAttackLogListResponse, error) {
	userId := CurrentUserId(ctx, s.users, s.sessions)
	user, _ := s.users.LoadUser(userId)
	return &pb.GetAttackLogListResponse{AttackLog: s.logToProto(user.PvpAttackLog)}, nil
}

func (s *PvpServiceServer) GetDefenseLogList(ctx context.Context, _ *emptypb.Empty) (*pb.GetDefenseLogListResponse, error) {
	userId := CurrentUserId(ctx, s.users, s.sessions)
	user, _ := s.users.LoadUser(userId)
	return &pb.GetDefenseLogListResponse{DefenseLog: s.logToProto(user.PvpDefenseLog)}, nil
}
