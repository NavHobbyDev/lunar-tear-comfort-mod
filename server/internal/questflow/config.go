package questflow

const (
	// QuestRewardMultiplier controls the server-side multiplier for ordinary
	// quest rewards, including the selected reward types in rewards.go and
	// gold rewards granted from both normal clears and skip clears.
	QuestRewardMultiplier int32 = 10
	
	// RewardGemBonus is the bonus amount added to gems in all rewards
	RewardGemBonus int32 = 10

	// MissionRewardGemBonus is the bonus multiplier for free gems in mission
	// rewards. It only applies to Challenge missions (MissionCategoryType 2);
	// every other mission category pays its gems as authored.
	MissionRewardGemBonus int32 = 10

	// GoldBonusRate is the multiplier rate for gold rewards
	GoldBonusRate int32 = 10

	// PartsDropCountPerQuest is how many memoirs drop per quest run (and per
	// skip iteration). Each drop position draws one piece deck-style from the
	// pool of all pieces of all advertised memoir sets; rarity is rolled per
	// piece and fixes the number of pre-unlocked sub-status slots.
	PartsDropCountPerQuest int32 = 5
)
