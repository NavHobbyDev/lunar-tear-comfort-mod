package masterdata

import (
	"fmt"
	"sort"
	"strings"

	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
	"lunar-tear/server/internal/utils"
)

type GachaMedalInfo struct {
	GachaMedalId        int32
	ConsumableItemId    int32
	AutoConvertDatetime int64
	ConversionRate      int32
}

const chapterGachaIdBase int32 = 200000

func LoadGachaCatalog() ([]store.GachaCatalogEntry, map[int32]GachaMedalInfo, error) {
	medals, err := utils.ReadTable[EntityMGachaMedal]("m_gacha_medal")
	if err != nil {
		return nil, nil, fmt.Errorf("load gacha medal table: %w", err)
	}
	banners, err := utils.ReadTable[EntityMMomBanner]("m_mom_banner")
	if err != nil {
		return nil, nil, fmt.Errorf("load mom banner table: %w", err)
	}

	gachaToMedal := make(map[int32]EntityMGachaMedal)
	medalInfoByGacha := make(map[int32]GachaMedalInfo)
	for _, m := range medals {
		gachaToMedal[m.ShopTransitionGachaId] = m
		medalInfoByGacha[m.ShopTransitionGachaId] = GachaMedalInfo{
			GachaMedalId:        m.GachaMedalId,
			ConsumableItemId:    m.ConsumableItemId,
			AutoConvertDatetime: m.AutoConvertDatetime,
			ConversionRate:      m.ConversionRate,
		}
	}

	stepupSteps := make(map[int32][]EntityMMomBanner)
	var entries []store.GachaCatalogEntry
	entries = append(entries, buildChapterGachaEntries()...)

	for _, b := range banners {
		if b.DestinationDomainType != model.MomBannerDomainGacha {
			continue
		}
		gachaId := b.DestinationDomainId

		if strings.HasPrefix(b.BannerAssetName, model.BannerPrefixStepUp) {
			if _, hasMedal := gachaToMedal[gachaId]; !hasMedal {
				continue
			}
			groupId := gachaId / model.StepUpGroupDivisor
			stepupSteps[groupId] = append(stepupSteps[groupId], b)
			continue
		}

		labelType := model.GachaLabelPremium
		modeType := model.GachaModeBasic
		decoration := model.GachaDecorationNormal

		isChapter := false
		isEvent := strings.EqualFold(b.BannerAssetName, "event") || strings.HasPrefix(strings.ToLower(b.BannerAssetName), "event_")

		if strings.HasPrefix(b.BannerAssetName, model.BannerPrefixLimited) {
			decoration = model.GachaDecorationFestival
		}
		if isChapter {
			labelType = model.GachaLabelChapter
			modeType = model.GachaModeBox
		} else if isEvent {
			labelType = model.GachaLabelEvent
			modeType = model.GachaModeBox
			decoration = model.GachaDecorationFestival
		}

		medal, hasMedal := gachaToMedal[gachaId]
		if !hasMedal && !isChapter && !isEvent {
			continue
		}
		var medalId int32
		var medalConsumableId int32
		var ceilingCount int32
		if hasMedal {
			medalId = medal.GachaMedalId
			medalConsumableId = medal.ConsumableItemId
			ceilingCount = model.PityCeilingCount
		}

		var pricePhases []store.GachaPricePhaseEntry
		if isEvent {
			pricePhases = buildEventPricePhases(gachaId)
		} else {
			pricePhases = buildPremiumBasicPricePhases(gachaId)
		}

		entries = append(entries, store.GachaCatalogEntry{
			GachaId:               gachaId,
			GachaLabelType:        labelType,
			GachaModeType:         modeType,
			GachaAutoResetType:    model.GachaAutoResetNone,
			IsUserGachaUnlock:     true,
			StartDatetime:         b.StartDatetime,
			EndDatetime:           b.EndDatetime,
			GachaMedalId:          medalId,
			MedalConsumableItemId: medalConsumableId,
			GachaDecorationType:   decoration,
			SortOrder:             b.SortOrderDesc,
			BannerAssetName:       b.BannerAssetName,
			GroupId:               gachaId,
			CeilingCount:          ceilingCount,
			PricePhases:           pricePhases,
		})
	}

	for _, steps := range stepupSteps {
		first := steps[0]
		gachaId := first.DestinationDomainId

		medal := gachaToMedal[first.DestinationDomainId]
		medalId := medal.GachaMedalId
		medalConsumableId := medal.ConsumableItemId

		pricePhases := buildStepUpPricePhases(gachaId, len(steps))

		var maxStep int32
		for _, p := range pricePhases {
			if p.StepNumber > maxStep {
				maxStep = p.StepNumber
			}
		}

		entries = append(entries, store.GachaCatalogEntry{
			GachaId:               gachaId,
			GachaLabelType:        model.GachaLabelPremium,
			GachaModeType:         model.GachaModeStepup,
			GachaAutoResetType:    model.GachaAutoResetNone,
			IsUserGachaUnlock:     true,
			StartDatetime:         first.StartDatetime,
			EndDatetime:           first.EndDatetime,
			GachaMedalId:          medalId,
			MedalConsumableItemId: medalConsumableId,
			GachaDecorationType:   model.GachaDecorationFestival,
			SortOrder:             first.SortOrderDesc,
			BannerAssetName:       first.BannerAssetName,
			GroupId:               gachaId,
			CeilingCount:          model.PityCeilingCount,
			PricePhases:           pricePhases,
			MaxStepNumber:         maxStep,
		})
	}

	return entries, medalInfoByGacha, nil
}

const chapterPromoMaxItems = 4
const maxSlideFeatured = 13

func EnrichCatalogPromotions(entries []store.GachaCatalogEntry, pool *GachaCatalog) {
	for i := range entries {
		if entries[i].GachaLabelType == model.GachaLabelChapter {
			entries[i].PromotionItems = buildChapterPromotionItems(entries[i].GachaId, pool)
			continue
		}

		featured := pool.FeaturedByGacha[entries[i].GachaId]

		maxRarity := int32(0)
		for _, c := range featured.Costumes {
			if c.RarityType > maxRarity {
				maxRarity = c.RarityType
			}
		}
		for _, w := range featured.Weapons {
			if w.RarityType > maxRarity {
				maxRarity = w.RarityType
			}
		}

		var topCostumes []GachaPoolItem
		for _, c := range featured.Costumes {
			if c.RarityType == maxRarity {
				topCostumes = append(topCostumes, c)
			}
		}
		var topWeapons []GachaPoolItem
		for _, w := range featured.Weapons {
			if w.RarityType == maxRarity {
				topWeapons = append(topWeapons, w)
			}
		}

		if len(topCostumes)+len(topWeapons) > maxSlideFeatured {
			topCostumes = topCostumes[:min(3, len(topCostumes))]
			topWeapons = topWeapons[:min(2, len(topWeapons))]
		}

		var items []store.GachaPromotionItem
		if entries[i].GachaModeType == model.GachaModeStepup && len(topCostumes) > 0 {
			items = append(items, toPromoItemWithBonus(topCostumes[0], pool))
			wid := pool.CostumeWeaponMap[topCostumes[0].PossessionId]
			items = append(items, toPromoItem(pool.WeaponById[wid]))
		} else {
			for _, c := range topCostumes {
				items = append(items, toPromoItemWithBonus(c, pool))
			}
			for _, w := range topWeapons {
				items = append(items, toPromoItemWithBonus(w, pool))
			}
		}

		entries[i].PromotionItems = items
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].SortOrder != entries[j].SortOrder {
			return entries[i].SortOrder < entries[j].SortOrder
		}
		return entries[i].GachaId < entries[j].GachaId
	})
}

func toPromoItem(item GachaPoolItem) store.GachaPromotionItem {
	return store.GachaPromotionItem{
		PossessionType: item.PossessionType,
		PossessionId:   item.PossessionId,
		IsTarget:       item.IsFeatured,
		MaxCount:       item.MaxCount,
		IsJackpot:      item.IsJackpot,
		Count:          item.Count,
		RarityType:     int32(item.RarityType),
	}
}

func toPromoItemWithBonus(item GachaPoolItem, pool *GachaCatalog) store.GachaPromotionItem {
	pi := store.GachaPromotionItem{
		PossessionType: item.PossessionType,
		PossessionId:   item.PossessionId,
		IsTarget:       item.IsFeatured,
		MaxCount:       item.MaxCount,
		IsJackpot:      item.IsJackpot,
		Count:          item.Count,
		RarityType:     int32(item.RarityType),
	}
	if item.PossessionType == int32(model.PossessionTypeCostume) {
		pi.BonusPossessionType = int32(model.PossessionTypeWeapon)
		pi.BonusPossessionId = pool.CostumeWeaponMap[item.PossessionId]
	}
	return pi
}

func buildChapterPromotionItems(gachaId int32, pool *GachaCatalog) []store.GachaPromotionItem {
	var materials []GachaPoolItem
	if chapterItems, ok := pool.ChapterGachaItems[gachaId]; ok && len(chapterItems) > 0 {
		materials = chapterItems
	} else {
		materials = pool.Materials
	}

	limit := 0
	for _, m := range materials {
		if m.IsFeatured && limit < chapterPromoMaxItems {
			limit++
		}
	}

	if limit == 0 {
		limit = min(chapterPromoMaxItems, len(materials))
		items := make([]store.GachaPromotionItem, 0, limit)
		for _, m := range materials[:limit] {
			item := toPromoItem(m)
			items = append(items, item)
		}
		return items
	}

	items := make([]store.GachaPromotionItem, 0, limit)
	count := 0
	for _, m := range materials {
		if m.IsFeatured && count < chapterPromoMaxItems {
			item := toPromoItem(m)
			items = append(items, item)
			count++
		}
	}
	return items
}

func buildPremiumBasicPricePhases(gachaId int32) []store.GachaPricePhaseEntry {
	return []store.GachaPricePhaseEntry{
		{
			PhaseId:      gachaId*model.PhaseIdMultiplier + 1,
			PriceType:    model.PriceTypeGem,
			Price:        model.PremiumSinglePullPrice,
			RegularPrice: model.PremiumSinglePullPrice,
			DrawCount:    1,
		},
		{
			PhaseId:        gachaId*model.PhaseIdMultiplier + 2,
			PriceType:      model.PriceTypeGem,
			Price:          model.PremiumMultiPullPrice,
			RegularPrice:   model.PremiumMultiPullPrice,
			DrawCount:      model.PremiumMultiPullCount,
			FixedRarityMin: model.RaritySRare,
			FixedCount:     1,
		},
		{
			PhaseId:      gachaId*model.PhaseIdMultiplier + 3,
			PriceType:    model.PriceTypeConsumableItem,
			PriceId:      model.ConsumableIdPremiumTicket,
			Price:        1,
			RegularPrice: 1,
			DrawCount:    1,
		},
	}
}

func buildStepUpPricePhases(gachaId int32, totalSteps int) []store.GachaPricePhaseEntry {
	stepCosts := []int32{model.StepUpStep1Cost, model.StepUpFreeCost, model.StepUpStep3Cost, model.StepUpFreeCost, model.StepUpStep5Cost}
	stepCosts = stepCosts[:min(totalSteps, len(stepCosts))]

	var phases []store.GachaPricePhaseEntry
	for i, cost := range stepCosts {
		step := int32(i + 1)
		priceType := model.PriceTypePaidGem
		if cost == 0 {
			priceType = model.PriceTypeGem
		}

		fixedRarityMin := int32(0)
		fixedCount := int32(0)
		if step == int32(len(stepCosts)) {
			fixedRarityMin = model.RaritySSRare
			fixedCount = 1
		}

		phases = append(phases, store.GachaPricePhaseEntry{
			PhaseId:        gachaId*model.PhaseIdMultiplier + step,
			PriceType:      priceType,
			Price:          cost,
			RegularPrice:   model.PremiumMultiPullPrice,
			DrawCount:      model.PremiumMultiPullCount,
			FixedRarityMin: fixedRarityMin,
			FixedCount:     fixedCount,
			LimitExecCount: 1,
			StepNumber:     step,
		})
	}
	return phases
}

func buildChapterPricePhases(gachaId, ticketId int32) []store.GachaPricePhaseEntry {
	return []store.GachaPricePhaseEntry{
		{
			PhaseId:      gachaId*model.PhaseIdMultiplier + 1,
			PriceType:    model.PriceTypeConsumableItem,
			PriceId:      ticketId,
			Price:        1,
			RegularPrice: 0,
			DrawCount:    1,
		},
		{
			PhaseId:      gachaId*model.PhaseIdMultiplier + 2,
			PriceType:    model.PriceTypeConsumableItem,
			PriceId:      ticketId,
			Price:        10,
			RegularPrice: 0,
			DrawCount:    model.PremiumMultiPullCount,
		},
	}
}

func buildEventPricePhases(gachaId int32) []store.GachaPricePhaseEntry {
	return []store.GachaPricePhaseEntry{
		{
			PhaseId:      gachaId*model.PhaseIdMultiplier + 1,
			PriceType:    model.PriceTypeConsumableItem,
			PriceId:      model.ConsumableIdPremiumTicket,
			Price:        1,
			RegularPrice: 1,
			DrawCount:    1,
		},
		{
			PhaseId:      gachaId*model.PhaseIdMultiplier + 2,
			PriceType:    model.PriceTypeConsumableItem,
			PriceId:      model.ConsumableIdPremiumTicket,
			Price:        10,
			RegularPrice: 10,
			DrawCount:    model.PremiumMultiPullCount,
		},
	}
}
