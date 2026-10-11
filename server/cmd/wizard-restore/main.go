package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

const (
	gameDBPath   = "db/game.db"
	backupDir    = "db/backups"
	backupSuffix = ".bak"
)

const banner = `
  _                        _____
 | |  _  _ _ _  __ _ _ _  |_   _|___ __ _ _ _
 | |_| || | ' \/ _` + "`" + ` | '_|   | |/ -_)/ _` + "`" + ` | '_|
 |____\_,_|_||_\__,_|_|     |_|\___|\__,_|_|

 ╭──────────────────────────────╮
 │   RESTORE                    │
 ╰──────────────────────────────╯
`

func main() {
	lipgloss.EnableLegacyWindowsANSI(os.Stdout)
	lipgloss.EnableLegacyWindowsANSI(os.Stderr)
	fmt.Print(banner)

	chosen, ok := pickBackup()
	if !ok {
		return
	}
	if !confirmOverwrite(chosen) {
		fmt.Println("  cancelled — nothing changed")
		return
	}
	if err := doRestore(chosen); err != nil {
		fmt.Fprintf(os.Stderr, "  restore failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  restored %s from %s\n", gameDBPath, chosen)
}

func pickBackup() (string, bool) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "  no backups found in", backupDir)
		return "", false
	}
	var backups []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "game.db.") && strings.HasSuffix(e.Name(), backupSuffix) {
			backups = append(backups, e.Name())
		}
	}
	if len(backups) == 0 {
		fmt.Fprintln(os.Stderr, "  no backups found in", backupDir)
		return "", false
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i] > backups[j] })

	options := make([]huh.Option[string], 0, len(backups)+1)
	for _, name := range backups {
		options = append(options, huh.NewOption(name, name))
	}
	options = append(options, huh.NewOption("Cancel", ""))

	var chosen string
	if err := huh.NewSelect[string]().
		Title("Pick a backup to restore").
		Description("db/game.db will be replaced by the chosen file.").
		Options(options...).
		Value(&chosen).
		Run(); err != nil || chosen == "" {
		return "", false
	}
	return chosen, true
}

func confirmOverwrite(chosen string) bool {
	confirm := false
	if err := huh.NewConfirm().
		Title("Overwrite db/game.db?").
		Description(fmt.Sprintf(
			"This will REPLACE db/game.db with %s.\n"+
				"Any progress since that backup will be lost.\n"+
				"(A fresh backup will be taken on the next ./wizard launch.)",
			chosen)).
		Affirmative("Yes, restore").
		Negative("Cancel").
		Value(&confirm).
		Run(); err != nil {
		return false
	}
	return confirm
}

func doRestore(chosen string) error {
	src := filepath.Join(backupDir, chosen)
	if _, err := os.Stat(src); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s no longer exists", src)
	}
	if err := copyFile(src, gameDBPath); err != nil {
		return err
	}
	_ = os.Remove(gameDBPath + "-wal")
	_ = os.Remove(gameDBPath + "-shm")
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
