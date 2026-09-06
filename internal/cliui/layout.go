package cliui

import (
	"fmt"
	"io"
	"strings"
	"unicode"
)

// Field is one human-facing label and value. Values may contain newlines.
type Field struct {
	Label string
	Value string
}

// WriteFields renders aligned labels when there is room and switches to a
// stacked layout on narrow terminals. Values are wrapped without terminal box
// drawing, preserving the design system's single reading order.
func WriteFields(writer io.Writer, width int, fields []Field) error {
	if width <= 0 {
		width = defaultTerminalWidth
	}
	labelWidth := 0
	for _, field := range fields {
		if length := textWidth(field.Label); length > labelWidth {
			labelWidth = length
		}
	}
	if labelWidth > 24 {
		labelWidth = 24
	}
	stacked := width < labelWidth+22
	for _, field := range fields {
		if stacked {
			if _, err := fmt.Fprintln(writer, field.Label); err != nil {
				return err
			}
			for _, line := range Wrap(field.Value, max(1, width-2)) {
				if _, err := fmt.Fprintf(writer, "  %s\n", line); err != nil {
					return err
				}
			}
			continue
		}
		indent := strings.Repeat(" ", labelWidth+2)
		lines := Wrap(field.Value, max(1, width-textWidth(indent)))
		if _, err := fmt.Fprintf(writer, "%-*s  %s\n", labelWidth, field.Label, lines[0]); err != nil {
			return err
		}
		for _, line := range lines[1:] {
			if _, err := fmt.Fprintf(writer, "%s%s\n", indent, line); err != nil {
				return err
			}
		}
	}
	return nil
}

// Wrap folds text to at most width runes per line. It preserves explicit
// paragraph breaks and hard-wraps unusually long machine facts such as paths.
func Wrap(text string, width int) []string {
	if width < 1 {
		width = 1
	}
	var result []string
	for _, paragraph := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}
		line := ""
		for _, word := range words {
			parts := splitWidth(word, width)
			for _, part := range parts {
				if line == "" {
					line = part
					continue
				}
				if textWidth(line)+1+textWidth(part) <= width {
					line += " " + part
					continue
				}
				result = append(result, line)
				line = part
			}
		}
		result = append(result, line)
	}
	if len(result) == 0 {
		return []string{""}
	}
	return result
}

func splitWidth(text string, width int) []string {
	if textWidth(text) <= width {
		return []string{text}
	}
	var parts []string
	var part []rune
	partWidth := 0
	for _, value := range text {
		valueWidth := 1
		if unicode.Is(unicode.Mn, value) || unicode.Is(unicode.Me, value) {
			valueWidth = 0
		}
		if partWidth > 0 && partWidth+valueWidth > width {
			parts = append(parts, string(part))
			part = nil
			partWidth = 0
		}
		part = append(part, value)
		partWidth += valueWidth
	}
	if len(part) > 0 {
		parts = append(parts, string(part))
	}
	return parts
}

func textWidth(text string) int {
	width := 0
	for _, value := range text {
		if !unicode.Is(unicode.Mn, value) && !unicode.Is(unicode.Me, value) {
			width++
		}
	}
	return width
}
