package gacha

import (
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
)

type DrawResult struct {
	Items               []DrawnItem
	BonusItems          map[int]DrawnItem
	Bonuses             []store.GachaBonusEntry
	DuplicateInfos      []DuplicateInfo
	BonusDuplicateInfos []DuplicateInfo
	MedalBonus          int32
}

type DuplicateInfo struct {
	Index   int
	Grade   int32
	Bonuses []model.DupExchangeEntry
}

type GachaHandler struct {
	Pool        *masterdata.GachaCatalog
	Config      *masterdata.GameConfig
	Granter     *store.PossessionGranter
	MedalInfo   map[int32]masterdata.GachaMedalInfo
	DupExchange map[int32][]model.DupExchangeEntry
	// WeaponSellPrice holds the level-1 sell price per weapon id, used to
	// auto-sell overflow weapons when the inventory is at its cap.
	WeaponSellPrice map[int32]int32
}

func NewGachaHandler(
	pool *masterdata.GachaCatalog,
	config *masterdata.GameConfig,
	granter *store.PossessionGranter,
	medalInfo map[int32]masterdata.GachaMedalInfo,
	dupExchange map[int32][]model.DupExchangeEntry,
) *GachaHandler {
	return &GachaHandler{
		Pool:        pool,
		Config:      config,
		Granter:     granter,
		MedalInfo:   medalInfo,
		DupExchange: dupExchange,
	}
}

func (h *GachaHandler) HandleDraw(
	user *store.UserState,
	entry store.GachaCatalogEntry,
	phaseId int32,
	execCount int32,
) (*DrawResult, error) {
	// The weapon inventory caps at 999 entries. Draws are still allowed at
	// the cap: overflow weapons are auto-sold (sub-SSR) or mailed to the
	// gift box (SSR) by grantItems instead of being granted directly.

	phase, err := findPhase(entry, phaseId)
	if err != nil {
		return nil, err
	}

	totalCost := phase.Price * execCount
	if totalCost > 0 {
		// Charge first: if the player cannot pay (missing tickets/gems), abort
		// the draw so no rewards are granted. The service layer turns this error
		// into an empty, non-fatal response.
		if err := store.DeductPrice(user, phase.PriceType, phase.PriceId, totalCost); err != nil {
			log.Printf("[GachaHandler] DeductPrice failed, aborting draw: %v", err)
			return nil, err
		}
	}

	drawCount := int(phase.DrawCount * execCount)
	nowMillis := gametime.NowMillis()

	bs := user.Gacha.BannerStates[entry.GachaId]
	bs.GachaId = entry.GachaId

	var items []DrawnItem

	switch entry.GachaLabelType {
	case model.GachaLabelPremium:
		items = h.drawPremium(user, entry, phase, drawCount)
	case model.GachaLabelChapter, model.GachaLabelRecycle:
		items = h.drawMaterial(entry, drawCount)
	case model.GachaLabelEvent:
		items = h.drawBox(&bs, drawCount)
	default:
		items = h.drawPremium(user, entry, phase, drawCount)
	}

	if entry.GachaModeType == model.GachaModeStepup && !h.Pool.IsOverriddenBanner(entry.GachaId) {
		bs.StepNumber++
		if bs.StepNumber > entry.MaxStepNumber {
			bs.StepNumber = 1
			bs.LoopCount++
		}
	}

	var medalBonus int32
	if entry.GachaMedalId != 0 {
		medalBonus = int32(drawCount)
		bs.MedalCount += medalBonus

		// Grant the medal item BEFORE the pity exchange below so the drain
		// sees the freshly granted medals (93 + 10 = 103 -> drain 100 -> 3).
		// Medals are granted exactly once, here; do not grant them again at
		// the end of HandleDraw or the visible counter drifts ahead.
		if medalBonus > 0 && entry.MedalConsumableItemId != 0 {
			store.GrantPossession(user, model.PossessionTypeConsumableItem, entry.MedalConsumableItemId, medalBonus)
		}

		// Pity exchange: every full 100 medals are consumed and replaced with
		// a gift-box mail (1000 paid gems + Mama Points + Countdown Resurrected
		// Event Medals). The consumable medal item is drained in sync so the
		// count shown to the player matches the internal counter.
		for bs.MedalCount >= 100 {
			bs.MedalCount -= 100

			if entry.MedalConsumableItemId != 0 {
				// Clamp to the real balance so the item count never goes
				// negative if medals were already spent in the exchange shop.
				balance := user.ConsumableItems[store.CanonicalConsumableMedalId(entry.MedalConsumableItemId)]
				if balance > 100 {
					balance = 100
				}
				if balance > 0 {
					store.GrantPossession(user, model.PossessionTypeConsumableItem, entry.MedalConsumableItemId, -balance)
				}
			}

			// Mailbox consolidation: unreceived mails that already carry the
			// payout items are removed and their counts folded into the new
			// mails, so the player gets one fresh letter per item instead of
			// accumulating stale duplicates (e.g. an old 3000-gem letter plus
			// this payout becomes a single 4000-gem letter).
			pending := consolidatePityGifts(user)

			// Build the pity gifts in the order they should appear in the
			// mailbox (first gift appears first). AddGiftsOrdered will adjust
			// timestamps to ensure correct ordering.
			expiry := nowMillis + int64(30*24*time.Hour/time.Millisecond)
			gifts := []store.NotReceivedGiftState{
				{
					GiftCommon: store.GiftCommonState{
						PossessionType: int32(model.PossessionTypePaidGem),
						PossessionId:   0,
						Count:          model.PityGiftPaidGemCount + pending[[2]int32{int32(model.PossessionTypePaidGem), 0}],
						GrantDatetime:  nowMillis,
					},
					ExpirationDatetime: expiry,
					UserGiftUuid:       uuid.New().String(),
				},
				{
					GiftCommon: store.GiftCommonState{
						PossessionType: int32(model.PossessionTypeConsumableItem),
						PossessionId:   model.PityGiftMamaPointId,
						Count:          model.PityGiftMamaPointCount + pending[[2]int32{int32(model.PossessionTypeConsumableItem), model.PityGiftMamaPointId}],
						GrantDatetime:  nowMillis,
					},
					ExpirationDatetime: expiry,
					UserGiftUuid:       uuid.New().String(),
				},
				{
					GiftCommon: store.GiftCommonState{
						PossessionType: int32(model.PossessionTypeConsumableItem),
						PossessionId:   model.PityGiftResurrectMedalId,
						Count:          model.PityGiftResurrectMedalCount + pending[[2]int32{int32(model.PossessionTypeConsumableItem), model.PityGiftResurrectMedalId}],
						GrantDatetime:  nowMillis,
					},
					ExpirationDatetime: expiry,
					UserGiftUuid:       uuid.New().String(),
				},
			}
			user.AddGiftsOrdered(gifts)
		}

		if bs.MedalCount > model.MedalCountCap {
			bs.MedalCount = model.MedalCountCap
		}
	}

	bs.DrawCount += int32(drawCount)

	if user.Gacha.BannerStates == nil {
		user.Gacha.BannerStates = make(map[int32]store.GachaBannerState)
	}
	user.Gacha.BannerStates[entry.GachaId] = bs

	dupInfos := h.grantItems(user, items, nowMillis)

	bonusMap := h.generateBonusItems(entry, items)
	bonusSlice := make([]DrawnItem, 0, len(bonusMap))
	for _, b := range bonusMap {
		bonusSlice = append(bonusSlice, b)
	}
	bonusDupInfos := h.grantItems(user, bonusSlice, nowMillis)

	result := &DrawResult{
		Items:               items,
		BonusItems:          bonusMap,
		DuplicateInfos:      dupInfos,
		BonusDuplicateInfos: bonusDupInfos,
		MedalBonus:          medalBonus,
	}

	// Overridden banners keep no extras: phase bonuses (first-draw and the
	// like) are skipped so every rewritten banner behaves identically.
	if !h.Pool.IsOverriddenBanner(entry.GachaId) {
		for _, p := range phase.Bonuses {
			store.GrantPossession(user, model.PossessionType(p.PossessionType), p.PossessionId, p.Count)
			result.Bonuses = append(result.Bonuses, p)
		}
	}

	return result, nil
}

// consolidatePityGifts scans the gift box for unreceived mails carrying the
// pity payout items (paid gems, Mama Points, Countdown Resurrected Event
// Medals), removes them and returns their counts summed per (type, id) so the
// caller can fold the totals into the fresh pity mails. Other mails are kept
// untouched, in order.
func consolidatePityGifts(user *store.UserState) map[[2]int32]int32 {
	return user.ConsolidateGifts(map[[2]int32]bool{
		{int32(model.PossessionTypePaidGem), 0}:                                     true,
		{int32(model.PossessionTypeConsumableItem), model.PityGiftMamaPointId}:      true,
		{int32(model.PossessionTypeConsumableItem), model.PityGiftResurrectMedalId}: true,
	})
}

func (h *GachaHandler) HandleResetBox(
	user *store.UserState,
	entry store.GachaCatalogEntry,
) error {
	bs := user.Gacha.BannerStates[entry.GachaId]
	bs.BoxDrewCounts = make(map[int32]int32)
	bs.BoxNumber++
	user.Gacha.BannerStates[entry.GachaId] = bs
	return nil
}

func clampDailyDraw(lastDate, todayStart int64, currentCount, maxCount, requested int32) (clamped, newCount int32, reset bool) {
	if lastDate < todayStart {
		currentCount = 0
		reset = true
	}
	remaining := maxCount - currentCount
	if remaining <= 0 {
		return 0, currentCount, reset
	}
	if requested > remaining {
		requested = remaining
	}
	return requested, currentCount + requested, reset
}

func (h *GachaHandler) HandleRewardDraw(
	user *store.UserState,
	count int32,
) ([]DrawnItem, error) {
	nowMillis := gametime.NowMillis()
	todayStart := gametime.StartOfDayMillis()

	maxCount := h.Config.RewardGachaDailyMaxCount
	if maxCount <= 0 {
		maxCount = model.DefaultDailyDrawLimit
	}

	clamped, newCount, _ := clampDailyDraw(
		user.Gacha.LastRewardDrawDate, todayStart,
		user.Gacha.TodaysCurrentDrawCount, maxCount, count,
	)
	if clamped <= 0 {
		return nil, fmt.Errorf("daily reward draw limit reached")
	}

	items := DrawReward(h.Pool.Materials, int(clamped))

	for _, item := range items {
		store.GrantPossession(user, model.PossessionType(item.PossessionType), item.PossessionId, item.Count)
	}

	user.Gacha.TodaysCurrentDrawCount = newCount
	user.Gacha.DailyMaxCount = maxCount
	user.Gacha.LastRewardDrawDate = nowMillis
	user.Gacha.RewardAvailable = newCount < maxCount

	return items, nil
}

func (h *GachaHandler) drawPremium(user *store.UserState, entry store.GachaCatalogEntry, phase store.GachaPricePhaseEntry, count int) []DrawnItem {
	fixedMin := phase.FixedRarityMin
	fixedCount := int(phase.FixedCount)

	bp := h.Pool.BannerPools[entry.GachaId]
	if bp == nil {
		bp = &masterdata.BannerPool{
			CostumesByRarity: h.Pool.CostumesByRarity,
			WeaponsByRarity:  h.Pool.WeaponsByRarity,
		}
	}

	// Overridden banners draw plain uniform items: the guarantee slot and the
	// featured rate-up are disabled, and step-up boosts never apply. The
	// banner id only keeps the correct client-side artwork.
	overridden := h.Pool.IsOverriddenBanner(entry.GachaId)
	if overridden {
		fixedMin = 0
		fixedCount = 0
		plain := *bp
		plain.Featured = nil
		bp = &plain
	}

	rateMultiplier := 1.0
	if !overridden && entry.GachaModeType == model.GachaModeStepup {
		switch phase.StepNumber {
		case 1, 3:
			rateMultiplier = model.StepUpRateBoost
		case 5:
			rateMultiplier = model.StepUpRateMaxBoost
		}
	}

	owned := &OwnedSets{
		Costumes: make(map[int32]bool, len(user.Costumes)),
		Weapons:  make(map[int32]bool, len(user.Weapons)),
	}
	for _, c := range user.Costumes {
		owned.Costumes[c.CostumeId] = true
	}
	for _, w := range user.Weapons {
		owned.Weapons[w.WeaponId] = true
	}

	return DrawPremium(bp, count, fixedMin, fixedCount, rateMultiplier, owned)
}

func (h *GachaHandler) drawMaterial(entry store.GachaCatalogEntry, count int) []DrawnItem {
	if entry.GachaLabelType == model.GachaLabelChapter {
		if chapterItems, ok := h.Pool.ChapterGachaItems[entry.GachaId]; ok && len(chapterItems) > 0 {
			return DrawChapterWeighted(chapterItems, count)
		}
	}
	return DrawChapterWeighted(h.Pool.Materials, count)
}

func (h *GachaHandler) drawBox(bs *store.GachaBannerState, count int) []DrawnItem {
	if bs.BoxDrewCounts == nil {
		bs.BoxDrewCounts = make(map[int32]int32)
	}

	boxItems := h.buildBoxPool()
	for i := range boxItems {
		boxItems[i].DrewCount = bs.BoxDrewCounts[boxItems[i].PossessionId]
	}

	result := DrawBox(boxItems, count)

	for _, item := range result {
		bs.BoxDrewCounts[item.PossessionId]++
	}

	return result
}

func (h *GachaHandler) buildBoxPool() []BoxItem {
	var items []BoxItem
	for _, mat := range h.Pool.Materials {
		items = append(items, BoxItem{
			PossessionType: mat.PossessionType,
			PossessionId:   mat.PossessionId,
			RarityType:     mat.RarityType,
			Count:          1,
			MaxCount:       model.BoxItemDefaultMax,
		})
		if len(items) >= model.BoxPoolMaxItems {
			break
		}
	}
	if len(items) < model.BoxPoolMinItems {
		items = append(items, BoxItem{
			PossessionType: int32(model.PossessionTypeMaterial),
			PossessionId:   model.BoxFallbackItemId,
			RarityType:     model.RarityNormal,
			Count:          1,
			MaxCount:       model.BoxFallbackItemMax,
		})
	}
	return items
}

func (h *GachaHandler) grantItems(user *store.UserState, items []DrawnItem, nowMillis int64) []DuplicateInfo {
	var dupInfos []DuplicateInfo
	for i, item := range items {
		switch model.PossessionType(item.PossessionType) {
		case model.PossessionTypeCostume:
			if dup, ok := h.tryCostumeDupExchange(user, item, i); ok {
				dupInfos = append(dupInfos, dup)
				continue
			}
			// Not owned (tryCostumeDupExchange already scanned user.Costumes),
			// so grant the new costume without re-checking ownership.
			h.Granter.GrantNewCostume(user, item.PossessionId, nowMillis)
		case model.PossessionTypeWeapon:
			if len(user.Weapons) >= int(model.WeaponInventoryCap) {
				h.grantOverflowWeapon(user, item, nowMillis)
				continue
			}
			h.Granter.GrantWeapon(user, item.PossessionId, nowMillis)
		default:
			if item.PossessionType != 0 {
				store.GrantPossession(user, model.PossessionType(item.PossessionType), item.PossessionId, item.Count)
			}
		}
	}
	return dupInfos
}

// grantOverflowWeapon handles a drawn weapon that does not fit into the
// weapon inventory (cap 999): SSR weapons are mailed to the gift box so the
// player can claim them after freeing space, everything below SSR is
// auto-sold for its regular sell price.
func (h *GachaHandler) grantOverflowWeapon(user *store.UserState, item DrawnItem, nowMillis int64) {
	if item.RarityType >= model.RaritySSRare {
		user.AddGift(store.NotReceivedGiftState{
			GiftCommon: store.GiftCommonState{
				PossessionType: int32(model.PossessionTypeWeapon),
				PossessionId:   item.PossessionId,
				Count:          item.Count,
				GrantDatetime:  nowMillis,
			},
			ExpirationDatetime: nowMillis + int64(30*24*time.Hour/time.Millisecond),
			UserGiftUuid:       uuid.New().String(),
		})
		log.Printf("[GachaHandler] weapon inventory full: mailed SSR weapon %d to gift box", item.PossessionId)
		return
	}

	var price int32
	if h.WeaponSellPrice != nil {
		price = h.WeaponSellPrice[item.PossessionId]
	}
	if price > 0 && h.Config != nil {
		user.ConsumableItems[h.Config.ConsumableItemIdForGold] += price
	}
	log.Printf("[GachaHandler] weapon inventory full: auto-sold weapon %d (rarity %d) for %d gold",
		item.PossessionId, item.RarityType, price)
}

// rollDupGrade picks a dup-exchange grade weighted by model.DupGradeWeights:
// grade 1 (best payout) is the rarest, grade 5 the most common.
func rollDupGrade() int32 {
	total := 0
	for grade := model.DupGradeMin; grade <= model.DupGradeMax; grade++ {
		total += model.DupGradeWeights[grade]
	}
	roll := rand.Intn(total)
	for grade := model.DupGradeMin; grade <= model.DupGradeMax; grade++ {
		roll -= model.DupGradeWeights[grade]
		if roll < 0 {
			return grade
		}
	}
	return model.DupGradeMax
}

func (h *GachaHandler) tryCostumeDupExchange(user *store.UserState, item DrawnItem, index int) (DuplicateInfo, bool) {
	for _, c := range user.Costumes {
		if c.CostumeId == item.PossessionId {
			grade := rollDupGrade()
			exchanges := h.DupExchange[item.PossessionId]
			bonuses := make([]model.DupExchangeEntry, 0, len(exchanges))
			for _, ex := range exchanges {
				// The rolled grade scales the dup payout: grade 1 grants 200% of
				// the base count (20 books), grade 5 only 100% (10).
				count := ex.Count * model.DupGradePayoutPercent[grade] / 100
				if count < 1 {
					count = 1
				}
				store.GrantPossession(user, model.PossessionType(ex.PossessionType), ex.PossessionId, count)
				bonuses = append(bonuses, model.DupExchangeEntry{
					PossessionType: ex.PossessionType,
					PossessionId:   ex.PossessionId,
					Count:          count,
				})
			}
			return DuplicateInfo{Index: index, Grade: grade, Bonuses: bonuses}, true
		}
	}
	return DuplicateInfo{}, false
}

func (h *GachaHandler) generateBonusItems(entry store.GachaCatalogEntry, mainItems []DrawnItem) map[int]DrawnItem {
	bonus := make(map[int]DrawnItem)
	for i, item := range mainItems {
		if item.PossessionType != int32(model.PossessionTypeCostume) {
			continue
		}
		wid, ok := h.Pool.CostumeWeaponMap[item.PossessionId]
		if !ok {
			continue
		}
		w, ok := h.Pool.WeaponById[wid]
		if !ok {
			continue
		}
		bonus[i] = DrawnItem{
			PossessionType: w.PossessionType,
			PossessionId:   w.PossessionId,
			RarityType:     w.RarityType,
			Count:          1,
		}
	}
	return bonus
}

func findPhase(entry store.GachaCatalogEntry, phaseId int32) (store.GachaPricePhaseEntry, error) {
	for _, p := range entry.PricePhases {
		if p.PhaseId == phaseId {
			return p, nil
		}
	}
	if len(entry.PricePhases) > 0 {
		log.Printf("[GachaHandler] phase %d not found for gacha %d, using first phase", phaseId, entry.GachaId)
		return entry.PricePhases[0], nil
	}
	return store.GachaPricePhaseEntry{}, fmt.Errorf("no price phases for gacha %d", entry.GachaId)
}
