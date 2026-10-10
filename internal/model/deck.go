package model

type DeckType int32

const (
	DeckTypeUnknown                     DeckType = 0
	DeckTypeQuest                       DeckType = 1
	DeckTypePvp                         DeckType = 2
	DeckTypeMulti                       DeckType = 3
	DeckTypeRestrictedQuest             DeckType = 4
	DeckTypeBigHunt                     DeckType = 5
	DeckTypeRestrictedLimitContentQuest DeckType = 6
)
