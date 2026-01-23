package app

import (
	"fmt"
	"time"
)

func RelativeTime(t time.Time) string {
	now := time.Now()
	if t.After(now) {
		return "just now"
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
	if days < 30 {
		return fmt.Sprintf("%dd ago", days)
	}

	months := days / 30
	if months < 12 {
		return fmt.Sprintf("%dmo ago", months)
	}

	years := months / 12
	return fmt.Sprintf("%dy ago", years)
}
