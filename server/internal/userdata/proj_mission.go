package userdata

import (
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/store"
)

var missionCat *masterdata.MissionCatalog

// SetMissionCatalog injects the already-loaded mission catalog so projectors
// can use it without triggering a second full load from the master data file.
func SetMissionCatalog(cat *masterdata.MissionCatalog) {
	missionCat = cat
}

func init() {
	register("IUserMissionCompletionProgress", func(user store.UserState) string {
		// Virtual table: aggregated clear counts per mission group.
		// Field names are not yet verified against the client schema, so we
		// return empty to avoid deserialization hangs.
		return "[]"
	})

	register("IUserMissionPassPoint", func(user store.UserState) string {
		// Virtual table: earned points per mission pass.
		// Field names are not yet verified against the client schema, so we
		// return empty to avoid deserialization hangs.
		return "[]"
	})
}
