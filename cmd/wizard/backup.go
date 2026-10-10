package main

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"charm.land/huh/v2/spinner"

	_ "modernc.org/sqlite"
)

const (
	gameDBPath    = "db/game.db"
	backupDir     = "db/backups"
	backupSuffix  = ".bak"
	backupRetainN = 10
)

func backupGameDB() {
	if _, err := os.Stat(gameDBPath); errors.Is(err, os.ErrNotExist) {
		return
	}

	if !sourceMode {
		fmt.Println("  Backing up db/game.db...")
		doBackupGameDB()
		return
	}

	_ = spinner.New().Title("  Backing up db/game.db...").Action(doBackupGameDB).Run()
}

func doBackupGameDB() {
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "  failed to create %s: %v\n", backupDir, err)
		return
	}

	ts := time.Now().UTC().Format("20060102T150405Z")
	dest := filepath.Join(backupDir, fmt.Sprintf("game.db.%s%s", ts, backupSuffix))

	db, err := sql.Open("sqlite", gameDBPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		fmt.Fprintf(os.Stderr, "  failed to open %s: %v\n", gameDBPath, err)
		return
	}
	defer db.Close()

	escaped := strings.ReplaceAll(dest, "'", "''")
	if _, err := db.Exec(fmt.Sprintf("VACUUM INTO '%s'", escaped)); err != nil {
		fmt.Fprintf(os.Stderr, "  VACUUM INTO failed: %v\n", err)
		_ = os.Remove(dest)
		return
	}

	pruneOldBackups()
}

func pruneOldBackups() {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return
	}
	var backups []os.DirEntry
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "game.db.") && strings.HasSuffix(e.Name(), backupSuffix) {
			backups = append(backups, e)
		}
	}
	if len(backups) <= backupRetainN {
		return
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].Name() < backups[j].Name() })
	for _, old := range backups[:len(backups)-backupRetainN] {
		_ = os.Remove(filepath.Join(backupDir, old.Name()))
	}
}
