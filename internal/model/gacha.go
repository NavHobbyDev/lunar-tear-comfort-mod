package model

const (
	GachaLabelUnknown    int32 = 0
	GachaLabelPremium    int32 = 1
	GachaLabelEvent      int32 = 2
	GachaLabelChapter    int32 = 3
	GachaLabelPortalCage int32 = 4
	GachaLabelRecycle    int32 = 5
)

const (
	GachaModeUnknown int32 = 0
	GachaModeBasic   int32 = 1
	GachaModeStepup  int32 = 2
	GachaModeBox     int32 = 3
)

const (
	GachaUnlockUnknown                  int32 = 0
	GachaUnlockNone                     int32 = 1
	GachaUnlockUserRankGreaterOrEqual   int32 = 2
	GachaUnlockWithinHoursFromGameStart int32 = 3
	GachaUnlockMainQuestClear           int32 = 4
	GachaUnlockMainFunctionReleased     int32 = 5
)

const (
	GachaAutoResetUnknown int32 = 0
	GachaAutoResetNone    int32 = 1
	GachaAutoResetDaily   int32 = 2
	GachaAutoResetMonthly int32 = 3
)

// Shop item limited-stock reset types use the same values as gacha reset
// types in the master data.
const (
	ShopItemAutoResetUnknown int32 = 0
	ShopItemAutoResetNone    int32 = 1
	ShopItemAutoResetDaily   int32 = 2
	ShopItemAutoResetMonthly int32 = 3
)

const (
	GachaDecorationUnknown  int32 = 0
	GachaDecorationNormal   int32 = 1
	GachaDecorationFestival int32 = 2
)

const (
	GachaBadgeTypeNone int32 = 1
)

const (
	PriceTypeUnknown         int32 = 0
	PriceTypeConsumableItem  int32 = 1
	PriceTypeGem             int32 = 2
	PriceTypePaidGem         int32 = 3
	PriceTypePlatformPayment int32 = 4
)

const (
	BannerPrefixLimited = "limited_"
	BannerPrefixStepUp  = "step_up_"
	BannerPrefixCommon  = "common_"
)

func IsMaterialBanner(labelType int32) bool {
	return labelType == GachaLabelChapter || labelType == GachaLabelRecycle || labelType == GachaLabelPortalCage
}

const MomBannerDomainGacha int32 = 1

const StepUpGroupDivisor int32 = 1000

const (
	PityCeilingCount   int32 = 200
	MedalCountCap      int32 = 9999
	WeaponInventoryCap int32 = 999
)

// Pity exchange mail payout: what is sent to the gift box for every full
// 100 gacha medals drained.
const (
	PityGiftPaidGemCount int32 = 1000

	PityGiftMamaPointId    int32 = 9001 // Mama Point (consumable)
	PityGiftMamaPointCount int32 = 100

	PityGiftResurrectMedalId    int32 = 242 // Countdown Resurrected Event Medal (consumable)
	PityGiftResurrectMedalCount int32 = 10
)

const (
	PremiumSinglePullPrice int32 = 300
	PremiumMultiPullPrice  int32 = 3000
	PremiumMultiPullCount  int32 = 10
)

const (
	StepUpStep1Cost int32 = 2000
	StepUpStep3Cost int32 = 3000
	StepUpStep5Cost int32 = 5000
	StepUpFreeCost  int32 = 0
)

const (
	FeaturedRateUpPercent int = 35
	FeaturedRateUpDenom   int = 100
)

const (
	StepUpRateBoost    float64 = 1.5
	StepUpRateMaxBoost float64 = 2.0
)

const (
	DupGradeMin int32 = 1
	DupGradeMax int32 = 5
)

// DupGradeWeights biases the dup-exchange grade roll: high grades (poorer
// payouts) are more common, grade 1 (the 200% jackpot) is the rarest.
var DupGradeWeights = map[int32]int{
	1: 5,
	2: 10,
	3: 20,
	4: 30,
	5: 35,
}

// DupGradePayoutPercent scales the dup-exchange payout by the rolled grade,
// in percent of the base count. With the base of 10 books this yields:
// grade 1 -> 20, 2 -> 16, 3 -> 14, 4 -> 12, 5 -> 10.
var DupGradePayoutPercent = map[int32]int32{
	1: 200,
	2: 160,
	3: 140,
	4: 120,
	5: 100,
}

type DupExchangeEntry struct {
	PossessionType int32
	PossessionId   int32
	Count          int32
}

const DefaultDailyDrawLimit int32 = 5

const (
	BoxPoolMaxItems    int   = 50
	BoxPoolMinItems    int   = 5
	BoxItemDefaultMax  int32 = 10
	BoxFallbackItemMax int32 = 20
	BoxFallbackItemId  int32 = 100001
)

const PhaseIdMultiplier int32 = 10

const (
	ConsumableIdPremiumTicket            int32 = 1
	ConsumableIdChapterTicket            int32 = 2
	ConsumableIdDarkMemorySummonTicket   int32 = 2002
)
