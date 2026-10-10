package model

type EvaluateConditionFunctionType int32

const (
	EvaluateConditionFunctionTypeUnknown              EvaluateConditionFunctionType = 0
	EvaluateConditionFunctionTypeRecursion            EvaluateConditionFunctionType = 1
	EvaluateConditionFunctionTypeCageTreasureHunt     EvaluateConditionFunctionType = 2
	EvaluateConditionFunctionTypeCageIntervalDropItem EvaluateConditionFunctionType = 3
	EvaluateConditionFunctionTypeQuestClear           EvaluateConditionFunctionType = 4
	EvaluateConditionFunctionTypeGimmickBitCount      EvaluateConditionFunctionType = 5
	EvaluateConditionFunctionTypeWeaponAcquisition    EvaluateConditionFunctionType = 6
	EvaluateConditionFunctionTypeTutorial             EvaluateConditionFunctionType = 7
	EvaluateConditionFunctionTypeMissionClear         EvaluateConditionFunctionType = 8
	EvaluateConditionFunctionTypeQuestMissionClear    EvaluateConditionFunctionType = 9
	EvaluateConditionFunctionTypeOtherGimmickBitCount EvaluateConditionFunctionType = 10
	EvaluateConditionFunctionTypeQuestSceneChoice     EvaluateConditionFunctionType = 11
	EvaluateConditionFunctionTypeQuestNotClear        EvaluateConditionFunctionType = 12
)

type EvaluateConditionEvaluateType int32

const (
	EvaluateConditionEvaluateTypeUnknown   EvaluateConditionEvaluateType = 0
	EvaluateConditionEvaluateTypeAnd       EvaluateConditionEvaluateType = 1
	EvaluateConditionEvaluateTypeOr        EvaluateConditionEvaluateType = 2
	EvaluateConditionEvaluateTypeGE        EvaluateConditionEvaluateType = 3
	EvaluateConditionEvaluateTypeIdContain EvaluateConditionEvaluateType = 4
	EvaluateConditionEvaluateTypeEQ        EvaluateConditionEvaluateType = 5
	EvaluateConditionEvaluateTypeLE        EvaluateConditionEvaluateType = 6
)
