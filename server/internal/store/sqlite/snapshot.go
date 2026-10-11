package sqlite

import (
	"database/sql"

	"lunar-tear/server/internal/store"
)

func (s *SQLiteStore) UpsertSnapshot(snap store.PlayerSnapshot) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO player_snapshots
		(player_id, user_name, level, max_deck_power, favorite_costume_id, pvp_point,
		 last_login_datetime, defense_deck_json, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		snap.PlayerId, snap.UserName, snap.Level, snap.MaxDeckPower, snap.FavoriteCostumeId,
		snap.PvpPoint, snap.LastLoginDatetime, snap.DefenseDeckJson, snap.UpdatedAt)
	return err
}

func (s *SQLiteStore) GetSnapshot(playerId int64) (store.PlayerSnapshot, error) {
	var snap store.PlayerSnapshot
	err := s.db.QueryRow(`SELECT player_id, user_name, level, max_deck_power, favorite_costume_id,
		pvp_point, last_login_datetime, defense_deck_json, updated_at
		FROM player_snapshots WHERE player_id=?`, playerId).Scan(
		&snap.PlayerId, &snap.UserName, &snap.Level, &snap.MaxDeckPower, &snap.FavoriteCostumeId,
		&snap.PvpPoint, &snap.LastLoginDatetime, &snap.DefenseDeckJson, &snap.UpdatedAt)
	if err == sql.ErrNoRows {
		return snap, store.ErrNotFound
	}
	return snap, err
}

func (s *SQLiteStore) DeleteSnapshot(playerId int64) error {
	_, err := s.db.Exec(`DELETE FROM player_snapshots WHERE player_id=?`, playerId)
	return err
}

func scanSnapshots(rows *sql.Rows) ([]store.PlayerSnapshot, error) {
	defer rows.Close()
	var out []store.PlayerSnapshot
	for rows.Next() {
		var s store.PlayerSnapshot
		if err := rows.Scan(&s.PlayerId, &s.UserName, &s.Level, &s.MaxDeckPower, &s.FavoriteCostumeId,
			&s.PvpPoint, &s.LastLoginDatetime, &s.DefenseDeckJson, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) ListSnapshotsNear(excludePlayerId int64, nearPoint int32, limit int) ([]store.PlayerSnapshot, error) {
	rows, err := s.db.Query(`SELECT player_id, user_name, level, max_deck_power, favorite_costume_id,
		pvp_point, last_login_datetime, defense_deck_json, updated_at
		FROM player_snapshots WHERE player_id<>?
		ORDER BY ABS(pvp_point - ?) ASC LIMIT ?`, excludePlayerId, nearPoint, limit)
	if err != nil {
		return nil, err
	}
	return scanSnapshots(rows)
}

func (s *SQLiteStore) ListSnapshotsByPointDesc(offset, limit int) ([]store.PlayerSnapshot, error) {
	rows, err := s.db.Query(`SELECT player_id, user_name, level, max_deck_power, favorite_costume_id,
		pvp_point, last_login_datetime, defense_deck_json, updated_at
		FROM player_snapshots ORDER BY pvp_point DESC, player_id ASC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	return scanSnapshots(rows)
}

func (s *SQLiteStore) CountSnapshots() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM player_snapshots`).Scan(&n)
	return n, err
}

func (s *SQLiteStore) RankOfPlayer(playerId int64) (int, error) {
	var pvpPoint int32
	err := s.db.QueryRow(`SELECT pvp_point FROM player_snapshots WHERE player_id=?`, playerId).Scan(&pvpPoint)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var ahead int
	err = s.db.QueryRow(`SELECT COUNT(*) FROM player_snapshots WHERE pvp_point > ?`, pvpPoint).Scan(&ahead)
	if err != nil {
		return 0, err
	}
	return ahead + 1, nil
}

func (s *SQLiteStore) MaxPointAbovePower(excludePlayerId int64, power int32) (int32, bool, error) {
	var capPoint sql.NullInt64
	err := s.db.QueryRow(`SELECT MAX(pvp_point) FROM player_snapshots
		WHERE max_deck_power > ? AND player_id <> ?`, power, excludePlayerId).Scan(&capPoint)
	if err != nil {
		return 0, false, err
	}
	if !capPoint.Valid {
		return 0, false, nil
	}
	return int32(capPoint.Int64), true, nil
}

func (s *SQLiteStore) AllUserIds() ([]int64, error) {
	rows, err := s.db.Query(`SELECT user_id FROM users ORDER BY user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

var _ store.SnapshotRepository = (*SQLiteStore)(nil)
