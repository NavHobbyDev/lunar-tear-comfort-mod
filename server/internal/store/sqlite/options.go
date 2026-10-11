package sqlite

import (
	"database/sql"
	"fmt"

	"lunar-tear/server/internal/store"
)

// GetUserOptions reads the user_options row. A missing row means the user never
// touched the octo options page, so defaults are returned: base variant, no
// auto-complete, news shown, no forced quest missions.
func (s *SQLiteStore) GetUserOptions(userId int64) (store.UserOptionsState, error) {
	var opts store.UserOptionsState
	var hidden, showNews, forceQuestMissions int64
	err := s.db.QueryRow(`SELECT masterdata_variant, hidden_story_missions, show_news, force_quest_missions FROM user_options WHERE user_id = ?`, userId).
		Scan(&opts.MasterDataVariant, &hidden, &showNews, &forceQuestMissions)
	if err == sql.ErrNoRows {
		// No row: base variant, no auto-complete, show news (true), no forced
		// quest missions.
		return store.UserOptionsState{ShowNews: true}, nil
	}
	if err != nil {
		return store.UserOptionsState{}, fmt.Errorf("load user_options: %w", err)
	}
	opts.ForceHiddenStoryMissions = hidden != 0
	opts.ShowNews = showNews != 0
	opts.ForceQuestMissions = forceQuestMissions != 0
	return opts, nil
}

// SaveUserOptions upserts the user_options row. The FK on user_id rejects
// writes for unknown users, which doubles as existence validation.
func (s *SQLiteStore) SaveUserOptions(userId int64, opts store.UserOptionsState) error {
	_, err := s.db.Exec(`INSERT INTO user_options (user_id, masterdata_variant, hidden_story_missions, show_news, force_quest_missions, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			masterdata_variant = excluded.masterdata_variant,
			hidden_story_missions = excluded.hidden_story_missions,
			show_news = excluded.show_news,
			force_quest_missions = excluded.force_quest_missions,
			updated_at = excluded.updated_at`,
		userId, opts.MasterDataVariant, boolToInt(opts.ForceHiddenStoryMissions), boolToInt(opts.ShowNews), boolToInt(opts.ForceQuestMissions), s.clock().UnixMilli())
	if err != nil {
		return fmt.Errorf("save user_options: %w", err)
	}
	return nil
}
