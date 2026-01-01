package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	
	if strings.HasSuffix(s, "d") {
		daysStr := strings.TrimSuffix(s, "d")
		days, err := strconv.ParseFloat(daysStr, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid duration format: %s", s)
		}
		return time.Duration(days * 24 * float64(time.Hour)), nil
	}
	
	return time.ParseDuration(s)
}

func ValidateTokenExpiry(d time.Duration) error {
	maxDuration := 365 * 24 * time.Hour
	if d > maxDuration {
		return fmt.Errorf("expiry duration exceeds maximum of 365 days")
	}
	if d <= 0 {
		return fmt.Errorf("expiry duration must be positive")
	}
	return nil
}

func ValidateSetupCodeExpiry(d time.Duration) error {
	maxDuration := 72 * time.Hour
	if d > maxDuration {
		return fmt.Errorf("code expiry exceeds maximum of 72 hours")
	}
	if d <= 0 {
		return fmt.Errorf("code expiry must be positive")
	}
	return nil
}
