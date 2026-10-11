package sqlite

import (
	"database/sql"
	"sync"
	"time"

	"lunar-tear/server/internal/store"
)

type cachedSession struct {
	userId   int64
	expireAt time.Time
}

type SQLiteStore struct {
	db    *sql.DB
	clock store.Clock

	// sessionCache avoids a sessions-table read on every RPC. CurrentUserId is
	// resolved on every call (including the UpdateSequence map-load burst), so
	// caching the session->user mapping removes one DB read + time.Parse per
	// call. Entries are honoured only until their stored expiry, so a refreshed
	// or removed session self-corrects on the next miss.
	sessionMu    sync.RWMutex
	sessionCache map[string]cachedSession

	// gimmickSeqPending buffers EnsureGimmickSequence registrations (one per
	// gimmick sequence, fired in a burst on map load) instead of writing each to
	// the DB. The buffer is flushed in a single batched transaction by the next
	// LoadUser -- the only reader of these rows -- so the burst pays no per-call
	// DB cost while reads stay consistent.
	gimmickSeqMu      sync.Mutex
	gimmickSeqPending map[int64]map[[2]int32]struct{} // userId -> set of {scheduleId, sequenceId}
}

var (
	_ store.UserRepository    = (*SQLiteStore)(nil)
	_ store.SessionRepository = (*SQLiteStore)(nil)
)

func New(db *sql.DB, clock store.Clock) *SQLiteStore {
	if clock == nil {
		clock = time.Now
	}
	return &SQLiteStore{
		db:                db,
		clock:             clock,
		sessionCache:      make(map[string]cachedSession),
		gimmickSeqPending: make(map[int64]map[[2]int32]struct{}),
	}
}
