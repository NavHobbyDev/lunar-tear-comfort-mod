package service

import (
	"encoding/json"

	pb "lunar-tear/server/gen/proto"
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/runtime"
	"lunar-tear/server/internal/store"
)

// PlayerCard is the common currency for friend/arena list rows.
type PlayerCard struct {
	PlayerId          int64
	Name              string
	Level             int32
	MaxDeckPower      int32
	FavoriteCostumeId int32
	PvpPoint          int32
	LastLoginDatetime int64 // millis; 0 for bots (proto mapper substitutes now)
	IsBot             bool
}

// PlayerDirectory answers "who else is out there?" from real snapshots + synthesized bots.
type PlayerDirectory struct {
	snaps  store.SnapshotRepository
	holder *runtime.Holder
}

func NewPlayerDirectory(snaps store.SnapshotRepository, holder *runtime.Holder) *PlayerDirectory {
	return &PlayerDirectory{snaps: snaps, holder: holder}
}

func (d *PlayerDirectory) pools() botPools {
	cat := d.holder.Get()
	p := botPools{}
	if cat != nil {
		if cat.Costume != nil {
			// Only player-character costumes: the costume table also holds
			// thousands of enemy/NPC models (CostumeAssetCategoryType 2 plus
			// NPC placeholders with ids >= 1000000 inside category 1). Sending
			// one of those in a bot deck makes the client hang while loading
			// the battle scene, because a monster model cannot be spawned as a
			// party member.
			for id, c := range cat.Costume.Costumes {
				if c.CostumeAssetCategoryType == 1 && id < 1000000 {
					p.costumeIds = append(p.costumeIds, id)
					if p.characterByCostume == nil {
						p.characterByCostume = make(map[int32]int32)
					}
					p.characterByCostume[id] = c.CharacterId
				}
			}
			sortInt32s(p.costumeIds)
		}
		if cat.Weapon != nil {
			// Same story for weapons: category 2 entries are enemy weapons, and
			// category 1 ids >= 1000000 are unnamed NPC replicas (rarity 10,
			// no evolution data) used in story battles, not player weapons.
			for id, w := range cat.Weapon.Weapons {
				if w.WeaponCategoryType == 1 && id < 1000000 {
					p.weaponIds = append(p.weaponIds, id)
				}
			}
			sortInt32s(p.weaponIds)
		}
		if cat.Companion != nil {
			p.companionIds = sortedInt32Keys(cat.Companion.CompanionById)
		}
	}
	return p
}

func cardFromSnapshot(s store.PlayerSnapshot) PlayerCard {
	return PlayerCard{
		PlayerId:          s.PlayerId,
		Name:              s.UserName,
		Level:             s.Level,
		MaxDeckPower:      s.MaxDeckPower,
		FavoriteCostumeId: s.FavoriteCostumeId,
		PvpPoint:          s.PvpPoint,
		LastLoginDatetime: s.LastLoginDatetime,
		IsBot:             IsBotId(s.PlayerId),
	}
}

// RealPlayersNear returns real snapshots (excluding the viewer) closest to nearPoint.
func (d *PlayerDirectory) RealPlayersNear(viewerId int64, nearPoint int32, limit int) []PlayerCard {
	snaps, err := d.snaps.ListSnapshotsNear(viewerId, nearPoint, limit)
	if err != nil {
		return nil
	}
	out := make([]PlayerCard, 0, len(snaps))
	for _, s := range snaps {
		out = append(out, cardFromSnapshot(s))
	}
	return out
}

// FillWithBots tops the list up to targetCount with deterministic bots seeded by viewer+day.
func (d *PlayerDirectory) FillWithBots(existing []PlayerCard, targetCount int, viewerId int64, dayBucket int64, nearPoint int32) []PlayerCard {
	if len(existing) >= targetCount {
		return existing
	}
	pools := d.pools()
	out := existing
	for slot := 0; len(out) < targetCount; slot++ {
		out = append(out, synthBot(pools, viewerId, slot, dayBucket, nearPoint))
	}
	return out
}

// DefenseDeckOf returns the opponent deck for a card, taken from the
// snapshot table whenever the opponent has a row there (real players and
// roster bots alike store their defense_deck_json) — the deck the board
// itself carries, never a re-rolled one. Only an id with no table row (an
// ephemeral fill bot) gets a synthesized deck. Either way the weapon
// ability/skill slot numbers stored server-side are resolved into the real
// masterdata ids the client expects before the deck goes out.
func (d *PlayerDirectory) DefenseDeckOf(card PlayerCard) []*pb.PvpDeckCharacter {
	var deck []*pb.PvpDeckCharacter
	if snap, err := d.snaps.GetSnapshot(card.PlayerId); err == nil && snap.DefenseDeckJson != "" && snap.DefenseDeckJson != "[]" {
		_ = json.Unmarshal([]byte(snap.DefenseDeckJson), &deck)
	}
	if len(deck) == 0 && card.IsBot {
		deck = synthBotDeck(d.pools(), card.PlayerId)
	}
	d.resolveWeaponSlotIds(deck)
	return deck
}

// resolveWeaponSlotIds rewrites WeaponAbilityInfo.AbilityId and
// WeaponSkillInfo.SkillId from the slot numbers (1,2,3 / 1,2) that decks are
// serialized with into the real masterdata ids for that weapon. The client
// looks these ids up in its own masterdata to compute opponent stats and
// skills; a bogus id like "1" breaks battle-scene preparation and leaves the
// loading screen stuck. Entries whose slot has no masterdata row are dropped.
func (d *PlayerDirectory) resolveWeaponSlotIds(deck []*pb.PvpDeckCharacter) {
	cat := d.holder.Get()
	if cat == nil || cat.Weapon == nil {
		return
	}
	for _, ch := range deck {
		if ch == nil {
			continue
		}
		d.resolveOneWeapon(cat.Weapon, ch.MainWeapon)
		for _, sw := range ch.SubWeapon {
			d.resolveOneWeapon(cat.Weapon, sw)
		}
	}
}

func (d *PlayerDirectory) resolveOneWeapon(wc *masterdata.WeaponCatalog, wi *pb.WeaponInfo) {
	if wi == nil {
		return
	}
	w, ok := wc.Weapons[wi.WeaponId]
	if !ok {
		wi.WeaponAbility = nil
		wi.WeaponSkill = nil
		return
	}
	abilityBySlot := make(map[int32]int32)
	for _, row := range wc.AbilityGroupsByGroupId[w.WeaponAbilityGroupId] {
		abilityBySlot[row.SlotNumber] = row.AbilityId
	}
	resolvedAbilities := wi.WeaponAbility[:0]
	for _, a := range wi.WeaponAbility {
		if id, ok := abilityBySlot[a.AbilityId]; ok {
			a.AbilityId = id
			resolvedAbilities = append(resolvedAbilities, a)
		}
	}
	wi.WeaponAbility = resolvedAbilities
	skillBySlot := make(map[int32]int32)
	for _, row := range wc.SkillGroupsByGroupId[w.WeaponSkillGroupId] {
		skillBySlot[row.SlotNumber] = row.SkillId
	}
	resolvedSkills := wi.WeaponSkill[:0]
	for _, sk := range wi.WeaponSkill {
		if id, ok := skillBySlot[sk.SkillId]; ok {
			sk.SkillId = id
			resolvedSkills = append(resolvedSkills, sk)
		}
	}
	wi.WeaponSkill = resolvedSkills
}

func (d *PlayerDirectory) IsBot(playerId int64) bool { return IsBotId(playerId) }

// CardFor resolves a single player/opponent by id, always preferring the
// snapshot table — the rating board itself. Roster bots and real players
// alike come back exactly as the table holds them (current name, avatar,
// rating, power), so battle history, matching and battle start never invent
// a different identity or a stale seeded rating. Only an id with no table
// row (an ephemeral fill bot) falls back to synthesis.
func (d *PlayerDirectory) CardFor(playerId int64) (PlayerCard, bool) {
	if snap, err := d.snaps.GetSnapshot(playerId); err == nil {
		return cardFromSnapshot(snap), true
	}
	if IsBotId(playerId) {
		return botCardFromId(d.pools(), playerId), true
	}
	return PlayerCard{}, false
}
