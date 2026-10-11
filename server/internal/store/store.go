package store

import (
	"errors"
	"time"

	"lunar-tear/server/internal/model"
)

var ErrNotFound = errors.New("store: not found")

type Clock func() time.Time

type UserRepository interface {
	CreateUser(uuid string, platform model.ClientPlatform) (int64, error)
	GetUserByUUID(uuid string) (int64, error)
	LoadUser(userId int64) (UserState, error)
	UpdateUser(userId int64, mutate func(*UserState)) (UserState, error)
	// EnsureGimmickSequence inserts a not-cleared gimmick-sequence row if one
	// does not already exist, leaving any existing (possibly cleared) row
	// untouched. It is a targeted fast path for GimmickService.UpdateSequence,
	// which the client fires once per sequence in a burst on map load; routing
	// each through the full LoadUser+clone+diff UpdateUser cycle made map loads
	// take many seconds.
	EnsureGimmickSequence(userId int64, scheduleId, sequenceId int32) error
	DefaultUserId() (int64, error)
	SetFacebookId(userId int64, facebookId int64) error
	GetUserByFacebookId(facebookId int64) (int64, error)
	GetFacebookId(userId int64) (int64, error)
	ClearFacebookId(userId int64) error
	UpdateUUID(userId int64, newUuid string) error
	// GetUserOptions returns the user_options row, or the zero value when the
	// user has never saved any option (missing row is not ErrNotFound).
	GetUserOptions(userId int64) (UserOptionsState, error)
	// SaveUserOptions upserts the user_options row for userId.
	SaveUserOptions(userId int64, opts UserOptionsState) error
}

type SessionRepository interface {
	CreateSession(uuid string, ttl time.Duration) (SessionState, error)
	ResolveUserId(sessionKey string) (int64, error)
}
