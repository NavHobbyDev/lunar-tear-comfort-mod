package service

import (
	"encoding/json"
	"log"
	"math/rand"
	"sort"

	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/runtime"
	"lunar-tear/server/internal/store"
)

// Daily arena auto-progress tuning. Battles are simulated honestly: each
// claimed login bonus plays out pvpAutoDailyBattles matches resolved strictly
// one at a time — after every single match BOTH ratings are recalculated (the
// player's and the opponent's, through the same pointDelta formula a real
// FinishBattle uses), and only then is the next opponent drawn from the rank
// window around where the player stands right now. Outcomes are drawn
// probabilistically from deck power (win chance scales with the relative
// difference, ±pvpAutoPowerBand being the contested range), one attack-log
// entry per match records the opponent exactly as they stood on the ladder at
// battle time, and the opponents' rating changes are written back to the
// ladder afterwards — so the history and the ranking table always agree.
const (
	// pvpAutoDailyBattles is how many matches one auto arena day plays out.
	pvpAutoDailyBattles = 20
	// pvpAutoPowerBand is the ±25% relative deck-power band that shapes a
	// simulated day: opponents are normally engaged only within this band of
	// the player's power, and inside it the win chance moves linearly from
	// 100% at the weak edge (-25%) down to 0% at the strong edge (+25%),
	// 50% at equal power. Outside the band the outcome is certain.
	pvpAutoPowerBand = 0.25
)

// RunPvpAutoProgress plays out one arena day for the user: 20 simulated
// battles resolved strictly one at a time — a battle moves BOTH ratings (the
// player's and the opponent's, via the same pointDelta formula a real
// FinishBattle uses, the player never above the strongest stronger bot), the
// player's rank is re-estimated from the new rating, and only then the next
// opponent is picked from the ladder window around that fresh rank (normally
// only opponents within ±25% of the player's deck power, weighted toward
// winnable fights, each opponent fought at most once per day). Opponents
// come exclusively from the ladder table — rated
// roster bots and rated players; rating-0 accounts have not opened the arena
// and are never fought. Each battle resolves through the power curve —
// certain win below the -25% edge, certain loss above the +25% edge, a linear
// chance inside — writes one attack-log entry carrying the opponent's rating
// at battle time, rolls the grade's per-match reward into the inventory on
// each win (a loss earns nothing), and feeds the real battle/win/streak
// counts into arena missions 15/16/17.
// Afterwards the opponents' rating changes are persisted (bot snapshot rows,
// real players' live rating plus defense log), all three reward tabs are
// mailed for the new ladder position (grade weekly, weekly ranking and the
// season ranking reward — every arena day, not just Mondays) and the lazy
// weekly cycle advances the legacy counters; all reward tables come from the
// latest masterdata season.
// The day only runs once the arena is open for the player (the PvP tutorial
// has been reached) and is idempotent per day (PvpState.LastAutoProgressDay).
// Invoked the moment the arena tutorial is reported (first arena day plays
// out in the same session it opens) and by each login bonus claim afterwards;
// the snapshot table is refreshed mid-flow so the new rank is live for
// ranking, matching and type-69 rank missions.
func RunPvpAutoProgress(users store.UserRepository, snaps store.SnapshotRepository, holder *runtime.Holder, userId int64) {
	now := gametime.NowMillis()
	day := gametime.StartOfDayMillis()
	cat := holder.Get()

	user, err := users.LoadUser(userId)
	if err != nil {
		log.Printf("[PvpAutoProgress] load user %d failed: %v", userId, err)
		return
	}
	if user.Pvp.LastAutoProgressDay >= day {
		return // already simulated today
	}
	// "Arena open" gate: the client only reveals the arena once the PvP
	// tutorial (type 11) unlocks, and entering it reports tutorial progress.
	// No recorded PvP tutorial means the player hasn't reached the arena yet,
	// so no arena day is simulated for them.
	if _, ok := user.Tutorials[int32(model.TutorialTypePvp)]; !ok {
		return
	}

	// Arena lineup guard: the server-managed arena deck is backed up in
	// PvpState.PvpDeckBackup whenever the server writes it. Any difference
	// from the backup — an edited lineup, a deleted deck, the defense
	// selection moved off slot #1 — restores the lineup from the backup, but
	// NEVER touches the rating: drift detection has false positives, and a
	// lost lineup must not cost the player their rank. Players without a
	// backup yet are seeded once from the strongest quest deck.
	//
	// Yesterday's standing for the daily reward payout and the starting rank
	// are captured BEFORE the sync: a restored deck must not change what the
	// previous day earned.
	payDailyRewards := user.Pvp.LastAutoProgressDay > 0
	yesterdayPoints := user.Pvp.PvpPoint
	yesterdayRank, _ := snaps.RankOfPlayer(user.PlayerId)
	if yesterdayRank <= 0 {
		if count, err := snaps.CountSnapshots(); err == nil {
			yesterdayRank = count + 1 // unranked: start at the ladder tail
		}
	}
	if _, err := users.UpdateUser(userId, func(u *store.UserState) {
		ensureArenaDeck(u, now)
	}); err != nil {
		log.Printf("[PvpAutoProgress] user %d: arena deck sync failed: %v", userId, err)
	}
	user, err = users.LoadUser(userId)
	if err != nil {
		return
	}
	// Refresh the snapshot immediately after any deck changes so the correct
	// power is used for matchmaking and ranking display (not a stale snapshot).
	if err := RefreshSnapshot(snaps, &user); err != nil {
		log.Printf("[PvpAutoProgress] snapshot refresh after deck sync failed: %v", err)
	}
	
	// Opponents come from the same rank window the matching list uses. The
	// window is re-centered before every battle on the rank the running
	// rating currently holds, so a rating climb/drop changes who is fought
	// next — exactly the battle -> new rating -> new opponent loop a real
	// arena session plays out.
	startRank := yesterdayRank

	// Resolve the active season once: both the player's own best-rank record
	// and every real opponent's are written against it.
	var seasonId int32
	if cat.Pvp != nil {
		if season, ok := cat.Pvp.CurrentSeason(now); ok {
			seasonId = season.SeasonId
		}
	}
	if payDailyRewards {
		mailPvpDailyRewardItems(users, holder, userId, yesterdayPoints, yesterdayRank, now)
	}

	// The player's own strength: the better of the snapshot's deck power
	// (their arena defense deck) and their strongest reported deck — the
	// defense deck is frequently unreported (power 0/100) and alone would
	// make the simulation treat a strong player as weak. Fresh accounts
	// without either field a minimal team so the simulation still resolves.
	myPower := int32(1000)
	if snap, err := snaps.GetSnapshot(user.PlayerId); err == nil && snap.MaxDeckPower > myPower {
		myPower = snap.MaxDeckPower
	}
	if sp := strongestDeckPower(&user); sp > myPower {
		myPower = sp
	}

	// The whole ladder in rating order, fetched once and kept mutable: every
	// simulated battle rewrites the ratings it moves right into this copy, so
	// each battle's rank estimate and opponent pool see the ladder exactly as
	// the battles so far have shaped it — no snapshot writes between battles.
	ladder, err := ladderSnapshots(snaps)
	if err != nil {
		log.Printf("[PvpAutoProgress] user %d: ladder fetch failed: %v", userId, err)
		return
	}
	ladderIdx := make(map[int64]int, len(ladder))
	for i, sn := range ladder {
		ladderIdx[sn.PlayerId] = i
	}
	rankAtPoint := func(playerId int64, point int32) int {
		n := 0
		for _, sn := range ladder {
			if sn.PlayerId != playerId && sn.PvpPoint > point {
				n++
			}
		}
		return n + 1
	}
	// rankOfAny is the ladder position of any rating as the ranking table
	// itself would show it (RankOfPlayer semantics). The battle log records
	// the OPPONENT's rank through this — the history screen displays it next
	// to the opponent, so it must be the opponent's place on the board.
	rankOfAny := func(point int32) int {
		n := 0
		for _, sn := range ladder {
			if sn.PvpPoint > point {
				n++
			}
		}
		return n + 1
	}

	// Power ladder cap, same rule FinishBattle enforces: the player may never
	// climb above a bot whose deck power beats theirs. The running rating is
	// clamped after every battle so each logged delta stays accurate.
	var capPoint int32
	var capped bool
	if cp, ok, capErr := snaps.MaxPointAbovePower(user.PlayerId, myPower); capErr == nil && ok {
		capPoint, capped = cp, true
	}

	r := newRand(seedFrom(userId, 0, day))
	myPoint := user.Pvp.PvpPoint
	pool := ladderPoolByRank(ladder, user.PlayerId, rankAtPoint(user.PlayerId, myPoint), myPower)
	if len(pool) == 0 {
		log.Printf("[PvpAutoProgress] user %d: no opponents around rank %d, day skipped", userId, startRank)
		return
	}
	streak := user.Pvp.AttackWinStreak
	maxStreak := streak
	var wins, losses int32
	var entries []store.BattleLogEntry
	var matchItems []masterdata.RewardItem
	fought := make(map[int64]bool)
	oppState := make(map[int64]*autoOpponentTrack)
	for i := 0; i < pvpAutoDailyBattles; i++ {
		curRank := rankAtPoint(user.PlayerId, myPoint)
		if candidates := ladderPoolByRank(ladder, user.PlayerId, curRank, myPower); len(candidates) > 0 {
			pool = candidates
		}
		// An opponent is fought at most once per day while the pool has
		// anyone fresh: a beaten bot belongs where the battle put it on the
		// ladder — re-drawing it (the weighted draw loves winnable fights)
		// drags the same name through battle after battle instead of letting
		// it stay at the bottom of the table.
		picks := pool
		if len(pool) > 1 {
			fresh := make([]PlayerCard, 0, len(pool))
			for _, c := range pool {
				if !fought[c.PlayerId] {
					fresh = append(fresh, c)
				}
			}
			if len(fresh) > 0 {
				picks = fresh
			}
		}
		// Winnable fights are picked more often: the draw is weighted by the
		// win chance of each matchup.
		opp := pickWeightedOpponent(r, picks, myPower)
		fought[opp.PlayerId] = true
		oppRank := rankOfAny(opp.PvpPoint) // the opponent's place on the board at battle time
		victory := simulateArenaOutcome(r, myPower, opp.MaxDeckPower)
		myBefore := myPoint
		newPoint := applyPointDelta(myPoint, pointDelta(myPoint, opp.PvpPoint, victory))
		if capped && newPoint > capPoint {
			newPoint = capPoint
		}
		delta := newPoint - myPoint
		myPoint = newPoint
		// The opponent's side of the same match, mirrored through the same
		// retail formula: beaten opponents drop rating, winning opponents
		// climb. The move lands in the living ladder immediately, so the
		// battles that follow draw their opponents from the updated board.
		oppNew := applyPointDelta(opp.PvpPoint, pointDelta(opp.PvpPoint, myBefore, !victory))
		if idx, ok := ladderIdx[opp.PlayerId]; ok {
			ladder[idx].PvpPoint = oppNew
		}
		tr := oppState[opp.PlayerId]
		if tr == nil {
			tr = &autoOpponentTrack{original: opp.PvpPoint, point: opp.PvpPoint}
			oppState[opp.PlayerId] = tr
		}
		tr.point = oppNew
		if victory {
			wins++
			streak++
			if streak > maxStreak {
				maxStreak = streak
			}
		} else {
			losses++
			streak = 0
		}
		if !IsBotId(opp.PlayerId) {
			// The defender's view of this match. Besides the attacker's identity
			// the defense tab shows two figures the client reads off the same
			// entry: the attacker's deck power (OpponentDeckPower) and the
			// defender's own rating move this match (FluctuatedPoint) — both
			// are already resolved here, so never leave them zero.
			if victory {
				tr.defLosses++
			} else {
				tr.defWins++
			}
			tr.logs = append(tr.logs, store.BattleLogEntry{
				Seq: now + int64(i), OpponentPlayerId: user.PlayerId, OpponentName: user.Profile.Name,
				OpponentPvpPoint: myBefore, OpponentDeckPower: myPower,
				IsVictory: !victory, BattleDatetime: now, FluctuatedPoint: oppNew - opp.PvpPoint, Rank: int32(curRank),
			})
		}
		entries = append(entries, store.BattleLogEntry{
			Seq: now + int64(i), OpponentPlayerId: opp.PlayerId, OpponentName: opp.Name,
			OpponentPvpPoint: opp.PvpPoint, OpponentDeckPower: opp.MaxDeckPower,
			IsVictory: victory, BattleDatetime: now, FluctuatedPoint: delta, Rank: int32(oppRank),
		})
		// Per-match reward roll at the post-battle grade, exactly like a real
		// FinishBattle grants it straight into the inventory — but only a win
		// pays out; a loss contributes no reward items.
		if victory && cat.Pvp != nil {
			if season, ok := cat.Pvp.CurrentSeason(now); ok {
				if grade, ok := cat.Pvp.GradeForPoint(season.GradeGroupId, myPoint); ok {
					if _, items := cat.Pvp.RollOneMatchReward(grade.OneMatchRewardGroupId, pvpRoll); len(items) > 0 {
						matchItems = append(matchItems, items...)
					}
				}
			}
		}
	}

	_, err = users.UpdateUser(userId, func(u *store.UserState) {
		if u.Pvp.LastAutoProgressDay >= day {
			return // same-day guard against races
		}
		u.Pvp.PvpPoint = myPoint
		u.Pvp.AttackWinCount += wins
		u.Pvp.AttackLoseCount += losses
		u.Pvp.AttackWinStreak = streak
		u.Pvp.LastFinishDay = day
		u.Pvp.LastAutoProgressDay = day
		u.Pvp.LatestVersion = now
		u.PvpAttackLog = capLog(append(u.PvpAttackLog, entries...))

		if cat.QuestHandler != nil {
			grantPvpRewardItems(u, cat.QuestHandler.Granter, matchItems, now)
		}

		// Arena missions with the honest tallies: battles played (15), wins
		// (16), best streak reached today (17; the engine keeps the best run).
		// The rank mission (69) fires below once the snapshot carries the new
		// rating.
		ApplyMissionProgressEvent(u, cat.Mission, MissionProgressEvent{ConditionType: missionConditionArenaPlay, Delta: int32(pvpAutoDailyBattles)}, now)
		ApplyMissionProgressEvent(u, cat.Mission, MissionProgressEvent{ConditionType: missionConditionArenaWin, Delta: wins}, now)
		ApplyMissionProgressEvent(u, cat.Mission, MissionProgressEvent{ConditionType: missionConditionArenaWinStreak, CurrentValue: maxStreak}, now)
	})
	if err != nil {
		log.Printf("[PvpAutoProgress] update user %d failed: %v", userId, err)
		return
	}

	after, err := users.LoadUser(userId)
	if err != nil {
		return
	}

	// Settle every opponent's simulated day before the player's own rank is
	// re-read, so the ranking table already carries the post-battle ratings
	// the attack log was written against.
	persistAutoOpponents(users, snaps, oppState, seasonId, now)
	if err := RefreshSnapshot(snaps, &after); err != nil {
		log.Printf("[PvpAutoProgress] snapshot refresh failed: %v", err)
	}

	newRank, _ := snaps.RankOfPlayer(after.PlayerId)
	if newRank <= 0 {
		newRank = startRank
	}

	// Post-battle bookkeeping: the lazy weekly cycle keeps the legacy
	// counters moving, the rank mission (69) fires against the fresh ladder
	// position, and the player-card best rank is recorded AFTER today's
	// battles moved the rating — so an improved position shows up on the
	// card immediately.
	users.UpdateUser(userId, func(u *store.UserState) {
		advanceWeeklyCycle(cat.Pvp, u, now)
		if cat.Mission != nil {
			ApplyMissionProgressEvent(u, cat.Mission, MissionProgressEvent{ConditionType: missionConditionArenaRank, CurrentValue: int32(newRank)}, now)
		}
		recordPvpBestRank(u, newRank, seasonId, now)
	})

	log.Printf("[PvpAutoProgress] user %d: arena day played (%d battles: %d wins/%d losses, point %d -> %d, rank %d -> %d)",
		userId, pvpAutoDailyBattles, wins, losses, user.Pvp.PvpPoint, after.Pvp.PvpPoint, startRank, newRank)
}

// ladderSnapshots fetches the whole arena ladder in rating order (one query),
// so the simulation can estimate a running rating's rank mid-day without
// writing snapshot rows between battles.
func ladderSnapshots(snaps store.SnapshotRepository) ([]store.PlayerSnapshot, error) {
	count, err := snaps.CountSnapshots()
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, nil
	}
	return snaps.ListSnapshotsByPointDesc(0, count)
}

// autoOpponentTrack accumulates everything one simulated day changes for one
// opponent: the running rating the later battles see (point), the rating they
// started the day with (original), and — for real players — the defense-log
// entries and tallies a real FinishBattle would have written.
type autoOpponentTrack struct {
	original  int32
	point     int32
	defWins   int32
	defLosses int32
	logs      []store.BattleLogEntry
}

// persistAutoOpponents writes the simulated rating moves back to the ladder:
// roster bots get their snapshot row re-pointed (same name/avatar/deck, so
// they stay exactly the entry the ranking table shows), real players get the
// net delta applied to their live rating plus their defense-log entries, and
// their snapshot is refreshed so the board and both histories agree. Real
// opponents also get their player-card best rank updated, exactly like the
// simulating player does.
func persistAutoOpponents(users store.UserRepository, snaps store.SnapshotRepository, oppState map[int64]*autoOpponentTrack, seasonId int32, now int64) {
	for pid, tr := range oppState {
		delta := tr.point - tr.original
		if delta == 0 && len(tr.logs) == 0 {
			continue
		}
		if IsBotId(pid) {
			snap, err := snaps.GetSnapshot(pid)
			if err != nil {
				continue
			}
			snap.PvpPoint = tr.point
			snap.UpdatedAt = now
			if err := snaps.UpsertSnapshot(snap); err != nil {
				log.Printf("[PvpAutoProgress] bot %d rating update failed: %v", pid, err)
			}
			continue
		}
		after, err := users.UpdateUser(pid, func(u *store.UserState) {
			if delta != 0 {
				u.Pvp.PvpPoint = applyPointDelta(u.Pvp.PvpPoint, delta)
			}
			u.Pvp.DefenseWinCount += tr.defWins
			u.Pvp.DefenseLoseCount += tr.defLosses
			u.PvpDefenseLog = capLog(append(u.PvpDefenseLog, tr.logs...))
		})
		if err != nil {
			log.Printf("[PvpAutoProgress] opponent %d rating update failed: %v", pid, err)
			continue
		}
		if err := RefreshSnapshot(snaps, &after); err != nil {
			log.Printf("[PvpAutoProgress] opponent %d snapshot refresh failed: %v", pid, err)
		}
		// Their rating moved, so their ladder position may have too — keep the
		// player-card best rank in step with the refreshed snapshot.
		if rank, err := snaps.RankOfPlayer(pid); err == nil && rank > 0 {
			users.UpdateUser(pid, func(u *store.UserState) {
				recordPvpBestRank(u, rank, seasonId, now)
			})
		}
	}
}

// ladderPoolByRank picks the candidate opponents for one simulated battle
// from the living ladder: the rank window the matching list uses, centered on
// the rank the running rating currently holds, restricted to rated entries
// (bots and players actually standing on the board — rating-0 accounts have
// not opened the arena) and filtered to the ±25% deck-power band.
func ladderPoolByRank(ladder []store.PlayerSnapshot, playerId int64, rank int, myPower int32) []PlayerCard {
	order := append([]store.PlayerSnapshot(nil), ladder...)
	sort.Slice(order, func(i, j int) bool {
		if order[i].PvpPoint != order[j].PvpPoint {
			return order[i].PvpPoint > order[j].PvpPoint
		}
		return order[i].PlayerId < order[j].PlayerId // matches the ladder table's own ordering
	})
	from := rank - 5
	if from < 1 {
		from = 1
	}
	to := rank + 20
	var pool []PlayerCard
	for i := from - 1; i < to && i < len(order); i++ {
		sn := order[i]
		if sn.PlayerId == playerId || sn.PvpPoint <= 0 {
			continue
		}
		pool = append(pool, cardFromSnapshot(sn))
	}
	return opponentsWithinPowerBand(pool, myPower)
}

// opponentsWithinPowerBand keeps only opponents whose deck power lies within
// ±pvpAutoPowerBand of the player's, so a simulated day normally stays in a
// sensible matchup range; falls back to the raw pool when the band filters
// everyone out.
func opponentsWithinPowerBand(pool []PlayerCard, myPower int32) []PlayerCard {
	lo := float64(myPower) * (1 - pvpAutoPowerBand)
	hi := float64(myPower) * (1 + pvpAutoPowerBand)
	var out []PlayerCard
	for _, c := range pool {
		p := float64(c.MaxDeckPower)
		if p >= lo && p <= hi {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return pool
	}
	return out
}

// arenaWinChance is the power curve for one simulated match: a certain win
// against anyone at least pvpAutoPowerBand weaker, a certain loss against
// anyone at least pvpAutoPowerBand stronger, and inside the band the chance
// falls linearly from 100% at the weak edge to 0% at the strong edge (50% at
// equal power).
func arenaWinChance(myPower, oppPower int32) float64 {
	if oppPower <= 0 || myPower <= 0 {
		return 1
	}
	ratio := float64(oppPower) / float64(myPower)
	if ratio >= 1+pvpAutoPowerBand {
		return 0
	}
	if ratio <= 1-pvpAutoPowerBand {
		return 1
	}
	return (1 + pvpAutoPowerBand - ratio) / (2 * pvpAutoPowerBand)
}

// simulateArenaOutcome resolves one simulated match through arenaWinChance.
func simulateArenaOutcome(r *rand.Rand, myPower, oppPower int32) bool {
	return r.Float64() < arenaWinChance(myPower, oppPower)
}

// pickWeightedOpponent draws one opponent with probability proportional to
// the win chance of the matchup, so winnable fights happen more often when
// they are available. Certain-loss opponents keep a small minimum weight so a
// day is never stuck when they are the only option.
func pickWeightedOpponent(r *rand.Rand, picks []PlayerCard, myPower int32) PlayerCard {
	const minWeight = 0.05
	weights := make([]float64, len(picks))
	var total float64
	for i, c := range picks {
		w := arenaWinChance(myPower, c.MaxDeckPower)
		if w < minWeight {
			w = minWeight
		}
		weights[i] = w
		total += w
	}
	roll := r.Float64() * total
	for i, w := range weights {
		roll -= w
		if roll < 0 {
			return picks[i]
		}
	}
	return picks[len(picks)-1]
}

// maybePromoteQuestDeckToPvp compares one freshly reported quest deck against
// the current arena slot #1 and copies it over when it is stronger. Only the
// deck that just reported is trusted: other quest decks may carry stale
// powers (the client reports power only when a deck is used), so they are
// never considered — a stale high number must not keep the arena lineup in
// a state the player no longer has, and a weaker report never downgrades
// the arena deck.
func maybePromoteQuestDeckToPvp(user *store.UserState, deckNumber int32, now int64) {
	questKey := store.DeckKey{DeckType: model.DeckTypeQuest, UserDeckNumber: deckNumber}
	deck, ok := user.Decks[questKey]
	if !ok {
		return
	}
	questPower := trustedDeckPower(user, deck)
	if questPower <= 0 {
		return
	}
	if pvp, ok := user.Decks[store.DeckKey{DeckType: model.DeckTypePvp, UserDeckNumber: 1}]; ok && trustedDeckPower(user, pvp) >= questPower {
		return
	}
	rebuildPvpSlotFromQuest(user, deckNumber, questPower, now)
}

// bootstrapArenaDeck seeds arena slot #1 for players who have no arena deck
// yet (or an empty one): the strongest quest deck by server-known power. A
// one-time choice — once slot #1 exists, only fresh reports may change it.
func bootstrapArenaDeck(user *store.UserState, now int64) {
	pvpKey := store.DeckKey{DeckType: model.DeckTypePvp, UserDeckNumber: 1}
	if deck, ok := user.Decks[pvpKey]; ok && trustedDeckPower(user, deck) > 0 {
		// Adopt the existing arena deck as the managed lineup so future days
		// can detect and revert manual edits (covers players whose deck
		// predates the backup column).
		if slots := store.ReadDeckSlots(user, model.DeckTypePvp, 1); hasOccupiedSlot(slots) {
			encodePvpBackup(user, slots, trustedDeckPower(user, deck), deck.Name)
		}
		return
	}
	var bestNumber, bestPower int32
	for key, deck := range user.Decks {
		if key.DeckType != model.DeckTypeQuest {
			continue
		}
		if power := trustedDeckPower(user, deck); power > bestPower {
			bestNumber, bestPower = key.UserDeckNumber, power
		}
	}
	if bestNumber == 0 {
		return
	}
	rebuildPvpSlotFromQuest(user, bestNumber, bestPower, now)
}

// ensureArenaDeck guards the server-managed arena deck before a simulated
// day. Any difference from the backup — an edited lineup, an empty slot #1
// (deck deleted) or the defense selection moved off slot #1 — restores the
// lineup from the backup (never from the quest deck, which may itself have
// changed since the copy). The rating is never touched here: drift
// detection has false positives, and restoring a lineup is a repair, not a
// punishment. Without a backup yet, fall back to bootstrap.
func ensureArenaDeck(user *store.UserState, now int64) {
	backup, ok := decodePvpBackup(user)
	if !ok {
		bootstrapArenaDeck(user, now)
		return
	}
	slots := store.ReadDeckSlots(user, model.DeckTypePvp, 1)
	if hasOccupiedSlot(slots) && slotsIdentical(slots, backup.Slots) && user.Pvp.DefenseDeckNumber == 1 {
		return
	}
	log.Printf("[PvpAutoProgress] user %d: arena deck drifted from backup, restoring (power %d); rating stays at %d",
		user.PlayerId, backup.Power, user.Pvp.PvpPoint)
	rebuildPvpSlot(user, backup.Slots, backup.Power, backup.Name, now)
}

// pvpDeckBackup is the JSON payload stored in PvpState.PvpDeckBackup: the
// exact lineup the server last wrote into arena slot #1.
type pvpDeckBackup struct {
	Name  string
	Power int32
	Slots []store.DeckCharacterInput
}

func encodePvpBackup(user *store.UserState, slots []store.DeckCharacterInput, power int32, name string) {
	data, err := json.Marshal(pvpDeckBackup{Name: name, Power: power, Slots: slots})
	if err != nil {
		return
	}
	user.Pvp.PvpDeckBackup = string(data)
}

func decodePvpBackup(user *store.UserState) (pvpDeckBackup, bool) {
	var b pvpDeckBackup
	if user.Pvp.PvpDeckBackup == "" {
		return b, false
	}
	if err := json.Unmarshal([]byte(user.Pvp.PvpDeckBackup), &b); err != nil || !hasOccupiedSlot(b.Slots) {
		return b, false
	}
	return b, true
}

func hasOccupiedSlot(slots []store.DeckCharacterInput) bool {
	for _, s := range slots {
		if s.UserCostumeUuid != "" {
			return true
		}
	}
	return false
}

// trustedDeckPower is the power the server may rely on for a deck: the
// character sum when every occupied slot has a reported power, otherwise the
// stored deck total.
func trustedDeckPower(user *store.UserState, deck store.DeckState) int32 {
	var sum int32
	occupied, known := 0, 0
	for _, uuid := range []string{deck.UserDeckCharacterUuid01, deck.UserDeckCharacterUuid02, deck.UserDeckCharacterUuid03} {
		if uuid == "" {
			continue
		}
		occupied++
		if dc, ok := user.DeckCharacters[uuid]; ok && dc.Power > 0 {
			known++
			sum += dc.Power
		}
	}
	if occupied == 0 {
		return 0
	}
	if known == occupied && sum > 0 {
		return sum
	}
	return deck.Power
}

// rebuildPvpSlotFromQuest rebuilds arena slot #1 as a fresh copy of quest
// deck deckNumber (with the given power). The copy mints fresh deck-character
// rows (same costumes/weapons), exactly like the client's own CopyDeck does.
func rebuildPvpSlotFromQuest(user *store.UserState, deckNumber int32, power int32, now int64) {
	slots := store.ReadDeckSlots(user, model.DeckTypeQuest, deckNumber)
	if slots == nil {
		return
	}
	name := user.Decks[store.DeckKey{DeckType: model.DeckTypeQuest, UserDeckNumber: deckNumber}].Name
	rebuildPvpSlot(user, slots, power, name, now)
	log.Printf("[PvpAutoProgress] user %d: mirrored quest deck #%d (power %d) into arena slot #1",
		user.PlayerId, deckNumber, power)
}

// rebuildPvpSlot rewrites arena slot #1 from the given lineup, removes every
// other PvP deck, and exposes slot #1 as the defense deck. The lineup also
// becomes the new PvpDeckBackup — the reference state that future auto days
// compare against to detect manual edits.
func rebuildPvpSlot(user *store.UserState, slots []store.DeckCharacterInput, power int32, name string, now int64) {
	// Recorded even when the rebuild below turns out to be a no-op, so the
	// backup always exists once the server has settled on a lineup.
	encodePvpBackup(user, slots, power, name)

	pvpKey := store.DeckKey{DeckType: model.DeckTypePvp, UserDeckNumber: 1}
	if deck, ok := user.Decks[pvpKey]; ok && deck.Power == power && user.Pvp.DefenseDeckNumber == 1 {
		if slotsIdentical(store.ReadDeckSlots(user, model.DeckTypePvp, 1), slots) {
			return
		}
	}

	// Wipe every existing arena deck (keys collected first: RemoveDeckData
	// mutates the map while deleting).
	var pvpNumbers []int32
	for key := range user.Decks {
		if key.DeckType == model.DeckTypePvp {
			pvpNumbers = append(pvpNumbers, key.UserDeckNumber)
		}
	}
	for _, num := range pvpNumbers {
		store.RemoveDeckData(user, model.DeckTypePvp, num)
	}

	store.ApplyDeckReplacement(user, model.DeckTypePvp, 1, slots, now)
	deck := user.Decks[pvpKey]
	deck.Name = name
	deck.Power = power
	deck.LatestVersion = now
	user.Decks[pvpKey] = deck
	user.Pvp.DefenseDeckNumber = 1
	if note := user.DeckTypeNotes[model.DeckTypePvp]; power > note.MaxDeckPower {
		note.DeckType = model.DeckTypePvp
		note.MaxDeckPower = power
		user.DeckTypeNotes[model.DeckTypePvp] = note
	}
}

// slotsIdentical reports whether two deck slot lists carry the same loadout
// in every position (same costumes, weapons, companions, thoughts, parts).
func slotsIdentical(a, b []store.DeckCharacterInput) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].UserCostumeUuid != b[i].UserCostumeUuid ||
			a[i].MainUserWeaponUuid != b[i].MainUserWeaponUuid ||
			a[i].UserCompanionUuid != b[i].UserCompanionUuid ||
			a[i].UserThoughtUuid != b[i].UserThoughtUuid ||
			a[i].DressupCostumeId != b[i].DressupCostumeId ||
			!slotUuidsIdentical(a[i].SubWeaponUuids, b[i].SubWeaponUuids) ||
			!slotUuidsIdentical(a[i].PartsUuids, b[i].PartsUuids) {
			return false
		}
	}
	return true
}

// slotUuidsIdentical compares two UUID lists for exact equality (including order).
// This is different from sameUuids in snapshot_refresh.go which ignores order.
func slotUuidsIdentical(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// mailPvpDailyRewardItems posts the three arena reward tabs to the gift box:
// grade weekly by rating, weekly ranking by rank, and the season ranking
// reward. The figures passed in are the rating and rank the player held at
// the end of the previous arena day — the payout is for yesterday, mailed
// before today's battles change anything.
func mailPvpDailyRewardItems(users store.UserRepository, holder *runtime.Holder, userId int64, currentPoints int32, rank int, now int64) {
	cat := holder.Get()

	log.Printf("[PvpAutoProgress] user %d: granting daily rewards for yesterday's standing (points: %d, rank: %d)", userId, currentPoints, rank)

	var dailyItems []masterdata.RewardItem
	if cat.Pvp != nil {
		if season, ok := cat.Pvp.CurrentSeason(now); ok {
			if grade, ok := cat.Pvp.GradeForPoint(season.GradeGroupId, currentPoints); ok {
				dailyItems = append(dailyItems, cat.Pvp.WeeklyRewards(grade.WeeklyRewardGroupId)...)
			}
			dailyItems = append(dailyItems, cat.Pvp.WeeklyRankRewardsForRank(season.WeeklyRankRewardRankGroupId, rank)...)
			dailyItems = append(dailyItems, cat.Pvp.SeasonRankRewardsForRank(season.SeasonRankRewardRankGroupId, rank)...)
		}
	}
	users.UpdateUser(userId, func(u *store.UserState) {
		mailPvpRewardItems(u, dailyItems, now)
	})
}
