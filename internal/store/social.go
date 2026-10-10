package store

// FriendEdge is one entry in a user's friend list. Keyed by the friend's playerId
// in UserState.Friends. Cheer flags are reset daily (see service layer).
type FriendEdge struct {
	PlayerId             int64
	BecameFriendsAt      int64 // millis
	CheerSentToday       bool  // I have cheered them today
	CheerReceivedPending bool  // they cheered me; I can still collect the reward
	StaminaReceivedToday bool  // I have collected the cheer reward today
	LastResetDay         int64 // gametime.StartOfDayMillis() bucket of last reset
	LatestVersion        int64
}

// FriendRequest is a pending request, keyed by the other player's id in
// UserState.IncomingFriendRequests / OutgoingFriendRequests.
type FriendRequest struct {
	PlayerId      int64
	RequestedAt   int64 // millis
	LatestVersion int64
}

// PvpState holds a user's Arena standing and counters.
type PvpState struct {
	PvpPoint         int32
	AttackWinCount   int32
	AttackLoseCount  int32
	DefenseWinCount  int32
	DefenseLoseCount int32
	// AttackWinStreak is the current run of consecutive attack victories,
	// reset to zero on any attack loss. Feeds type-17 "win N in a row" missions.
	AttackWinStreak int32
	LastFinishDay   int64
	// DefenseDeckNumber is the PvP deck slot the player exposes on defense,
	// chosen via SetPvpDefenseDeck. Zero means "not set" (fall back to PvP deck 1).
	DefenseDeckNumber int32
	// WeeklyRewardVersion is the WeeklyVersion (Monday-00:00 bucket) the player
	// is currently credited for. When a later week is first observed, the grade
	// reward for the just-ended week is made pending and this advances.
	WeeklyRewardVersion int64
	// PendingWeeklyRewardGroupId is the grade weekly reward group awaiting claim
	// via RewardService.ReceivePvpReward (0 = nothing pending). The Point/SeasonId
	// companions describe it for the client's WeeklyGradeResult display.
	PendingWeeklyRewardGroupId  int32
	PendingWeeklyRewardPoint    int32
	PendingWeeklyRewardSeasonId int32
	// LastAutoProgressDay is the StartOfDay bucket for which the daily arena
	// auto-progress tick (login bonus companion) already ran. Zero/older means
	// the next login-bonus claim triggers a new tick.
	LastAutoProgressDay int64
	// MatchingRefreshCount is how many times the player has rebuilt their
	// arena opponent list today (capped by the arena's daily refresh limit).
	// MatchingRefreshDay is the StartOfDay bucket the count belongs to; when
	// the day rolls over the count resets to zero.
	MatchingRefreshCount int32
	MatchingRefreshDay   int64
	// SeasonIndex is a legacy visual season counter advanced once per finished
	// reward week; it no longer selects reward tables (those always come from
	// the latest valid masterdata season).
	SeasonIndex   int32
	LatestVersion int64
	// MaxSeasonRank is the best (lowest number) ladder rank the player reached
	// during the season identified by MaxSeasonRankSeasonId; 0 means "no rank
	// recorded yet". Refreshed after every rated battle (own FinishBattle and
	// the daily auto-progress, including real opponents' defense side). When
	// the active masterdata season id no longer matches, the record belongs to
	// a past season and is treated as unset.
	MaxSeasonRank         int32
	MaxSeasonRankSeasonId int32
	// PvpDeckBackup is a JSON snapshot of the server-managed arena deck #1
	// (slots, power, name) taken whenever the server copies a lineup into the
	// arena. Before each auto-battle day the arena deck is compared against
	// this backup and restored from it if the player edited the deck manually
	// — restoring from the quest deck directly is not safe, because that deck
	// may have been changed too.
	PvpDeckBackup string
}

// BattleLogEntry is one row of attack or defense history (capped to the most recent N).
type BattleLogEntry struct {
	Seq               int64 // monotonic per-user ordering key (use nowMillis at insert)
	OpponentPlayerId  int64
	OpponentName      string
	OpponentPvpPoint  int32
	OpponentDeckPower int32
	IsVictory         bool
	BattleDatetime    int64 // millis
	FluctuatedPoint   int32
	Rank              int32
}

// MatchingEntry is a cached opponent shown in the current matching list, so
// StartBattle can recall the chosen opponent's snapshot/bot identity.
type MatchingEntry struct {
	PlayerId              int64
	Name                  string
	PvpPoint              int32
	Rank                  int32
	DeckPower             int32
	IsBot                 bool
	MostPowerfulCostumeId int32
	// DeckMainWeaponAttributeTypes holds the element (attribute) of each of the
	// opponent deck's main weapons. The client renders one element icon per slot
	// on the battle-target screen and indexes fixed slots, so an empty list here
	// freezes that screen.
	DeckMainWeaponAttributeTypes []int32
}
