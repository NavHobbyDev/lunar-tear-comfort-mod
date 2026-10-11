package service

import (
	"testing"

	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
)

func TestBuildPvpDeckCharacters_minimalDeck(t *testing.T) {
	u := &store.UserState{}
	u.EnsureMaps()
	u.Costumes["c1"] = store.CostumeState{UserCostumeUuid: "c1", CostumeId: 1001, Level: 50}
	u.Weapons["w1"] = store.WeaponState{UserWeaponUuid: "w1", WeaponId: 2001, Level: 40}
	u.DeckCharacters["dc1"] = store.DeckCharacterState{
		UserDeckCharacterUuid: "dc1", UserCostumeUuid: "c1", MainUserWeaponUuid: "w1",
	}
	u.Decks[store.DeckKey{DeckType: model.DeckTypePvp, UserDeckNumber: 1}] = store.DeckState{
		DeckType: model.DeckTypePvp, UserDeckNumber: 1, UserDeckCharacterUuid01: "dc1",
	}
	got := BuildPvpDeckCharacters(u, model.DeckTypePvp, 1)
	if len(got) != 1 {
		t.Fatalf("want 1 character, got %d", len(got))
	}
	if got[0].Costume == nil || got[0].Costume.CostumeId != 1001 {
		t.Fatalf("costume not projected: %+v", got[0].Costume)
	}
	if got[0].MainWeapon == nil || got[0].MainWeapon.WeaponId != 2001 {
		t.Fatalf("main weapon not projected: %+v", got[0].MainWeapon)
	}
}

func TestPickDefenseDeck_pvpSetUsesIt(t *testing.T) {
	u := &store.UserState{}
	u.EnsureMaps()
	u.Decks[store.DeckKey{DeckType: model.DeckTypePvp, UserDeckNumber: 1}] = store.DeckState{
		DeckType:                model.DeckTypePvp,
		UserDeckNumber:          1,
		UserDeckCharacterUuid01: "dc1",
	}
	dt, num := PickDefenseDeck(u)
	if dt != model.DeckTypePvp || num != 1 {
		t.Fatalf("expected pvp deck 1, got type=%d num=%d", dt, num)
	}
}

func TestPickDefenseDeck_fallsBackToQuest(t *testing.T) {
	u := &store.UserState{}
	u.EnsureMaps()
	// No PvP deck set.
	dt, num := PickDefenseDeck(u)
	if dt != model.DeckTypeQuest || num != 1 {
		t.Fatalf("expected quest deck 1 fallback, got type=%d num=%d", dt, num)
	}
}

func TestBuildPvpDeckCharacters_emptyDeckKey(t *testing.T) {
	u := &store.UserState{}
	u.EnsureMaps()
	got := BuildPvpDeckCharacters(u, model.DeckTypePvp, 1)
	if got != nil {
		t.Fatalf("expected nil for missing deck, got %v", got)
	}
}

func TestBuildPvpDeckCharacters_subWeaponsAndParts(t *testing.T) {
	u := &store.UserState{}
	u.EnsureMaps()
	u.Costumes["c1"] = store.CostumeState{UserCostumeUuid: "c1", CostumeId: 1001, Level: 40}
	u.Weapons["w1"] = store.WeaponState{UserWeaponUuid: "w1", WeaponId: 2001, Level: 30}
	u.Weapons["sw1"] = store.WeaponState{UserWeaponUuid: "sw1", WeaponId: 3001, Level: 20}
	u.Parts["p1"] = store.PartsState{UserPartsUuid: "p1", PartsId: 4001, Level: 10, PartsStatusMainId: 5001}
	u.DeckCharacters["dc1"] = store.DeckCharacterState{
		UserDeckCharacterUuid: "dc1", UserCostumeUuid: "c1", MainUserWeaponUuid: "w1",
	}
	u.DeckSubWeapons["dc1"] = []string{"sw1"}
	u.DeckParts["dc1"] = []string{"p1"}
	u.Decks[store.DeckKey{DeckType: model.DeckTypePvp, UserDeckNumber: 1}] = store.DeckState{
		DeckType: model.DeckTypePvp, UserDeckNumber: 1, UserDeckCharacterUuid01: "dc1",
	}
	got := BuildPvpDeckCharacters(u, model.DeckTypePvp, 1)
	if len(got) != 1 {
		t.Fatalf("want 1 character, got %d", len(got))
	}
	ch := got[0]
	if len(ch.SubWeapon) != 1 || ch.SubWeapon[0].WeaponId != 3001 {
		t.Fatalf("sub-weapon not projected: %+v", ch.SubWeapon)
	}
	if len(ch.Parts) != 1 || ch.Parts[0].PartsId != 4001 {
		t.Fatalf("parts not projected: %+v", ch.Parts)
	}
	if ch.Parts[0].PartsMainStatusId != 5001 {
		t.Fatalf("parts main status not projected: got %d", ch.Parts[0].PartsMainStatusId)
	}
}
