package service

import (
	"testing"

	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
)

// TestToProtoGachaForwardsUnlockConditions guards the chapter-gacha gating fix:
// the unlock conditions computed at startup (EnrichGachaUnlockConditions) must
// reach the client, otherwise every chapter gacha renders as already unlocked.
func TestToProtoGachaForwardsUnlockConditions(t *testing.T) {
	entry := store.GachaCatalogEntry{
		GachaId:        200005,
		GachaLabelType: model.GachaLabelChapter,
		GachaModeType:  model.GachaModeBox,
		UnlockConditions: []store.GachaUnlockConditionEntry{{
			GachaUnlockConditionType: model.GachaUnlockMainQuestClear,
			ConditionValue:           4242,
		}},
	}
	g := toProtoGacha(entry, &store.GachaBannerState{})
	if len(g.GachaUnlockCondition) != 1 {
		t.Fatalf("expected 1 unlock condition, got %d", len(g.GachaUnlockCondition))
	}
	uc := g.GachaUnlockCondition[0]
	if uc.GachaUnlockConditionType != model.GachaUnlockMainQuestClear || uc.ConditionValue != 4242 {
		t.Fatalf("unlock condition not forwarded: type=%d value=%d", uc.GachaUnlockConditionType, uc.ConditionValue)
	}
}

// TestToProtoGachaDefaultsToNoneWhenNoConditions checks that banners without a
// computed condition still emit the explicit None entry the client expects.
func TestToProtoGachaDefaultsToNoneWhenNoConditions(t *testing.T) {
	g := toProtoGacha(store.GachaCatalogEntry{GachaId: 100}, &store.GachaBannerState{})
	if len(g.GachaUnlockCondition) != 1 ||
		g.GachaUnlockCondition[0].GachaUnlockConditionType != model.GachaUnlockNone {
		t.Fatalf("expected default None unlock condition, got %+v", g.GachaUnlockCondition)
	}
}
