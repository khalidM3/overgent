package cliui

// Style is intentionally limited to Overgent's status and hierarchy channels.
// Adding a severity rainbow here would violate the product design system.
type Style uint8

const (
	StyleAlert Style = iota
	StyleLive
	StyleMuted
	StyleBold
)

const ansiReset = "\x1b[0m"

// Style applies an ANSI style when color is enabled. The returned text always
// carries the same meaning without ANSI escapes.
func (terminal Terminal) Style(style Style, text string) string {
	if text == "" || !terminal.ColorEnabled() {
		return text
	}
	var sequence string
	switch style {
	case StyleAlert:
		sequence = "\x1b[31m"
	case StyleLive:
		sequence = "\x1b[32m"
	case StyleMuted:
		sequence = "\x1b[2m"
	case StyleBold:
		sequence = "\x1b[1m"
	default:
		return text
	}
	return sequence + text + ansiReset
}
