package model

type TutorialType int32

const (
	TutorialTypeUnknown                    TutorialType = 0
	TutorialTypeGameStart                  TutorialType = 1
	TutorialTypeMenuFirst                  TutorialType = 2
	TutorialTypeMenuSecond                 TutorialType = 3
	TutorialTypeBattleWeaponSkill          TutorialType = 4
	TutorialTypeBattleCostumeSkill         TutorialType = 5
	TutorialTypeBlackBird                  TutorialType = 6
	TutorialTypeEnhance                    TutorialType = 7
	TutorialTypeCompanion                  TutorialType = 8
	TutorialTypeParts                      TutorialType = 9
	TutorialTypeExplore                    TutorialType = 10
	TutorialTypePvp                        TutorialType = 11
	TutorialTypeMainQuestHard              TutorialType = 12
	TutorialTypeMainQuestVeryHard          TutorialType = 13
	TutorialTypeEventQuestFirst            TutorialType = 14
	TutorialTypeEventQuestCharacter        TutorialType = 15
	TutorialTypeEventQuestMarathon         TutorialType = 16
	TutorialTypeEventQuestHunt             TutorialType = 17
	TutorialTypeEventQuestDungeon          TutorialType = 18
	TutorialTypeEventQuestDayOfTheWeek     TutorialType = 19
	TutorialTypeEventQuestGuerrilla        TutorialType = 20
	TutorialTypeEndContents                TutorialType = 21
	TutorialTypeEndContentsQuest           TutorialType = 22
	TutorialTypeExploreGame1               TutorialType = 23
	TutorialTypeExploreGame2               TutorialType = 24
	TutorialTypePortalCage                 TutorialType = 25
	TutorialTypePortalCageMainQuest        TutorialType = 26
	TutorialTypeCage                       TutorialType = 27
	TutorialTypePortalCageDailyQuest       TutorialType = 28
	TutorialTypePortalCageDailyGacha       TutorialType = 29
	TutorialTypePortalCageDropItem         TutorialType = 30
	TutorialTypePortalCageReachedLastScene TutorialType = 31
	TutorialTypePortalCageCharacter1       TutorialType = 32
	TutorialTypePortalCageCharacter2       TutorialType = 33
	TutorialTypePortalCageCharacter3       TutorialType = 34
	TutorialTypePortalCageCharacter4       TutorialType = 35
	TutorialTypePortalCageCharacter5       TutorialType = 36
	TutorialTypeBlackBirdCharacter1        TutorialType = 37
	TutorialTypeBlackBirdCharacter2        TutorialType = 38
	TutorialTypeBlackBirdCharacter3        TutorialType = 39
	TutorialTypeGohobi                     TutorialType = 40
	TutorialTypeGohobiDrop                 TutorialType = 41
	TutorialTypeBattleCancelEnemyCast1     TutorialType = 42
	TutorialTypeBattleCancelEnemyCast2     TutorialType = 43
	TutorialTypeLoseFirst                  TutorialType = 44
	TutorialTypeRewardGacha                TutorialType = 45
	TutorialTypeBigWinBonusFirst           TutorialType = 46
	TutorialTypeBigHunt                    TutorialType = 47
	TutorialTypeTripleDeck                 TutorialType = 48
	TutorialTypeCharacterBoard             TutorialType = 49
	TutorialTypeCharacterBoardBasic        TutorialType = 50
	TutorialTypeCharacterBoardBigHunt      TutorialType = 51
	TutorialTypeWorldMap                   TutorialType = 52
	TutorialTypeMapItemFull                TutorialType = 53
	TutorialTypeWorldMapBlackBird          TutorialType = 54
	TutorialTypeWorldMapTreasure           TutorialType = 55
	TutorialTypeBrokenObelisk              TutorialType = 56
	TutorialTypeLoseFirstAfterChapter      TutorialType = 57
	TutorialTypeReplayFlowSkip             TutorialType = 58
	TutorialTypeWorldMapOutgame            TutorialType = 59
	TutorialTypeBattleCertainKillSkill     TutorialType = 60
	TutorialTypeSmartPhoneFirst            TutorialType = 101
	TutorialTypePhotoFirst                 TutorialType = 102
	TutorialTypeDailyGacha                 TutorialType = 103
	TutorialTypePortalCageSeason           TutorialType = 104
	TutorialTypeQuestSkip                  TutorialType = 201
	TutorialTypePortalCageChapter          TutorialType = 202
	TutorialTypeCharacterBoardUnlock       TutorialType = 301
	TutorialTypeBlackBirdSistersFirst      TutorialType = 401
	TutorialTypeCostumeLevelBonus          TutorialType = 501
	TutorialTypeWorldMapReport             TutorialType = 601
	TutorialTypeBossSpecialEffect          TutorialType = 701
	TutorialTypeEventQuestGuerrillaFree    TutorialType = 801
	TutorialTypeExploreHard                TutorialType = 901
	TutorialTypeCageMemory                 TutorialType = 1001
	TutorialTypeDressupCostume             TutorialType = 1101
	TutorialTypeCostumeAwaken              TutorialType = 1201
	TutorialTypeThoughtOrganization        TutorialType = 1202
	TutorialTypeHideObelisk                TutorialType = 1301
	TutorialTypeLimitContent               TutorialType = 1302
	TutorialTypeFieldEffect                TutorialType = 1303
	TutorialTypeLimitContentCage           TutorialType = 1304
	TutorialTypeCharacterViewer            TutorialType = 1305
	TutorialTypeRecycleGacha               TutorialType = 1306
	TutorialTypeMomPoint                   TutorialType = 1401
	TutorialTypeStainedGlass               TutorialType = 1402
	TutorialTypeCharacterRebirth           TutorialType = 1501
	TutorialTypeWeaponAwaken               TutorialType = 1502
	TutorialTypeEventQuestLabyrinth        TutorialType = 1503
	TutorialTypeProperAttribute            TutorialType = 1601
	TutorialTypeMissionPass                TutorialType = 1701
	TutorialTypeWeaponAllOrganization      TutorialType = 1702
	TutorialTypeCostumeLotteryEffect       TutorialType = 1801
	TutorialTypeAnotherRoute               TutorialType = 2001
	TutorialTypeDeleteCostumeFio           TutorialType = 2101
)

type TutorialUnlockConditionType int32

const (
	TutorialUnlockConditionTypeFunctionReleased         TutorialUnlockConditionType = 1
	TutorialUnlockConditionTypeReachSpecifiedQuestScene TutorialUnlockConditionType = 2
	TutorialUnlockConditionTypeUntilReachSpecifiedScene TutorialUnlockConditionType = 3
)

// TutorialPhase values are runtime-initialized in the client (static readonly),
// so only observed values are listed here.
type TutorialPhase int32

const (
	TutorialPhaseMomMenuEditDeck TutorialPhase = 20
)
