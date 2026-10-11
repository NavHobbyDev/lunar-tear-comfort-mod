package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var FS embed.FS

// requiredColumns lists columns the store layer SELECTs/INSERTs unconditionally,
// together with the DDL used to add them when missing. A restored pre-migration
// backup (or a database created by an older revision of the initial migration
// file) can lack these columns while goose_db_version still claims the migration
// as applied; without this repair the first load/save would fail deep inside a
// transactional save (rolling back unrelated tables like the login bonus) with
// an opaque "no such column" error. All entries carry safe defaults, so adding
// them back is safe and keeps old databases working.
var requiredColumns = map[string]map[string]string{
	"user_options": {
		// Databases that predate the migration with a hand-made user_options
		// table (e.g. carrying only hidden_story_missions_enabled) would let
		// CREATE TABLE IF NOT EXISTS silently keep the old shape; LoadUser then
		// fails with "no such column: hidden_story_missions".
		"masterdata_variant":    "TEXT NOT NULL DEFAULT ''",
		"hidden_story_missions": "INTEGER NOT NULL DEFAULT 0",
		// News popups are on by default, so a repaired column keeps every
		// existing and new player on the normal behaviour.
		"show_news":             "INTEGER NOT NULL DEFAULT 1",
		// Quest missions are off by default, so a repaired column keeps the
		// legitimate-clear behaviour for existing players.
		"force_quest_missions":  "INTEGER NOT NULL DEFAULT 0",
		"updated_at":            "INTEGER NOT NULL DEFAULT 0",
	},
	"user_pvp_state": {
		"defense_deck_number":             "INTEGER NOT NULL DEFAULT 0",
		"weekly_reward_version":           "INTEGER NOT NULL DEFAULT 0",
		"pending_weekly_reward_group_id":  "INTEGER NOT NULL DEFAULT 0",
		"pending_weekly_reward_point":     "INTEGER NOT NULL DEFAULT 0",
		"pending_weekly_reward_season_id": "INTEGER NOT NULL DEFAULT 0",
		"last_auto_progress_day":          "INTEGER NOT NULL DEFAULT 0",
	},
	"user_main_quest": {
		"suppress_replay_flow": "INTEGER NOT NULL DEFAULT 0",
	},
	"user_battle": {
		"is_active":                       "INTEGER NOT NULL DEFAULT 0",
		"start_count":                     "INTEGER NOT NULL DEFAULT 0",
		"finish_count":                    "INTEGER NOT NULL DEFAULT 0",
		"last_started_at":                 "INTEGER NOT NULL DEFAULT 0",
		"last_finished_at":                "INTEGER NOT NULL DEFAULT 0",
		"last_user_party_count":           "INTEGER NOT NULL DEFAULT 0",
		"last_npc_party_count":            "INTEGER NOT NULL DEFAULT 0",
		"last_battle_binary_size":         "INTEGER NOT NULL DEFAULT 0",
		"last_elapsed_frame_count":        "INTEGER NOT NULL DEFAULT 0",
		"last_character_death_count":      "INTEGER NOT NULL DEFAULT 0",
		"last_costume_skill_used_count":   "INTEGER NOT NULL DEFAULT 0",
		"last_weapon_skill_used_count":    "INTEGER NOT NULL DEFAULT 0",
		"last_companion_skill_used_count": "INTEGER NOT NULL DEFAULT 0",
		"last_critical_count":             "INTEGER NOT NULL DEFAULT 0",
		"last_combo_count":                "INTEGER NOT NULL DEFAULT 0",
		"last_combo_max_damage":           "INTEGER NOT NULL DEFAULT 0",
		"last_costume_alive_count":        "INTEGER NOT NULL DEFAULT 0",
		"last_costume_party_size":         "INTEGER NOT NULL DEFAULT 3",
		"last_costume_hp_percent_0":       "INTEGER NOT NULL DEFAULT 0",
		"last_costume_hp_percent_1":       "INTEGER NOT NULL DEFAULT 0",
		"last_costume_hp_percent_2":       "INTEGER NOT NULL DEFAULT 0",
		"last_total_recover_point":        "INTEGER NOT NULL DEFAULT 0",
		"last_party_character_id_0":       "INTEGER NOT NULL DEFAULT 0",
		"last_party_character_id_1":       "INTEGER NOT NULL DEFAULT 0",
		"last_party_character_id_2":       "INTEGER NOT NULL DEFAULT 0",
		"last_main_weapon_id_0":           "INTEGER NOT NULL DEFAULT 0",
		"last_main_weapon_id_1":           "INTEGER NOT NULL DEFAULT 0",
		"last_main_weapon_id_2":           "INTEGER NOT NULL DEFAULT 0",
		// Added by 20260919120000, whose Up was neutralized (the columns are
		// folded into the initial schema for fresh databases); databases that
		// predate the inlining and never ran the ALTER get them repaired here.
		"last_started_quest_id":           "INTEGER NOT NULL DEFAULT 0",
		"last_started_quest_at":           "INTEGER NOT NULL DEFAULT 0",
	},
}

func Up(ctx context.Context, db *sql.DB) error {
	goose.SetBaseFS(FS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("sqlite3"); err != nil {
		return err
	}
	if err := goose.UpContext(ctx, db, ".", goose.WithAllowMissing()); err != nil {
		return err
	}
	return repairSchema(db)
}

func repairSchema(db *sql.DB) error {
	tables := make([]string, 0, len(requiredColumns))
	for t := range requiredColumns {
		tables = append(tables, t)
	}
	sort.Strings(tables)
	for _, table := range tables {
		have := make(map[string]bool)
		rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
		if err != nil {
			return fmt.Errorf("verify schema of %s: %w", table, err)
		}
		for rows.Next() {
			var cid int
			var name, ctype string
			var notnull, pk int
			var dflt any
			if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
				rows.Close()
				return fmt.Errorf("verify schema of %s: %w", table, err)
			}
			have[name] = true
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("verify schema of %s: %w", table, err)
		}
		if len(have) == 0 {
			return fmt.Errorf("table %s does not exist: the database was likely restored from a pre-migration backup; delete db/game.db (fresh start) or restore a newer backup, then start again", table)
		}
		missing := make([]string, 0)
		for col := range requiredColumns[table] {
			if !have[col] {
				missing = append(missing, col)
			}
		}
		sort.Strings(missing)
		for _, col := range missing {
			stmt := fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, col, requiredColumns[table][col])
			if _, err := db.Exec(stmt); err != nil {
				// The game server and octo-cdn can both run this repair against
				// the same file on boot. A peer may add the column between our
				// table_info check and the ALTER, and SQLite has no
				// ADD COLUMN IF NOT EXISTS. The DDL here is identical and
				// default-valued, so an "already exists" from a concurrent
				// repair is a success, not a failure.
				if isDuplicateColumn(err) {
					log.Printf("[migrations] column %s.%s already added (concurrent repair), skipping", table, col)
					continue
				}
				return fmt.Errorf("repair schema of %s.%s: %w", table, col, err)
			}
			log.Printf("[migrations] restored missing column %s.%s (pre-migration backup detected)", table, col)
		}
	}
	return nil
}

// isDuplicateColumn reports whether err is SQLite's "duplicate column name"
// error, which the driver surfaces as a plain SQL logic error text.
func isDuplicateColumn(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate column name")
}
