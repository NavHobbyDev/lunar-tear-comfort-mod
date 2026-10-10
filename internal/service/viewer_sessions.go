package service

import (
	"sync"
	"time"
)

// viewerSessionWindow bounds how long a Character Viewer "open" mark keeps a
// StartWave authorised after the player entered the viewer. The viewer enters a
// location by firing StartWave with no active quest; once the player leaves the
// viewer and does anything else (starts a quest, walks the world map) a fresh
// mark is no longer present, so the window simply guarantees a stale mark can
// never keep authorising quest-less waves indefinitely.
const viewerSessionWindow = 30 * time.Minute

// ViewerSessions tracks, per user, the last time the Character Viewer top
// screen was opened. It is shared between the CharacterViewer service (which
// marks the session) and the Battle service (which consults it in StartWave) so
// a legitimate 3D-preview wave can be told apart from a quest-less orphan wave.
//
// This is ephemeral UX state, not game state: it is intentionally kept in
// memory (not the database) and reset on restart. Re-opening the viewer
// re-marks it, so losing it on restart is harmless.
type ViewerSessions struct {
	mu       sync.Mutex
	lastMark map[int64]time.Time
}

func NewViewerSessions() *ViewerSessions {
	return &ViewerSessions{lastMark: make(map[int64]time.Time)}
}

// Mark records that the user just opened the Character Viewer. A nil receiver
// is tolerated so callers never need to guard construction.
func (v *ViewerSessions) Mark(userId int64) {
	if v == nil {
		return
	}
	v.mu.Lock()
	v.lastMark[userId] = time.Now()
	v.mu.Unlock()
}

// InViewer reports whether the user opened the Character Viewer within the
// session window. Expired marks are pruned on read.
func (v *ViewerSessions) InViewer(userId int64) bool {
	if v == nil {
		return false
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	marked, ok := v.lastMark[userId]
	if !ok {
		return false
	}
	if time.Since(marked) > viewerSessionWindow {
		delete(v.lastMark, userId)
		return false
	}
	return true
}
