package service

import (
	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
	"sort"
)

// PickDefenseDeck returns the deck a player exposes to opponents. Arena decks
// are their own set (DeckTypePvp): it honors the slot chosen via
// SetPvpDefenseDeck, falls back to PvP deck #1 if that slot is empty, and only
// then borrows the quest deck #1 so brand-new players still field a valid team.
func PickDefenseDeck(user *store.UserState) (model.DeckType, int32) {
	num := user.Pvp.DefenseDeckNumber
	if num > 0 {
		key := store.DeckKey{DeckType: model.DeckTypePvp, UserDeckNumber: num}
		if d, ok := user.Decks[key]; ok && d.UserDeckCharacterUuid01 != "" {
			return model.DeckTypePvp, num
		}
	}
	pvpKey := store.DeckKey{DeckType: model.DeckTypePvp, UserDeckNumber: 1}
	if d, ok := user.Decks[pvpKey]; ok && d.UserDeckCharacterUuid01 != "" {
		return model.DeckTypePvp, 1
	}
	return model.DeckTypeQuest, 1
}

// BuildPvpDeckCharacters projects a stored deck into the rich battle representation the
// client needs for StartBattle. Empty slots are skipped.
func BuildPvpDeckCharacters(user *store.UserState, deckType model.DeckType, deckNumber int32) []*pb.PvpDeckCharacter {
	key := store.DeckKey{DeckType: deckType, UserDeckNumber: deckNumber}
	deck, ok := user.Decks[key]
	if !ok {
		return nil
	}
	uuids := []string{deck.UserDeckCharacterUuid01, deck.UserDeckCharacterUuid02, deck.UserDeckCharacterUuid03}
	var out []*pb.PvpDeckCharacter
	for _, dcUuid := range uuids {
		if dcUuid == "" {
			continue
		}
		dc, ok := user.DeckCharacters[dcUuid]
		if !ok {
			continue
		}
		out = append(out, buildOnePvpCharacter(user, dcUuid, dc))
	}
	return out
}

func buildOnePvpCharacter(user *store.UserState, dcUuid string, dc store.DeckCharacterState) *pb.PvpDeckCharacter {
	ch := &pb.PvpDeckCharacter{}

	if c, ok := user.Costumes[dc.UserCostumeUuid]; ok {
		ci := &pb.CostumeInfo{
			CostumeId:                             c.CostumeId,
			LimitBreakCount:                       c.LimitBreakCount,
			Level:                                 c.Level,
			CharacterLevel:                        c.Level,
			CostumeLotteryEffectUnlockedSlotCount: c.CostumeLotteryEffectUnlockedSlotCount,
		}
		if skill, ok := user.CostumeActiveSkills[dc.UserCostumeUuid]; ok {
			ci.ActiveSkillLevel = skill.Level
		}
		ch.Costume = ci
		appendCostumeLotteryEffects(ch, user, dc.UserCostumeUuid)
	}

	if comp, ok := user.Companions[dc.UserCompanionUuid]; ok {
		ch.Companion = &pb.CompanionInfo{CompanionId: comp.CompanionId, Level: comp.Level}
	}

	if w, ok := user.Weapons[dc.MainUserWeaponUuid]; ok {
		ch.MainWeapon = buildWeaponInfo(user, w)
	}

	if t, ok := user.Thoughts[dc.UserThoughtUuid]; ok {
		ch.Thought = &pb.ThoughtInfo{ThoughtId: t.ThoughtId}
	}

	for _, wu := range user.DeckSubWeapons[dcUuid] {
		if w, ok := user.Weapons[wu]; ok {
			ch.SubWeapon = append(ch.SubWeapon, buildWeaponInfo(user, w))
		}
	}

	for _, pu := range user.DeckParts[dcUuid] {
		if p, ok := user.Parts[pu]; ok {
			ch.Parts = append(ch.Parts, &pb.PartsInfo{
				PartsId:           p.PartsId,
				Level:             p.Level,
				PartsMainStatusId: p.PartsStatusMainId,
			})
		}
	}

	return ch
}

func appendCostumeLotteryEffects(ch *pb.PvpDeckCharacter, user *store.UserState, costumeUuid string) {
	abilityKeys := make([]store.CostumeLotteryEffectKey, 0)
	for k := range user.CostumeLotteryEffectAbilities {
		if k.UserCostumeUuid == costumeUuid {
			abilityKeys = append(abilityKeys, k)
		}
	}
	sort.Slice(abilityKeys, func(i, j int) bool { return abilityKeys[i].SlotNumber < abilityKeys[j].SlotNumber })
	for _, k := range abilityKeys {
		a := user.CostumeLotteryEffectAbilities[k]
		ch.CostumeLotteryEffectAbilities = append(ch.CostumeLotteryEffectAbilities, &pb.CostumeLotteryEffectAbilityInfo{AbilityId: a.AbilityId, Level: a.AbilityLevel})
	}
	statusKeys := make([]store.CostumeLotteryEffectStatusUpKey, 0)
	for k := range user.CostumeLotteryEffectStatusUps {
		if k.UserCostumeUuid == costumeUuid {
			statusKeys = append(statusKeys, k)
		}
	}
	sort.Slice(statusKeys, func(i, j int) bool { return statusKeys[i].StatusCalculationType < statusKeys[j].StatusCalculationType })
	for _, k := range statusKeys {
		s := user.CostumeLotteryEffectStatusUps[k]
		ch.CostumeLotteryEffectStatusUps = append(ch.CostumeLotteryEffectStatusUps, &pb.CostumeLotteryEffectStatusUpInfo{StatusCalculationType: s.StatusCalculationType, Hp: s.Hp, Attack: s.Attack, Vitality: s.Vitality, Agility: s.Agility, CriticalRatio: s.CriticalRatio, CriticalAttack: s.CriticalAttack})
	}
}

func buildWeaponInfo(user *store.UserState, w store.WeaponState) *pb.WeaponInfo {
	wi := &pb.WeaponInfo{
		WeaponId:        w.WeaponId,
		LimitBreakCount: w.LimitBreakCount,
		Level:           w.Level,
	}
	for _, a := range user.WeaponAbilities[w.UserWeaponUuid] {
		wi.WeaponAbility = append(wi.WeaponAbility, &pb.WeaponAbilityInfo{
			AbilityId: a.SlotNumber,
			Level:     a.Level,
		})
	}
	for _, sk := range user.WeaponSkills[w.UserWeaponUuid] {
		wi.WeaponSkill = append(wi.WeaponSkill, &pb.WeaponSkillInfo{
			SkillId: sk.SlotNumber,
			Level:   sk.Level,
		})
	}
	return wi
}
