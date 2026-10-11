package gacha

import (
	"log"
	"math/rand"

	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/model"
)

type RateTier struct {
	Weight         int
	PossessionType int32
	RarityType     model.RarityType
}

type DrawnItem struct {
	PossessionType int32
	PossessionId   int32
	RarityType     model.RarityType
	CharacterId    int32
	Count          int32
}

// OwnedSets tracks items the player already owns so premium draws can avoid
// handing out duplicate high-rarity entries.
type OwnedSets struct {
	Costumes map[int32]bool
	Weapons  map[int32]bool
}

var premiumRates = []RateTier{
	{200, int32(model.PossessionTypeCostume), model.RaritySSRare},
	{300, int32(model.PossessionTypeWeapon), model.RaritySSRare},
	{500, int32(model.PossessionTypeCostume), model.RaritySRare},
	{1000, int32(model.PossessionTypeWeapon), model.RaritySRare},
	{8000, int32(model.PossessionTypeWeapon), model.RarityRare},
}

func DrawPremium(bp *masterdata.BannerPool, count int, fixedRarityMin int32, fixedCount int, rateMultiplier float64, owned *OwnedSets) []DrawnItem {
	result := make([]DrawnItem, 0, count)
	rates := adjustRates(premiumRates, rateMultiplier)
	totalWeight := 0
	for _, r := range rates {
		totalWeight += r.Weight
	}

	// Batch-local copies so replacements handed out within this single
	// multi-draw are treated as owned by later slots.
	var batchCostumes, batchWeapons map[int32]bool
	if owned != nil {
		batchCostumes = make(map[int32]bool, len(owned.Costumes))
		for id := range owned.Costumes {
			batchCostumes[id] = true
		}
		batchWeapons = make(map[int32]bool, len(owned.Weapons))
		for id := range owned.Weapons {
			batchWeapons[id] = true
		}
	}

	for i := range count {
		isGuaranteeSlot := fixedCount > 0 && i >= count-fixedCount
		item := rollOne(bp, rates, totalWeight)

		if isGuaranteeSlot && item.RarityType < fixedRarityMin {
			item = rollAtMinRarity(bp, rates, fixedRarityMin)
		}
		if owned != nil {
			item = dedupeHighRarity(bp, batchCostumes, batchWeapons, item)
		}
		result = append(result, item)
	}
	return result
}

// dedupeHighRarity replaces a drawn SSR+ (rarity >= 40) costume or weapon the
// player already owns with a random unowned item of the same type and rarity.
// If every candidate in that bucket is already owned, the original draw
// stands (duplicates then go through the normal dup exchange).
func dedupeHighRarity(bp *masterdata.BannerPool, batchCostumes, batchWeapons map[int32]bool, item DrawnItem) DrawnItem {
	if item.RarityType < model.RaritySSRare {
		return item
	}

	switch model.PossessionType(item.PossessionType) {
	case model.PossessionTypeCostume:
		if !batchCostumes[item.PossessionId] {
			batchCostumes[item.PossessionId] = true
			return item
		}
		var candidates []masterdata.GachaPoolItem
		for _, cand := range bp.CostumesByRarity[item.RarityType] {
			if !batchCostumes[cand.PossessionId] {
				candidates = append(candidates, cand)
			}
		}
		if len(candidates) == 0 {
			return item
		}
		pick := candidates[rand.Intn(len(candidates))]
		batchCostumes[pick.PossessionId] = true
		return DrawnItem{
			PossessionType: pick.PossessionType,
			PossessionId:   pick.PossessionId,
			RarityType:     pick.RarityType,
			CharacterId:    pick.CharacterId,
		}
	case model.PossessionTypeWeapon:
		if !batchWeapons[item.PossessionId] {
			batchWeapons[item.PossessionId] = true
			return item
		}
		var candidates []masterdata.GachaPoolItem
		for _, cand := range bp.WeaponsByRarity[item.RarityType] {
			if !batchWeapons[cand.PossessionId] {
				candidates = append(candidates, cand)
			}
		}
		if len(candidates) == 0 {
			return item
		}
		pick := candidates[rand.Intn(len(candidates))]
		batchWeapons[pick.PossessionId] = true
		return DrawnItem{
			PossessionType: pick.PossessionType,
			PossessionId:   pick.PossessionId,
			RarityType:     pick.RarityType,
		}
	default:
		return item
	}
}

func DrawBox(items []BoxItem, count int) []DrawnItem {
	var available []int
	for i, item := range items {
		remaining := item.MaxCount - item.DrewCount
		for range remaining {
			available = append(available, i)
		}
	}

	result := make([]DrawnItem, 0, count)
	for i := 0; i < count && len(available) > 0; i++ {
		pick := rand.Intn(len(available))
		idx := available[pick]
		item := items[idx]
		result = append(result, DrawnItem{
			PossessionType: item.PossessionType,
			PossessionId:   item.PossessionId,
			RarityType:     item.RarityType,
			Count:          1,
		})
		items[idx].DrewCount++
		available = append(available[:pick], available[pick+1:]...)
	}
	return result
}

func DrawReward(materials []masterdata.GachaPoolItem, count int) []DrawnItem {
	if len(materials) == 0 {
		return nil
	}
	result := make([]DrawnItem, 0, count)
	for range count {
		m := materials[rand.Intn(len(materials))]
		result = append(result, DrawnItem{
			PossessionType: m.PossessionType,
			PossessionId:   m.PossessionId,
			RarityType:     m.RarityType,
			Count:          1,
		})
	}
	return result
}

// DrawChapterWeighted selects materials based on their weights (for Chapter gacha)
func DrawChapterWeighted(materials []masterdata.GachaPoolItem, count int) []DrawnItem {
	if len(materials) == 0 {
		return nil
	}

	// Compute the total weight
	totalWeight := int32(0)
	for _, m := range materials {
		totalWeight += m.Weight
	}

	if totalWeight == 0 {
		// If all weights are 0, return an empty result
		return nil
	}

	result := make([]DrawnItem, 0, count)
	for range count {
		roll := rand.Int31n(totalWeight)
		cumulative := int32(0)

		for _, m := range materials {
			cumulative += m.Weight
			if roll < cumulative {
				result = append(result, DrawnItem{
					PossessionType: m.PossessionType,
					PossessionId:   m.PossessionId,
					RarityType:     m.RarityType,
					Count:          m.Count,
				})
				break
			}
		}
	}
	return result
}

type BoxItem struct {
	PossessionType int32
	PossessionId   int32
	RarityType     model.RarityType
	Count          int32
	MaxCount       int32
	DrewCount      int32
	IsTarget       bool
}

func adjustRates(base []RateTier, multiplier float64) []RateTier {
	if multiplier == 1.0 || multiplier == 0 {
		return base
	}
	adjusted := make([]RateTier, len(base))
	copy(adjusted, base)

	var fourStarExtra int
	var nonFourStar int
	for i, r := range adjusted {
		if r.RarityType >= model.RaritySSRare {
			extra := int(float64(r.Weight) * (multiplier - 1.0))
			adjusted[i].Weight += extra
			fourStarExtra += extra
		} else {
			nonFourStar += r.Weight
		}
	}
	if nonFourStar > 0 && fourStarExtra > 0 {
		for i, r := range adjusted {
			if r.RarityType < model.RaritySSRare {
				reduction := fourStarExtra * r.Weight / nonFourStar
				adjusted[i].Weight -= reduction
				if adjusted[i].Weight < 1 {
					adjusted[i].Weight = 1
				}
			}
		}
	}
	return adjusted
}

func rollOne(bp *masterdata.BannerPool, rates []RateTier, totalWeight int) DrawnItem {
	roll := rand.Intn(totalWeight)
	cumulative := 0
	var tier RateTier
	for _, r := range rates {
		cumulative += r.Weight
		if roll < cumulative {
			tier = r
			break
		}
	}

	if item, ok := tryFeaturedRateUp(bp, tier); ok {
		return item
	}
	return pickFromPool(bp, tier.PossessionType, tier.RarityType)
}

func tryFeaturedRateUp(bp *masterdata.BannerPool, tier RateTier) (DrawnItem, bool) {
	var matches []masterdata.GachaPoolItem
	for _, f := range bp.Featured {
		if f.PossessionType == tier.PossessionType && f.RarityType == tier.RarityType {
			matches = append(matches, f)
		}
	}
	if len(matches) == 0 {
		return DrawnItem{}, false
	}
	if rand.Intn(model.FeaturedRateUpDenom) >= model.FeaturedRateUpPercent {
		return DrawnItem{}, false
	}
	f := matches[rand.Intn(len(matches))]
	return DrawnItem{
		PossessionType: f.PossessionType,
		PossessionId:   f.PossessionId,
		RarityType:     f.RarityType,
		CharacterId:    f.CharacterId,
	}, true
}

func rollAtMinRarity(bp *masterdata.BannerPool, rates []RateTier, minRarity model.RarityType) DrawnItem {
	var filtered []RateTier
	filteredTotal := 0
	for _, r := range rates {
		if r.RarityType >= minRarity {
			filtered = append(filtered, r)
			filteredTotal += r.Weight
		}
	}
	if filteredTotal == 0 {
		return pickFromPool(bp, int32(model.PossessionTypeWeapon), minRarity)
	}
	return rollOne(bp, filtered, filteredTotal)
}

func pickFromPool(bp *masterdata.BannerPool, possessionType int32, rarityType model.RarityType) DrawnItem {
	if possessionType == int32(model.PossessionTypeCostume) {
		items := bp.CostumesByRarity[rarityType]
		if len(items) == 0 {
			items = bp.CostumesByRarity[model.RaritySSRare]
		}
		if len(items) == 0 {
			log.Printf("[pickFromPool] empty costume pool for rarity=%d, returning phantom item", rarityType)
			return DrawnItem{PossessionType: int32(model.PossessionTypeWeapon), RarityType: rarityType}
		}
		pick := items[rand.Intn(len(items))]
		return DrawnItem{
			PossessionType: pick.PossessionType,
			PossessionId:   pick.PossessionId,
			RarityType:     pick.RarityType,
			CharacterId:    pick.CharacterId,
		}
	}

	items := bp.WeaponsByRarity[rarityType]
	if len(items) == 0 {
		items = bp.WeaponsByRarity[model.RarityRare]
	}
	if len(items) == 0 {
		log.Printf("[pickFromPool] empty weapon pool for rarity=%d, returning phantom item", rarityType)
		return DrawnItem{PossessionType: int32(model.PossessionTypeWeapon), RarityType: rarityType}
	}
	pick := items[rand.Intn(len(items))]
	return DrawnItem{
		PossessionType: pick.PossessionType,
		PossessionId:   pick.PossessionId,
		RarityType:     pick.RarityType,
	}
}
