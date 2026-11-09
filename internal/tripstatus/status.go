// Package tripstatus computes a trip's lifecycle state from its calendar dates.
package tripstatus

import "time"

const defaultTZ = "Asia/Bangkok"

// Status describes where a trip is in its lifecycle.
type Status struct {
	State     string `json:"state"`      // "planning" | "upcoming" | "ongoing" | "completed"
	DaysUntil int    `json:"days_until"` // upcoming: calendar days from today to departure (>=1)
	DayNumber int    `json:"day_number"` // ongoing: 1-based day of the trip
	TotalDays int    `json:"total_days"` // upcoming/ongoing/completed: return - departure + 1
}

// Location resolves a trip timezone, falling back to Asia/Bangkok (then UTC) when tz is empty or invalid.
func Location(tz string) *time.Location {
	if loc, err := time.LoadLocation(tz); err == nil && tz != "" {
		return loc
	}
	if loc, err := time.LoadLocation(defaultTZ); err == nil {
		return loc
	}
	return time.UTC
}

// calendarDay normalises a date's Y-M-D to midnight UTC so days can be diffed.
func calendarDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func daysBetween(a, b time.Time) int {
	return int(b.Sub(a).Hours() / 24)
}

// Compute returns the trip status. departure and ret are stored calendar days
// (their UTC date is used, time-of-day ignored); "today" is now in the trip's
// timezone. A draft or a trip without dates is "planning". An invalid tz falls back to Asia/Bangkok.
func Compute(departure, ret time.Time, tz string, isDraft bool, now time.Time) Status {
	if isDraft || departure.IsZero() || ret.IsZero() {
		return Status{State: "planning"}
	}
	dep := calendarDay(departure.UTC())
	end := calendarDay(ret.UTC())
	if end.Before(dep) {
		end = dep
	}
	today := calendarDay(now.In(Location(tz)))
	total := daysBetween(dep, end) + 1

	switch {
	case today.Before(dep):
		return Status{State: "upcoming", DaysUntil: daysBetween(today, dep), TotalDays: total}
	case today.After(end):
		return Status{State: "completed", TotalDays: total}
	default:
		return Status{State: "ongoing", DayNumber: daysBetween(dep, today) + 1, TotalDays: total}
	}
}
