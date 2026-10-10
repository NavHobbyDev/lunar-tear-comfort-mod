package service

import (
	"log"

	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/runtime"
	"lunar-tear/server/internal/store"
)

// BackfillCostumeLevelBonuses rebuilds every player's permanent character bonuses from
// all of their owned costumes. Rebuilding, instead of only adding levels that have not
// been confirmed before, repairs saves made while this feature was incomplete.
func BackfillCostumeLevelBonuses(users store.UserRepository, snaps store.SnapshotRepository, holder *runtime.Holder) {
	cat := holder.Get()
	if cat == nil || cat.Costume == nil {
		return
	}
	catalog := cat.Costume

	ids, err := snaps.AllUserIds()
	if err != nil {
		log.Printf("[costumebonus] backfill: list users failed: %v", err)
		return
	}
	rebuilt := 0
	for _, id := range ids {
		users.UpdateUser(id, func(u *store.UserState) {
			rebuildCharacterCostumeLevelBonuses(catalog, u, gametime.NowMillis())
			rebuilt++
		})
	}
	log.Printf("[costumebonus] rebuilt character costume bonuses for %d account(s)", rebuilt)
}

// rebuildCharacterCostumeLevelBonuses calculates the total for every character
// from every owned costume. A bonus is shared by all costumes of that character,
// so the old aggregate must never be used as an input to this calculation.
func rebuildCharacterCostumeLevelBonuses(catalog *masterdata.CostumeCatalog, user *store.UserState, nowMillis int64) {
	user.EnsureMaps()
	user.CharacterCostumeLevelBonuses = make(map[store.CharacterCostumeLevelBonusKey]store.CharacterCostumeLevelBonusState)

	for _, costume := range user.Costumes {
		cm, ok := catalog.Costumes[costume.CostumeId]
		if !ok || cm.CostumeLevelBonusId == 0 {
			continue
		}
		applyCostumeLevelBonus(catalog, user, cm.CharacterId, costume.CostumeId, 0, costume.Level, nowMillis)

		existing := user.CostumeLevelBonusReleaseStatuses[costume.CostumeId]
		if existing.LastReleasedBonusLevel == costume.Level && existing.ConfirmedBonusLevel == costume.Level {
			// No level change for this costume – skip touching the release
			// record to avoid unnecessary LatestVersion bumps that bloat diffs.
			continue
		}
		existing.CostumeId = costume.CostumeId
		existing.LastReleasedBonusLevel = costume.Level
		existing.ConfirmedBonusLevel = costume.Level
		existing.LatestVersion = nowMillis
		user.CostumeLevelBonusReleaseStatuses[costume.CostumeId] = existing
	}
}
