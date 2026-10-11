package service

import (
	"math/rand"
	"time"

	"github.com/google/uuid"

	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
)

// pvpGiftExpiryMillis is how long an auto-mailed PvP reward stays claimable.
const pvpGiftExpiryMillis = int64(30 * 24 * time.Hour / time.Millisecond)

// pvpWeekMillis is the length of one weekly-reward bucket (Monday to Monday).
const pvpWeekMillis = int64(7 * 24 * time.Hour / time.Millisecond)

// grantPvpRewardItems drops reward items straight into the player's inventory
// (arena coins, materials, important items) via the shared granter, mirroring
// how BigHunt rewards are handed out.
func grantPvpRewardItems(user *store.UserState, granter *store.PossessionGranter, items []masterdata.RewardItem, nowMillis int64) {
	for _, item := range items {
		granter.GrantFull(user, model.PossessionType(item.PossessionType), item.PossessionId, item.Count, nowMillis)
	}
}

// mailPvpRewardItems posts reward items to the gift box (the in-game mail).
// Counts are merged per (possession type, id) first so a whole day of
// simulated battles lands as one compact set of gift rows. Existing unreceived
// mails carrying the same items are consolidated (removed and summed into the
// new ones) to prevent mailbox clutter. This is how the arena reward tabs are
// delivered: every arena day the items are mailed automatically and the client
// only gets the announcement popup on top.
func mailPvpRewardItems(user *store.UserState, items []masterdata.RewardItem, nowMillis int64) {
	merged := mergeRewardItems(items)
	if len(merged) == 0 {
		return
	}

	// Build key set for consolidation
	keys := make(map[[2]int32]bool, len(merged))
	for _, item := range merged {
		keys[[2]int32{item.PossessionType, item.PossessionId}] = true
	}
	pending := user.ConsolidateGifts(keys)

	expiry := nowMillis + pvpGiftExpiryMillis
	gifts := make([]store.NotReceivedGiftState, 0, len(merged))
	for _, item := range merged {
		key := [2]int32{item.PossessionType, item.PossessionId}
		gifts = append(gifts, store.NotReceivedGiftState{
			GiftCommon: store.GiftCommonState{
				PossessionType: item.PossessionType,
				PossessionId:   item.PossessionId,
				Count:          item.Count + pending[key],
				GrantDatetime:  nowMillis,
			},
			ExpirationDatetime: expiry,
			UserGiftUuid:       uuid.New().String(),
		})
	}
	user.AddGiftsOrdered(gifts)
}

// mergeRewardItems sums counts per (possession type, id), preserving the
// first-seen order so the gift box shows a stable item layout.
// BP consumables (4001-4003) are excluded from mailing.
func mergeRewardItems(items []masterdata.RewardItem) []masterdata.RewardItem {
	if len(items) == 0 {
		return nil
	}
	type key struct{ typ, id int32 }
	order := make([]key, 0, len(items))
	counts := make(map[key]int32, len(items))
	for _, it := range items {
		// Filter out BP consumables (4001-4003) - don't mail them
		if it.PossessionType == int32(model.PossessionTypeConsumableItem) &&
			it.PossessionId >= 4001 && it.PossessionId <= 4003 {
			continue
		}

		k := key{it.PossessionType, it.PossessionId}
		if _, seen := counts[k]; !seen {
			order = append(order, k)
		}
		counts[k] += it.Count
	}
	out := make([]masterdata.RewardItem, 0, len(order))
	for _, k := range order {
		out = append(out, masterdata.RewardItem{PossessionType: k.typ, PossessionId: k.id, Count: counts[k]})
	}
	return out
}

// advanceWeeklyCycle implements the lazy weekly boundary (Monday 00:00).
// The rewards themselves are all handled by the caller every arena day
// (per-match to inventory + every mailed reward tab, season ranking reward
// included); this function only runs the once-a-week bookkeeping left over
// from the weekly design: it advances the legacy SeasonIndex counter and
// snapshots the grade weekly result for the one-shot aggregate-report popup
// (GetTopData surfaces it once, then clears the flag). This must run inside
// an UpdateUser closure so the mutation is persisted.
func advanceWeeklyCycle(pvpCat *masterdata.PvpCatalog, user *store.UserState, nowMillis int64) {
	if pvpCat == nil {
		return
	}
	wv := gametime.WeeklyVersion(nowMillis)
	if user.Pvp.WeeklyRewardVersion == 0 {
		// First contact: start tracking from the current week, no back-pay.
		user.Pvp.WeeklyRewardVersion = wv
		return
	}
	if wv <= user.Pvp.WeeklyRewardVersion {
		return
	}

	missedWeeks := int((wv - user.Pvp.WeeklyRewardVersion) / pvpWeekMillis)
	if missedWeeks < 1 {
		missedWeeks = 1
	}
	user.Pvp.SeasonIndex += int32(missedWeeks) // legacy visual counter, no longer reward-relevant

	// Snapshot the new week's grade weekly result for the one-shot
	// announcement popup (GetTopData surfaces it once, then clears the flag).
	user.Pvp.PendingWeeklyRewardGroupId = 0
	user.Pvp.PendingWeeklyRewardPoint = 0
	user.Pvp.PendingWeeklyRewardSeasonId = 0
	if season, ok := pvpCat.CurrentSeason(nowMillis); ok {
		if grade, gok := pvpCat.GradeForPoint(season.GradeGroupId, user.Pvp.PvpPoint); gok && grade.WeeklyRewardGroupId != 0 {
			user.Pvp.PendingWeeklyRewardGroupId = grade.WeeklyRewardGroupId
			user.Pvp.PendingWeeklyRewardPoint = user.Pvp.PvpPoint
			user.Pvp.PendingWeeklyRewardSeasonId = season.SeasonId
		}
	}
	user.Pvp.WeeklyRewardVersion = wv
}

// pvpRoll returns a uniform value in [0,total) for weighted reward selection.
func pvpRoll(total int32) int32 {
	if total <= 0 {
		return 0
	}
	return rand.Int31n(total)
}
