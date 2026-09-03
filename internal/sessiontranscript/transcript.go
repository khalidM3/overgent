// Package sessiontranscript reads the vendor session record for an agent
// session so a member can see their own work (ADR-036).
//
// Each vendor writes a different record, so a format is detected and handed to
// its own adapter (ADR-039). Files are read locally and bounded from the tail;
// nothing here copies a transcript to a second durable store. Raw tool results,
// command output, attachments, and any vendor-encrypted reasoning are dropped
// during parsing and never become candidates for sharing.
package sessiontranscript

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	// MaxTailBytes bounds how much of a long-running session is read. Sessions
	// routinely reach tens of megabytes; only recent turns are useful.
	MaxTailBytes = 2 << 20
	// headBytes bounds the separate metadata scan for a large file, so a session
	// keeps a stable title instead of one that drifts with the tail window.
	headBytes = 256 << 10
	// MaxMessages bounds how many parsed messages are retained.
	MaxMessages = 200
	// MaxMessageBytes bounds a single rendered message.
	MaxMessageBytes = 8000
	// maxLineBytes bounds the scanner's buffer for one record.
	maxLineBytes = 8 << 20
	// maxRecordBytes skips outsized records outright. A rollout can embed inline
	// image data in a single multi-megabyte line; decoding those costs seconds
	// and they never contain readable conversation, which is capped far lower.
	maxRecordBytes = 512 << 10
)

// Kinds a transcript can yield. "thinking" is reasoning the vendor itself
// surfaced; Overgent never reads encrypted reasoning or infers hidden chain of
// thought. "tool" carries a name only and is never shareable content.
const (
	KindUser      = "user"
	KindAssistant = "assistant"
	KindThinking  = "thinking"
	KindSystem    = "system"
	KindTool      = "tool"
)

type Message struct {
	Kind string `json:"kind"`
	Text string `json:"text,omitempty"`
	Tool string `json:"tool,omitempty"`
	At   string `json:"at,omitempty"`
}

type Session struct {
	SessionID string    `json:"sessionId"`
	Vendor    string    `json:"vendor,omitempty"`
	Title     string    `json:"title,omitempty"`
	Branch    string    `json:"branch,omitempty"`
	Messages  []Message `json:"messages"`
}

// adapter is one vendor's record format.
type adapter interface {
	// name identifies the vendor for reporting.
	name() string
	// meta folds one record into session-level metadata such as title or branch.
	meta(line []byte, session *Session)
	// messages converts one record into zero or more conversation messages.
	messages(line []byte) []Message
}

// Read parses the session record at path, keeping at most the last `limit`
// messages. A malformed record is skipped so one bad line never hides a
// session; an unreadable or unrecognized file is an error.
func Read(path string, limit int) (Session, error) {
	if limit < 1 || limit > MaxMessages {
		limit = MaxMessages
	}
	clean, err := validatePath(path)
	if err != nil {
		return Session{}, err
	}
	file, err := os.Open(clean)
	if err != nil {
		return Session{}, fmt.Errorf("open transcript: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return Session{}, fmt.Errorf("stat transcript: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Session{}, errors.New("transcript is not a regular file")
	}

	head := make([]byte, min64(info.Size(), headBytes))
	if _, err = io.ReadFull(file, head); err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return Session{}, fmt.Errorf("read transcript: %w", err)
	}
	format := detect(head)
	if format == nil {
		// The head can be one outsized record with no parsable line inside it.
		// Stream past those so a single giant record cannot hide a session.
		if _, err = file.Seek(0, io.SeekStart); err != nil {
			return Session{}, fmt.Errorf("seek transcript: %w", err)
		}
		format = detectStream(file)
	}
	if format == nil {
		return Session{}, errors.New("transcript format is not recognized")
	}
	session := Session{Vendor: format.name()}

	// Metadata comes from the head so a long session keeps a stable title even as
	// the tail window moves past its opening request.
	opening := make([]Message, 0, 4)
	forEachLine(bytes.NewReader(head), func(line []byte) {
		format.meta(line, &session)
		if len(opening) >= cap(opening) {
			return
		}
		for _, message := range format.messages(line) {
			if message.Kind == KindUser {
				opening = append(opening, message)
			}
		}
	})

	var reader io.Reader
	if info.Size() > MaxTailBytes {
		if _, err = file.Seek(info.Size()-MaxTailBytes, io.SeekStart); err != nil {
			return Session{}, fmt.Errorf("seek transcript: %w", err)
		}
		trimmed := bufio.NewReaderSize(file, 1<<20)
		// The seek lands mid-record; drop the partial first line.
		if _, err = trimmed.ReadString('\n'); err != nil && !errors.Is(err, io.EOF) {
			return Session{}, fmt.Errorf("read transcript: %w", err)
		}
		reader = trimmed
	} else {
		if _, err = file.Seek(0, io.SeekStart); err != nil {
			return Session{}, fmt.Errorf("seek transcript: %w", err)
		}
		reader = file
	}

	forEachLine(reader, func(line []byte) {
		if info.Size() <= MaxTailBytes {
			// Small files were not covered by a separate head scan beyond
			// headBytes, so keep folding metadata as we go.
			format.meta(line, &session)
		}
		for _, message := range format.messages(line) {
			// Streamed records can repeat the same text; keep it readable.
			if last := len(session.Messages) - 1; last >= 0 && session.Messages[last] == message {
				continue
			}
			session.Messages = append(session.Messages, message)
		}
	})
	if len(session.Messages) > limit {
		session.Messages = session.Messages[len(session.Messages)-limit:]
	}
	if session.Title == "" {
		session.Title = derivedTitle(opening)
	}
	if session.Title == "" {
		session.Title = derivedTitle(session.Messages)
	}
	return session, nil
}

// detect picks the adapter for a record format. Claude Code writes one message
// object per line; Codex wraps every record in a typed payload envelope.
func detect(head []byte) adapter {
	found := adapter(nil)
	forEachLine(bytes.NewReader(head), func(line []byte) {
		if found != nil {
			return
		}
		var probe struct {
			Type      string          `json:"type"`
			Payload   json.RawMessage `json:"payload"`
			SessionID string          `json:"sessionId"`
			Message   json.RawMessage `json:"message"`
		}
		if json.Unmarshal(line, &probe) != nil {
			return
		}
		switch {
		case len(probe.Payload) > 0 && codexRecordTypes[probe.Type]:
			found = codexAdapter{}
		case probe.SessionID != "" || len(probe.Message) > 0 || claudeRecordTypes[probe.Type]:
			found = claudeAdapter{}
		}
	})
	return found
}

// detectStream finds the format by reading past outsized records, bounded so a
// very large session never turns detection into a full scan.
func detectStream(reader io.Reader) adapter {
	const budget = 4 << 20
	buffered := bufio.NewReaderSize(reader, 1<<20)
	for consumed := 0; consumed < budget; {
		line, err := buffered.ReadBytes('\n')
		consumed += len(line)
		if len(line) > 0 && len(line) <= maxRecordBytes {
			if found := detect(line); found != nil {
				return found
			}
		}
		if err != nil {
			return nil
		}
	}
	return nil
}

func forEachLine(reader io.Reader, visit func([]byte)) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 1<<20), maxLineBytes)
	for scanner.Scan() {
		if line := scanner.Bytes(); len(line) > 0 && len(line) <= maxRecordBytes {
			visit(line)
		}
	}
}

// derivedTitle labels a session that carries no title of its own, using its
// opening request. It is a label taken verbatim from what the member typed,
// never a summary Overgent invented.
func derivedTitle(messages []Message) string {
	for _, message := range messages {
		if message.Kind != KindUser || message.Text == "" {
			continue
		}
		title := strings.TrimSpace(strings.SplitN(strings.TrimSpace(message.Text), "\n", 2)[0])
		// A resumed or compacted session opens with a machine-written preamble
		// rather than a request; that is not a useful label.
		if title == "" || isSyntheticPreamble(title) {
			continue
		}
		title = strings.TrimSpace(strings.TrimPrefix(title, "&#x20;"))
		if len([]rune(title)) > 72 {
			title = strings.TrimSpace(string([]rune(title)[:72])) + "…"
		}
		if title != "" {
			return title
		}
	}
	return ""
}

// syntheticPreambles are opening lines a vendor writes on the member's behalf
// when resuming or compacting a session.
var syntheticPreambles = []string{
	"the following is the codex agent history",
	"this session is being continued from a previous",
	"caveat: the messages below were generated",
	"<recommended_plugins>",
	"# agents.md instructions",
}

func isSyntheticPreamble(title string) bool {
	lowered := strings.ToLower(title)
	for _, prefix := range syntheticPreambles {
		if strings.HasPrefix(lowered, prefix) {
			return true
		}
	}
	return false
}

// validatePath keeps reads inside real session records and refuses anything
// that is not a plain absolute path to a JSONL file.
func validatePath(path string) (string, error) {
	if path == "" || len(path) > 4096 || strings.ContainsRune(path, '\x00') {
		return "", errors.New("transcript path is missing or invalid")
	}
	if !filepath.IsAbs(path) {
		return "", errors.New("transcript path must be absolute")
	}
	clean := filepath.Clean(path)
	if filepath.Ext(clean) != ".jsonl" {
		return "", errors.New("transcript path is not a JSONL transcript")
	}
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return "", fmt.Errorf("resolve transcript: %w", err)
	}
	return resolved, nil
}

func bounded(value string, limit int) string {
	value = strings.TrimSpace(value)
	if value == "" || !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return ""
	}
	if len(value) > limit {
		// Cut on a rune boundary in one pass. Rebuilding the string per rune is
		// quadratic, and instruction blocks reach hundreds of kilobytes.
		cut := limit
		for cut > 0 && !utf8.RuneStart(value[cut]) {
			cut--
		}
		value = strings.TrimSpace(value[:cut])
	}
	return value
}

func maxInt64(value int64, floor int64) int64 {
	if value > floor {
		return value
	}
	return floor
}

func min64(value int64, limit int64) int64 {
	if value < limit {
		return value
	}
	return limit
}
