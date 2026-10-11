package sqlite

import (
	"database/sql"
	"fmt"
	"log"
	"runtime/debug"

	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
)

func (s *SQLiteStore) CreateUser(uuid string, platform model.ClientPlatform) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	nowMillis := s.clock().UnixMilli()

	res, err := tx.Exec(`INSERT INTO users (uuid, player_id, os_type, platform_type, user_restriction_type,
		register_datetime, game_start_datetime, latest_version, birth_year, birth_month,
		backup_token, charge_money_this_month) VALUES (?, 0, ?, ?, 0, ?, ?, 0, 2000, 1, 'mock-backup-token', 0)`,
		uuid, platform.OsType, platform.PlatformType, nowMillis, nowMillis)
	if err != nil {
		return 0, fmt.Errorf("insert user: %w", err)
	}
	userId, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}

	// player_id = user_id
	if _, err := tx.Exec(`UPDATE users SET player_id = ? WHERE user_id = ?`, userId, userId); err != nil {
		return 0, fmt.Errorf("update player_id: %w", err)
	}

	user := store.SeedUserState(userId, uuid, nowMillis, platform)
	if err := writeUserState(tx, userId, user); err != nil {
		return 0, fmt.Errorf("write seed state: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return userId, nil
}

func (s *SQLiteStore) GetUserByUUID(uuid string) (int64, error) {
	var userId int64
	err := s.db.QueryRow(`SELECT user_id FROM users WHERE uuid = ?`, uuid).Scan(&userId)
	if err == sql.ErrNoRows {
		return 0, store.ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("query user: %w", err)
	}
	return userId, nil
}

func (s *SQLiteStore) DefaultUserId() (int64, error) {
	var userId int64
	err := s.db.QueryRow(`SELECT min(user_id) FROM users`).Scan(&userId)
	if err != nil || userId == 0 {
		return 0, store.ErrNotFound
	}
	return userId, nil
}

// ImportUser replaces all data for u.UserId in the database with the
// contents of u.  Any pre-existing rows for that user are deleted first.
func (s *SQLiteStore) ImportUser(u *store.UserState) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	uid := u.UserId

	// Child tables in reverse-dependency order (matches schema's goose Down).
	childTables := []string{
		"user_event_quest_labyrinth_stages",
		"user_event_quest_labyrinth_seasons",
		"user_event_quest_tower_accumulation_rewards",
		"user_cage_ornament_accesses",
		"user_cage_ornament_rewards",
		"user_shop_replaceable_lineup",
		"user_shop_items",
		"user_gacha_banner_box_drew_counts",
		"user_gacha_banners",
		"user_gacha_converted_medals",
		"user_gifts",
		"user_dokan_confirmed",
		"user_drawn_omikuji",
		"user_contents_stories",
		"user_viewed_movies",
		"user_navi_cutin_played",
		"user_auto_sale_settings",
		"user_quest_auto_orbit",
		"user_explore_scores",
		"user_tutorials",
		"user_premium_items",
		"user_important_items",
		"user_materials",
		"user_consumable_items",
		"user_gimmick_unlocks",
		"user_gimmick_sequences",
		"user_gimmick_ornament_progress",
		"user_gimmick_progress",
		"user_big_hunt_weekly_statuses",
		"user_big_hunt_weekly_max_scores",
		"user_big_hunt_schedule_max_scores",
		"user_big_hunt_statuses",
		"user_big_hunt_max_scores",
		"user_quest_limit_content_status",
		"user_side_story_quests",
		"user_main_quest_season_routes",
		"user_mission_pass_points",
		"user_missions",
		"user_quest_missions",
		"user_quests",
		"user_deck_type_notes",
		"user_deck_parts",
		"user_deck_sub_weapons",
		"user_decks",
		"user_deck_characters",
		"user_parts_status_subs",
		"user_parts_preset_tags",
		"user_parts_presets",
		"user_parts_group_notes",
		"user_parts",
		"user_thoughts",
		"user_companions",
		"user_weapon_notes",
		"user_weapon_stories",
		"user_weapon_awakens",
		"user_weapon_abilities",
		"user_weapon_skills",
		"user_weapons",
		"user_costume_awaken_status_ups",
		"user_costume_active_skills",
		"user_costumes",
		"user_character_rebirths",
		"user_character_viewer_fields",
		"user_character_board_status_ups",
		"user_character_board_abilities",
		"user_character_boards",
		"user_characters",
		"user_gacha",
		"user_shop_replaceable",
		"user_explore",
		"user_guerrilla_free_open",
		"user_portal_cage",
		"user_notification",
		"user_battle",
		"user_big_hunt_state",
		"user_side_story_active",
		"user_extra_quest",
		"user_event_quest",
		"user_main_quest",
		"user_login_bonus",
		"user_login",
		"user_profile",
		"user_gem",
		"user_status",
		"user_setting",
		"user_options",
		"sessions",
	}

	for _, t := range childTables {
		if _, err := tx.Exec(fmt.Sprintf(`DELETE FROM %s WHERE user_id = ?`, t), uid); err != nil {
			return fmt.Errorf("delete from %s: %w", t, err)
		}
	}
	if _, err := tx.Exec(`DELETE FROM users WHERE user_id = ?`, uid); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	if _, err := tx.Exec(`INSERT INTO users (user_id, uuid, player_id, os_type, platform_type,
		user_restriction_type, register_datetime, game_start_datetime, latest_version,
		birth_year, birth_month, backup_token, charge_money_this_month)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		uid, u.Uuid, u.PlayerId, u.OsType, u.PlatformType, u.UserRestrictionType,
		u.RegisterDatetime, u.GameStartDatetime, u.LatestVersion,
		u.BirthYear, u.BirthMonth, u.BackupToken, u.ChargeMoneyThisMonth); err != nil {
		return fmt.Errorf("insert user: %w", err)
	}

	if err := writeUserState(tx, uid, u); err != nil {
		return fmt.Errorf("write user state: %w", err)
	}

	// Carry the web-page options over from the imported state (they were wiped
	// together with the old rows above).
	if _, err := tx.Exec(`INSERT INTO user_options (user_id, masterdata_variant, hidden_story_missions, show_news, force_quest_missions, updated_at)
		VALUES (?,?,?,?,?,0)`, uid, u.Options.MasterDataVariant, boolToInt(u.Options.ForceHiddenStoryMissions), boolToInt(u.Options.ShowNews), boolToInt(u.Options.ForceQuestMissions)); err != nil {
		return fmt.Errorf("insert user_options: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

func (s *SQLiteStore) SetFacebookId(userId int64, facebookId int64) error {
	_, err := s.db.Exec(`UPDATE users SET facebook_id = ? WHERE user_id = ?`, facebookId, userId)
	if err != nil {
		return fmt.Errorf("set facebook_id: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetUserByFacebookId(facebookId int64) (int64, error) {
	var userId int64
	err := s.db.QueryRow(`SELECT user_id FROM users WHERE facebook_id = ?`, facebookId).Scan(&userId)
	if err == sql.ErrNoRows {
		return 0, store.ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("query user by facebook_id: %w", err)
	}
	return userId, nil
}

func (s *SQLiteStore) GetFacebookId(userId int64) (int64, error) {
	var fbId sql.NullInt64
	err := s.db.QueryRow(`SELECT facebook_id FROM users WHERE user_id = ?`, userId).Scan(&fbId)
	if err != nil {
		return 0, store.ErrNotFound
	}
	return fbId.Int64, nil
}

func (s *SQLiteStore) ClearFacebookId(userId int64) error {
	_, err := s.db.Exec(`UPDATE users SET facebook_id = NULL WHERE user_id = ?`, userId)
	if err != nil {
		return fmt.Errorf("clear facebook_id: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UpdateUUID(userId int64, newUuid string) error {
	_, err := s.db.Exec(`UPDATE users SET uuid = ? WHERE user_id = ?`, newUuid, userId)
	if err != nil {
		return fmt.Errorf("update uuid: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UpdateUser(userId int64, mutate func(*store.UserState)) (out store.UserState, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[PANIC] UpdateUser user=%d: %v\n%s", userId, r, debug.Stack())
			out = store.UserState{}
			err = fmt.Errorf("panic in UpdateUser: %v", r)
		}
	}()

	before, err := s.LoadUser(userId)
	if err != nil {
		return store.UserState{}, err
	}

	after := store.CloneUserState(before)
	mutate(&after)

	tx, err := s.db.Begin()
	if err != nil {
		return store.UserState{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if err := diffAndSave(tx, userId, &before, &after); err != nil {
		return store.UserState{}, fmt.Errorf("diff and save: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return store.UserState{}, fmt.Errorf("commit: %w", err)
	}

	return after, nil
}

// EnsureGimmickSequence inserts a fresh (not-cleared) sequence row only when one
// does not already exist. INSERT OR IGNORE leaves an existing cleared row intact,
// matching the old handler that read the row, set its key, and wrote it back. This
// is a single statement — no full-user load, clone, or diff.
//
// The client fires this once per gimmick sequence in a burst on map load, so the
// registration is buffered in memory (deduped) rather than written per call. The
// buffer is flushed in one batched transaction by flushGimmickSequences, which
// LoadUser (the only reader of these rows) calls before reading -- keeping reads
// consistent while the burst pays no per-call DB cost.
func (s *SQLiteStore) EnsureGimmickSequence(userId int64, scheduleId, sequenceId int32) error {
	s.gimmickSeqMu.Lock()
	set := s.gimmickSeqPending[userId]
	if set == nil {
		set = make(map[[2]int32]struct{})
		s.gimmickSeqPending[userId] = set
	}
	set[[2]int32{scheduleId, sequenceId}] = struct{}{}
	s.gimmickSeqMu.Unlock()
	return nil
}

// flushGimmickSequences persists a user's buffered gimmick-sequence
// registrations in one batched transaction. Called by LoadUser before it reads,
// so the read sees the writes. On failure the rows are re-buffered for retry on
// the next load (and the client re-syncs them every map load regardless).
func (s *SQLiteStore) flushGimmickSequences(userId int64) {
	s.gimmickSeqMu.Lock()
	set := s.gimmickSeqPending[userId]
	delete(s.gimmickSeqPending, userId)
	s.gimmickSeqMu.Unlock()
	if len(set) == 0 {
		return
	}

	if err := s.writeGimmickSequences(userId, set); err != nil {
		log.Printf("[gimmick] flush failed for user %d (%d sequences), will retry: %v", userId, len(set), err)
		s.gimmickSeqMu.Lock()
		existing := s.gimmickSeqPending[userId]
		if existing == nil {
			s.gimmickSeqPending[userId] = set
		} else {
			for k := range set {
				existing[k] = struct{}{}
			}
		}
		s.gimmickSeqMu.Unlock()
	}
}

func (s *SQLiteStore) writeGimmickSequences(userId int64, set map[[2]int32]struct{}) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("flush gimmick sequences: begin: %w", err)
	}
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO user_gimmick_sequences
		(user_id, gimmick_sequence_schedule_id, gimmick_sequence_id, is_gimmick_sequence_cleared, clear_datetime, latest_version)
		VALUES (?, ?, ?, 0, 0, 0)`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("flush gimmick sequences: prepare: %w", err)
	}
	defer stmt.Close()
	for k := range set {
		if _, err := stmt.Exec(userId, k[0], k[1]); err != nil {
			tx.Rollback()
			return fmt.Errorf("flush gimmick sequences: exec: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("flush gimmick sequences: commit: %w", err)
	}
	return nil
}
