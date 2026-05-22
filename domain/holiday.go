package domain

import "time"

// HolidayCalendar reports whether a given calendar day is a non-working day
// beyond the weekend. Implementations live outside the domain; the default
// EmptyCalendar treats only Saturday/Sunday as non-working — sufficient as
// the §0.5 fallback until a production source (e.g. AT-published table) is wired.
type HolidayCalendar interface {
	IsHoliday(date time.Time) bool
}

// EmptyCalendar is the zero-config fallback: weekend-only, no national or
// regional holidays. Use until a real calendar source is configured.
type EmptyCalendar struct{}

func (EmptyCalendar) IsHoliday(time.Time) bool { return false }

// workingDaysBetween counts working days in the half-open interval (start, end]
// at calendar-day granularity in start's location. Weekends and cal.IsHoliday
// days are excluded. Returns 0 if end is not strictly after start.
func workingDaysBetween(start, end time.Time, cal HolidayCalendar) int {
	if cal == nil {
		cal = EmptyCalendar{}
	}
	s := dateOnly(start)
	e := dateOnly(end)
	if !s.Before(e) {
		return 0
	}
	days := 0
	for cur := s.AddDate(0, 0, 1); !cur.After(e); cur = cur.AddDate(0, 0, 1) {
		switch cur.Weekday() {
		case time.Saturday, time.Sunday:
			continue
		}
		if cal.IsHoliday(cur) {
			continue
		}
		days++
	}
	return days
}
