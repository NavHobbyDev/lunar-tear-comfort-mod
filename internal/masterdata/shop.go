package masterdata

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/utils"
)

type ExchangeShopCell struct {
	SortOrder  int32
	ShopItemId int32
}

// LimitedStockResetRule describes how a limited-stock counter is reset:
// ResetType is daily(2)/monthly(3), ResetPeriod is the cycle length in that
// unit (e.g. type=2 period=7 means weekly, reset on Monday 00:00).
type LimitedStockResetRule struct {
	ResetType   int32
	ResetPeriod int32
}

type ShopCatalog struct {
	Items                 map[int32]EntityMShopItem
	Contents              map[int32][]EntityMShopItemContentPossession
	Effects               map[int32][]EntityMShopItemContentEffect
	MaxStaminaMillis      map[int32]int32                 // level -> max stamina in millis
	LimitedStock          map[int32]int32                 // stock id -> max count
	LimitedStockReset     map[int32]LimitedStockResetRule // stock id -> reset rule (daily, weekly, monthly)
	PlatformPaymentPrices map[int32]int32                 // platform payment id -> gem price (from m_platform_payment_price)
	ItemShopPool          []int32                         // shop item IDs for the replaceable item shop, sorted by cell sort order
	ItemShopIds           map[int32]bool                  // shop IDs whose ShopGroupType is the item shop (mission option group 2)
	ExchangeShopCells     map[int32][]ExchangeShopCell    // shopId -> sorted cells for exchange shops
}

// parsePaymentPrice converts the master data's decimal price string (e.g.
// "120.00") into its whole-unit amount. Returns 0 when the value cannot be
// parsed.
func parsePaymentPrice(s string) int32 {
	if i := strings.IndexByte(s, '.'); i >= 0 {
		s = s[:i]
	}
	n, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0
	}
	return int32(n)
}

func LoadShopCatalog() (*ShopCatalog, error) {
	items, err := utils.ReadTable[EntityMShopItem]("m_shop_item")
	if err != nil {
		return nil, fmt.Errorf("load shop item table: %w", err)
	}
	contents, err := utils.ReadTable[EntityMShopItemContentPossession]("m_shop_item_content_possession")
	if err != nil {
		return nil, fmt.Errorf("load shop content possession table: %w", err)
	}
	effects, err := utils.ReadTable[EntityMShopItemContentEffect]("m_shop_item_content_effect")
	if err != nil {
		return nil, fmt.Errorf("load shop content effect table: %w", err)
	}
	userLevels, err := utils.ReadTable[EntityMUserLevel]("m_user_level")
	if err != nil {
		return nil, fmt.Errorf("load user level table: %w", err)
	}
	stockRows, err := utils.ReadTable[EntityMShopItemLimitedStock]("m_shop_item_limited_stock")
	if err != nil {
		return nil, fmt.Errorf("load shop item limited stock table: %w", err)
	}
	paymentPriceRows, err := utils.ReadTable[EntityMPlatformPaymentPrice]("m_platform_payment_price")
	if err != nil {
		return nil, fmt.Errorf("load platform payment price table: %w", err)
	}

	catalog := &ShopCatalog{
		Items:                 make(map[int32]EntityMShopItem, len(items)),
		Contents:              make(map[int32][]EntityMShopItemContentPossession, len(contents)),
		Effects:               make(map[int32][]EntityMShopItemContentEffect, len(effects)),
		MaxStaminaMillis:      make(map[int32]int32, len(userLevels)),
		LimitedStock:          make(map[int32]int32, len(stockRows)),
		LimitedStockReset:     make(map[int32]LimitedStockResetRule, len(stockRows)),
		PlatformPaymentPrices: make(map[int32]int32, len(paymentPriceRows)),
	}
	for _, row := range items {
		catalog.Items[row.ShopItemId] = row
	}
	for _, row := range contents {
		catalog.Contents[row.ShopItemId] = append(catalog.Contents[row.ShopItemId], row)
	}
	for _, row := range effects {
		catalog.Effects[row.ShopItemId] = append(catalog.Effects[row.ShopItemId], row)
	}
	for _, ul := range userLevels {
		catalog.MaxStaminaMillis[ul.UserLevel] = ul.MaxStamina * 1000
	}
	for _, row := range stockRows {
		catalog.LimitedStock[row.ShopItemLimitedStockId] = row.MaxCount
		catalog.LimitedStockReset[row.ShopItemLimitedStockId] = LimitedStockResetRule{
			ResetType:   row.ShopItemAutoResetType,
			ResetPeriod: row.ShopItemAutoResetPeriod,
		}
	}
	// The same payment id appears once per platform type with an identical
	// price, so the first row per id wins.
	for _, row := range paymentPriceRows {
		if _, ok := catalog.PlatformPaymentPrices[row.PlatformPaymentId]; ok {
			continue
		}
		catalog.PlatformPaymentPrices[row.PlatformPaymentId] = parsePaymentPrice(row.Price)
	}

	shops, err := utils.ReadTable[EntityMShop]("m_shop")
	if err != nil {
		return nil, fmt.Errorf("load shop table: %w", err)
	}
	cellGroups, err := utils.ReadTable[EntityMShopItemCellGroup]("m_shop_item_cell_group")
	if err != nil {
		return nil, fmt.Errorf("load shop item cell group table: %w", err)
	}
	cells, err := utils.ReadTable[EntityMShopItemCell]("m_shop_item_cell")
	if err != nil {
		return nil, fmt.Errorf("load shop item cell table: %w", err)
	}

	cellIdToItemId := make(map[int32]int32, len(cells))
	for _, c := range cells {
		cellIdToItemId[c.ShopItemCellId] = c.ShopItemId
	}

	cellGroupByCGId := make(map[int32][]EntityMShopItemCellGroup, len(cellGroups))
	for _, cg := range cellGroups {
		cellGroupByCGId[cg.ShopItemCellGroupId] = append(cellGroupByCGId[cg.ShopItemCellGroupId], cg)
	}

	catalog.ExchangeShopCells = make(map[int32][]ExchangeShopCell)
	catalog.ItemShopIds = make(map[int32]bool)
	for _, s := range shops {
		if s.ShopGroupType == model.ShopGroupTypeItemShop {
			catalog.ItemShopIds[s.ShopId] = true
		}
		entries := cellGroupByCGId[s.ShopItemCellGroupId]
		if len(entries) == 0 {
			continue
		}

		switch s.ShopGroupType {
		case model.ShopGroupTypeItemShop:
			var poolCells []ExchangeShopCell
			for _, cg := range entries {
				if itemId, ok := cellIdToItemId[cg.ShopItemCellId]; ok {
					poolCells = append(poolCells, ExchangeShopCell{cg.SortOrder, itemId})
				}
			}
			sort.Slice(poolCells, func(i, j int) bool { return poolCells[i].SortOrder < poolCells[j].SortOrder })
			catalog.ItemShopPool = make([]int32, len(poolCells))
			for i, pc := range poolCells {
				catalog.ItemShopPool[i] = pc.ShopItemId
			}

		case model.ShopGroupTypeExchangeShop:
			var sc []ExchangeShopCell
			for _, cg := range entries {
				if itemId, ok := cellIdToItemId[cg.ShopItemCellId]; ok {
					sc = append(sc, ExchangeShopCell{cg.SortOrder, itemId})
				}
			}
			sort.Slice(sc, func(i, j int) bool { return sc[i].SortOrder < sc[j].SortOrder })
			catalog.ExchangeShopCells[s.ShopId] = sc
		}
	}

	return catalog, nil
}
