package service

import (
	"log"

	"lunar-tear/server/internal/store"
)

// BackfillSnapshots writes a snapshot for every existing account so real players are
// visible in the friend/arena directory immediately, without each having to log in first.
// Logins, deck edits, and battles keep snapshots fresh thereafter. RefreshSnapshot skips
// accounts with no display name, so onboarding-incomplete test accounts are not included.
func BackfillSnapshots(users store.UserRepository, snaps store.SnapshotRepository) {
	ids, err := snaps.AllUserIds()
	if err != nil {
		log.Printf("[snapshot] backfill: list users failed: %v", err)
		return
	}
	written := 0
	for _, id := range ids {
		user, err := users.LoadUser(id)
		if err != nil {
			continue
		}
		if user.Profile.Name == "" {
			continue
		}
		if err := RefreshSnapshot(snaps, &user); err != nil {
			log.Printf("[snapshot] backfill: user %d failed: %v", id, err)
			continue
		}
		written++
	}
	log.Printf("[snapshot] backfill complete: %d player snapshots from %d accounts", written, len(ids))
}
