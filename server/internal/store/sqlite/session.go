package sqlite

import (
	"fmt"
	"time"

	"lunar-tear/server/internal/store"
)

func (s *SQLiteStore) CreateSession(uuid string, ttl time.Duration) (store.SessionState, error) {
	var userId int64
	err := s.db.QueryRow(`SELECT user_id FROM users WHERE uuid = ?`, uuid).Scan(&userId)
	if err != nil {
		return store.SessionState{}, store.ErrNotFound
	}

	now := s.clock()
	sessionKey := fmt.Sprintf("session_%d_%d", userId, now.UnixNano())
	expireAt := now.Add(ttl)

	_, err = s.db.Exec(
		`INSERT INTO sessions (session_key, user_id, uuid, expire_at) VALUES (?, ?, ?, ?)`,
		sessionKey, userId, uuid, expireAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return store.SessionState{}, fmt.Errorf("insert session: %w", err)
	}

	return store.SessionState{
		SessionKey: sessionKey,
		UserId:     userId,
		Uuid:       uuid,
		ExpireAt:   expireAt,
	}, nil
}

func (s *SQLiteStore) ResolveUserId(sessionKey string) (int64, error) {
	now := s.clock()

	s.sessionMu.RLock()
	cached, ok := s.sessionCache[sessionKey]
	s.sessionMu.RUnlock()
	if ok && now.Before(cached.expireAt) {
		return cached.userId, nil
	}

	var userId int64
	var expireStr string
	err := s.db.QueryRow(
		`SELECT user_id, expire_at FROM sessions WHERE session_key = ?`, sessionKey,
	).Scan(&userId, &expireStr)
	if err != nil {
		return 0, store.ErrNotFound
	}

	expireAt, err := time.Parse(time.RFC3339Nano, expireStr)
	if err != nil {
		return 0, store.ErrNotFound
	}
	if now.After(expireAt) {
		return 0, store.ErrNotFound
	}

	s.sessionMu.Lock()
	s.sessionCache[sessionKey] = cachedSession{userId: userId, expireAt: expireAt}
	s.sessionMu.Unlock()

	return userId, nil
}
