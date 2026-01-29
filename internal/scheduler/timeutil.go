package scheduler

import (
	"fmt"
	"strings"
	"time"
)

func NextRun(entry ScheduleEntry, now time.Time) (time.Time, error) {
	loc := time.Local
	if entry.Timezone != "" {
		if location, err := time.LoadLocation(entry.Timezone); err == nil {
			loc = location
		}
	}

	switch entry.Schedule.Type {
	case "once":
		parsed, err := parseDateTime(entry.Schedule.Date, entry.Schedule.Time, loc)
		if err != nil {
			return time.Time{}, err
		}
		if !parsed.After(now.In(loc)) {
			return time.Time{}, fmt.Errorf("scheduled time is in the past")
		}
		return parsed, nil
	case "daily":
		return nextDaily(entry.Schedule.Time, now.In(loc), loc), nil
	case "weekly":
		return nextWeekly(entry.Schedule.Weekday, entry.Schedule.Time, now.In(loc), loc)
	default:
		return time.Time{}, fmt.Errorf("unknown schedule type: %s", entry.Schedule.Type)
	}
}

func parseDateTime(date, clock string, loc *time.Location) (time.Time, error) {
	if date == "" || clock == "" {
		return time.Time{}, fmt.Errorf("date/time required")
	}
	parsed, err := time.ParseInLocation("2006-01-02 15:04", fmt.Sprintf("%s %s", date, clock), loc)
	if err != nil {
		return time.Time{}, err
	}
	return parsed, nil
}

func nextDaily(clock string, now time.Time, loc *time.Location) time.Time {
	hour, min := parseClock(clock)
	candidate := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, loc)
	if !candidate.After(now) {
		candidate = candidate.Add(24 * time.Hour)
	}
	return candidate
}

func nextWeekly(weekdayName, clock string, now time.Time, loc *time.Location) (time.Time, error) {
	target, ok := parseWeekday(weekdayName)
	if !ok {
		return time.Time{}, fmt.Errorf("invalid weekday: %s", weekdayName)
	}
	hour, min := parseClock(clock)
	delta := (int(target) - int(now.Weekday()) + 7) % 7
	candidate := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, loc).AddDate(0, 0, delta)
	if !candidate.After(now) {
		candidate = candidate.AddDate(0, 0, 7)
	}
	return candidate, nil
}

func parseClock(clock string) (int, int) {
	if len(clock) != 5 || clock[2] != ':' {
		return 0, 0
	}
	hour := parseInt(clock[0:2])
	min := parseInt(clock[3:5])
	return hour, min
}

func parseInt(value string) int {
	n := 0
	for _, r := range value {
		if r < '0' || r > '9' {
			return n
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func parseWeekday(name string) (time.Weekday, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "sunday":
		return time.Sunday, true
	case "monday":
		return time.Monday, true
	case "tuesday":
		return time.Tuesday, true
	case "wednesday":
		return time.Wednesday, true
	case "thursday":
		return time.Thursday, true
	case "friday":
		return time.Friday, true
	case "saturday":
		return time.Saturday, true
	default:
		return time.Sunday, false
	}
}

func WeekdayNumber(name string) (int, bool) {
	day, ok := parseWeekday(name)
	if !ok {
		return 0, false
	}
	if day == time.Sunday {
		return 0, true
	}
	return int(day), true
}

func FormatPMSet(t time.Time) string {
	return t.Format("01/02/06 15:04:05")
}

func RelativeLabel(t time.Time, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	if t.After(now) {
		delta := t.Sub(now)
		if delta < time.Minute {
			return "in <1m"
		}
		if delta < time.Hour {
			return fmt.Sprintf("in %dm", int(delta.Minutes()))
		}
		if delta < 24*time.Hour {
			return fmt.Sprintf("in %dh", int(delta.Hours()))
		}
		days := int(delta.Hours() / 24)
		return fmt.Sprintf("in %dd", days)
	}

	delta := now.Sub(t)
	if delta < time.Minute {
		return "just now"
	}
	if delta < time.Hour {
		return fmt.Sprintf("%dm ago", int(delta.Minutes()))
	}
	if delta < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(delta.Hours()))
	}
	days := int(delta.Hours() / 24)
	return fmt.Sprintf("%dd ago", days)
}
