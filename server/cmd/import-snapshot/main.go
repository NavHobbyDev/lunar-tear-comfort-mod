package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"

	"lunar-tear/server/internal/database"
	"lunar-tear/server/internal/store"
	"lunar-tear/server/internal/store/sqlite"
)

func main() {
	dbPath := flag.String("db", "db/game.db", "SQLite database path")
	snapshotPath := flag.String("snapshot", "", "Path to JSON snapshot file (required)")
	userUuid := flag.String("uuid", "", "UUID to assign to the imported user (must match the client's UUID)")
	flag.Parse()

	if *snapshotPath == "" {
		log.Fatal("--snapshot flag is required")
	}
	if *userUuid == "" {
		log.Fatal("--uuid flag is required")
	}

	data, err := os.ReadFile(*snapshotPath)
	if err != nil {
		log.Fatalf("read snapshot: %v", err)
	}
	log.Printf("read %d bytes from %s", len(data), *snapshotPath)

	var u store.UserState
	if err := json.Unmarshal(data, &u); err != nil {
		log.Fatalf("unmarshal snapshot: %v", err)
	}
	u.EnsureMaps()
	u.Uuid = *userUuid

	log.Printf("parsed user %d (uuid=%s, costumes=%d, weapons=%d, characters=%d, quests=%d)",
		u.UserId, u.Uuid, len(u.Costumes), len(u.Weapons), len(u.Characters), len(u.Quests))

	db, err := database.Open(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	userStore := sqlite.New(db, nil)

	if err := userStore.ImportUser(&u); err != nil {
		log.Fatalf("import user: %v", err)
	}

	log.Printf("imported user %d successfully", u.UserId)
}
