package model

type GimmickType int32

const (
	GimmickTypeUnknown                 GimmickType = 0
	GimmickTypeCageTreasureHunt        GimmickType = 1  // "Fickle Black Birds" — in-cage flick-and-tap birds
	GimmickTypeCageIntervalDropItem    GimmickType = 2  // "Lost Items" — in-cage 3-dot drops that respawn on interval
	GimmickTypeBrokenObelisk           GimmickType = 3  // "Broken Scarecrow" (per tip text); zero rows in m_gimmick, unused
	GimmickTypeIronGrill               GimmickType = 4  // unused (zero rows in m_gimmick), in-game name unknown
	GimmickTypeRadioMessage            GimmickType = 5  // unused (zero rows in m_gimmick); client has GimmickRadioMessage class but no data
	GimmickTypeFirstBrokenObelisk      GimmickType = 6  // variant of Broken Scarecrow; zero rows in m_gimmick, unused
	GimmickTypeMapOnlyCageTreasureHunt GimmickType = 7  // "Hidden Black Birds" — world-map birds; per-tap reward from m_cage_ornament_reward
	GimmickTypeMapOnlyCageIntervalDrop GimmickType = 8  // map-side variant of Lost Items
	GimmickTypeReport                  GimmickType = 9  // "Hidden Stories" — hidden mission markers
	GimmickTypeCageMemory              GimmickType = 10 // "Lost Archives" — collectible library entries (one-shot ImportantItem type-4)
	GimmickTypeMapOnlyHideObelisk      GimmickType = 11 // "Stray Scarecrow" — world-map scarecrows (not yet implemented)
)
