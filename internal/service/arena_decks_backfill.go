package service

import (
	"log"

	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
)

// SeedArenaDecks makes sure every account owns at least one arena (Pvp-type)
// deck. Arena decks are a separate deck set from quest and BigHunt decks, but
// the client's arena battle deck-picker and defense-setting screens iterate the
// player's Pvp decks and hang locally when that set is empty — which it always
// is for accounts created before arena decks existed. To bootstrap a usable,
// independent arena set we clone each quest deck into the same-numbered Pvp
// slot (fresh deck-character uuids, so later arena edits never touch quest).
//
// Idempotent: accounts that already own any Pvp deck are left untouched, so we
// never clobber a player's customized arena teams on later boots.
func SeedArenaDecks(users store.UserRepository, snaps store.SnapshotRepository) {
	ids, err := snaps.AllUserIds()
	if err != nil {
		log.Printf("[arena] deck seed: list users failed: %v", err)
		return
	}
	seeded := 0
	for _, id := range ids {
		user, err := users.LoadUser(id)
		if err != nil {
			continue
		}
		if user.Profile.Name == "" {
			continue
		}
		if !needsArenaDeckSeed(&user) {
			continue
		}
		after, err := users.UpdateUser(id, func(u *store.UserState) {
			if !needsArenaDeckSeed(u) {
				return
			}
			cloneQuestDecksToArena(u, gametime.NowMillis())
		})
		if err != nil {
			log.Printf("[arena] deck seed: user %d failed: %v", id, err)
			continue
		}
		if snaps != nil {
			if err := RefreshSnapshot(snaps, &after); err != nil {
				log.Printf("[arena] deck seed: user %d snapshot refresh failed: %v", id, err)
			}
		}
		seeded++
	}
	log.Printf("[arena] deck seed complete: seeded arena decks for %d accounts", seeded)
}

// needsArenaDeckSeed reports whether the user owns no arena deck yet but does
// have at least one quest deck to clone from.
func needsArenaDeckSeed(u *store.UserState) bool {
	hasQuest := false
	for key := range u.Decks {
		switch key.DeckType {
		case model.DeckTypePvp:
			return false
		case model.DeckTypeQuest:
			hasQuest = true
		}
	}
	return hasQuest
}

// cloneQuestDecksToArena copies every quest deck into the same-numbered arena
// slot, preserving names/power but minting independent deck-character records.
func cloneQuestDecksToArena(u *store.UserState, nowMillis int64) {
	type src struct {
		number int32
		name   string
		power  int32
	}
	var sources []src
	for key, deck := range u.Decks {
		if key.DeckType != model.DeckTypeQuest {
			continue
		}
		sources = append(sources, src{number: key.UserDeckNumber, name: deck.Name, power: deck.Power})
	}
	for _, s := range sources {
		slots := store.ReadDeckSlots(u, model.DeckTypeQuest, s.number)
		if slots == nil {
			continue
		}
		store.ApplyDeckReplacement(u, model.DeckTypePvp, s.number, slots, nowMillis)

		key := store.DeckKey{DeckType: model.DeckTypePvp, UserDeckNumber: s.number}
		deck := u.Decks[key]
		if s.name != "" {
			deck.Name = s.name
		}
		if s.power > 0 {
			deck.Power = s.power
		}
		u.Decks[key] = deck

		note := u.DeckTypeNotes[model.DeckTypePvp]
		if deck.Power > note.MaxDeckPower {
			note.DeckType = model.DeckTypePvp
			note.MaxDeckPower = deck.Power
			u.DeckTypeNotes[model.DeckTypePvp] = note
		}
	}
}
