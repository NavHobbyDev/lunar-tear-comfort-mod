package store

import (
	"fmt"
	"log"
	"math/rand"
	"sort"
	"time"

	"github.com/google/uuid"

	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/model"
)

func DeductPrice(user *UserState, priceType, priceId, amount int32) error {
	switch priceType {
	case model.PriceTypeConsumableItem:
		cur := user.ConsumableItems[priceId]
		if cur < amount {
			return fmt.Errorf("insufficient consumable %d: have %d, need %d", priceId, cur, amount)
		}
		user.ConsumableItems[priceId] = cur - amount
	case model.PriceTypeGem:
		total := user.Gem.FreeGem + user.Gem.PaidGem
		if total < amount {
			return fmt.Errorf("insufficient gems: have %d, need %d", total, amount)
		}
		if user.Gem.FreeGem >= amount {
			user.Gem.FreeGem -= amount
		} else {
			amount -= user.Gem.FreeGem
			user.Gem.FreeGem = 0
			user.Gem.PaidGem -= amount
		}
	case model.PriceTypePaidGem:
		if user.Gem.PaidGem < amount {
			return fmt.Errorf("insufficient paid gems: have %d, need %d", user.Gem.PaidGem, amount)
		}
		user.Gem.PaidGem -= amount
	case model.PriceTypePlatformPayment:
		// real-money purchase -- paid gems
		if user.Gem.PaidGem < amount {
			return fmt.Errorf("insufficient paid gems: have %d, need %d", user.Gem.PaidGem, amount)
		}
		user.Gem.PaidGem -= amount
	default:
		log.Printf("[DeductPrice] unhandled priceType=%d priceId=%d amount=%d", priceType, priceId, amount)
	}
	return nil
}

func DeductPossession(user *UserState, possessionType model.PossessionType, possessionId, count int32) {
	switch possessionType {
	case model.PossessionTypeMaterial:
		user.Materials[possessionId] -= count
		if user.Materials[possessionId] <= 0 {
			delete(user.Materials, possessionId)
		}
	case model.PossessionTypeConsumableItem:
		user.ConsumableItems[possessionId] -= count
		if user.ConsumableItems[possessionId] <= 0 {
			delete(user.ConsumableItems, possessionId)
		}
	case model.PossessionTypePaidGem:
		user.Gem.PaidGem -= count
	case model.PossessionTypeFreeGem:
		user.Gem.FreeGem -= count
	default:
		log.Printf("[DeductPossession] unhandled type=%d id=%d count=%d", possessionType, possessionId, count)
	}
}

// consumableMedalRemap fixes a masterdata defect where an event awards a tiered
// medal variant ("Bronze/Silver/Gold/Copper X Medal") but the event's exchange
// shop only accepts the base "X Medal", leaving the awarded medal unspendable.
// The awarded tier variant is remapped to the spendable base medal.
//
//	53, 54     -> 22: Coffin of Repose Medal (Copper/Silver -> base)
//	63, 64, 65 -> 29: Rhythm's Citadel Medal (Bronze/Silver/Gold -> base)
var consumableMedalRemap = map[int32]int32{
	53: 22, 54: 22, // Coffin of Repose Medal
	63: 29, 64: 29, 65: 29, // Rhythm's Citadel Medal
}

// CanonicalConsumableMedalId returns the spendable base medal id for an awarded
// consumable medal, leaving every other id unchanged. Applied both where rewards
// are loaded for display (so quest reward popups already show the base medal)
// and at grant time.
func CanonicalConsumableMedalId(id int32) int32 {
	if mapped, ok := consumableMedalRemap[id]; ok {
		return mapped
	}
	return id
}

// staminaGoldPrice is the per-unit gold each stamina consumable converts into.
// Single source of truth shared by grant-time (GrantPossession) and load-time
// (ConvertStaminaToGold) conversion so they never drift.
var staminaGoldPrice = map[int32]int32{
	3001: 120,
	3002: 240,
	3003: 360,
}

// ConvertStaminaToGold transforms existing stamina consumables (3001-3003) to gold (1)
// Called from login bonus to convert and save existing stamina in inventory
// Each id converts at its own price (see staminaGoldPrice).
func ConvertStaminaToGold(user *UserState) {
	if user.ConsumableItems == nil {
		return // No consumables to convert
	}

	totalGoldToAdd := int32(0)
	for staminaId, price := range staminaGoldPrice {
		if count, exists := user.ConsumableItems[staminaId]; exists && count > 0 {
			gold := count * price
			totalGoldToAdd += gold
			delete(user.ConsumableItems, staminaId)
			log.Printf("[ConvertStaminaToGold] Converted %d stamina (ID %d) to %d gold", count, staminaId, gold)
		}
	}

	if totalGoldToAdd > 0 {
		user.ConsumableItems[1] += totalGoldToAdd
		log.Printf("[ConvertStaminaToGold] Added total: +%d gold (ID 1)", totalGoldToAdd)
	}
}

// summonTicketGemPrice maps summon-ticket consumables to the free-gem value each
// converts into. These tickets are not kept in inventory: on receipt
// (GrantPossession) and on load (ConvertSummonTicketsToGems) they are auto-converted
// into free gems, mirroring the stamina->gold conversion. Single source of truth so
// the two paths never drift. Chapter/dark-memory gacha tickets (id 2 / 2002) are
// deliberately NOT listed: those are consumed directly in chapter gacha.
var summonTicketGemPrice = map[int32]int32{
	1002: 300,  // Premium Summon Ticket
	1003: 3000, // Premium Summon Ticket x10
	1004: 1500, // Silver Summon Ticket x10
	1005: 1800, // ***+ Premium Summon Ticket
	1006: 6000, // **** Premium Summon Ticket
	1201: 3000, // Mama's Choice Summon Ticket
	1202: 3000, // Mama's Choice Summon Ticket
}

func init() {
	// All Guaranteed Summon Tickets (1203-1224) convert at 9000 free gems each.
	for id := int32(1203); id <= 1224; id++ {
		summonTicketGemPrice[id] = 9000
	}
}

// ConvertItemToGems removes the held count of consumable itemID from the inventory,
// converting it into free gems at gemPrice each, and returns the gems added (0 if
// the user holds none).
func ConvertItemToGems(user *UserState, itemID, gemPrice int32) int32 {
	count, exists := user.ConsumableItems[itemID]
	if !exists || count <= 0 {
		return 0
	}
	gems := count * gemPrice
	delete(user.ConsumableItems, itemID)
	log.Printf("[ConvertItemToGems] Converted %d tickets (ID %d) to %d free gems", count, itemID, gems)
	return gems
}

// ConvertSummonTicketsToGems cashes out any summon tickets left in the inventory
// into free gems. Called on user load (GameStart / ReceiveStamp) alongside
// ConvertStaminaToGold so tickets that predate conversion, or were persisted by
// other paths, are cleaned up. Prices come from summonTicketGemPrice.
func ConvertSummonTicketsToGems(user *UserState) {
	if user.ConsumableItems == nil {
		return // No consumables to convert
	}

	totalGems := int32(0)
	for ticketId, price := range summonTicketGemPrice {
		totalGems += ConvertItemToGems(user, ticketId, price)
	}

	if totalGems > 0 {
		user.Gem.FreeGem += totalGems
		log.Printf("[ConvertSummonTicketsToGems] Added total: +%d free gems", totalGems)
	}
}

func GrantPossession(user *UserState, possessionType model.PossessionType, possessionId, count int32) {
	if possessionType == model.PossessionTypeConsumableItem {
		possessionId = CanonicalConsumableMedalId(possessionId)

		// Convert stamina consumables (3001-3003) to gold (1)
		// Each id converts at its own price (see staminaGoldPrice).
		if price, ok := staminaGoldPrice[possessionId]; ok {
			log.Printf("[GrantPossession] Converting stamina ID=%d count=%d to gold (%d per item)", possessionId, count, price)
			possessionId = 1 // Gold ID
			count = count * price
		}

		// Convert summon tickets to free gems instead of storing them.
		if price, ok := summonTicketGemPrice[possessionId]; ok {
			gems := count * price
			log.Printf("[GrantPossession] Converting summon ticket ID=%d count=%d to %d free gems", possessionId, count, gems)
			user.Gem.FreeGem += gems
			return
		}
	}
	switch possessionType {
	case model.PossessionTypeMaterial:
		user.Materials[possessionId] += count
	case model.PossessionTypeConsumableItem:
		user.ConsumableItems[possessionId] += count
	case model.PossessionTypePaidGem:
		user.Gem.PaidGem += count
	case model.PossessionTypeFreeGem:
		user.Gem.FreeGem += count
	case model.PossessionTypeImportantItem:
		user.ImportantItems[possessionId] += count
	case model.PossessionTypePremiumItem:
		user.PremiumItems[possessionId] = gametime.NowMillis()
	default:
		log.Printf("[GrantPossession] unhandled type=%d id=%d count=%d", possessionType, possessionId, count)
	}
}

type CostumeRef struct {
	CharacterId int32
}

type CompanionEnhancedRef struct {
	CompanionId int32
	Level       int32
}

type WeaponRef struct {
	WeaponSkillGroupId                 int32
	WeaponAbilityGroupId               int32
	WeaponStoryReleaseConditionGroupId int32
}

type WeaponStoryReleaseCond struct {
	StoryIndex                      int32
	WeaponStoryReleaseConditionType model.WeaponStoryReleaseConditionType
	ConditionValue                  int32
}

type PartsRef struct {
	PartsGroupId                  int32
	RarityType                    int32
	PartsInitialLotteryId         int32
	PartsStatusMainLotteryGroupId int32
	PartsStatusSubLotteryGroupId  int32
}

// PartsStatusSubDef carries the per-lottery-id sub-status shape needed at
// grant time. Held here so the store package does not import masterdata.
type PartsStatusSubDef struct {
	StatusKindType           int32
	StatusCalculationType    int32
	StatusChangeInitialValue int32
	StatusFunc               func(level int32) int32
}

type PossessionGranter struct {
	CostumeById           map[int32]CostumeRef
	CompanionEnhancedById map[int32]CompanionEnhancedRef
	WeaponById            map[int32]WeaponRef
	WeaponSkillSlots      map[int32][]int32
	WeaponAbilitySlots    map[int32][]int32
	ReleaseConditions     map[int32][]WeaponStoryReleaseCond

	PartsById                            map[int32]PartsRef
	DefaultPartsStatusMainByLotteryGroup map[int32]int32
	PartsVariantsByGroupRarity           map[int32]map[int32][]int32
	// PartsSetGroupIdsByGroupId maps a PartsGroupId to every group id of its
	// memoir set, so a drop can roll any piece of the set instead of only the
	// one wired in the quest's drop data.
	PartsSetGroupIdsByGroupId map[int32][]int32
	PartsSubStatusPool        map[int32][]int32
	PartsSubStatusDefs        map[int32]PartsStatusSubDef

	PartsSellPriceL1ByRarity map[int32]int32
	GoldConsumableItemId     int32

	LastChangedStoryWeaponIds []int32

	CostumeDupExchange   map[int32][]model.DupExchangeEntry
	CompanionDupExchange map[int32][]model.DupExchangeEntry

	// OnCostumeGranted is called after a new costume is added to the user.
	// The service layer sets this to rebuild costume level bonuses.
	OnCostumeGranted func(user *UserState)

	// OnEncyclopediaEntryAdded is called after a new encyclopedia entry is added to the user.
	// The service layer sets this to update encyclopedia mission progress.
	OnEncyclopediaEntryAdded func(user *UserState)
}

func (g *PossessionGranter) DrainChangedStoryWeaponIds() []int32 {
	ids := g.LastChangedStoryWeaponIds
	g.LastChangedStoryWeaponIds = nil
	return ids
}

func (g *PossessionGranter) GrantFull(user *UserState, possessionType model.PossessionType, possessionId, count int32, nowMillis int64) {
	switch possessionType {
	case model.PossessionTypeCostume, model.PossessionTypeCostumeEnhanced:
		g.GrantCostume(user, possessionId, nowMillis)
	case model.PossessionTypeWeapon, model.PossessionTypeWeaponEnhanced:
		g.GrantWeapon(user, possessionId, nowMillis)
	case model.PossessionTypeCompanion:
		g.GrantCompanion(user, possessionId, nowMillis)
	case model.PossessionTypeCompanionEnhanced:
		enhanced, ok := g.CompanionEnhancedById[possessionId]
		if !ok {
			return
		}
		g.grantCompanion(user, enhanced.CompanionId, enhanced.Level, nowMillis)
	case model.PossessionTypeParts, model.PossessionTypePartsEnhanced:
		g.GrantParts(user, possessionId, nowMillis)
	default:
		GrantPossession(user, possessionType, possessionId, count)
	}
}

func grantDuplicateExchange(user *UserState, exchange []model.DupExchangeEntry) {
	if exchange == nil || len(exchange) == 0 {
		return
	}
	log.Printf("[grantDuplicateExchange] granting %d items for duplicate", len(exchange))
	for _, item := range exchange {
		log.Printf("[grantDuplicateExchange] granting type=%d id=%d count=%d", item.PossessionType, item.PossessionId, item.Count)
		GrantPossession(user, model.PossessionType(item.PossessionType), item.PossessionId, item.Count)
	}
}

func (g *PossessionGranter) GrantCostume(user *UserState, costumeId int32, nowMillis int64) {
	for _, row := range user.Costumes {
		if row.CostumeId == costumeId {
			log.Printf("[GrantCostume] duplicate detected: costumeId=%d, applying dup exchange", costumeId)
			grantDuplicateExchange(user, g.CostumeDupExchange[costumeId])
			return
		}
	}
	g.GrantNewCostume(user, costumeId, nowMillis)
}

// GrantNewCostume creates a brand-new costume and its active-skill row without
// the ownership check. The gacha draw path calls it directly after its own
// duplicate scan has already confirmed the costume is not owned, avoiding a
// second pass over user.Costumes. Reward-path callers should use GrantCostume,
// which dup-checks first.
func (g *PossessionGranter) GrantNewCostume(user *UserState, costumeId int32, nowMillis int64) {
	if cm, ok := g.CostumeById[costumeId]; ok {
		if _, exists := user.Characters[cm.CharacterId]; !exists {
			user.Characters[cm.CharacterId] = CharacterState{
				CharacterId: cm.CharacterId,
				Level:       1,
			}
		}
	}
	key := uuid.New().String()
	user.Costumes[key] = CostumeState{
		UserCostumeUuid:     key,
		CostumeId:           costumeId,
		Level:               1,
		HeadupDisplayViewId: 1,
		AcquisitionDatetime: nowMillis,
	}
	user.CostumeActiveSkills[key] = CostumeActiveSkillState{
		UserCostumeUuid:     key,
		Level:               1,
		AcquisitionDatetime: nowMillis,
	}
	if g.OnCostumeGranted != nil {
		g.OnCostumeGranted(user)
	}
	if g.OnEncyclopediaEntryAdded != nil {
		g.OnEncyclopediaEntryAdded(user)
	}
}

func (g *PossessionGranter) GrantCompanion(user *UserState, companionId int32, nowMillis int64) {
	g.grantCompanion(user, companionId, 1, nowMillis)
}

func (g *PossessionGranter) grantCompanion(user *UserState, companionId, level int32, nowMillis int64) {
	for _, row := range user.Companions {
		if row.CompanionId == companionId {
			log.Printf("[grantCompanion] duplicate detected: companionId=%d, applying dup exchange", companionId)
			grantDuplicateExchange(user, g.CompanionDupExchange[companionId])
			return
		}
	}
	key := uuid.New().String()
	user.Companions[key] = CompanionState{
		UserCompanionUuid:   key,
		CompanionId:         companionId,
		Level:               level,
		HeadupDisplayViewId: 1,
		AcquisitionDatetime: nowMillis,
	}
	if g.OnEncyclopediaEntryAdded != nil {
		g.OnEncyclopediaEntryAdded(user)
	}
}

func (g *PossessionGranter) GrantParts(user *UserState, requestedPartsId int32, nowMillis int64) {
	chosenPartsId, chosenRef, ok := g.rollPartsVariant(requestedPartsId)
	if !ok {
		g.grantBareParts(user, requestedPartsId, nowMillis)
		return
	}
	g.createParts(user, chosenPartsId, chosenRef, nowMillis)
}

// GrantPartsExact creates the exact named memoir without re-rolling the
// variant — used when claiming mailed overflow drops so the player receives
// the very piece (rarity and rank) that was rolled.
func (g *PossessionGranter) GrantPartsExact(user *UserState, partsId int32, nowMillis int64) {
	ref, ok := g.PartsById[partsId]
	if !ok {
		g.grantBareParts(user, partsId, nowMillis)
		return
	}
	g.createParts(user, partsId, ref, nowMillis)
}

// PartsDropRoll is one rolled memoir grant from a pool drop: the concrete
// variant id and whether the auto-sale rules sold it instead of keeping the
// inventory row.
type PartsDropRoll struct {
	PartsId int32
	Sold    bool
}

// GrantOrSellPartsPoolDrop grants count memoirs drawn deck-style from the
// union pool of set pieces (groups) behind the quest's wired drop ids: the
// pool is shuffled and drawn without replacement, reshuffling once exhausted,
// so a single run can only repeat a piece after every piece in the pool has
// dropped. Each drawn piece rolls an independent weighted rarity capped at
// rarityCap (0 = the wired part's own rarity); the rank (pre-unlocked
// sub-status slots) is fixed by that rarity, so rarity is the whole prize.
func (g *PossessionGranter) GrantOrSellPartsPoolDrop(user *UserState, wiredPartsIds []int32, count, rarityCap int32, raritySet, rankSet map[int32]bool, nowMillis int64) []PartsDropRoll {
	type poolPiece struct {
		groupId int32
		wiredId int32
	}
	pool := make([]poolPiece, 0, len(wiredPartsIds)*3)
	seen := map[int32]bool{}
	for _, wiredId := range wiredPartsIds {
		ref, ok := g.PartsById[wiredId]
		if !ok {
			continue
		}
		groups := g.PartsSetGroupIdsByGroupId[ref.PartsGroupId]
		if len(groups) == 0 {
			groups = []int32{ref.PartsGroupId}
		}
		for _, groupId := range groups {
			if seen[groupId] {
				continue
			}
			seen[groupId] = true
			pool = append(pool, poolPiece{groupId: groupId, wiredId: wiredId})
		}
	}
	rolls := make([]PartsDropRoll, 0, count)
	if len(pool) == 0 {
		// No wired part is known to master data: grant them bare, one per slot.
		for k := int32(0); k < count && len(wiredPartsIds) > 0; k++ {
			wiredId := wiredPartsIds[int(k)%len(wiredPartsIds)]
			rolls = append(rolls, PartsDropRoll{PartsId: wiredId, Sold: g.grantOrSellBareDropPart(user, wiredId, nowMillis)})
		}
		return rolls
	}
	for int32(len(rolls)) < count {
		deck := append([]poolPiece(nil), pool...)
		rand.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
		for _, piece := range deck {
			if int32(len(rolls)) >= count {
				break
			}
			chosenPartsId, chosenRef, ok := g.rollPartsDropPiece(piece.wiredId, piece.groupId, rarityCap)
			if !ok {
				rolls = append(rolls, PartsDropRoll{PartsId: piece.wiredId, Sold: g.grantOrSellBareDropPart(user, piece.wiredId, nowMillis)})
				continue
			}
			rolls = append(rolls, PartsDropRoll{
				PartsId: chosenPartsId,
				Sold:    g.grantOrSellRolledPart(user, chosenPartsId, chosenRef, raritySet, rankSet, nowMillis),
			})
		}
	}
	return rolls
}

// partsAtCap reports whether the memoir inventory has reached the client's
// hard cap. Quest finishes must never be blocked at the cap (a refused
// finish leaves the quest un-cleared and jams progression/auto-orbit), so
// re-farmable drop parts are sold for gold instead — except SSR (rarity 40)
// drops, which are mailed to the gift box. One-time rewards (first clear,
// missions, shop, gifts) may hold rare memoirs and are granted past the cap
// rather than lost.
func partsAtCap(user *UserState) bool {
	return len(user.Parts) >= int(model.PartsInventoryCap)
}

// sellPartsForGold credits the level-1 sell price for the rarity. Returns
// false when the rarity has no price and the part cannot be sold.
func (g *PossessionGranter) sellPartsForGold(user *UserState, partsId, rarity int32) bool {
	price, ok := g.PartsSellPriceL1ByRarity[rarity]
	if !ok {
		return false
	}
	user.ConsumableItems[g.GoldConsumableItemId] += price
	log.Printf("[GrantParts] sold chosen=%d rarity=%d -> %d gold", partsId, rarity, price)
	return true
}

// lowestPartsSellPrice is the fallback for parts without a known rarity:
// the cheapest table entry, so the cap still holds.
func (g *PossessionGranter) lowestPartsSellPrice() (int32, bool) {
	lowest := int32(0)
	found := false
	for _, price := range g.PartsSellPriceL1ByRarity {
		if !found || price < lowest {
			lowest = price
			found = true
		}
	}
	return lowest, found
}

// grantOrSellRolledPart applies the overflow and auto-sale rules to one
// rolled memoir. SSR (rarity 40+) drops are never sold: at the inventory
// cap they are mailed to the gift box, otherwise granted normally. Lower
// rarities follow the player's auto-sale settings, and at the cap they are
// sold for gold regardless of the settings. Reports whether the part was
// sold for gold (mailed parts report false, like granted ones).
func (g *PossessionGranter) grantOrSellRolledPart(user *UserState, chosenPartsId int32, chosenRef PartsRef, raritySet, rankSet map[int32]bool, nowMillis int64) bool {
	rarity := chosenRef.RarityType
	rank := chosenRef.PartsInitialLotteryId
	if rarity >= int32(model.RaritySSRare) {
		if partsAtCap(user) {
			g.mailPartsOverflow(user, chosenPartsId, rarity, nowMillis)
		} else {
			g.createParts(user, chosenPartsId, chosenRef, nowMillis)
		}
		return false
	}
	if price, ok := g.PartsSellPriceL1ByRarity[rarity]; ok && raritySet[rarity] && rankSet[rank] {
		user.ConsumableItems[g.GoldConsumableItemId] += price
		log.Printf("[GrantParts] auto-sold chosen=%d rarity=%d rank=%d -> %d gold", chosenPartsId, rarity, rank, price)
		return true
	}
	// Inventory at the cap: drops are re-farmable, so sell regardless of the
	// auto-sale settings instead of overflowing the memoir inventory.
	if partsAtCap(user) && g.sellPartsForGold(user, chosenPartsId, rarity) {
		return true
	}
	g.createParts(user, chosenPartsId, chosenRef, nowMillis)
	return false
}

// partsGiftExpiryMillis keeps overflow memoir mails for 30 game days,
// matching the other gift-box sources.
const partsGiftExpiryMillis = int64(30 * 24 * time.Hour / time.Millisecond)

// AddGift posts one mail to the gift box, enforcing the mailbox cap: when
// the box already holds GiftInventoryCap mails, the oldest ones are dropped
// to make room for the new arrival. Keeps the unread badge in sync. Load
// paths insert DB rows directly and never evict existing mails.
func (u *UserState) AddGift(gift NotReceivedGiftState) {
	for int32(len(u.Gifts.NotReceived)) >= model.GiftInventoryCap {
		dropped := u.Gifts.NotReceived[0]
		u.Gifts.NotReceived = u.Gifts.NotReceived[1:]
		log.Printf("[Gifts] mailbox full (%d): dropped oldest mail uuid=%s type=%d id=%d count=%d",
			model.GiftInventoryCap, dropped.UserGiftUuid, dropped.GiftCommon.PossessionType, dropped.GiftCommon.PossessionId, dropped.GiftCommon.Count)
	}
	u.Gifts.NotReceived = append(u.Gifts.NotReceived, gift)
	u.Notifications.GiftNotReceiveCount = int32(len(u.Gifts.NotReceived))
}

// AddGiftsOrdered posts multiple mails to the gift box in the specified order.
// Since the mailbox displays mails newest-first (descending by GrantDatetime),
// each subsequent gift has its timestamp decremented by 1ms so the first gift
// in the slice appears first in the mailbox. This ensures the player sees
// rewards in the intended sequence rather than in arbitrary order.
func (u *UserState) AddGiftsOrdered(gifts []NotReceivedGiftState) {
	if len(gifts) == 0 {
		return
	}
	baseTime := gifts[0].GiftCommon.GrantDatetime
	for i := range gifts {
		gifts[i].GiftCommon.GrantDatetime = baseTime - int64(i)
		// Also adjust expiration to maintain the same relative window
		gifts[i].ExpirationDatetime = gifts[i].ExpirationDatetime - int64(i)
		u.AddGift(gifts[i])
	}
}

// ConsolidateGifts removes unreceived mails whose (PossessionType, PossessionId)
// matches any key in the provided set and returns their summed counts. Other
// mails are kept untouched, in order. This prevents mailbox clutter when the
// same reward items are sent repeatedly (e.g. daily PvP or BigHunt rewards):
// old unclaimed mails are folded into the new ones.
func (u *UserState) ConsolidateGifts(keys map[[2]int32]bool) map[[2]int32]int32 {
	pending := map[[2]int32]int32{}
	kept := make([]NotReceivedGiftState, 0, len(u.Gifts.NotReceived))
	for _, mail := range u.Gifts.NotReceived {
		key := [2]int32{mail.GiftCommon.PossessionType, mail.GiftCommon.PossessionId}
		if keys[key] {
			pending[key] += mail.GiftCommon.Count
			continue
		}
		kept = append(kept, mail)
	}
	u.Gifts.NotReceived = kept
	return pending
}

// mailPartsOverflow posts an SSR memoir that does not fit into the capped
// inventory to the gift box. The exact rolled variant id is mailed; the
// claim path grants it back without re-rolling (GrantPartsExact).
func (g *PossessionGranter) mailPartsOverflow(user *UserState, partsId, rarity int32, nowMillis int64) {
	user.AddGift(NotReceivedGiftState{
		GiftCommon: GiftCommonState{
			PossessionType: int32(model.PossessionTypeParts),
			PossessionId:   partsId,
			Count:          1,
			GrantDatetime:  nowMillis,
		},
		ExpirationDatetime: nowMillis + partsGiftExpiryMillis,
		UserGiftUuid:       uuid.New().String(),
	})
	log.Printf("[GrantParts] inventory at cap: mailed SSR partsId=%d rarity=%d to gift box", partsId, rarity)
}

// grantOrSellBareDropPart handles a dropped part unknown to master data:
// at the inventory cap it sells for the cheapest table price (drops are
// re-farmable), otherwise it is granted bare. Reports whether it was sold.
func (g *PossessionGranter) grantOrSellBareDropPart(user *UserState, partsId int32, nowMillis int64) bool {
	if partsAtCap(user) {
		if price, ok := g.lowestPartsSellPrice(); ok {
			user.ConsumableItems[g.GoldConsumableItemId] += price
			log.Printf("[GrantParts] unknown drop partsId=%d sold at the cap -> %d gold", partsId, price)
			return true
		}
	}
	g.grantBareParts(user, partsId, nowMillis)
	return false
}

// grantBareParts creates a row for a part unknown to master data.
func (g *PossessionGranter) grantBareParts(user *UserState, partsId int32, nowMillis int64) {
	key := uuid.New().String()
	user.Parts[key] = PartsState{
		UserPartsUuid:        key,
		PartsId:              partsId,
		Level:                1,
		PartsStatusMainValue: 0, // Initialize to 0, will be set on first enhance
		AcquisitionDatetime:  nowMillis,
	}
	log.Printf("[GrantParts] unknown partsId=%d, granted as-is with no variant roll", partsId)
	if g.OnEncyclopediaEntryAdded != nil {
		g.OnEncyclopediaEntryAdded(user)
	}
}

// rollPartsVariant rolls a memoir from the drop's group and rarity. Quest
// rewards are multiplied into several independent memoir drops; each needs a
// fresh rank/sub-status roll instead of cloning the exact requested PartsId.
// Keeping the rarity fixed preserves the quest's intended reward tier, so this
// is the roll for guaranteed rewards (first clear, missions, shop, gifts).
func (g *PossessionGranter) rollPartsVariant(requestedPartsId int32) (int32, PartsRef, bool) {
	ref, ok := g.PartsById[requestedPartsId]
	if !ok {
		return requestedPartsId, PartsRef{}, false
	}
	variants := g.PartsVariantsByGroupRarity[ref.PartsGroupId][ref.RarityType]
	if len(variants) == 0 {
		return requestedPartsId, ref, true
	}
	chosenPartsId := variants[rand.Intn(len(variants))]
	chosenRef, ok := g.PartsById[chosenPartsId]
	if !ok {
		return requestedPartsId, ref, true
	}
	return chosenPartsId, chosenRef, true
}

// partsDropRarityWeights holds the relative odds of each memoir rarity tier on
// battle drops. Master data has no drop-odds table: each quest wires one fixed
// PartsId per memoir set whose rarity scales with difficulty, so the quest's
// max advertised rarity acts as the cap and the eligible weights below it are
// renormalized.
var partsDropRarityWeights = map[int32]int32{10: 50, 20: 30, 30: 15, 40: 5}

// partsDropLotteryByRarity maps a rolled drop rarity to the variant rank
// (PartsInitialLotteryId) it must come with: rank-1 sub-status slots come
// pre-unlocked, so the max rarity drops with all 4 slots filled, the next
// tier with 3, and so on (rank 1 / zero slots never drops).
var partsDropLotteryByRarity = map[int32]int32{10: 2, 20: 3, 30: 4, 40: 5}

// rollPartsDropPiece rolls one piece of a battle-drop memoir set. The wired
// PartsId is only the drop-preview representative; groupId names the concrete
// set piece to roll. The roll picks a weighted rarity up to rarityCap (0 =
// the wired part's own rarity); the rank inside that tier is then fixed by
// partsDropLotteryByRarity, so rarity alone decides how many sub-status
// slots the drop comes with.
func (g *PossessionGranter) rollPartsDropPiece(requestedPartsId, groupId, rarityCap int32) (int32, PartsRef, bool) {
	ref, ok := g.PartsById[requestedPartsId]
	if !ok {
		return requestedPartsId, PartsRef{}, false
	}
	if rarityCap <= 0 {
		rarityCap = ref.RarityType
	}

	byRarity := g.PartsVariantsByGroupRarity[groupId]
	rarities := make([]int32, 0, len(byRarity))
	total := int32(0)
	for r, ids := range byRarity {
		if r <= rarityCap && len(ids) > 0 && partsDropRarityWeights[r] > 0 {
			rarities = append(rarities, r)
			total += partsDropRarityWeights[r]
		}
	}
	if total == 0 {
		return g.rollPartsVariant(requestedPartsId)
	}
	sort.Slice(rarities, func(i, j int) bool { return rarities[i] < rarities[j] })
	pick := rand.Int31n(total)
	chosenRarity := rarities[len(rarities)-1]
	for _, r := range rarities {
		if pick < partsDropRarityWeights[r] {
			chosenRarity = r
			break
		}
		pick -= partsDropRarityWeights[r]
	}
	variants := byRarity[chosenRarity]
	chosenPartsId := g.pickVariantByLottery(variants, partsDropLotteryByRarity[chosenRarity])
	chosenRef, ok := g.PartsById[chosenPartsId]
	if !ok {
		return g.rollPartsVariant(requestedPartsId)
	}
	log.Printf("[GrantParts] drop roll wired=%d cap=%d -> chosen=%d group=%d rarity=%d rank=%d", requestedPartsId, rarityCap, chosenPartsId, chosenRef.PartsGroupId, chosenRef.RarityType, chosenRef.PartsInitialLotteryId)
	return chosenPartsId, chosenRef, true
}

// pickVariantByLottery selects the variant whose rank matches wantLottery,
// falling back to the closest lower rank and finally to a uniform pick when
// the master data has no such variant.
func (g *PossessionGranter) pickVariantByLottery(variants []int32, wantLottery int32) int32 {
	bestId := int32(0)
	bestLottery := int32(-1)
	for _, id := range variants {
		ref, ok := g.PartsById[id]
		if !ok {
			continue
		}
		if ref.PartsInitialLotteryId == wantLottery {
			return id
		}
		if ref.PartsInitialLotteryId < wantLottery && ref.PartsInitialLotteryId > bestLottery {
			bestLottery = ref.PartsInitialLotteryId
			bestId = id
		}
	}
	if bestLottery >= 0 {
		return bestId
	}
	return variants[rand.Intn(len(variants))]
}

// createParts creates one memoir row. Guaranteed rewards (first clear,
// missions, shop, gifts) reach this without a cap check on purpose: they
// can carry rare one-time memoirs that must never be auto-sold.
func (g *PossessionGranter) createParts(user *UserState, chosenPartsId int32, chosenRef PartsRef, nowMillis int64) {
	mainStatId := g.DefaultPartsStatusMainByLotteryGroup[chosenRef.PartsStatusMainLotteryGroupId]
	if _, exists := user.PartsGroupNotes[chosenRef.PartsGroupId]; !exists {
		user.PartsGroupNotes[chosenRef.PartsGroupId] = PartsGroupNoteState{
			PartsGroupId:             chosenRef.PartsGroupId,
			FirstAcquisitionDatetime: nowMillis,
			LatestVersion:            nowMillis,
		}
	}

	// Calculate initial main stat value at level 1
	var initialMainStatValue int32
	if def, ok := g.PartsSubStatusDefs[mainStatId]; ok {
		initialMainStatValue = def.StatusChangeInitialValue
		if def.StatusFunc != nil {
			initialMainStatValue = def.StatusFunc(1)
		}
	}

	key := uuid.New().String()
	user.Parts[key] = PartsState{
		UserPartsUuid:        key,
		PartsId:              chosenPartsId,
		Level:                1,
		PartsStatusMainId:    mainStatId,
		PartsStatusMainValue: initialMainStatValue,
		AcquisitionDatetime:  nowMillis,
	}

	initialCount := chosenRef.PartsInitialLotteryId
	pool := g.PartsSubStatusPool[chosenRef.PartsStatusSubLotteryGroupId]
	if initialCount > 1 && len(pool) > 0 {
		for i := int32(0); i < initialCount-1; i++ {
			pickId := pool[rand.Intn(len(pool))]
			def, ok := g.PartsSubStatusDefs[pickId]
			if !ok {
				continue
			}
			val := def.StatusChangeInitialValue
			if def.StatusFunc != nil {
				val = def.StatusFunc(1)
			}
			user.PartsStatusSubs[PartsStatusSubKey{UserPartsUuid: key, StatusIndex: i + 1}] = PartsStatusSubState{
				UserPartsUuid:           key,
				StatusIndex:             i + 1,
				PartsStatusSubLotteryId: pickId,
				Level:                   1,
				StatusKindType:          def.StatusKindType,
				StatusCalculationType:   def.StatusCalculationType,
				StatusChangeValue:       val,
				LatestVersion:           nowMillis,
			}
		}
	}

	log.Printf("[GrantParts] chosen=%d group=%d rarity=%d preUnlockedSubs=%d", chosenPartsId, chosenRef.PartsGroupId, chosenRef.RarityType, initialCount-1)
	if g.OnEncyclopediaEntryAdded != nil {
		g.OnEncyclopediaEntryAdded(user)
	}
}

func (g *PossessionGranter) GrantWeapon(user *UserState, weaponId int32, nowMillis int64) {
	key := uuid.New().String()
	user.Weapons[key] = WeaponState{
		UserWeaponUuid:      key,
		WeaponId:            weaponId,
		Level:               1,
		AcquisitionDatetime: nowMillis,
	}
	if _, exists := user.WeaponNotes[weaponId]; !exists {
		user.WeaponNotes[weaponId] = WeaponNoteState{
			WeaponId:                 weaponId,
			MaxLevel:                 1,
			MaxLimitBreakCount:       0,
			FirstAcquisitionDatetime: nowMillis,
			LatestVersion:            nowMillis,
		}
	}
	weapon, ok := g.WeaponById[weaponId]
	if !ok {
		return
	}

	g.populateWeaponSkillsAbilities(user, key, weapon)
	if weapon.WeaponStoryReleaseConditionGroupId != 0 {
		changed := false
		for _, cond := range g.ReleaseConditions[weapon.WeaponStoryReleaseConditionGroupId] {
			switch cond.WeaponStoryReleaseConditionType {
			case model.WeaponStoryReleaseConditionTypeAcquisition:
				if grantWeaponStoryUnlock(user, weaponId, cond.StoryIndex, nowMillis) {
					changed = true
				}
			case model.WeaponStoryReleaseConditionTypeQuestClear:
				if qs, ok := user.Quests[cond.ConditionValue]; ok && qs.QuestStateType == model.UserQuestStateTypeCleared {
					if grantWeaponStoryUnlock(user, weaponId, cond.StoryIndex, nowMillis) {
						changed = true
					}
				}
			}
		}
		if changed {
			g.LastChangedStoryWeaponIds = append(g.LastChangedStoryWeaponIds, weaponId)
		}
	}
	if g.OnEncyclopediaEntryAdded != nil {
		g.OnEncyclopediaEntryAdded(user)
	}
}

func (g *PossessionGranter) populateWeaponSkillsAbilities(user *UserState, weaponUuid string, weapon WeaponRef) {
	if slots, ok := g.WeaponSkillSlots[weapon.WeaponSkillGroupId]; ok {
		skills := make([]WeaponSkillState, len(slots))
		for i, slot := range slots {
			skills[i] = WeaponSkillState{
				UserWeaponUuid: weaponUuid,
				SlotNumber:     slot,
				Level:          1,
			}
		}
		user.WeaponSkills[weaponUuid] = skills
	}
	if slots, ok := g.WeaponAbilitySlots[weapon.WeaponAbilityGroupId]; ok {
		abilities := make([]WeaponAbilityState, len(slots))
		for i, slot := range slots {
			abilities[i] = WeaponAbilityState{
				UserWeaponUuid: weaponUuid,
				SlotNumber:     slot,
				Level:          1,
			}
		}
		user.WeaponAbilities[weaponUuid] = abilities
	}
}

func GrantWeaponStoryUnlock(user *UserState, weaponId, storyIndex int32, nowMillis int64) bool {
	return grantWeaponStoryUnlock(user, weaponId, storyIndex, nowMillis)
}

func grantWeaponStoryUnlock(user *UserState, weaponId, storyIndex int32, nowMillis int64) bool {
	hasWeapon := false
	for _, row := range user.Weapons {
		if row.WeaponId == weaponId {
			hasWeapon = true
			break
		}
	}
	if !hasWeapon {
		log.Printf("[grantWeaponStoryUnlock] skipping weaponId=%d (weapon not in user.Weapons)", weaponId)
		return false
	}
	if user.WeaponStories == nil {
		user.WeaponStories = make(map[int32]WeaponStoryState)
	}
	cur := user.WeaponStories[weaponId]
	if storyIndex <= cur.ReleasedMaxStoryIndex {
		return false
	}
	user.WeaponStories[weaponId] = WeaponStoryState{
		WeaponId:              weaponId,
		ReleasedMaxStoryIndex: storyIndex,
		LatestVersion:         nowMillis,
	}
	return true
}

func EnsureDefaultDeck(user *UserState, nowMillis int64) {
	if len(user.Costumes) == 0 || len(user.Decks) > 0 {
		return
	}

	const rionCostumeId = int32(10100)
	const rionWeaponId = int32(101001)

	var costumeUuid, weaponUuid string
	for k, v := range user.Costumes {
		if v.CostumeId == rionCostumeId {
			costumeUuid = k
			break
		}
	}
	for k, v := range user.Weapons {
		if v.WeaponId == rionWeaponId {
			weaponUuid = k
			break
		}
	}

	dcUuid := uuid.New().String()
	user.DeckCharacters[dcUuid] = DeckCharacterState{
		UserDeckCharacterUuid: dcUuid,
		UserCompanionUuid:     "",
		UserCostumeUuid:       costumeUuid,
		MainUserWeaponUuid:    weaponUuid,
		Power:                 100,
		LatestVersion:         nowMillis,
	}
	user.Decks[DeckKey{DeckType: model.DeckTypeQuest, UserDeckNumber: 1}] = DeckState{
		DeckType:                model.DeckTypeQuest,
		UserDeckNumber:          1,
		UserDeckCharacterUuid01: dcUuid,
		Name:                    "Deck 1",
		Power:                   100,
		LatestVersion:           nowMillis,
	}

	if _, exists := user.DeckTypeNotes[model.DeckTypeQuest]; !exists {
		user.DeckTypeNotes[model.DeckTypeQuest] = DeckTypeNoteState{
			DeckType:      model.DeckTypeQuest,
			MaxDeckPower:  100,
			LatestVersion: nowMillis,
		}
	}
}

func FirstSortedKey[V any](m map[string]V) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys[0]
}

// sameUuidSet compares two uuid lists ignoring order.
func sameUuidSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]string(nil), a...)
	y := append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

// slotLoadoutUnchanged reports whether a replacement slot is byte-for-byte the
// same loadout as the old slot occupant. Only then is the old client-reported
// power still valid: any weapon/companion/thought swap changes the power.
func slotLoadoutUnchanged(old DeckCharacterState, oldSubs, oldParts []string, slot DeckCharacterInput) bool {
	if old.UserCostumeUuid != slot.UserCostumeUuid ||
		old.MainUserWeaponUuid != slot.MainUserWeaponUuid ||
		old.UserCompanionUuid != slot.UserCompanionUuid ||
		old.UserThoughtUuid != slot.UserThoughtUuid ||
		old.DressupCostumeId != slot.DressupCostumeId {
		return false
	}
	return sameUuidSet(oldSubs, slot.SubWeaponUuids) && sameUuidSet(oldParts, slot.PartsUuids)
}

func ApplyDeckReplacement(user *UserState, deckType model.DeckType, userDeckNumber int32, slots []DeckCharacterInput, nowMillis int64) {
	deckKey := DeckKey{DeckType: deckType, UserDeckNumber: userDeckNumber}
	deck := user.Decks[deckKey]
	deck.DeckType = deckType
	deck.UserDeckNumber = userDeckNumber
	if deck.Name == "" {
		deck.Name = fmt.Sprintf("Deck %d", userDeckNumber)
	}
	oldDc := [3]DeckCharacterState{}
	oldExists := [3]bool{}
	oldPowers := [3]int32{}
	oldSubs := [3][]string{}
	oldParts := [3][]string{}
	for i, oldUuid := range []string{deck.UserDeckCharacterUuid01, deck.UserDeckCharacterUuid02, deck.UserDeckCharacterUuid03} {
		if oldUuid == "" {
			continue
		}
		if dc, ok := user.DeckCharacters[oldUuid]; ok {
			oldExists[i] = true
			oldDc[i] = dc
			oldPowers[i] = dc.Power
			oldSubs[i] = user.DeckSubWeapons[oldUuid]
			oldParts[i] = user.DeckParts[oldUuid]
		}
		delete(user.DeckCharacters, oldUuid)
		delete(user.DeckSubWeapons, oldUuid)
		delete(user.DeckParts, oldUuid)
	}

	var newUuids [3]string
	for i := range 3 {
		if i >= len(slots) || slots[i].UserCostumeUuid == "" {
			continue
		}
		slot := slots[i]
		dcUuid := uuid.New().String()
		dc := DeckCharacterState{
			UserDeckCharacterUuid: dcUuid,
			UserCostumeUuid:       slot.UserCostumeUuid,
			MainUserWeaponUuid:    slot.MainUserWeaponUuid,
			UserCompanionUuid:     slot.UserCompanionUuid,
			UserThoughtUuid:       slot.UserThoughtUuid,
			DressupCostumeId:      slot.DressupCostumeId,
			LatestVersion:         nowMillis,
		}
		// Carry the client-reported power over only when the slot's loadout is
		// completely unchanged. A stale carried power is worse than unknown: it
		// shows a wrong number in the ranking AND counts as "known", which
		// blocks RecalcDeckPower from picking up the next fresh report.
		if oldExists[i] && slotLoadoutUnchanged(oldDc[i], oldSubs[i], oldParts[i], slot) {
			dc.Power = oldPowers[i]
		}
		user.DeckCharacters[dcUuid] = dc
		user.DeckSubWeapons[dcUuid] = slot.SubWeaponUuids
		user.DeckParts[dcUuid] = slot.PartsUuids
		newUuids[i] = dcUuid
	}

	// Second pass: a new character that still has no power borrows the known
	// power of the same costume from any discarded slot of the old deck. The
	// client only reports power on some flows (fresh deck creation), so
	// without this an edited/reduced deck stays "unknown" forever and the
	// ranking falls back to the 100 floor. Same costume keeps the estimate in
	// the right ballpark; the next genuine report overwrites it.
	for i := range 3 {
		u := newUuids[i]
		if u == "" {
			continue
		}
		dc := user.DeckCharacters[u]
		if dc.Power > 0 {
			continue
		}
		for j := range 3 {
			if oldExists[j] && oldPowers[j] > 0 && oldDc[j].UserCostumeUuid == dc.UserCostumeUuid {
				dc.Power = oldPowers[j]
				user.DeckCharacters[u] = dc
				break
			}
		}
	}

	deck.UserDeckCharacterUuid01 = newUuids[0]
	deck.UserDeckCharacterUuid02 = newUuids[1]
	deck.UserDeckCharacterUuid03 = newUuids[2]
	deck.LatestVersion = nowMillis

	// Derive the stored total from the carried-over character powers and never
	// keep the old total: the client does NOT re-report the true total after
	// every edit, so a kept total perpetuates a number that matches no current
	// deck. The 100 floor keeps freshly composed decks assignable (the client
	// refuses a defense deck whose stored power is 0).
	var carried int32
	for _, u := range newUuids {
		if u == "" {
			continue
		}
		carried += user.DeckCharacters[u].Power
	}
	if carried > 0 {
		deck.Power = carried
	} else {
		deck.Power = 100
	}
	user.Decks[deckKey] = deck
}

// DeckPowerReportEntry is one client-reported per-character power
// (DeckCharacterPower in the proto), flattened in slot order 1..3.
type DeckPowerReportEntry struct {
	UserDeckCharacterUuid string
	Power                 int32
}

// ApplyDeckPowerReport writes client-reported character powers into a deck's
// slot characters. Entries are matched by deck-character uuid first; entries
// whose uuid no longer exists (a report can lag one ReplaceDeck behind, since
// the replacement mints fresh uuids) fall back to slot position —
// DeckCharacterPower01..03 mirror the deck slots in order. Returns how many
// entries were applied.
func ApplyDeckPowerReport(user *UserState, deckType model.DeckType, userDeckNumber int32, entries []DeckPowerReportEntry) int {
	key := DeckKey{DeckType: deckType, UserDeckNumber: userDeckNumber}
	deck, ok := user.Decks[key]
	if !ok {
		return 0
	}
	slotUuids := [3]string{deck.UserDeckCharacterUuid01, deck.UserDeckCharacterUuid02, deck.UserDeckCharacterUuid03}
	matchedByUuid := make([]bool, len(entries))
	applied := 0
	for i, e := range entries {
		if e.Power <= 0 || e.UserDeckCharacterUuid == "" {
			continue
		}
		if dc, ok := user.DeckCharacters[e.UserDeckCharacterUuid]; ok {
			dc.Power = e.Power
			user.DeckCharacters[e.UserDeckCharacterUuid] = dc
			matchedByUuid[i] = true
			applied++
		}
	}
	for i, e := range entries {
		if matchedByUuid[i] || e.Power <= 0 || i >= 3 {
			continue
		}
		uuid := slotUuids[i]
		if uuid == "" {
			continue
		}
		if dc, ok := user.DeckCharacters[uuid]; ok {
			dc.Power = e.Power
			user.DeckCharacters[uuid] = dc
			applied++
		}
	}
	return applied
}

// RecalcDeckPower rewrites a deck's stored total power as the sum of its
// characters' client-reported powers, but only when every occupied slot has a
// known power (a partial sum would understate the real total). This is the
// trustworthy source of a deck's strength: the reported deck total can be a
// stale echo of an earlier save, while the per-character powers match the
// client's own math exactly. Returns the stored power after the update.
func RecalcDeckPower(user *UserState, deckType model.DeckType, userDeckNumber int32) int32 {
	key := DeckKey{DeckType: deckType, UserDeckNumber: userDeckNumber}
	deck, ok := user.Decks[key]
	if !ok {
		return 0
	}
	var sum int32
	occupied, known := 0, 0
	for _, u := range []string{deck.UserDeckCharacterUuid01, deck.UserDeckCharacterUuid02, deck.UserDeckCharacterUuid03} {
		if u == "" {
			continue
		}
		occupied++
		if dc, ok := user.DeckCharacters[u]; ok && dc.Power > 0 {
			known++
			sum += dc.Power
		}
	}
	if occupied > 0 && known == occupied {
		deck.Power = sum
		user.Decks[key] = deck
	}
	return deck.Power
}

func RemoveDeckData(user *UserState, deckType model.DeckType, userDeckNumber int32) {
	deckKey := DeckKey{DeckType: deckType, UserDeckNumber: userDeckNumber}
	deck, ok := user.Decks[deckKey]
	if !ok {
		return
	}
	for _, dcUuid := range []string{deck.UserDeckCharacterUuid01, deck.UserDeckCharacterUuid02, deck.UserDeckCharacterUuid03} {
		if dcUuid == "" {
			continue
		}
		delete(user.DeckCharacters, dcUuid)
		delete(user.DeckSubWeapons, dcUuid)
		delete(user.DeckParts, dcUuid)
	}
	delete(user.Decks, deckKey)
}

func ReadDeckSlots(user *UserState, deckType model.DeckType, userDeckNumber int32) []DeckCharacterInput {
	deckKey := DeckKey{DeckType: deckType, UserDeckNumber: userDeckNumber}
	deck, ok := user.Decks[deckKey]
	if !ok {
		return nil
	}
	slots := make([]DeckCharacterInput, 3)
	for i, dcUuid := range []string{deck.UserDeckCharacterUuid01, deck.UserDeckCharacterUuid02, deck.UserDeckCharacterUuid03} {
		if dcUuid == "" {
			continue
		}
		dc, ok := user.DeckCharacters[dcUuid]
		if !ok {
			continue
		}
		slots[i] = DeckCharacterInput{
			UserCostumeUuid:    dc.UserCostumeUuid,
			MainUserWeaponUuid: dc.MainUserWeaponUuid,
			SubWeaponUuids:     user.DeckSubWeapons[dcUuid],
			PartsUuids:         user.DeckParts[dcUuid],
			UserCompanionUuid:  dc.UserCompanionUuid,
			UserThoughtUuid:    dc.UserThoughtUuid,
			DressupCostumeId:   dc.DressupCostumeId,
		}
	}
	return slots
}
