package gametime

import (
	"log"
	"os"
	"strconv"
	"time"
)

// Official game calendar is Japan time. Override with GAME_TZ if needed.
var loc *time.Location = mustLoadLocation("Asia/Tokyo")

// DayResetHour is the hour (0–23) in `loc` when a new game day starts.
// Official client commonly uses 4 (04:00 JST). Set GAME_DAY_RESET_HOUR=0 for midnight.
var DayResetHour = 4

func mustLoadLocation(name string) *time.Location {
	l, err := time.LoadLocation(name)
	if err != nil {
		log.Printf("[gametime] load %s failed (%v), using UTC", name, err)
		return time.UTC
	}
	return l
}

func init() {
	if tz := os.Getenv("GAME_TZ"); tz != "" {
		if loaded, err := time.LoadLocation(tz); err != nil {
			log.Printf("[gametime] GAME_TZ=%q invalid (%v)", tz, err)
		} else {
			loc = loaded
		}
	}
	if h := os.Getenv("GAME_DAY_RESET_HOUR"); h != "" {
		if v, err := strconv.Atoi(h); err == nil && v >= 0 && v <= 23 {
			DayResetHour = v
		}
	}
	n := time.Now().In(loc)
	name, off := n.Zone()
	log.Printf("[gametime] zone=%s offset=%+dh day_reset=%02d:00 local_now=%s start_of_day_ms=%d",
		name, off/3600, DayResetHour, n.Format(time.RFC3339), StartOfDayMillisAt(n.UnixMilli()))
}

func Now() time.Time {
	return time.Now().In(loc)
}

func NowMillis() int64 {
	return time.Now().UnixMilli() // always real UTC epoch — client sync stays honest
}

func StartOfDayMillis() int64 {
	return StartOfDayMillisAt(NowMillis())
}

// StartOfDayMillisAt: last day-reset boundary at or before millis (e.g. today 04:00 JST).
func StartOfDayMillisAt(millis int64) int64 {
	n := time.UnixMilli(millis).In(loc)
	boundary := time.Date(n.Year(), n.Month(), n.Day(), DayResetHour, 0, 0, 0, loc)
	if n.Before(boundary) {
		boundary = boundary.AddDate(0, 0, -1)
	}
	return boundary.UnixMilli()
}

func WeeklyVersion(millis int64) int64 {
	start := StartOfDayMillisAt(millis)
	t := time.UnixMilli(start).In(loc)
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	monday := time.Date(t.Year(), t.Month(), t.Day()-(weekday-1), DayResetHour, 0, 0, 0, loc)
	return monday.UnixMilli()
}

func MonthlyVersion(millis int64) int64 {
	t := time.UnixMilli(millis).In(loc)
	first := time.Date(t.Year(), t.Month(), 1, DayResetHour, 0, 0, 0, loc)
	if t.Before(first) {
		first = time.Date(t.Year(), t.Month()-1, 1, DayResetHour, 0, 0, 0, loc)
	}
	return first.UnixMilli()
}