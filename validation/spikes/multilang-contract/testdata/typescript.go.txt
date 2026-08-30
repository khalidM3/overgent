package contract

import "strings"

// TypeScript and TSX extraction is a bounded scanner rather than a parser:
// ADR-019 keeps the root Go module free of CGO and of Node invocation, so no
// TypeScript grammar is available. The scanner recognizes top-level exported
// declarations and captures the declaration header up to the start of the body.
//
// Recognized forms, each optionally preceded by `declare`, `abstract`, or
// `async`:
//
//	export function f(...): T      header up to the body brace or a `;` overload
//	export class C extends B       header up to the body brace
//	export interface I             header up to the body brace
//	export enum E / export const enum E
//	export type T = ...            the whole alias, including its right side
//	export const c: T              header up to `=`, so the value is not derived
//
// Known limitations, accepted as best effort:
//   - `export default`, `export { … }`, `export * from`, and `export let`/`var`
//     contribute no symbols.
//   - The value of an exported const is deliberately not part of the contract,
//     so changing only a literal is invisible.
//   - Regular-expression literals are not tokenized. One containing a quote or
//     a comment opener desynchronizes the scan; the file then usually fails the
//     balance check and yields no fingerprint instead of a wrong one.
//   - `<` is read as a type-argument opener only after an identifier or `>`, so
//     a comparison inside a declaration header can mis-nest.
const (
	kindFunction  = "function"
	kindClass     = "class"
	kindInterface = "interface"
	kindEnum      = "enum"
	kindAlias     = "type"
	kindConstant  = "const"
)

// maxDeclarationRunes bounds one header capture. A declaration that runs past
// it contributes no symbol rather than an unbounded scan.
const maxDeclarationRunes = 4000

const (
	tsCode = iota
	tsComment
	tsString
)

type stopMode int

const (
	// stopBody ends a header at the brace that opens the declaration body.
	stopBody stopMode = iota
	// stopAssign ends a header at the `=` that begins an initializer.
	stopAssign
	// stopStatement ends a declaration at the end of the statement.
	stopStatement
)

// extractTypeScript records the exported surface of one .ts or .tsx file. It
// reports false when a comment or string literal is unterminated or when
// brackets do not balance, which is the scanner's definition of unparseable.
func extractTypeScript(source []byte) ([]Symbol, bool) {
	runes := []rune(string(source))
	kinds, ok := classifyTypeScript(runes)
	if !ok {
		return nil, false
	}
	var symbols []Symbol
	brace, paren, bracket := 0, 0, 0
	lineStart := true
	for index := 0; index < len(runes); index++ {
		if runes[index] == '\n' {
			lineStart = true
			continue
		}
		if kinds[index] != tsCode {
			continue
		}
		character := runes[index]
		if character == ' ' || character == '\t' || character == '\r' {
			continue
		}
		if lineStart && brace == 0 && paren == 0 && bracket == 0 && wordAt(runes, kinds, index, "export") {
			if symbol, next, found := parseExport(runes, kinds, index); found {
				symbols = append(symbols, symbol)
				lineStart = false
				index = next - 1
				continue
			}
		}
		lineStart = false
		switch character {
		case '{':
			brace++
		case '}':
			if brace == 0 {
				return nil, false
			}
			brace--
		case '(':
			paren++
		case ')':
			if paren == 0 {
				return nil, false
			}
			paren--
		case '[':
			bracket++
		case ']':
			if bracket == 0 {
				return nil, false
			}
			bracket--
		}
	}
	if brace != 0 || paren != 0 || bracket != 0 {
		return nil, false
	}
	return symbols, true
}

// classifyTypeScript marks every rune as code, comment, or string literal so
// later passes can ignore braces and quotes that carry no structure.
func classifyTypeScript(runes []rune) ([]byte, bool) {
	kinds := make([]byte, len(runes))
	index := 0
	for index < len(runes) {
		character := runes[index]
		switch {
		case character == '/' && index+1 < len(runes) && runes[index+1] == '/':
			for index < len(runes) && runes[index] != '\n' {
				kinds[index] = tsComment
				index++
			}
		case character == '/' && index+1 < len(runes) && runes[index+1] == '*':
			kinds[index], kinds[index+1] = tsComment, tsComment
			index += 2
			closed := false
			for index < len(runes) {
				kinds[index] = tsComment
				if runes[index] == '*' && index+1 < len(runes) && runes[index+1] == '/' {
					kinds[index+1] = tsComment
					index += 2
					closed = true
					break
				}
				index++
			}
			if !closed {
				return nil, false
			}
		case character == '\'' || character == '"' || character == '`':
			quote := character
			kinds[index] = tsString
			index++
			closed := false
			for index < len(runes) {
				if runes[index] == '\\' {
					kinds[index] = tsString
					if index+1 < len(runes) {
						kinds[index+1] = tsString
						index += 2
						continue
					}
					index++
					continue
				}
				if quote != '`' && runes[index] == '\n' {
					break
				}
				kinds[index] = tsString
				if runes[index] == quote {
					index++
					closed = true
					break
				}
				index++
			}
			if !closed {
				return nil, false
			}
		default:
			index++
		}
	}
	return kinds, true
}

// parseExport reads one top-level `export` declaration. The third result is
// false when the form is not one this scanner records, in which case the caller
// resumes ordinary scanning at the same position.
func parseExport(runes []rune, kinds []byte, start int) (Symbol, int, bool) {
	word, next := nextWord(runes, kinds, start+len("export"))
	for word == "declare" || word == "abstract" || word == "async" {
		word, next = nextWord(runes, kinds, next)
	}
	kind, mode := "", stopBody
	switch word {
	case "function":
		kind = kindFunction
	case "class":
		kind = kindClass
	case "interface":
		kind = kindInterface
	case "enum":
		kind = kindEnum
	case "type":
		kind, mode = kindAlias, stopStatement
	case "const":
		if peek, peekNext := nextWord(runes, kinds, next); peek == "enum" {
			kind, next = kindEnum, peekNext
		} else {
			kind, mode = kindConstant, stopAssign
		}
	default:
		return Symbol{}, start, false
	}
	name, _ := nextWord(runes, kinds, next)
	if !identifier(name) {
		return Symbol{}, start, false
	}
	// Scanning restarts at the `export` keyword so bracket nesting and the
	// preceding-token history are complete when the terminator is chosen.
	stop, ok := scanDeclaration(runes, kinds, start, mode)
	if !ok {
		return Symbol{}, start, false
	}
	return newSymbol(name, kind, declarationText(runes, kinds, start, stop)), stop, true
}

// scanDeclaration finds where a declaration's captured header ends.
func scanDeclaration(runes []rune, kinds []byte, from int, mode stopMode) (int, bool) {
	paren, bracket, brace, angle := 0, 0, 0, 0
	limit := min(len(runes), from+maxDeclarationRunes)
	previous, previousWord := rune(0), ""
	for index := from; index < limit; index++ {
		if kinds[index] == tsComment {
			continue
		}
		character := runes[index]
		if kinds[index] == tsString {
			previous, previousWord = character, ""
			continue
		}
		balanced := paren == 0 && bracket == 0 && brace == 0 && angle == 0
		switch character {
		case '(':
			paren++
		case ')':
			if paren == 0 {
				return 0, false
			}
			paren--
		case '[':
			bracket++
		case ']':
			if bracket == 0 {
				return 0, false
			}
			bracket--
		case '<':
			if identifierRune(previous) || previous == '>' {
				angle++
			}
		case '>':
			if angle > 0 {
				angle--
			}
		case '{':
			if mode == stopBody && balanced && !typePositionBrace(previous, previousWord) {
				return index, true
			}
			brace++
		case '}':
			if brace == 0 {
				return 0, false
			}
			brace--
		case ';':
			if balanced {
				return index, true
			}
		case '=':
			if mode == stopAssign && balanced {
				return index, true
			}
		case '\n':
			if mode != stopBody && balanced && !continuesAfter(previous, previousWord) &&
				!continuesBefore(runes, kinds, index) {
				return index, true
			}
		}
		if character == '\n' || character == ' ' || character == '\t' || character == '\r' {
			continue
		}
		if identifierRune(character) {
			previousWord += string(character)
		} else {
			previousWord = ""
		}
		previous = character
	}
	return 0, false
}

// declarationText renders the captured span, replacing comments with a space so
// removing them can never fuse two tokens together.
func declarationText(runes []rune, kinds []byte, start, stop int) string {
	var builder strings.Builder
	for index := start; index < stop; index++ {
		if kinds[index] == tsComment {
			builder.WriteRune(' ')
			continue
		}
		builder.WriteRune(runes[index])
	}
	return builder.String()
}

// typePositionBrace reports whether a brace at zero depth opens a type rather
// than a declaration body, as in a function whose return type is an object.
func typePositionBrace(previous rune, previousWord string) bool {
	if strings.ContainsRune(":|&,=([<", previous) {
		return true
	}
	switch previousWord {
	case "keyof", "typeof", "infer", "as", "is", "satisfies":
		return true
	}
	return false
}

// continuesAfter reports whether the text before a newline leaves a declaration
// obviously unfinished, as in a line ending with `=` or `|`.
func continuesAfter(previous rune, previousWord string) bool {
	if strings.ContainsRune("=|&,<([{?:.+-*/!~^", previous) {
		return true
	}
	switch previousWord {
	case "extends", "keyof", "typeof", "infer", "readonly", "in", "as", "is", "new", "satisfies":
		return true
	}
	return false
}

// continuesBefore reports whether the next line opens with a continuation, the
// style used when union members are written one per line with a leading bar.
func continuesBefore(runes []rune, kinds []byte, newline int) bool {
	for index := newline + 1; index < len(runes); index++ {
		if kinds[index] == tsComment {
			continue
		}
		character := runes[index]
		if character == ' ' || character == '\t' || character == '\r' || character == '\n' {
			continue
		}
		if kinds[index] == tsString {
			return false
		}
		if strings.ContainsRune("|&?:=.,<>)]}", character) {
			return true
		}
		word, _ := nextWord(runes, kinds, index)
		return word == "extends"
	}
	return false
}

// nextWord skips whitespace, comments, and a generator star, then reads the
// following identifier. It returns the empty string when the next code rune is
// not an identifier rune.
func nextWord(runes []rune, kinds []byte, from int) (string, int) {
	index := from
	for index < len(runes) {
		if kinds[index] == tsComment {
			index++
			continue
		}
		character := runes[index]
		if character == ' ' || character == '\t' || character == '\r' || character == '\n' || character == '*' {
			index++
			continue
		}
		break
	}
	start := index
	for index < len(runes) && kinds[index] == tsCode && identifierRune(runes[index]) {
		index++
	}
	return string(runes[start:index]), index
}

// wordAt reports whether the code runes at index spell word and end on a
// boundary, so `exports` never matches `export`.
func wordAt(runes []rune, kinds []byte, index int, word string) bool {
	target := []rune(word)
	if index+len(target) > len(runes) {
		return false
	}
	for offset, expected := range target {
		if kinds[index+offset] != tsCode || runes[index+offset] != expected {
			return false
		}
	}
	after := index + len(target)
	return after == len(runes) || !identifierRune(runes[after])
}

func identifier(word string) bool {
	if word == "" {
		return false
	}
	for offset, character := range word {
		if offset == 0 && character >= '0' && character <= '9' {
			return false
		}
		if !identifierRune(character) {
			return false
		}
	}
	return true
}

func identifierRune(character rune) bool {
	return character == '_' || character == '$' ||
		character >= 'a' && character <= 'z' ||
		character >= 'A' && character <= 'Z' ||
		character >= '0' && character <= '9'
}
