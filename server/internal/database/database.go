package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create db directory %q: %w", dir, err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}

	// Pin to a single connection so the per-connection pragmas below
	// (busy_timeout, synchronous) actually apply to every query instead of
	// only whichever pooled connection happened to run them. For a single
	// instance this also serializes writes, removing SQLite lock contention.
	db.SetMaxOpenConns(1)

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		// NORMAL is crash-safe under WAL and skips the fsync on every commit
		// (only checkpoints sync). Without it the default FULL fsyncs each
		// commit, so a burst of small writes — e.g. the flood of per-sequence
		// UpdateSequence calls on map load — pays one fsync per RPC.
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("exec %q: %w", p, err)
		}
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite %q: %w", path, err)
	}

	return db, nil
}

func Checkpoint(db *sql.DB) {
	if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		log.Printf("WAL checkpoint: %v", err)
	}
}
