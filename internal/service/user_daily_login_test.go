package service

import (
	"testing"
	"time"

	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/store"
)

func TestRefreshDailyLoginStateAdvancesNewDay(t *testing.T) {
	loc := time.Local
	now := time.Date(2026, time.July, 14, 12, 5, 0, 0, loc).UnixMilli()
	lastLogin := time.Date(2026, time.July, 13, 23, 55, 0, 0, loc).UnixMilli()

	user := &store.UserState{
		Login: store.UserLoginState{
			TotalLoginCount:        7,
			ContinualLoginCount:    3,
			MaxContinualLoginCount: 4,
			LastLoginDatetime:      lastLogin,
		},
		LoginBonus: store.UserLoginBonusState{
			LatestRewardReceiveDatetime: lastLogin,
			LatestVersion:               0,
		},
	}

	if !RefreshDailyLoginState(user, now) {
		t.Fatalf("expected daily login refresh to trigger")
	}
	if user.Login.TotalLoginCount != 8 {
		t.Fatalf("total login count = %d, want 8", user.Login.TotalLoginCount)
	}
	if user.Login.ContinualLoginCount != 4 {
		t.Fatalf("continual login count = %d, want 4", user.Login.ContinualLoginCount)
	}
	if user.Login.MaxContinualLoginCount != 4 {
		t.Fatalf("max continual login count = %d, want 4", user.Login.MaxContinualLoginCount)
	}
	if user.Login.LastLoginDatetime != now {
		t.Fatalf("last login datetime = %d, want %d", user.Login.LastLoginDatetime, now)
	}
	if user.Login.LatestVersion != now {
		t.Fatalf("login latest version = %d, want %d", user.Login.LatestVersion, now)
	}
	if user.LoginBonus.LatestVersion != now {
		t.Fatalf("login bonus latest version = %d, want %d", user.LoginBonus.LatestVersion, now)
	}
}

func TestRefreshDailyLoginStateSameDayNoop(t *testing.T) {
	loc := time.Local
	now := time.Date(2026, time.July, 14, 12, 5, 0, 0, loc).UnixMilli()
	lastLogin := time.Date(2026, time.July, 14, 0, 1, 0, 0, loc).UnixMilli()

	user := &store.UserState{
		Login: store.UserLoginState{
			TotalLoginCount:        7,
			ContinualLoginCount:    3,
			MaxContinualLoginCount: 4,
			LastLoginDatetime:      lastLogin,
		},
		LoginBonus: store.UserLoginBonusState{
			LatestVersion: 123,
		},
	}

	if RefreshDailyLoginState(user, now) {
		t.Fatalf("same-day login must not refresh")
	}
	if user.Login.LastLoginDatetime != lastLogin {
		t.Fatalf("last login datetime changed: %d", user.Login.LastLoginDatetime)
	}
	if user.LoginBonus.LatestVersion != 123 {
		t.Fatalf("login bonus latest version changed: %d", user.LoginBonus.LatestVersion)
	}
	if gametime.StartOfDayMillisAt(now) != gametime.StartOfDayMillisAt(lastLogin) {
		t.Fatalf("test setup is not same-day")
	}
}
