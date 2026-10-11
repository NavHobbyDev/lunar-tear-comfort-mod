package service

import (
	"encoding/json"
	"log"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/store"
)

// botRosterSize is how many persistent filler bots populate the arena ladder so
// the ranking board is never empty and every player (and every matching bot)
// resolves to a real, non-zero rank. Real players are ranked among these bots
// by their PvP points, so a brand-new account starts at the bottom of a
// populated board instead of standing alone at rank 1. The roster is large
// enough that the simulated daily climb (starting at rank 2000, gaining 100
// ranks per day) always resolves against real ladder entries.
const botRosterSize = 2200

// botRosterTopPoint / botRosterBottomPoint bound the synthetic ladder. The
// spread roughly tracks the grade thresholds (0..46000) so bots occupy the
// whole climb and matching always finds opponents near the viewer's rating.
const (
	botRosterTopPoint    = 45000
	botRosterBottomPoint = 300
)

// Bot deck-power bounds: power scales with the ladder rating so stronger
// bots sit higher, and the strongest bots cap near 500 000 so fully-built
// endgame decks can still contest the top of the board.
const (
	botMinDeckPower = 3000
	botMaxDeckPower = 500000
)

// powerForPoint maps a ladder rating linearly onto the deck-power range
// [botMinDeckPower, botMaxDeckPower].
func powerForPoint(point int32) int32 {
	span := int64(botMaxDeckPower - botMinDeckPower)
	p := int64(botMinDeckPower) + int64(point)*span/int64(botRosterTopPoint)
	if p > int64(botMaxDeckPower) {
		p = int64(botMaxDeckPower)
	}
	return int32(p)
}

// rosterSeedPoint is the seeded rating of ladder slot i: a linear spread
// from botRosterTopPoint (slot 0) down to botRosterBottomPoint (last slot),
// computed in int64 so the floor is actually reached (a truncated int step
// used to strand the bottom bot ~700 points above the intended floor).
func rosterSeedPoint(i int) int32 {
	if botRosterSize <= 1 {
		return botRosterTopPoint
	}
	span := int64(botRosterTopPoint - botRosterBottomPoint)
	return int32(int64(botRosterTopPoint) - span*int64(i)/int64(botRosterSize-1))
}

// rosterPointForId reconstructs the seeded ladder rating of a roster bot
// from its fixed id (BotIdBase + index + 1), for the rare paths that must
// synthesize a roster bot without a snapshot row.
func rosterPointForId(playerId int64) (int32, bool) {
	idx := playerId - BotIdBase - 1
	if idx < 0 || idx >= int64(botRosterSize) {
		return 0, false
	}
	return rosterSeedPoint(int(idx)), true
}

// SeedBotRoster writes a fixed, deterministic set of filler bots into the
// snapshot table. Idempotent: bot ids are stable (BotIdBase+i+1), so re-running
// on every boot refreshes the same rows without accumulating duplicates and
// without ever touching real players (whose ids are always below BotIdBase).
// A bot that already has a row keeps the rating it has since fought for:
// auto battles move bot ratings, and rewinding them to the seed on restart
// would desync the battle history from the ranking table.
func SeedBotRoster(dir *PlayerDirectory, snaps store.SnapshotRepository) {
	pools := dir.pools()
	now := gametime.NowMillis()

	written := 0
	kept := 0
	for i := 0; i < botRosterSize; i++ {
		playerId := BotIdBase + int64(i) + 1
		point := rosterSeedPoint(i)
		if existing, err := snaps.GetSnapshot(playerId); err == nil {
			point = existing.PvpPoint // preserve battle-won rating moves
			kept++
		}
		// The deck is generated first and the card borrows its lead costume
		// as the avatar: the ranking board and the battle history must show
		// the very same bot, not two strangers sharing a name.
		deck := synthBotDeck(pools, playerId)
		card := rosterBotWithDeck(pools, playerId, point, deck)

		// Store the deck itself so every code path that reads
		// defense_deck_json sees exactly the lineup the bot fields.
		deckJSON, err := json.Marshal(deck)
		if err != nil {
			deckJSON = []byte("[]")
		}

		snap := store.PlayerSnapshot{
			PlayerId:          playerId,
			UserName:          card.Name,
			Level:             card.Level,
			MaxDeckPower:      card.MaxDeckPower,
			FavoriteCostumeId: card.FavoriteCostumeId,
			PvpPoint:          card.PvpPoint,
			LastLoginDatetime: now,
			DefenseDeckJson:   string(deckJSON),
			UpdatedAt:         now,
		}
		if err := snaps.UpsertSnapshot(snap); err != nil {
			log.Printf("[snapshot] bot roster: seed bot %d failed: %v", playerId, err)
			continue
		}
		written++
	}
	log.Printf("[snapshot] bot roster seeded: %d/%d filler bots (%d kept their battle-won rating)",
		written, botRosterSize, kept)
}

// rosterBot builds a deterministic ladder bot with a fixed id and rating. Deck
// power tracks the rating so stronger bots sit higher, matching the leaderboard.
func rosterBot(pools botPools, playerId int64, point int32) PlayerCard {
	return rosterBotWithDeck(pools, playerId, point, synthBotDeck(pools, playerId))
}

// rosterBotWithDeck builds the same deterministic bot, taking the avatar from
// the deck itself: the favorite costume is the lead character's costume, so
// the ranking board's avatar and the battle history's deck icons always
// depict the same bot.
func rosterBotWithDeck(pools botPools, playerId int64, point int32, deck []*pb.PvpDeckCharacter) PlayerCard {
	r := newRand(playerId)
	level := int32(50 + r.Intn(30))
	lead := deckLeadCostume(deck)
	if lead == 0 {
		lead = pick(r, pools.costumeIds, 1)
	}
	return PlayerCard{
		PlayerId:          playerId,
		Name:              botName(playerId),
		Level:             level,
		MaxDeckPower:      powerForPoint(point),
		FavoriteCostumeId: lead,
		PvpPoint:          point,
		IsBot:             true,
	}
}

// deckLeadCostume is the costume of a deck's first character — the face its
// owner shows to the world.
func deckLeadCostume(deck []*pb.PvpDeckCharacter) int32 {
	if len(deck) > 0 && deck[0] != nil && deck[0].Costume != nil {
		return deck[0].Costume.CostumeId
	}
	return 0
}
