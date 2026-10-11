package userdata

import (
	"lunar-tear/server/internal/store"
)

// masterDataVariants lists recognized master-data variant tokens. The octo
// options web page offers them in its picker; the game server appends the
// chosen token to the master-data version string and octo validates the
// requested file suffix against this set (see IsValidMasterDataVariant).
var masterDataVariants = map[string]bool{
	"Origin":     true,
	"Clear":      true,
	"Rec":        true,
	"Var":        true,
	"Gacha":      true,
	"Bonus":      true,
	"AllWoBonus": true,
}

// IsValidMasterDataVariant reports whether variant is a recognized master-data
// variant token.
func IsValidMasterDataVariant(variant string) bool {
	return masterDataVariants[variant]
}

// Options holds per-user game-logic knobs resolved from the persisted
// user_options row — the single source of truth, edited on the octo
// /information web page.
type Options struct {
	// MasterDataVariant selects which master-data file variant to serve.
	// Empty string means the base variant.
	MasterDataVariant string

	// ForceHiddenStoryMissions controls whether the hidden-story unlock
	// missions (category 7) are presented to the client as already
	// completed. When false, the player must clear them legitimately.
	ForceHiddenStoryMissions bool

	// ShowNews controls in-game news (dokan) popups. When true (the default
	// for every player, including those with no saved row), active news are
	// shown normally. When false, active dokans are reported as already-seen
	// in the data projection only — nothing is written to the database; a
	// real DB record happens solely via RegisterDokanConfirmed on a genuine view.
	ShowNews bool

	// ForceQuestMissions controls whether the additional challenge missions
	// inside quests (the three per-quest missions plus their "Clear all
	// missions" meta-mission) count as achieved for the quest being cleared
	// right now. When false (the default), the player must clear them
	// legitimately. Applied in questflow's clearQuestMissions where the clear is
	// recorded, so the missions become real cleared rows in the database and the
	// normal finish logic pays their rewards (including the BigWin bonus). Not a
	// projection trick; never rewrites already-finished quests.
	ForceQuestMissions bool
}

// ResolveOptions maps a stored user_options row onto game-logic knobs. The
// ForceHiddenStoryMissions bit mirrors the "Auto-complete hidden story unlock
// missions" checkbox: 1 = present the missions as already completed; 0 (the
// default, also when no row exists) = the player must clear them.
func ResolveOptions(opts store.UserOptionsState) Options {
	return Options{
		MasterDataVariant:        opts.MasterDataVariant,
		ForceHiddenStoryMissions: opts.ForceHiddenStoryMissions,
		ShowNews:                 opts.ShowNews,
		ForceQuestMissions:       opts.ForceQuestMissions,
	}
}
