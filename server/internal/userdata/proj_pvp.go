package userdata

import (
	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/store"
	"lunar-tear/server/internal/utils"
)

// pvpMaxBattlePointMilli is the arena BP cap in milli-units. The client stores
// arena BP in the same "stamina" shape as the main AP (1 unit = 1000 milli) and
// caps it at m_config PVP_MAX_BATTLE_POINT (= 100). Each battle costs
// PVP_BATTLE_CONSUME_BATTLE_POINT (10) and a reroll costs
// PVP_UPDATE_MATCHING_CONSUME_BATTLE_POINT (5); BP regenerates 1 unit every
// USER_BATTLE_POINT_RECOVERY_SECOND (180s).
const pvpMaxBattlePointMilli = 100 * 1000

func init() {
	// IUserPvpStatus carries the player's arena BP (as staminaMilliValue) plus
	// win-streak and reward-receipt bookkeeping. The client reads BP from this
	// record before letting you press Start; if the table is empty the arena
	// throws locally ("fail to reconnect") and shows 0 BP. The server does not
	// yet track/deduct BP, so we report it full on every fetch (effectively
	// unlimited BP, matching the preservation-server stance). Fields and casing
	// match the client's EntityIUserPvpStatus.
	register("IUserPvpStatus", func(user store.UserState) string {
		now := gametime.NowMillis()
		s, _ := utils.EncodeJSONMaps(map[string]any{
			"userId":                              user.UserId,
			"staminaMilliValue":                   pvpMaxBattlePointMilli,
			"staminaUpdateDatetime":               now,
			"latestRewardReceivePvpSeasonId":      user.Pvp.PendingWeeklyRewardSeasonId,
			"latestRewardReceivePvpWeeklyVersion": user.Pvp.WeeklyRewardVersion,
			"winStreakCount":                      user.Pvp.AttackWinStreak,
			"winStreakCountUpdateDatetime":        user.Pvp.LastFinishDay,
			"latestVersion":                       now,
		})
		return s
	})

	// IUserPvpDefenseDeck tells the client which PvP deck slot is exposed on
	// defense. Without this record the client treats the defense deck as unset
	// and falls back to its own auto-pick/auto-create behavior, so the slot
	// chosen via SetPvpDefenseDeck must be surfaced here (and flagged in
	// ChangedTables so the SetPvpDefenseDeck diff carries it).
	register("IUserPvpDefenseDeck", func(user store.UserState) string {
		if user.Pvp.DefenseDeckNumber <= 0 {
			return "[]"
		}
		s, _ := utils.EncodeJSONMaps(map[string]any{
			"userId":         user.UserId,
			"userDeckNumber": user.Pvp.DefenseDeckNumber,
			"latestVersion":  gametime.NowMillis(),
		})
		return s
	})

	// IUserPvpWeeklyResult (past weekly standings) has no server state yet;
	// an empty record set is valid (no history).
	registerStatic("IUserPvpWeeklyResult")
}
