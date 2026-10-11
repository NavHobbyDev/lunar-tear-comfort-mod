package masterdata

import (
	"fmt"
	"log"
	"sort"

	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/utils"
)

type PartsStatusMainDef struct {
	StatusKindType            int32
	StatusCalculationType     int32
	StatusChangeInitialValue  int32
	StatusNumericalFunctionId int32
}

type PartsCatalog struct {
	PartsById                            map[int32]EntityMParts
	DefaultPartsStatusMainByLotteryGroup map[int32]int32
	RarityByRarityType                   map[model.RarityType]EntityMPartsRarity
	RateByGroupAndLevel                  map[int32]map[int32]int32
	PriceByGroupAndLevel                 map[int32]map[int32]int32
	SellPriceByRarity                    map[model.RarityType]NumericalFunc

	// SetGroupIdsByGroupId maps a PartsGroupId to every group of its memoir
	// set. A series spans several set generations (e.g. groups 1-3, 401-403,
	// 437-439 all sit in series 1), so a set is a run of consecutive group ids
	// within one series.
	SetGroupIdsByGroupId map[int32][]int32

	PartsStatusMainById map[int32]PartsStatusMainDef
	SubStatusPool       map[int32][]int32                // lotteryGroupId -> eligible PartsStatusMainIds
	SubStatusUnlockLvls map[model.RarityType][]int32     // rarity -> levels where sub-slots unlock
	EnhanceProbability  []EntityMPartsEnhanceProbability // enhancement patterns
	FuncResolver        *FunctionResolver
}

func LoadPartsCatalog() (*PartsCatalog, error) {
	partsRows, err := utils.ReadTable[EntityMParts]("m_parts")
	if err != nil {
		return nil, fmt.Errorf("load parts table: %w", err)
	}

	rarityRows, err := utils.ReadTable[EntityMPartsRarity]("m_parts_rarity")
	if err != nil {
		return nil, fmt.Errorf("load parts rarity table: %w", err)
	}

	groupRows, err := utils.ReadTable[EntityMPartsGroup]("m_parts_group")
	if err != nil {
		return nil, fmt.Errorf("load parts group table: %w", err)
	}

	rateRows, err := utils.ReadTable[EntityMPartsLevelUpRateGroup]("m_parts_level_up_rate_group")
	if err != nil {
		return nil, fmt.Errorf("load parts level up rate table: %w", err)
	}

	priceRows, err := utils.ReadTable[EntityMPartsLevelUpPriceGroup]("m_parts_level_up_price_group")
	if err != nil {
		return nil, fmt.Errorf("load parts level up price table: %w", err)
	}

	var enhanceProbRows []EntityMPartsEnhanceProbability
	enhanceProbRows, err = utils.ReadTable[EntityMPartsEnhanceProbability]("m_parts_enhance_probability")
	if err != nil {
		// Table doesn't exist, use default probabilities
		enhanceProbRows = []EntityMPartsEnhanceProbability{
			{EnhancePatternId: 1, EnhanceMain: 1, EnhanceSubCount: 0, ProbabilityPermil: 5000}, // 50% - main only
			{EnhancePatternId: 2, EnhanceMain: 1, EnhanceSubCount: 1, ProbabilityPermil: 3000}, // 30% - main + 1 sub
			{EnhancePatternId: 3, EnhanceMain: 1, EnhanceSubCount: 2, ProbabilityPermil: 1200}, // 12% - main + 2 sub
			{EnhancePatternId: 4, EnhanceMain: 0, EnhanceSubCount: 1, ProbabilityPermil: 500},  // 5% - 1 sub only
			{EnhancePatternId: 5, EnhanceMain: 1, EnhanceSubCount: 3, ProbabilityPermil: 200},  // 2% - main + 3 sub
			{EnhancePatternId: 6, EnhanceMain: 0, EnhanceSubCount: 2, ProbabilityPermil: 80},   // 0.8% - 2 sub only
			{EnhancePatternId: 7, EnhanceMain: 1, EnhanceSubCount: 4, ProbabilityPermil: 20},   // 0.2% - main + 4 sub
		}
		log.Printf("[PartsCatalog] Using default enhancement probabilities (table not found)")
	} else if len(enhanceProbRows) == 0 {
		// Table exists but is empty, use default probabilities
		enhanceProbRows = []EntityMPartsEnhanceProbability{
			{EnhancePatternId: 1, EnhanceMain: 1, EnhanceSubCount: 0, ProbabilityPermil: 5000}, // 50% - main only
			{EnhancePatternId: 2, EnhanceMain: 1, EnhanceSubCount: 1, ProbabilityPermil: 3000}, // 30% - main + 1 sub
			{EnhancePatternId: 3, EnhanceMain: 1, EnhanceSubCount: 2, ProbabilityPermil: 1200}, // 12% - main + 2 sub
			{EnhancePatternId: 4, EnhanceMain: 0, EnhanceSubCount: 1, ProbabilityPermil: 500},  // 5% - 1 sub only
			{EnhancePatternId: 5, EnhanceMain: 1, EnhanceSubCount: 3, ProbabilityPermil: 200},  // 2% - main + 3 sub
			{EnhancePatternId: 6, EnhanceMain: 0, EnhanceSubCount: 2, ProbabilityPermil: 80},   // 0.8% - 2 sub only
			{EnhancePatternId: 7, EnhanceMain: 1, EnhanceSubCount: 4, ProbabilityPermil: 20},   // 0.2% - main + 4 sub
		}
		log.Printf("[PartsCatalog] Using default enhancement probabilities (table was empty)")
	}

	partsById := make(map[int32]EntityMParts, len(partsRows))
	for _, p := range partsRows {
		partsById[p.PartsId] = p
	}

	// Split each series into memoir sets: runs of consecutive group ids.
	groupIdsBySeries := make(map[int32][]int32)
	for _, g := range groupRows {
		groupIdsBySeries[g.PartsSeriesId] = append(groupIdsBySeries[g.PartsSeriesId], g.PartsGroupId)
	}
	setGroupIdsByGroupId := make(map[int32][]int32, len(groupRows))
	for _, ids := range groupIdsBySeries {
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		start := 0
		for i := 1; i <= len(ids); i++ {
			if i == len(ids) || ids[i] != ids[i-1]+1 {
				run := ids[start:i]
				for _, id := range run {
					setGroupIdsByGroupId[id] = run
				}
				start = i
			}
		}
	}

	// Lottery group ID encodes tier (first digit 1-4) and stat category
	// (second digit 1-6). Formula: mainStatId = (category - 1) * 4 + tier.
	defaultPartsStatusMainByLotteryGroup := make(map[int32]int32, 24)
	for tier := int32(1); tier <= 4; tier++ {
		for cat := int32(1); cat <= 6; cat++ {
			groupId := tier*10 + cat
			mainStatId := (cat-1)*4 + tier
			defaultPartsStatusMainByLotteryGroup[groupId] = mainStatId
		}
	}

	funcResolver, err := LoadFunctionResolver()
	if err != nil {
		return nil, fmt.Errorf("load function resolver: %w", err)
	}

	rarityByRarityType := make(map[model.RarityType]EntityMPartsRarity, len(rarityRows))
	sellPriceByRarity := make(map[model.RarityType]NumericalFunc, len(rarityRows))
	for _, r := range rarityRows {
		rarityByRarityType[r.RarityType] = r
		if f, ok := funcResolver.Resolve(r.SellPriceNumericalFunctionId); ok {
			sellPriceByRarity[r.RarityType] = f
		}
	}

	rateByGroupAndLevel := make(map[int32]map[int32]int32)
	for _, r := range rateRows {
		if rateByGroupAndLevel[r.PartsLevelUpRateGroupId] == nil {
			rateByGroupAndLevel[r.PartsLevelUpRateGroupId] = make(map[int32]int32)
		}
		rateByGroupAndLevel[r.PartsLevelUpRateGroupId][r.LevelLowerLimit] = r.SuccessRatePermil
	}

	priceByGroupAndLevel := make(map[int32]map[int32]int32)
	for _, p := range priceRows {
		if priceByGroupAndLevel[p.PartsLevelUpPriceGroupId] == nil {
			priceByGroupAndLevel[p.PartsLevelUpPriceGroupId] = make(map[int32]int32)
		}
		priceByGroupAndLevel[p.PartsLevelUpPriceGroupId][p.LevelLowerLimit] = p.Gold
	}

	partsStatusMainById, subStatusPool := buildPartsStatusMain()

	unlockLvls := []int32{3, 6, 9, 12}
	subStatusUnlockLvls := map[model.RarityType][]int32{
		model.RarityNormal: unlockLvls,
		model.RarityRare:   unlockLvls,
		model.RaritySRare:  unlockLvls,
		model.RaritySSRare: unlockLvls,
	}

	return &PartsCatalog{
		PartsById:                            partsById,
		DefaultPartsStatusMainByLotteryGroup: defaultPartsStatusMainByLotteryGroup,
		RarityByRarityType:                   rarityByRarityType,
		RateByGroupAndLevel:                  rateByGroupAndLevel,
		PriceByGroupAndLevel:                 priceByGroupAndLevel,
		SellPriceByRarity:                    sellPriceByRarity,
		SetGroupIdsByGroupId:                 setGroupIdsByGroupId,
		PartsStatusMainById:                  partsStatusMainById,
		SubStatusPool:                        subStatusPool,
		SubStatusUnlockLvls:                  subStatusUnlockLvls,
		EnhanceProbability:                   enhanceProbRows,
		FuncResolver:                         funcResolver,
	}, nil
}

// buildPartsStatusMain constructs the 36 PartsStatusMain definitions and
// groups them into sub-status lottery pools by tier (1-4).
// The data mirrors EntityMPartsStatusMainTable.json which is structured as
// 9 stat categories x 4 tiers. Tier within each category maps to the
// PartsStatusSubLotteryGroupId on the part definition.
func buildPartsStatusMain() (map[int32]PartsStatusMainDef, map[int32][]int32) {
	type statCat struct {
		kindType  int32
		calcType  int32
		initVals  [4]int32
		funcStart int32
	}
	cats := []statCat{
		{2, 1, [4]int32{50, 100, 150, 250}, 101},     // Attack flat
		{7, 1, [4]int32{50, 100, 150, 250}, 101},     // Vitality flat
		{2, 2, [4]int32{10, 30, 70, 120}, 105},       // Attack %
		{7, 2, [4]int32{10, 30, 70, 120}, 105},       // Vitality %
		{6, 2, [4]int32{10, 30, 70, 120}, 105},       // HP %
		{6, 1, [4]int32{600, 1200, 1800, 3000}, 109}, // HP flat
		{4, 1, [4]int32{10, 30, 70, 120}, 113},       // CritRatio
		{3, 1, [4]int32{20, 50, 80, 100}, 117},       // CritAttack
		{1, 1, [4]int32{10, 20, 30, 40}, 121},        // Agility
	}

	defs := make(map[int32]PartsStatusMainDef, 36)
	pool := map[int32][]int32{1: {}, 2: {}, 3: {}, 4: {}}
	id := int32(1)
	for _, c := range cats {
		for tier := 0; tier < 4; tier++ {
			defs[id] = PartsStatusMainDef{
				StatusKindType:            c.kindType,
				StatusCalculationType:     c.calcType,
				StatusChangeInitialValue:  c.initVals[tier],
				StatusNumericalFunctionId: c.funcStart + int32(tier),
			}
			pool[int32(tier+1)] = append(pool[int32(tier+1)], id)
			id++
		}
	}
	// Newer parts groups (PartsGroupId 401-490) use PartsStatusSubLotteryGroupId
	// 11/12 for rarities 10/20 instead of 1/2. Same stat pools — alias them.
	pool[11] = pool[1]
	pool[12] = pool[2]
	return defs, pool
}
