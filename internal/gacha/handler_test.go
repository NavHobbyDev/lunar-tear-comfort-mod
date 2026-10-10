package gacha

import (
	"fmt"
	"strings"
	"testing"

	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
)

func fillWeapons(user *store.UserState, n int) {
	user.EnsureMaps()
	for i := range n {
		user.Weapons[fmt.Sprintf("w%d", i)] = store.WeaponState{WeaponId: int32(i)}
	}
}

func TestHandleDrawAllowedAtWeaponInventoryCap(t *testing.T) {
	user := &store.UserState{}
	fillWeapons(user, int(model.WeaponInventoryCap))

	h := &GachaHandler{}
	_, err := h.HandleDraw(user, store.GachaCatalogEntry{}, 0, 1)
	if err != nil && strings.Contains(err.Error(), "weapon inventory full") {
		t.Fatalf("did not expect inventory-full refusal, got %v", err)
	}
}

// TestHandleDrawAbortsWhenCannotPay guards the charge-before-draw fix: a
// chapter (or any ticket/gem-priced) draw must be refused and grant nothing
// when the player cannot pay, instead of silently proceeding for free.
func TestHandleDrawAbortsWhenCannotPay(t *testing.T) {
	user := &store.UserState{}
	user.EnsureMaps()

	const gachaId = int32(200001)
	phaseId := gachaId*model.PhaseIdMultiplier + 1
	entry := store.GachaCatalogEntry{
		GachaId:        gachaId,
		GachaLabelType: model.GachaLabelChapter,
		GachaModeType:  model.GachaModeBox,
		PricePhases: []store.GachaPricePhaseEntry{{
			PhaseId:   phaseId,
			PriceType: model.PriceTypeConsumableItem,
			PriceId:   1008,
			Price:     1,
			DrawCount: 1,
		}},
	}
	h := &GachaHandler{Pool: &masterdata.GachaCatalog{
		ChapterGachaItems: map[int32][]masterdata.GachaPoolItem{
			gachaId: {{PossessionType: int32(model.PossessionTypeMaterial), PossessionId: 301001, Count: 1, Weight: 1}},
		},
	}}

	// User holds no chapter ticket 1008: the draw must abort and grant nothing.
	res, err := h.HandleDraw(user, entry, phaseId, 1)
	if err == nil {
		t.Fatalf("expected draw to abort when player cannot pay, got result %+v", res)
	}
	if res != nil {
		t.Fatalf("expected nil result on aborted draw, got %+v", res)
	}
	if user.Materials[301001] != 0 {
		t.Fatalf("expected no reward granted on aborted draw, got material count %d", user.Materials[301001])
	}
}

func TestGrantItemsOverflowWeaponsAtCap(t *testing.T) {
	user := &store.UserState{}
	fillWeapons(user, int(model.WeaponInventoryCap))

	h := &GachaHandler{
		Config:          &masterdata.GameConfig{ConsumableItemIdForGold: 1},
		WeaponSellPrice: map[int32]int32{301: 500, 302: 700},
		Granter:         &store.PossessionGranter{},
	}
	items := []DrawnItem{
		{PossessionType: int32(model.PossessionTypeWeapon), PossessionId: 301, RarityType: model.RarityRare},
		{PossessionType: int32(model.PossessionTypeWeapon), PossessionId: 302, RarityType: model.RaritySRare},
		{PossessionType: int32(model.PossessionTypeWeapon), PossessionId: 303, RarityType: model.RaritySSRare},
	}
	h.grantItems(user, items, 123)

	if len(user.Weapons) != int(model.WeaponInventoryCap) {
		t.Fatalf("expected no weapon granted at cap, got %d", len(user.Weapons))
	}
	if got := user.ConsumableItems[1]; got != 1200 {
		t.Fatalf("expected 1200 gold from auto-sell, got %d", got)
	}
	if len(user.Gifts.NotReceived) != 1 {
		t.Fatalf("expected SSR weapon mailed to gift box, got %d gifts", len(user.Gifts.NotReceived))
	}
	gift := user.Gifts.NotReceived[0]
	if gift.GiftCommon.PossessionType != int32(model.PossessionTypeWeapon) || gift.GiftCommon.PossessionId != 303 {
		t.Fatalf("expected mailed gift to be weapon 303, got type=%d id=%d",
			gift.GiftCommon.PossessionType, gift.GiftCommon.PossessionId)
	}
	if user.Notifications.GiftNotReceiveCount != 1 {
		t.Fatalf("expected gift badge count 1, got %d", user.Notifications.GiftNotReceiveCount)
	}
}

func TestGrantItemsWeaponFillsLastSlotThenOverflows(t *testing.T) {
	user := &store.UserState{}
	fillWeapons(user, int(model.WeaponInventoryCap)-1)

	h := &GachaHandler{
		Config:          &masterdata.GameConfig{ConsumableItemIdForGold: 1},
		WeaponSellPrice: map[int32]int32{302: 700},
		Granter:         &store.PossessionGranter{},
	}
	items := []DrawnItem{
		{PossessionType: int32(model.PossessionTypeWeapon), PossessionId: 301, RarityType: model.RarityRare},
		{PossessionType: int32(model.PossessionTypeWeapon), PossessionId: 302, RarityType: model.RaritySRare},
	}
	h.grantItems(user, items, 123)

	if len(user.Weapons) != int(model.WeaponInventoryCap) {
		t.Fatalf("expected inventory filled to cap, got %d", len(user.Weapons))
	}
	if got := user.ConsumableItems[1]; got != 700 {
		t.Fatalf("expected second weapon auto-sold for 700 gold, got %d", got)
	}
}

func dedupeTestPool() *masterdata.BannerPool {
	return &masterdata.BannerPool{
		CostumesByRarity: map[int32][]masterdata.GachaPoolItem{
			model.RaritySSRare: {
				{PossessionType: int32(model.PossessionTypeCostume), PossessionId: 101, RarityType: model.RaritySSRare, CharacterId: 1},
				{PossessionType: int32(model.PossessionTypeCostume), PossessionId: 102, RarityType: model.RaritySSRare, CharacterId: 1},
			},
		},
		WeaponsByRarity: map[int32][]masterdata.GachaPoolItem{
			model.RaritySSRare: {
				{PossessionType: int32(model.PossessionTypeWeapon), PossessionId: 201, RarityType: model.RaritySSRare},
				{PossessionType: int32(model.PossessionTypeWeapon), PossessionId: 202, RarityType: model.RaritySSRare},
			},
		},
	}
}

func TestDedupeHighRaritySwapsOwnedItems(t *testing.T) {
	bp := dedupeTestPool()
	costumes := map[int32]bool{101: true}
	weapons := map[int32]bool{201: true}

	// Owned SSR drops are swapped for the unowned item of same type+rarity.
	got := dedupeHighRarity(bp, costumes, weapons, DrawnItem{PossessionType: int32(model.PossessionTypeCostume), PossessionId: 101, RarityType: model.RaritySSRare})
	if got.PossessionId != 102 {
		t.Fatalf("expected costume swap to 102, got %d", got.PossessionId)
	}
	got = dedupeHighRarity(bp, costumes, weapons, DrawnItem{PossessionType: int32(model.PossessionTypeWeapon), PossessionId: 201, RarityType: model.RaritySSRare})
	if got.PossessionId != 202 {
		t.Fatalf("expected weapon swap to 202, got %d", got.PossessionId)
	}

	// All candidates owned (102/202 were marked by the swaps above): keep the draw.
	got = dedupeHighRarity(bp, costumes, weapons, DrawnItem{PossessionType: int32(model.PossessionTypeCostume), PossessionId: 101, RarityType: model.RaritySSRare})
	if got.PossessionId != 101 {
		t.Fatalf("expected owned costume kept when all alternatives owned, got %d", got.PossessionId)
	}

	// Sub-SSR items are never touched.
	got = dedupeHighRarity(bp, costumes, weapons, DrawnItem{PossessionType: int32(model.PossessionTypeWeapon), PossessionId: 201, RarityType: model.RarityRare})
	if got.PossessionId != 201 {
		t.Fatalf("expected low-rarity item untouched, got %d", got.PossessionId)
	}
}

func TestDrawPremiumNeverHandsOwnedSSRWhileAlternativeAvailable(t *testing.T) {
	bp := dedupeTestPool()
	owned := &OwnedSets{
		Costumes: map[int32]bool{101: true},
		Weapons:  map[int32]bool{201: true},
	}

	items := DrawPremium(bp, 2000, 0, 0, 1.0, owned)

	costumeFreeSeen, weaponFreeSeen := false, false
	ssrSeen := 0
	for _, item := range items {
		if item.RarityType < model.RaritySSRare {
			continue
		}
		ssrSeen++
		switch model.PossessionType(item.PossessionType) {
		case model.PossessionTypeCostume:
			if item.PossessionId == 102 {
				costumeFreeSeen = true
			}
			if item.PossessionId == 101 && !costumeFreeSeen {
				t.Fatalf("owned costume 101 dropped while unowned 102 was still available")
			}
		case model.PossessionTypeWeapon:
			if item.PossessionId == 202 {
				weaponFreeSeen = true
			}
			if item.PossessionId == 201 && !weaponFreeSeen {
				t.Fatalf("owned weapon 201 dropped while unowned 202 was still available")
			}
		}
	}
	if ssrSeen == 0 {
		t.Fatal("no SSR items drawn in 2000 draws")
	}
	if !costumeFreeSeen || !weaponFreeSeen {
		t.Fatalf("expected unowned SSR alternatives to appear (costume=%v weapon=%v)", costumeFreeSeen, weaponFreeSeen)
	}
}

func TestCostumeDupExchangeGradeScalesBookCount(t *testing.T) {
	user := &store.UserState{}
	user.EnsureMaps()
	user.Costumes["c1"] = store.CostumeState{CostumeId: 555}

	h := &GachaHandler{
		DupExchange: map[int32][]model.DupExchangeEntry{
			555: {{PossessionType: int32(model.PossessionTypeMaterial), PossessionId: 9001, Count: 10}},
		},
	}

	seen := map[int32]bool{}
	for range 200 {
		user.Materials[9001] = 0
		dup, ok := h.tryCostumeDupExchange(user, DrawnItem{PossessionType: int32(model.PossessionTypeCostume), PossessionId: 555}, 0)
		if !ok {
			t.Fatal("expected dup exchange to trigger for owned costume")
		}
		if dup.Grade < model.DupGradeMin || dup.Grade > model.DupGradeMax {
			t.Fatalf("grade %d out of range %d..%d", dup.Grade, model.DupGradeMin, model.DupGradeMax)
		}
		want := 10 * model.DupGradePayoutPercent[dup.Grade] / 100
		if user.Materials[9001] != want {
			t.Fatalf("grade %d: expected %d books, granted %d", dup.Grade, want, user.Materials[9001])
		}
		if len(dup.Bonuses) != 1 || dup.Bonuses[0].Count != want {
			t.Fatalf("grade %d: bonus report mismatch: %+v", dup.Grade, dup.Bonuses)
		}
		seen[want] = true
	}
	if len(seen) < 2 {
		t.Fatalf("grade roll looks fixed, book counts seen: %v", seen)
	}
}

// Grade rolls are weighted per model.DupGradeWeights: frequencies must grow
// monotonically from grade 1 (rarest) to grade 5 (most common).
func TestDupGradeRollIsWeighted(t *testing.T) {
	counts := map[int32]int{}
	const trials = 20000
	for range trials {
		counts[rollDupGrade()]++
	}
	var prev int
	for grade := model.DupGradeMin; grade <= model.DupGradeMax; grade++ {
		if counts[grade] == 0 {
			t.Fatalf("grade %d never rolled in %d trials", grade, trials)
		}
		if counts[grade] <= prev {
			t.Fatalf("expected grade %d more common than grade %d: %d vs %d", grade, grade-1, counts[grade], prev)
		}
		prev = counts[grade]
	}
}

// Simulates banner 45 (single character with exactly one SR costume) and checks
// that the SR+ guarantee slot is not deterministically a costume.
func TestGuaranteeSlotNotStuckOnSingleCostume(t *testing.T) {
	srWeapons := make([]masterdata.GachaPoolItem, 0, 6)
	for i := range 6 {
		srWeapons = append(srWeapons, masterdata.GachaPoolItem{
			PossessionType: int32(model.PossessionTypeWeapon),
			PossessionId:   int32(110051 + i*10000),
			RarityType:     model.RaritySRare,
		})
	}
	srCostume := masterdata.GachaPoolItem{
		PossessionType: int32(model.PossessionTypeCostume),
		PossessionId:   22009,
		RarityType:     model.RaritySRare,
		CharacterId:    1025,
	}
	ssrCostume := masterdata.GachaPoolItem{
		PossessionType: int32(model.PossessionTypeCostume),
		PossessionId:   31018,
		RarityType:     model.RaritySSRare,
		CharacterId:    1025,
	}
	featured := append([]masterdata.GachaPoolItem{srCostume, ssrCostume}, srWeapons...)

	bp := &masterdata.BannerPool{
		CostumesByRarity: map[int32][]masterdata.GachaPoolItem{
			model.RaritySRare:  {srCostume},
			model.RaritySSRare: {ssrCostume},
		},
		WeaponsByRarity: map[int32][]masterdata.GachaPoolItem{
			model.RaritySRare: srWeapons,
		},
		Featured: featured,
	}

	const trials = 20000
	costumeGuarantees := 0
	for range trials {
		items := DrawPremium(bp, 10, model.RaritySRare, 1, 1.0, nil)
		if model.PossessionType(items[9].PossessionType) == model.PossessionTypeCostume {
			costumeGuarantees++
		}
	}
	ratio := float64(costumeGuarantees) / trials
	t.Logf("guarantee slot costume share: %.3f", ratio)
	// Costume tiers hold 700 of the 2000 SR+ weight -> expect ~0.35.
	if ratio < 0.15 || ratio > 0.45 {
		t.Fatalf("guarantee slot costume share %.3f outside sane range", ratio)
	}
}

// Simulates the real banner 45 composition: 1 SR costume (22009), 8 SSR
// costumes, 5 R + 21 SR + 53 SSR weapons. Verifies SSR costumes do drop and
// the guarantee slot is not stuck on 22009.
func TestBanner45CompositionDrops(t *testing.T) {
	costumes := []masterdata.GachaPoolItem{
		{PossessionType: int32(model.PossessionTypeCostume), PossessionId: 22009, RarityType: model.RaritySRare, CharacterId: 1025},
	}
	for _, id := range []int32{31018, 31030, 32028, 32034, 33020, 34031, 34047, 35029} {
		costumes = append(costumes, masterdata.GachaPoolItem{
			PossessionType: int32(model.PossessionTypeCostume), PossessionId: id, RarityType: model.RaritySSRare, CharacterId: 1025,
		})
	}
	weapons := make([]masterdata.GachaPoolItem, 0, 79)
	for i := range 5 {
		weapons = append(weapons, masterdata.GachaPoolItem{PossessionType: int32(model.PossessionTypeWeapon), PossessionId: int32(110051 + i*10000), RarityType: model.RarityRare})
	}
	for i := range 21 {
		weapons = append(weapons, masterdata.GachaPoolItem{PossessionType: int32(model.PossessionTypeWeapon), PossessionId: int32(210141 + i), RarityType: model.RaritySRare})
	}
	for i := range 53 {
		weapons = append(weapons, masterdata.GachaPoolItem{PossessionType: int32(model.PossessionTypeWeapon), PossessionId: int32(310171 + i), RarityType: model.RaritySSRare})
	}

	bp := &masterdata.BannerPool{
		CostumesByRarity: map[int32][]masterdata.GachaPoolItem{
			model.RaritySRare:  {costumes[0]},
			model.RaritySSRare: costumes[1:],
		},
		WeaponsByRarity: map[int32][]masterdata.GachaPoolItem{
			model.RarityRare:   weapons[:5],
			model.RaritySRare:  weapons[5:26],
			model.RaritySSRare: weapons[26:],
		},
		Featured: append(append([]masterdata.GachaPoolItem(nil), costumes...), weapons...),
	}

	const pulls = 20000
	ssrCostumeDrops := 0
	guarantee22009 := 0
	guaranteeCostume := 0
	for range pulls {
		items := DrawPremium(bp, 10, model.RaritySRare, 1, 1.0, nil)
		for i, it := range items {
			if model.PossessionType(it.PossessionType) == model.PossessionTypeCostume && it.RarityType == model.RaritySSRare {
				ssrCostumeDrops++
			}
			if i == 9 && model.PossessionType(it.PossessionType) == model.PossessionTypeCostume {
				guaranteeCostume++
				if it.PossessionId == 22009 {
					guarantee22009++
				}
			}
		}
	}
	t.Logf("per 10-pull: SSR costume drops=%.3f, guarantee costume=%.3f, guarantee 22009=%.3f",
		float64(ssrCostumeDrops)/pulls, float64(guaranteeCostume)/pulls, float64(guarantee22009)/pulls)

	// Expected with premiumRates totaling 10000 (costume SSR tier 200 = 2%):
	// 9 normal slots x 2% + guarantee (2% direct + 80% re-roll x 200/2000)
	// ~= 0.28 SSR costume drops per 10-pull.
	if ssrCostumeDrops == 0 {
		t.Fatal("no SSR costume drops at all - routing bug")
	}
	if rate := float64(ssrCostumeDrops) / pulls; rate < 0.20 || rate > 0.35 {
		t.Fatalf("SSR costume drop rate %.3f outside expected ~0.28", rate)
	}
}

// Overridden banners must ignore the phase guarantee slot: draws stay plain
// uniform rolls from the replaced pool.
func TestOverriddenBannerIgnoresGuarantee(t *testing.T) {
	const ssrWeapon = int32(330999)
	pool := &masterdata.GachaCatalog{
		CostumeById: map[int32]masterdata.GachaPoolItem{
			22009: {PossessionType: int32(model.PossessionTypeCostume), PossessionId: 22009, RarityType: model.RaritySRare, CharacterId: 1025},
		},
		WeaponById: map[int32]masterdata.GachaPoolItem{
			110051:    {PossessionType: int32(model.PossessionTypeWeapon), PossessionId: 110051, RarityType: model.RarityRare},
			ssrWeapon: {PossessionType: int32(model.PossessionTypeWeapon), PossessionId: ssrWeapon, RarityType: model.RaritySSRare},
		},
		SkillfulWeaponTypeByCostume: map[int32]int32{22009: 7},
		WeaponTypeById:              map[int32]int32{110051: 7, ssrWeapon: 7},
		BannerPools:                 map[int32]*masterdata.BannerPool{},
		FeaturedByGacha:             map[int32]masterdata.FeaturedSet{},
	}
	pool.OverrideBannerForCharacters(45, []int32{1025}, nil, nil)
	if !pool.IsOverriddenBanner(45) {
		t.Fatal("banner 45 should be marked overridden")
	}

	h := &GachaHandler{Pool: pool}
	user := &store.UserState{}
	user.EnsureMaps()
	entry := store.GachaCatalogEntry{GachaId: 45}
	phase := store.GachaPricePhaseEntry{FixedRarityMin: model.RaritySSRare, FixedCount: 1}

	const pulls = 300
	ssrGuarantees := 0
	for range pulls {
		items := h.drawPremium(user, entry, phase, 10)
		if len(items) != 10 {
			t.Fatalf("expected 10 items, got %d", len(items))
		}
		if items[9].PossessionId == ssrWeapon {
			ssrGuarantees++
		}
	}
	// With the guarantee disabled, slot 9 hits the SSR weapon tier (300/10000
	// = 3%) like any normal slot (~9 of 300). An active guarantee would push
	// this above ~60%.
	if ssrGuarantees > 100 {
		t.Fatalf("guarantee slot looks active on overridden banner: %d/%d SSR hits in slot 9", ssrGuarantees, pulls)
	}
}

// mailFixture builds one unreceived gift-box mail for testing.
func mailFixture(posType, posId, count int32) store.NotReceivedGiftState {
	return store.NotReceivedGiftState{
		GiftCommon:   store.GiftCommonState{PossessionType: posType, PossessionId: posId, Count: count},
		UserGiftUuid: fmt.Sprintf("%d-%d-%d", posType, posId, count),
	}
}

// TestConsolidatePityGifts verifies that old unreceived mails carrying the
// pity payout items are removed with their counts summed, while unrelated
// mails survive untouched and in order.
func TestConsolidatePityGifts(t *testing.T) {
	user := &store.UserState{}
	user.Gifts.NotReceived = []store.NotReceivedGiftState{
		mailFixture(int32(model.PossessionTypePaidGem), 0, 3000), // stale pity mail
		mailFixture(int32(model.PossessionTypeWeapon), 303, 1),   // unrelated: kept
		mailFixture(int32(model.PossessionTypeConsumableItem), model.PityGiftMamaPointId, 300),
		mailFixture(int32(model.PossessionTypeFreeGem), 0, 50), // free gems: kept
		mailFixture(int32(model.PossessionTypeConsumableItem), model.PityGiftMamaPointId, 200),
		mailFixture(int32(model.PossessionTypeConsumableItem), model.PityGiftResurrectMedalId, 10),
	}

	pending := consolidatePityGifts(user)

	if got := pending[[2]int32{int32(model.PossessionTypePaidGem), 0}]; got != 3000 {
		t.Fatalf("expected 3000 paid gems pending, got %d", got)
	}
	if got := pending[[2]int32{int32(model.PossessionTypeConsumableItem), model.PityGiftMamaPointId}]; got != 500 {
		t.Fatalf("expected 500 Mama Points pending (300+200), got %d", got)
	}
	if got := pending[[2]int32{int32(model.PossessionTypeConsumableItem), model.PityGiftResurrectMedalId}]; got != 10 {
		t.Fatalf("expected 10 event medals pending, got %d", got)
	}

	if len(user.Gifts.NotReceived) != 2 {
		t.Fatalf("expected 2 unrelated mails kept, got %d", len(user.Gifts.NotReceived))
	}
	if user.Gifts.NotReceived[0].GiftCommon.PossessionId != 303 ||
		user.Gifts.NotReceived[1].GiftCommon.PossessionType != int32(model.PossessionTypeFreeGem) {
		t.Fatalf("unexpected surviving mails: %+v", user.Gifts.NotReceived)
	}
}
