package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"lunar-tear/server/migrations"
)

var sourceMode bool

func isSourceCheckout() bool {
	if _, err := os.Stat("go.mod"); err != nil {
		return false
	}
	if _, err := os.Stat("proto"); err != nil {
		return false
	}
	return true
}

func runMigrateEmbedded() {
	fmt.Println("  Running migrations...")
	if err := os.MkdirAll("db", 0755); err != nil {
		fmt.Fprintf(os.Stderr, "  Failed to create db/: %v\n", err)
		os.Exit(1)
	}
	db, err := sql.Open("sqlite", "db/game.db?_pragma=busy_timeout(5000)")
	if err != nil {
		fmt.Fprintf(os.Stderr, "  open db: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := migrations.Up(context.Background(), db); err != nil {
		fmt.Fprintf(os.Stderr, "  migration failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  Migrations completed successfully")
}
