package cmd

import (
	"fmt"
	"math"
	"time"
)

func FormatRelativeTime(t time.Time) string {
	now := time.Now()
	duration := t.Sub(now)

	absDuration := duration
	if absDuration < 0 {
		absDuration = -absDuration
	}

	var value int
	var unit string

	if absDuration < time.Hour {
		value = int(math.Round(absDuration.Minutes()))
		unit = "minute"
	} else if absDuration < 24*time.Hour {
		value = int(math.Round(absDuration.Hours()))
		unit = "hour"
	} else {
		value = int(math.Round(absDuration.Hours() / 24))
		unit = "day"
	}

	if value != 1 {
		unit += "s"
	}

	if duration < 0 {
		return fmt.Sprintf("%d %s ago", value, unit)
	}

	return fmt.Sprintf("in %d %s", value, unit)
}
