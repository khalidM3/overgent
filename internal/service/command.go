package service

import "strings"

// commandError carries what a service-control tool printed, because launchctl,
// systemctl and schtasks all report the interesting part of a failure on their
// output and leave the exit status as a bare "exit status 1".
type commandError struct {
	err    error
	output string
}

func (e *commandError) Error() string { return strings.TrimSpace(e.output) }
func (e *commandError) Unwrap() error { return e.err }

// mentions reports whether an error's text contains any of these fragments,
// case-insensitively. Every classification helper in this package is string
// matching against tool output, because that is the only signal these tools
// give us: their exit statuses do not distinguish "already done" from "broken".
func mentions(err error, fragments ...string) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	for _, fragment := range fragments {
		if strings.Contains(text, fragment) {
			return true
		}
	}
	return false
}
