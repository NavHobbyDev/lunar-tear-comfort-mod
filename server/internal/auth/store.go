package auth

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserExists   = errors.New("username already taken")
	ErrInvalidCreds = errors.New("invalid username or password")
)

type AuthUser struct {
	ID       int64
	Username string
}

type AuthStore struct {
	db *sql.DB
}

func NewAuthStore(db *sql.DB) (*AuthStore, error) {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS auth_users (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			username   TEXT    NOT NULL UNIQUE,
			password   BLOB   NOT NULL,
			created_at TEXT    NOT NULL
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("create auth_users table: %w", err)
	}
	return &AuthStore{db: db}, nil
}

func (s *AuthStore) CreateUser(username, password string) (AuthUser, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return AuthUser{}, fmt.Errorf("hash password: %w", err)
	}

	res, err := s.db.Exec(
		`INSERT INTO auth_users (username, password, created_at) VALUES (?, ?, ?)`,
		username, hash, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return AuthUser{}, ErrUserExists
		}
		return AuthUser{}, fmt.Errorf("insert user: %w", err)
	}

	id, _ := res.LastInsertId()
	return AuthUser{ID: id, Username: username}, nil
}

func (s *AuthStore) VerifyUser(username, password string) (AuthUser, error) {
	var id int64
	var hash []byte
	err := s.db.QueryRow(
		`SELECT id, password FROM auth_users WHERE username = ?`, username,
	).Scan(&id, &hash)
	if err != nil {
		return AuthUser{}, ErrInvalidCreds
	}

	if err := bcrypt.CompareHashAndPassword(hash, []byte(password)); err != nil {
		return AuthUser{}, ErrInvalidCreds
	}

	return AuthUser{ID: id, Username: username}, nil
}

func (s *AuthStore) UserExists(username string) bool {
	var n int
	err := s.db.QueryRow(`SELECT 1 FROM auth_users WHERE username = ?`, username).Scan(&n)
	return err == nil
}

func isUniqueViolation(err error) bool {
	return err != nil && (errors.Is(err, sql.ErrNoRows) ||
		// modernc.org/sqlite returns error strings containing "UNIQUE constraint failed"
		fmt.Sprintf("%v", err) == fmt.Sprintf("%v", err) &&
			contains(err.Error(), "UNIQUE constraint failed"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
