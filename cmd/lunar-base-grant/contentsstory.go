// Contents-story actions for the lunar-base-grant shim:
//   - mark_contents_stories_played
//
// Mirrors lunar-tear's ContentsStoryService.RegisterPlayed: writes
// `user.ContentsStories[id] = nowMillis` for each id, marking the
// cutscene as already viewed. Self-skips already-played ids.
//
// Use case: granting multiple Dark Memory weapons at once queues a
// "first acquisition" cutscene per weapon. The game only plays one
// per launch, and the queued ones soft-lock progression until they
// drain. Mass-marking the cutscene set as played clears the queue
// so the player isn't forced to relaunch + skip 21 times.

package main

import (
	"errors"
	"fmt"
	"time"

	"lunar-tear/server/internal/store"
)

func runMarkContentsStoriesPlayed(req *request) (int, error) {
	if len(req.ContentsStoryIDs) == 0 {
		return 0, errors.New("contents_story_ids list is empty")
	}

	db, st, err := openDB(req.DBPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	now := time.Now().UnixMilli()
	applied := 0
	_, err = st.UpdateUser(req.UserID, func(u *store.UserState) {
		for _, id := range req.ContentsStoryIDs {
			if _, exists := u.ContentsStories[id]; exists {
				continue
			}
			u.ContentsStories[id] = now
			applied++
		}
	})
	if err != nil {
		return 0, fmt.Errorf("mark contents stories: %w", err)
	}
	return applied, nil
}
