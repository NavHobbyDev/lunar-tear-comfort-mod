package service

import (
	"encoding/json"
	"sort"

	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
)

// RefreshSnapshot writes the user's public face to the snapshot table.
// Best-effort: returns an error for logging but must not fail the caller's RPC.
// Accounts without a display name (onboarding not completed) are skipped so they
// don't appear as blank rows in other players' friend/arena lists. Accounts
// that never opened the arena (no PvP tutorial progress) have no business on
// the arena ladder: their row is removed instead, so 0-point placeholders
// never push rated players around in the ranking.
func RefreshSnapshot(snaps store.SnapshotRepository, user *store.UserState) error {
	if user.Profile.Name == "" {
		return nil
	}
	if _, ok := user.Tutorials[int32(model.TutorialTypePvp)]; !ok {
		return snaps.DeleteSnapshot(user.PlayerId)
	}
	dt, dn := PickDefenseDeck(user)
	deck := BuildPvpDeckCharacters(user, dt, dn)
	deckJSON, err := json.Marshal(deck)
	if err != nil {
		deckJSON = []byte("[]")
	}
	snap := store.PlayerSnapshot{
		PlayerId:          user.PlayerId,
		UserName:          user.Profile.Name,
		Level:             playerLevel(user),
		MaxDeckPower:      defenseDeckPower(user, dt, dn),
		FavoriteCostumeId: favoriteCostumeId(user),
		PvpPoint:          user.Pvp.PvpPoint,
		LastLoginDatetime: gametime.NowMillis(),
		DefenseDeckJson:   string(deckJSON),
		UpdatedAt:         gametime.NowMillis(),
	}
	return snaps.UpsertSnapshot(snap)
}

func playerLevel(user *store.UserState) int32 {
	return user.Status.Level
}

// defenseDeckPower is the strength the player actually fields in arena
// battles: the power of the defense deck PickDefenseDeck chose, NOT the
// strongest deck of any type (a powerful quest deck must not inflate the
// arena rating shown in the ranking or the matchmaking band). The primary
// source is the sum of the deck's per-character powers — those match the
// client's own math exactly, while the stored deck total can be a stale echo
// of an earlier save (the client reports back whatever the server stored).
// The stored total is used only while some occupied slot still has no known
// character power; the all-type max is a last resort for brand-new players.
// The per-type historical max (DeckTypeNotes) is deliberately NOT used: it
// survives lineup changes and deck deletions, so it surfaces numbers that
// match no current deck.
func defenseDeckPower(user *store.UserState, dt model.DeckType, dn int32) int32 {
	key := store.DeckKey{DeckType: dt, UserDeckNumber: dn}
	deck, ok := user.Decks[key]
	if ok {
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
				continue
			}
			// The client reports a deck's power only on some flows (fresh
			// creation), so an edited or reduced deck can stay unreported
			// indefinitely. Borrow the power the client already computed for
			// the same costume in another of the player's decks.
			if est := inferCharacterPower(user, key, user.DeckCharacters[uuid]); est > 0 {
				known++
				sum += est
			}
		}
		if occupied > 0 && known == occupied {
			return sum
		}
		if deck.Power > 0 {
			return deck.Power
		}
		if sum > 0 {
			return sum
		}
	}
	if p := maxDeckPower(user); p > 0 {
		return p
	}
	// Never expose 0: the ranking/matching UI treats it as a broken entry.
	return 100
}

func maxDeckPower(user *store.UserState) int32 {
	var max int32
	for _, note := range user.DeckTypeNotes {
		if note.MaxDeckPower > max {
			max = note.MaxDeckPower
		}
	}
	return max
}

// strongestDeckPower is the player's honest strength for arena progression:
// the best deck across every type, measured from client-reported character
// powers (falling back to the reported deck total). The client only reports
// a deck's power in some flows, and the exposed defense deck is often built
// before any report arrives — so progression must not trust the defense deck
// alone. A deck full of unreported characters contributes nothing rather
// than a misleading placeholder. DeckTypeNotes is skipped on purpose: it is
// a historical max that survives lineup changes.
func strongestDeckPower(user *store.UserState) int32 {
	var best int32
	for _, deck := range user.Decks {
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
		if occupied > 0 && known == occupied && sum > best {
			best = sum
		} else if deck.Power > best {
			best = deck.Power
		}
	}
	return best
}

// inferCharacterPower estimates the power of a deck character the client has
// not reported yet, borrowing the client-reported power of the same costume
// from the player's other decks. Preference order: identical loadout (exact
// same power), same costume + main weapon, then any same costume. Among
// candidates of the same class the freshest report wins. Returns 0 when the
// costume is unknown everywhere, leaving the caller's fallbacks in charge.
func inferCharacterPower(user *store.UserState, selfKey store.DeckKey, dc store.DeckCharacterState) int32 {
	if dc.UserCostumeUuid == "" {
		return 0
	}
	var exact, weapon, costume store.DeckCharacterState
	exactV, weaponV, costumeV := int64(-1), int64(-1), int64(-1)
	for key, deck := range user.Decks {
		if key == selfKey {
			continue
		}
		for _, uuid := range []string{deck.UserDeckCharacterUuid01, deck.UserDeckCharacterUuid02, deck.UserDeckCharacterUuid03} {
			if uuid == "" {
				continue
			}
			other, ok := user.DeckCharacters[uuid]
			if !ok || other.Power <= 0 || other.UserCostumeUuid != dc.UserCostumeUuid {
				continue
			}
			switch {
			case loadoutIdentical(user, dc, uuid, other):
				if other.LatestVersion >= exactV {
					exact, exactV = other, other.LatestVersion
				}
			case other.MainUserWeaponUuid == dc.MainUserWeaponUuid:
				if other.LatestVersion >= weaponV {
					weapon, weaponV = other, other.LatestVersion
				}
			default:
				if other.LatestVersion >= costumeV {
					costume, costumeV = other, other.LatestVersion
				}
			}
		}
	}
	switch {
	case exactV >= 0:
		return exact.Power
	case weaponV >= 0:
		return weapon.Power
	case costumeV >= 0:
		return costume.Power
	}
	return 0
}

// loadoutIdentical reports whether two deck characters carry exactly the same
// equipment, in which case the client computes the same power for both.
func loadoutIdentical(user *store.UserState, dc store.DeckCharacterState, otherUuid string, other store.DeckCharacterState) bool {
	if dc.MainUserWeaponUuid != other.MainUserWeaponUuid ||
		dc.UserCompanionUuid != other.UserCompanionUuid ||
		dc.UserThoughtUuid != other.UserThoughtUuid ||
		dc.DressupCostumeId != other.DressupCostumeId {
		return false
	}
	return sameUuids(user.DeckSubWeapons[dc.UserDeckCharacterUuid], user.DeckSubWeapons[otherUuid]) &&
		sameUuids(user.DeckParts[dc.UserDeckCharacterUuid], user.DeckParts[otherUuid])
}

func sameUuids(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]string(nil), a...)
	y := append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

func favoriteCostumeId(user *store.UserState) int32 {
	return user.Profile.FavoriteCostumeId
}
