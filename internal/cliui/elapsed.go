package cliui

import (
	"fmt"
	"time"
)

// FormatElapsed implements the product-wide elapsed-time contract: 47s,
// 12m 16s, and 1h 04m. Negative durations are clamped to zero. Durations never
// use a colon because that reads as wall-clock time.
func FormatElapsed(duration time.Duration) string {
	seconds := max(int64(0), int64(duration/time.Second))
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%dm %02ds", seconds/60, seconds%60)
	}
	return fmt.Sprintf("%dh %02dm", seconds/3600, (seconds%3600)/60)
}
