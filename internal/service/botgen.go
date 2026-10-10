package service

import (
	"hash/fnv"
	"math/rand"
	"sort"

	pb "lunar-tear/server/gen/proto"
)

// BotIdBase is the reserved high range for bot player ids; real ids never reach it.
const BotIdBase int64 = 1 << 40

func IsBotId(playerId int64) bool { return playerId >= BotIdBase }

// botPools holds sorted, stable id slices sampled to build bots.
type botPools struct {
	costumeIds   []int32
	weaponIds    []int32
	companionIds []int32
	// characterByCostume maps a costume id to its owning character, so bot
	// decks can guarantee three distinct characters like any legal deck.
	characterByCostume map[int32]int32
}

func seedFrom(viewerId int64, slot int, dayBucket int64) int64 {
	h := fnv.New64a()
	var buf [8]byte
	for _, v := range []int64{viewerId, int64(slot), dayBucket} {
		for i := 0; i < 8; i++ {
			buf[i] = byte(v >> (8 * i))
		}
		h.Write(buf[:])
	}
	return int64(h.Sum64() & 0x7fffffffffffffff)
}

func newRand(seed int64) *rand.Rand { return rand.New(rand.NewSource(seed)) }

func pick(r *rand.Rand, ids []int32, fallback int32) int32 {
	if len(ids) == 0 {
		return fallback
	}
	return ids[r.Intn(len(ids))]
}

// synthBot builds a deterministic bot PlayerCard near targetPoint.
func synthBot(pools botPools, viewerId int64, slot int, dayBucket int64, targetPoint int32) PlayerCard {
	seed := seedFrom(viewerId, slot, dayBucket)
	r := rand.New(rand.NewSource(seed))
	costumeId := pick(r, pools.costumeIds, 1)
	base := targetPoint
	if base < 1000 {
		base = 1000
	}
	delta := int32(r.Intn(int(base/6+1))) - base/12
	// Power follows the same ladder scale as roster bots, with a small jitter
	// so same-rating fill bots don't all field identical strength.
	power := powerForPoint(base)
	power += power * int32(r.Intn(11)-5) / 100
	if power < botMinDeckPower {
		power = botMinDeckPower
	}
	// A fill bot always carries a real rating: never surface a zero-rating
	// opponent (that reads as a player who hasn't opened the arena). The
	// floor matches the ladder tail, where the weakest roster bots stand.
	point := targetPoint + (delta / 4)
	if point < botRosterBottomPoint {
		point = botRosterBottomPoint
	}
	return PlayerCard{
		PlayerId:          BotIdBase + seed%1_000_000_000,
		Name:              botName(seed),
		Level:             int32(40 + r.Intn(40)),
		MaxDeckPower:      power,
		FavoriteCostumeId: costumeId,
		PvpPoint:          point,
		IsBot:             true,
	}
}

var botFirst = []string{"Aoi", "Levin", "Argo", "Fio", "Noelle", "Dimos", "Lars", "Yuzu", "Renah", "Gayle"}
var botLast = []string{"v", "x", "z", "q", "", "II", "EX", "+", "α", "Ω"}

func botName(seed int64) string {
	f := botFirst[seed%int64(len(botFirst))]
	l := botLast[(seed/7)%int64(len(botLast))]
	return f + l
}

// synthBotWeapon builds a fully-populated weapon (main or sub) matching the
// shape of a real developed deck's weapon: id + level + limit break, plus the
// three ability slots and two skill slots the client resolves per weapon. The
// slot ids (1,2,3 / 1,2) are slot numbers, not real ability ids, exactly as a
// real defense deck serializes them (see buildWeaponInfo).
func synthBotWeapon(r *rand.Rand, pools botPools) *pb.WeaponInfo {
	return &pb.WeaponInfo{
		WeaponId:        pick(r, pools.weaponIds, 1),
		Level:           int32(80 + r.Intn(21)),
		LimitBreakCount: 4,
		WeaponAbility: []*pb.WeaponAbilityInfo{
			{AbilityId: 1, Level: 15}, {AbilityId: 2, Level: 15}, {AbilityId: 3, Level: 15},
		},
		WeaponSkill: []*pb.WeaponSkillInfo{
			{SkillId: 1, Level: 15}, {SkillId: 2, Level: 15},
		},
	}
}

// synthBotDeck builds a full, valid 3-character PvP deck for a bot, mirroring
// the structure of a real developed defense deck: each character carries a
// costume (with an active skill level), an optional companion, and a complete
// weapon trio (one main + two sub weapons, each with ability/skill slots).
// The PvP battle is simulated client-side and stalls at load if the opponent
// deck is one the game itself could never build, so bots must respect the
// same rules as the deck editor: full weapon sets and, critically, three
// DIFFERENT characters (two costumes of the same character are illegal).
func synthBotDeck(pools botPools, playerId int64) []*pb.PvpDeckCharacter {
	r := rand.New(rand.NewSource(playerId))
	usedChars := make(map[int32]bool, 3)
	var out []*pb.PvpDeckCharacter
	for i := 0; i < 3; i++ {
		costumeId := pickUniqueCharCostume(r, pools, usedChars)
		lvl := int32(40 + r.Intn(40))
		ch := &pb.PvpDeckCharacter{
			Costume: &pb.CostumeInfo{
				CostumeId:        costumeId,
				Level:            lvl,
				CharacterLevel:   lvl,
				ActiveSkillLevel: 1,
				LimitBreakCount:  int32(r.Intn(5)),
			},
			MainWeapon: synthBotWeapon(r, pools),
			SubWeapon: []*pb.WeaponInfo{
				synthBotWeapon(r, pools),
				synthBotWeapon(r, pools),
			},
		}
		if len(pools.companionIds) > 0 {
			ch.Companion = &pb.CompanionInfo{CompanionId: pick(r, pools.companionIds, 1), Level: int32(20 + r.Intn(20))}
		}
		out = append(out, ch)
	}
	return out
}

// pickUniqueCharCostume samples a costume whose character is not already in
// the deck, then marks that character as used. Falls back to any costume if
// the pool cannot satisfy uniqueness (tiny/test pools).
func pickUniqueCharCostume(r *rand.Rand, pools botPools, usedChars map[int32]bool) int32 {
	for attempt := 0; attempt < 40; attempt++ {
		id := pick(r, pools.costumeIds, 1)
		charId, ok := pools.characterByCostume[id]
		if !ok {
			charId = id // no mapping (tests): treat the costume itself as the character
		}
		if !usedChars[charId] {
			usedChars[charId] = true
			return id
		}
	}
	return pick(r, pools.costumeIds, 1)
}

func sortedInt32Keys[V any](m map[int32]V) []int32 {
	out := make([]int32, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sortInt32s(out)
	return out
}

func sortInt32s(s []int32) {
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
}
