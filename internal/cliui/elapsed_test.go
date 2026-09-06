package cliui

import (
	"testing"
	"time"
)

func TestFormatElapsed(t *testing.T) {
	tests := []struct {
		duration time.Duration
		want     string
	}{
		{-time.Second, "0s"},
		{47*time.Second + 900*time.Millisecond, "47s"},
		{12*time.Minute + 16*time.Second, "12m 16s"},
		{time.Hour + 4*time.Minute + 59*time.Second, "1h 04m"},
		{27*time.Hour + 9*time.Minute, "27h 09m"},
	}
	for _, test := range tests {
		if got := FormatElapsed(test.duration); got != test.want {
			t.Errorf("FormatElapsed(%s) = %q, want %q", test.duration, got, test.want)
		}
	}
}
