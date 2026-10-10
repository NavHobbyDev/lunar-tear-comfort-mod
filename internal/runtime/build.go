package runtime

import (
	"fmt"
	"log"

	"lunar-tear/server/internal/campaign"
	"lunar-tear/server/internal/gacha"
	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/masterdata/memorydb"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/questflow"
	"lunar-tear/server/internal/store"
	"lunar-tear/server/internal/userdata"
)

// buildCatalogs runs the full Load*/Build*/Enrich* sequence against whatever
// memorydb currently holds and returns a fully populated *Catalogs. Called
// once at startup and again on every reload.
func buildCatalogs() (*Catalogs, error) {
	log.Printf("master data loaded (%d tables)", memorydb.TableCount())

	gameConfig, err := masterdata.LoadGameConfig()
	if err != nil {
		return nil, fmt.Errorf("load game config: %w", err)
	}
	log.Printf("game config loaded (goldId=%d, skipTicketId=%d, rebirthGold=%d)",
		gameConfig.ConsumableItemIdForGold, gameConfig.ConsumableItemIdForQuestSkipTicket, gameConfig.CharacterRebirthConsumeGold)

	partsCatalog, err := masterdata.LoadPartsCatalog()
	if err != nil {
		return nil, fmt.Errorf("load parts catalog: %w", err)
	}
	log.Printf("parts catalog loaded: %d parts, %d rarities", len(partsCatalog.PartsById), len(partsCatalog.RarityByRarityType))

	questCatalog, err := masterdata.LoadQuestCatalog(partsCatalog)
	if err != nil {
		return nil, fmt.Errorf("load quest catalog: %w", err)
	}
	limitContentCatalog, err := masterdata.LoadLimitContentCatalog()
	if err != nil {
		return nil, fmt.Errorf("load limit content catalog: %w", err)
	}
	log.Printf("limit content catalog loaded: %d chapters", len(limitContentCatalog.ContentsByChapter))
	sideStoryCatalog := masterdata.LoadSideStoryCatalog()
	campaignCatalog, err := campaign.Load()
	if err != nil {
		return nil, fmt.Errorf("load campaign catalog: %w", err)
	}
	log.Printf("campaign catalog loaded: %d enhance, %d quest", campaignCatalog.EnhanceCount(), campaignCatalog.QuestCount())
	characterRebirthCatalog, err := masterdata.LoadCharacterRebirthCatalog()
	if err != nil {
		return nil, fmt.Errorf("load character rebirth catalog: %w", err)
	}
	log.Printf("character rebirth catalog loaded: %d characters", len(characterRebirthCatalog.StepGroupByCharacterId))

	materialCatalog, err := masterdata.LoadMaterialCatalog()
	if err != nil {
		return nil, fmt.Errorf("load material catalog: %w", err)
	}
	log.Printf("material catalog loaded: %d materials", len(materialCatalog.All))

	consumableItemCatalog, err := masterdata.LoadConsumableItemCatalog()
	if err != nil {
		return nil, fmt.Errorf("load consumable item catalog: %w", err)
	}
	log.Printf("consumable item catalog loaded: %d items", len(consumableItemCatalog.All))

	questHandler := questflow.NewQuestHandler(questCatalog, gameConfig, sideStoryCatalog, campaignCatalog, characterRebirthCatalog, materialCatalog, consumableItemCatalog)
	userdata.SetQuestHandler(questHandler)

	gachaEntries, medalInfo, err := masterdata.LoadGachaCatalog()
	if err != nil {
		return nil, fmt.Errorf("load gacha catalog: %w", err)
	}
	masterdata.EnrichGachaUnlockConditions(gachaEntries, questCatalog)
	log.Printf("gacha catalog loaded: %d entries", len(gachaEntries))

	gachaPool, err := masterdata.LoadGachaPool()
	if err != nil {
		return nil, fmt.Errorf("load gacha pool: %w", err)
	}
	log.Printf("gacha pool loaded: costumes=%d rarities, weapons=%d rarities, materials=%d",
		len(gachaPool.CostumesByRarity), len(gachaPool.WeaponsByRarity), len(gachaPool.Materials))

	shopCatalog, err := masterdata.LoadShopCatalog()
	if err != nil {
		return nil, fmt.Errorf("load shop catalog: %w", err)
	}
	log.Printf("shop catalog loaded: %d items, %d content groups, %d exchange shops",
		len(shopCatalog.Items), len(shopCatalog.Contents), len(shopCatalog.ExchangeShopCells))

	// Inject custom items into the pool so they survive PruneUnpairedCostumes
	// and appear as featured in specific banners.
	// TODO: replace CharacterId with the real value for costume 24008.
	/* customCostume := masterdata.GachaPoolItem{
		PossessionType: int32(model.PossessionTypeCostume),
		PossessionId:   24008,
		RarityType:     30, // SSR rarity — 2% pool instead of 5%
		CharacterId:    1001,
	}
	customWeapon := masterdata.GachaPoolItem{
		PossessionType: int32(model.PossessionTypeWeapon),
		PossessionId:   240271,
		RarityType:     40, // SSR rarity
	}
	gachaPool.CostumeById[24008] = customCostume
	gachaPool.WeaponById[240271] = customWeapon
	gachaPool.CostumeWeaponMap[24008] = 240271 */

	gachaPool.BuildShopFeatured(shopCatalog)
	gachaPool.PruneUnpairedCostumes()
	gachaPool.BuildFeaturedFromTerms(gachaEntries)
	gachaPool.BuildBannerPools(gachaEntries)

	// Fully replace a banner's pool with the items of one or several
	// characters. Costumes: all gacha-eligible costumes of the characters are
	// pulled automatically (including custom costumes injected above), minus
	// the exclusion list. Weapons: only weapons whose WeaponType matches the
	// SkillfulWeaponType of the characters' costumes, minus the exclusion
	// list. Must run after BuildBannerPools and before EnrichCatalogPromotions.
	// Solo banner example:
	// gachaPool.OverrideBannerForCharacters(
	// 	45,                    // DestinationDomainId (gacha id)
	// 	[]int32{1001},         // character ids
	// 	[]int32{110010},       // costume exclusions (nil = none)
	// 	[]int32{210011},       // weapon exclusions (nil = none)
	// )
	// For a group of characters (e.g. crossover units) sharing one banner:
	// gachaPool.OverrideBannerForCharacters(
	// 	418,                     // DestinationDomainId (gacha id)
	// 	[]int32{2001, 2002},     // character ids
	// 	nil,                     // costume exclusions
	// 	nil,                     // weapon exclusions
	// )

	// Force-include weapons that are normally excluded from the gacha pool
	// because their source quests are disabled. Each banner only receives the
	// weapons whose type its characters can wield:
	//   320021 Faith            (Anecdote: Stars, one-handed sword, water)
	//   330311 Jubilant Spirit  (Anecdote: The World, spear, wind)
	//   Blackhorn subjugation set:
	//   500011 Blackhorn Atrocity (two-handed sword, fire)
	//   500021 Blackhorn Spite    (spear, water)
	//   500031 Blackhorn Loathing (gun, wind)
	//   500041 Blackhorn Mourning (staff, light)
	//   500051 Blackhorn Hate     (fists, dark)
	// Must run before OverrideBannerForCharacters.
	gachaPool.AddExtraGachaWeapons(
		[]int32{403, 40, 47, 9000023, 176, 38, 9000078, 178, 34, 121, 50, 594, 150, 8, 188, 99, 132, 168, 416, 426, 489, 498, 544, 613},
		[]int32{320021, 330311, 340401, 500011, 500021, 500031, 500041, 500051},
	)

	gachaPool.OverrideBannerForCharacters(
		403,
		[]int32{1008},
		[]int32{},
		[]int32{},
	)
	gachaPool.OverrideBannerForCharacters(
		40,
		[]int32{1009},
		[]int32{},
		[]int32{},
	)
	gachaPool.OverrideBannerForCharacters(
		47,
		[]int32{1007},
		[]int32{},
		[]int32{},
	)
	gachaPool.OverrideBannerForCharacters(
		9000023,
		[]int32{1015},
		[]int32{},
		[]int32{},
	)
	gachaPool.OverrideBannerForCharacters(
		176,
		[]int32{1006},
		[]int32{},
		[]int32{},
	)
	gachaPool.OverrideBannerForCharacters(
		38,
		[]int32{1011},
		[]int32{},
		[]int32{},
	)
	gachaPool.OverrideBannerForCharacters(
		9000078,
		[]int32{1012},
		[]int32{},
		[]int32{},
	)
	gachaPool.OverrideBannerForCharacters(
		178,
		[]int32{1013},
		[]int32{},
		[]int32{},
	)
	gachaPool.OverrideBannerForCharacters(
		34,
		[]int32{1014},
		[]int32{},
		[]int32{},
	)
	gachaPool.OverrideBannerForCharacters(
		121,
		[]int32{1010},
		[]int32{},
		[]int32{},
	)
	gachaPool.OverrideBannerForCharacters(
		50,
		[]int32{1004},
		[]int32{},
		[]int32{},
	)
	gachaPool.OverrideBannerForCharacters(
		594,
		[]int32{1019},
		[]int32{},
		[]int32{},
	)
	gachaPool.OverrideBannerForCharacters(
		150,
		[]int32{1001, 1002, 1003, 100001},
		[]int32{},
		[]int32{},
	)
	gachaPool.OverrideBannerForCharacters(
		8,
		[]int32{1016, 1017, 1018, 1031},
		[]int32{},
		[]int32{},
	)
	gachaPool.OverrideBannerForCharacters(
		188,
		[]int32{1044, 1045, 1046, 1047},
		[]int32{},
		[]int32{},
	)
	gachaPool.OverrideBannerForCharacters(
		99,
		[]int32{1022},
		[]int32{},
		[]int32{},
	)
	gachaPool.OverrideBannerForCharacters(
		132,
		[]int32{1023},
		[]int32{},
		[]int32{},
	)
	gachaPool.OverrideBannerForCharacters(
		168,
		[]int32{1024},
		[]int32{},
		[]int32{},
	)
	gachaPool.OverrideBannerForCharacters(
		416,
		[]int32{1025},
		[]int32{},
		[]int32{},
	)
	gachaPool.OverrideBannerForCharacters(
		426,
		[]int32{1027},
		[]int32{},
		[]int32{},
	)
	gachaPool.OverrideBannerForCharacters(
		489,
		[]int32{1026},
		[]int32{},
		[]int32{},
	)
	gachaPool.OverrideBannerForCharacters(
		498,
		[]int32{1020},
		[]int32{},
		[]int32{},
	)
	gachaPool.OverrideBannerForCharacters(
		544,
		[]int32{1021},
		[]int32{},
		[]int32{},
	)
	gachaPool.OverrideBannerForCharacters(
		613,
		[]int32{1048},
		[]int32{},
		[]int32{},
	)

	masterdata.EnrichCatalogPromotions(gachaEntries, gachaPool)

	dupExchange, err := masterdata.LoadDupExchange()
	if err != nil {
		return nil, fmt.Errorf("load dup exchange: %w", err)
	}
	dupAdded, err := masterdata.EnrichDupExchange(dupExchange, gachaPool)
	if err != nil {
		return nil, fmt.Errorf("enrich dup exchange: %w", err)
	}
	log.Printf("dup exchange loaded: %d entries (%d derived from limit-break materials)", len(dupExchange), dupAdded)

	companionDupExchange, err := masterdata.LoadCompanionDupExchange()
	if err != nil {
		return nil, fmt.Errorf("load companion dup exchange: %w", err)
	}
	companionDupAdded, err := masterdata.EnrichCompanionDupExchange(companionDupExchange)
	if err != nil {
		return nil, fmt.Errorf("enrich companion dup exchange: %w", err)
	}
	log.Printf("companion dup exchange loaded: %d entries (%d derived from enhancement materials)", len(companionDupExchange), companionDupAdded)

	importantItems := masterdata.LoadImportantItemCatalog()

	gachaHandler := gacha.NewGachaHandler(gachaPool, gameConfig, questHandler.Granter, medalInfo, dupExchange)

	questHandler.Granter.CostumeDupExchange = dupExchange
	questHandler.Granter.CompanionDupExchange = companionDupExchange

	// Ensure maps are initialized
	if questHandler.Granter.CostumeDupExchange == nil {
		questHandler.Granter.CostumeDupExchange = make(map[int32][]model.DupExchangeEntry)
	}
	if questHandler.Granter.CompanionDupExchange == nil {
		questHandler.Granter.CompanionDupExchange = make(map[int32][]model.DupExchangeEntry)
	}

	conditionResolver, err := masterdata.LoadConditionResolver()
	if err != nil {
		return nil, fmt.Errorf("load condition resolver: %w", err)
	}

	cageOrnamentCatalog := masterdata.LoadCageOrnamentCatalog()
	lostItemsCatalog := masterdata.LoadLostItemsCatalog()
	loginBonusCatalog := masterdata.LoadLoginBonusCatalog()
	characterViewerCatalog := masterdata.LoadCharacterViewerCatalog(conditionResolver)
	omikujiCatalog := masterdata.LoadOmikujiCatalog()

	questHandler.FirstClearUnmultipliedMaterialIds = make(map[int32]bool)
	for materialId := range materialCatalog.ByType[model.MaterialTypeCostumeLimitBreak] {
		questHandler.FirstClearUnmultipliedMaterialIds[materialId] = true
	}

	costumeCatalog, err := masterdata.LoadCostumeCatalog(materialCatalog)
	if err != nil {
		return nil, fmt.Errorf("load costume catalog: %w", err)
	}
	log.Printf("costume catalog loaded: %d costumes, %d materials, %d rarity curves", len(costumeCatalog.Costumes), len(costumeCatalog.Materials), len(costumeCatalog.ExpByRarity))

	// Wire up costume grant callback so level bonuses and costume-collection
	// missions are updated when a costume is obtained from any source.
	// missionCatalog is captured by pointer-to-local and is set further down in
	// this function, so the closure always reads the final non-nil value.
	var missionCatalogRef *masterdata.MissionCatalog
	questHandler.Granter.OnCostumeGranted = func(user *store.UserState) {
		now := gametime.NowMillis()
		rebuildCostumeLevelBonuses(costumeCatalog, user, now)
		applyCostumeCollectionMissions(user, missionCatalogRef, now)
	}

	// Wire up encyclopedia entry callback so encyclopedia missions are updated
	// when a new entry (costume, weapon, companion, parts, thought) is obtained.
	questHandler.Granter.OnEncyclopediaEntryAdded = func(user *store.UserState) {
		now := gametime.NowMillis()
		applyEncyclopediaMissions(user, missionCatalogRef, now)
	}

	weaponCatalog, err := masterdata.LoadWeaponCatalog(materialCatalog)
	if err != nil {
		return nil, fmt.Errorf("load weapon catalog: %w", err)
	}
	log.Printf("weapon catalog loaded: %d weapons, %d materials, %d enhance configs", len(weaponCatalog.Weapons), len(weaponCatalog.Materials), len(weaponCatalog.ExpByEnhanceId))

	// Gacha auto-sells overflow weapons at their level-1 sell price when the
	// weapon inventory is full, so the handler needs the resolved prices.
	gachaHandler.WeaponSellPrice = buildWeaponSellPrices(weaponCatalog)

	exploreCatalog, err := masterdata.LoadExploreCatalog()
	if err != nil {
		return nil, fmt.Errorf("load explore catalog: %w", err)
	}
	log.Printf("explore catalog loaded: %d explores, %d grade assets", len(exploreCatalog.Explores), len(exploreCatalog.GradeAssets))

	gimmickCatalog, err := masterdata.LoadGimmickCatalog(conditionResolver, cageOrnamentCatalog)
	if err != nil {
		return nil, fmt.Errorf("load gimmick catalog: %w", err)
	}
	// Publish the catalog so the world-map projector's visibleGimmickRecords
	// filter can consult SequenceAvailable without a runtime.Handle round-trip.
	userdata.SetGimmickCatalog(gimmickCatalog)

	characterBoardCatalog, err := masterdata.LoadCharacterBoardCatalog()
	if err != nil {
		return nil, fmt.Errorf("load character board catalog: %w", err)
	}
	log.Printf("character board catalog loaded: %d panels, %d boards", len(characterBoardCatalog.PanelById), len(characterBoardCatalog.BoardById))

	companionCatalog, err := masterdata.LoadCompanionCatalog()
	if err != nil {
		return nil, fmt.Errorf("load companion catalog: %w", err)
	}
	log.Printf("companion catalog loaded: %d companions, %d categories", len(companionCatalog.CompanionById), len(companionCatalog.GoldCostByCategory))

	bigHuntCatalog := masterdata.LoadBigHuntCatalog()

	towerCatalog := masterdata.LoadTowerCatalog()

	labyrinthCatalog := masterdata.LoadLabyrinthCatalog()

	pvpCatalog := masterdata.LoadPvpCatalog()

	dokanCatalog := masterdata.LoadDokanCatalog()
	userdata.SetDokanCatalog(dokanCatalog)

	missionCatalog := masterdata.LoadMissionCatalog()
	missionCatalogRef = missionCatalog
	questHandler.MissionCatalog = missionCatalog
	questHandler.ImportantItems = importantItems

	return &Catalogs{
		GameConfig:        gameConfig,
		Parts:             partsCatalog,
		Quest:             questCatalog,
		LimitContent:      limitContentCatalog,
		GachaEntries:      gachaEntries,
		GachaMedals:       medalInfo,
		GachaPool:         gachaPool,
		Shop:              shopCatalog,
		DupExchange:       dupExchange,
		ConditionResolver: conditionResolver,
		CageOrnament:      cageOrnamentCatalog,
		LostItems:         lostItemsCatalog,
		LoginBonus:        loginBonusCatalog,
		CharacterViewer:   characterViewerCatalog,
		Omikuji:           omikujiCatalog,
		Material:          materialCatalog,
		ConsumableItem:    consumableItemCatalog,
		Costume:           costumeCatalog,
		Weapon:            weaponCatalog,
		Explore:           exploreCatalog,
		Gimmick:           gimmickCatalog,
		CharacterBoard:    characterBoardCatalog,
		CharacterRebirth:  characterRebirthCatalog,
		Companion:         companionCatalog,
		SideStory:         sideStoryCatalog,
		BigHunt:           bigHuntCatalog,
		Tower:             towerCatalog,
		Labyrinth:         labyrinthCatalog,
		Mission:           missionCatalog,
		ImportantItems:    importantItems,
		Pvp:               pvpCatalog,
		Dokan:             dokanCatalog,
		Campaign:          campaignCatalog,
		QuestHandler:      questHandler,
		GachaHandler:      gachaHandler,
	}, nil
}

// buildWeaponSellPrices resolves the level-1 sell price for every weapon so
// the gacha handler can auto-sell overflow weapons at the inventory cap.
func buildWeaponSellPrices(catalog *masterdata.WeaponCatalog) map[int32]int32 {
	prices := make(map[int32]int32, len(catalog.Weapons))
	for weaponId, wm := range catalog.Weapons {
		if sellFunc, ok := catalog.SellPriceByEnhanceId[wm.WeaponSpecificEnhanceId]; ok {
			prices[weaponId] = sellFunc.Evaluate(1)
		}
	}
	return prices
}

// rebuildCostumeLevelBonuses recalculates permanent character bonuses from all
// owned costumes. Called via OnCostumeGranted when a new costume is obtained.
func rebuildCostumeLevelBonuses(catalog *masterdata.CostumeCatalog, user *store.UserState, nowMillis int64) {
	user.EnsureMaps()
	user.CharacterCostumeLevelBonuses = make(map[store.CharacterCostumeLevelBonusKey]store.CharacterCostumeLevelBonusState)

	for _, costume := range user.Costumes {
		cm, ok := catalog.Costumes[costume.CostumeId]
		if !ok || cm.CostumeLevelBonusId == 0 {
			continue
		}
		for _, row := range catalog.BonusesUpToLevel(costume.CostumeId, costume.Level) {
			if row.Level <= 0 {
				continue
			}
			key := store.CharacterCostumeLevelBonusKey{
				CharacterId:           cm.CharacterId,
				StatusCalculationType: 1, // Add
			}
			state := user.CharacterCostumeLevelBonuses[key]
			state.CharacterId = cm.CharacterId
			state.StatusCalculationType = 1
			switch row.CostumeLevelBonusType {
			case 3: // Attack
				state.Attack += row.EffectValue
			case 7: // Hp
				state.Hp += row.EffectValue
			case 9: // Vitality (defense)
				state.Vitality += row.EffectValue
			case 1: // Agility
				state.Agility += row.EffectValue
			case 4: // CriticalRatio
				state.CriticalRatio += row.EffectValue
			}
			state.LatestVersion = nowMillis
			user.CharacterCostumeLevelBonuses[key] = state
		}

		existing := user.CostumeLevelBonusReleaseStatuses[costume.CostumeId]
		if existing.LastReleasedBonusLevel == costume.Level && existing.ConfirmedBonusLevel == costume.Level {
			continue
		}
		existing.CostumeId = costume.CostumeId
		existing.LastReleasedBonusLevel = costume.Level
		existing.ConfirmedBonusLevel = costume.Level
		existing.LatestVersion = nowMillis
		user.CostumeLevelBonusReleaseStatuses[costume.CostumeId] = existing
	}
}

// applyCostumeCollectionMissions advances type-49 missions ("Acquire costumes
// for X and Y") based on the user's current costume inventory. Called via
// OnCostumeGranted whenever a new costume is added. The function is defined
// here rather than in the service package to avoid a circular import.
func applyCostumeCollectionMissions(user *store.UserState, cat *masterdata.MissionCatalog, nowMillis int64) {
	if cat == nil || len(cat.CompleteMissionCostumesByMissionId) == 0 {
		return
	}

	const missionConditionCostumeSetCollected int32 = 49
	const missionProgressStatusTypeClear int32 = 2 // model.MissionProgressStatusTypeClear

	ownedCostumeIds := make(map[int32]bool, len(user.Costumes))
	for _, c := range user.Costumes {
		ownedCostumeIds[c.CostumeId] = true
	}

	clearedCount := int32(0)
	for _, mission := range cat.ActiveMissionsAt(nowMillis) {
		if mission.MissionClearConditionType != missionConditionCostumeSetCollected {
			continue
		}
		required := cat.CompleteMissionCostumesByMissionId[mission.MissionId]
		if len(required) == 0 {
			continue
		}

		progress := user.Missions[mission.MissionId]
		if progress.MissionProgressStatusType >= missionProgressStatusTypeClear {
			continue
		}

		ownedFromSet := int32(0)
		for _, costumeId := range required {
			if ownedCostumeIds[costumeId] {
				ownedFromSet++
			}
		}
		if ownedFromSet == 0 {
			continue
		}
		if ownedFromSet <= progress.ProgressValue {
			continue
		}
		if progress.MissionId == 0 {
			progress.MissionId = mission.MissionId
			progress.StartDatetime = nowMillis
		}
		progress.ProgressValue = ownedFromSet
		progress.LatestVersion = nowMillis

		if progress.ProgressValue >= mission.ClearConditionValue {
			progress.ProgressValue = mission.ClearConditionValue
			progress.MissionProgressStatusType = missionProgressStatusTypeClear
			progress.ClearDatetime = nowMillis
			clearedCount++
			log.Printf("[Mission] mission %d CLEARED by costume collection (%d/%d)",
				mission.MissionId, progress.ProgressValue, mission.ClearConditionValue)
		} else {
			progress.MissionProgressStatusType = 1 // InProgress
		}
		user.Missions[mission.MissionId] = progress
	}
	if clearedCount > 0 {
		log.Printf("[applyCostumeCollectionMissions] cleared %d costume-collection missions", clearedCount)
	}
}

// applyEncyclopediaMissions advances type-39 missions ("Encyclopedia entries ≥ X")
// based on the user's current encyclopedia entry count. Called via
// OnEncyclopediaEntryAdded whenever a new entry is added. The function is defined
// here rather than in the service package to avoid a circular import.
func applyEncyclopediaMissions(user *store.UserState, cat *masterdata.MissionCatalog, nowMillis int64) {
	if cat == nil {
		return
	}

	const missionConditionEncyclopediaEntries int32 = 39
	const missionProgressStatusTypeClear int32 = 2 // model.MissionProgressStatusTypeClear

	// Calculate total encyclopedia entry count (unique IDs only)
	// Encyclopedia entries include: costumes, weapons, companions, parts, thoughts
	costumeIds := make(map[int32]bool, len(user.Costumes))
	for _, c := range user.Costumes {
		costumeIds[c.CostumeId] = true
	}
	weaponIds := make(map[int32]bool, len(user.Weapons))
	for _, w := range user.Weapons {
		weaponIds[w.WeaponId] = true
	}
	companionIds := make(map[int32]bool, len(user.Companions))
	for _, c := range user.Companions {
		companionIds[c.CompanionId] = true
	}
	partsIds := make(map[int32]bool, len(user.Parts))
	for _, p := range user.Parts {
		partsIds[p.PartsId] = true
	}
	thoughtIds := make(map[int32]bool, len(user.Thoughts))
	for _, t := range user.Thoughts {
		thoughtIds[t.ThoughtId] = true
	}

	totalEntries := int32(len(costumeIds) + len(weaponIds) + len(companionIds) + len(partsIds) + len(thoughtIds))

	clearedCount := int32(0)
	for _, mission := range cat.ActiveMissionsAt(nowMillis) {
		if mission.MissionClearConditionType != missionConditionEncyclopediaEntries {
			continue
		}

		progress := user.Missions[mission.MissionId]
		if progress.MissionProgressStatusType >= missionProgressStatusTypeClear {
			continue
		}

		if totalEntries <= progress.ProgressValue {
			continue
		}
		if progress.MissionId == 0 {
			progress.MissionId = mission.MissionId
			progress.StartDatetime = nowMillis
		}
		progress.ProgressValue = totalEntries
		progress.LatestVersion = nowMillis

		if totalEntries >= mission.ClearConditionValue {
			progress.MissionProgressStatusType = missionProgressStatusTypeClear
			progress.ClearDatetime = nowMillis
			user.Missions[mission.MissionId] = progress
			clearedCount++
		} else {
			progress.MissionProgressStatusType = 1 // InProgress
			user.Missions[mission.MissionId] = progress
		}
	}
	if clearedCount > 0 {
		log.Printf("[applyEncyclopediaMissions] cleared %d encyclopedia missions", clearedCount)
	}
}
