package tripstatus

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestCompute(t *testing.T) {
	bkk := time.FixedZone("ICT", 7*3600)
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)

	dep := day(2026, 12, 12)
	ret := day(2026, 12, 16)

	tests := []struct {
		name    string
		dep     time.Time
		ret     time.Time
		tz      string
		isDraft bool
		now     time.Time
		want    Status
	}{
		{"draft is planning", dep, ret, "Asia/Bangkok", true, time.Date(2026, 12, 13, 12, 0, 0, 0, bkk), Status{State: "planning"}},
		{"3 days before", dep, ret, "Asia/Bangkok", false, time.Date(2026, 12, 9, 10, 0, 0, 0, bkk), Status{State: "upcoming", DaysUntil: 3, TotalDays: 5}},
		{"departure day", dep, ret, "Asia/Bangkok", false, time.Date(2026, 12, 12, 0, 5, 0, 0, bkk), Status{State: "ongoing", DayNumber: 1, TotalDays: 5}},
		{"return day", dep, ret, "Asia/Bangkok", false, time.Date(2026, 12, 16, 23, 59, 0, 0, bkk), Status{State: "ongoing", DayNumber: 5, TotalDays: 5}},
		{"day after return", dep, ret, "Asia/Bangkok", false, time.Date(2026, 12, 17, 0, 0, 0, 0, bkk), Status{State: "completed", TotalDays: 5}},
		{"tokyo return day 00:30 JST", dep, ret, "Asia/Tokyo", false, time.Date(2026, 12, 16, 0, 30, 0, 0, tokyo), Status{State: "ongoing", DayNumber: 5, TotalDays: 5}},
		{"tokyo day after 00:10 JST", dep, ret, "Asia/Tokyo", false, time.Date(2026, 12, 17, 0, 10, 0, 0, tokyo), Status{State: "completed", TotalDays: 5}},
		{"tokyo same instant given in UTC", dep, ret, "Asia/Tokyo", false, time.Date(2026, 12, 15, 15, 30, 0, 0, time.UTC), Status{State: "ongoing", DayNumber: 5, TotalDays: 5}},
		{"invalid tz falls back", dep, ret, "Mars/Base", false, time.Date(2026, 12, 9, 10, 0, 0, 0, bkk), Status{State: "upcoming", DaysUntil: 3, TotalDays: 5}},
		{"empty tz falls back", dep, ret, "", false, time.Date(2026, 12, 12, 8, 0, 0, 0, bkk), Status{State: "ongoing", DayNumber: 1, TotalDays: 5}},
		{"time-of-day ignored", time.Date(2026, 12, 12, 17, 0, 0, 0, time.UTC), time.Date(2026, 12, 16, 23, 0, 0, 0, time.UTC), "Asia/Bangkok", false, time.Date(2026, 12, 12, 1, 0, 0, 0, bkk), Status{State: "ongoing", DayNumber: 1, TotalDays: 5}},
		{"return before departure is single day", dep, day(2026, 12, 10), "Asia/Bangkok", false, time.Date(2026, 12, 12, 9, 0, 0, 0, bkk), Status{State: "ongoing", DayNumber: 1, TotalDays: 1}},
		{"return before departure completed next day", dep, day(2026, 12, 10), "Asia/Bangkok", false, time.Date(2026, 12, 13, 9, 0, 0, 0, bkk), Status{State: "completed", TotalDays: 1}},
		{"one day before", dep, ret, "Asia/Bangkok", false, time.Date(2026, 12, 11, 23, 59, 0, 0, bkk), Status{State: "upcoming", DaysUntil: 1, TotalDays: 5}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Compute(tc.dep, tc.ret, tc.tz, tc.isDraft, tc.now))
		})
	}
}
