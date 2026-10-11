package userdata

import (
	"testing"

	"lunar-tear/server/internal/store"
)

func TestResolveOptions_DefaultRequiresPlaying(t *testing.T) {
	// No row (or an explicit 0) keeps the hidden-story unlock missions playable.
	settings := ResolveOptions(store.UserOptionsState{})
	if settings.ForceHiddenStoryMissions {
		t.Error("default must NOT force-complete: the player should clear the missions")
	}
	if settings.MasterDataVariant != "" {
		t.Errorf("expected base variant, got %q", settings.MasterDataVariant)
	}
	// Quest missions are OFF by default too: the player clears them normally.
	if settings.ForceQuestMissions {
		t.Error("default must NOT force-complete quest missions")
	}
}

func TestResolveOptions_OptOutForceCompletes(t *testing.T) {
	// Clearing the checkbox on the page stores the opt-out (1): missions are
	// presented as completed; the variant token passes through untouched.
	settings := ResolveOptions(store.UserOptionsState{MasterDataVariant: "Clear", ForceHiddenStoryMissions: true})
	if !settings.ForceHiddenStoryMissions {
		t.Error("stored opt-out must force-complete the hidden-story unlock missions")
	}
	if settings.MasterDataVariant != "Clear" {
		t.Errorf("expected variant pass-through, got %q", settings.MasterDataVariant)
	}
}

func TestIsValidMasterDataVariant(t *testing.T) {
	for _, v := range []string{"Origin", "Clear", "Rec", "Var", "Gacha", "Bonus", "AllWoBonus"} {
		if !IsValidMasterDataVariant(v) {
			t.Errorf("expected %q to be accepted", v)
		}
	}
	for _, v := range []string{"", "all", "rAll", "vAll", "Clear "} {
		if IsValidMasterDataVariant(v) {
			t.Errorf("expected %q to be rejected", v)
		}
	}
}
