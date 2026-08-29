package sessiontranscript

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// codexAdapter reads a Codex rollout file, which wraps every record in a typed
// envelope: {"timestamp":…,"type":…,"payload":{…}}.
//
// Conversation text comes from the `event_msg` stream, which is what Codex
// actually shows its own user. The `response_item` stream is raw model I/O and
// additionally carries injected context, tool framing, and instruction blocks
// that were never part of the conversation, so it is used only for tool names
// and the operating instructions. The `reasoning` payload's `encrypted_content`
// is vendor-held hidden reasoning and is never read (ADR-036).
type codexAdapter struct{}

var codexRecordTypes = map[string]bool{
	"session_meta": true, "response_item": true, "event_msg": true,
	"turn_context": true, "world_state": true, "compacted": true,
}

type codexLine struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type codexPayload struct {
	Type      string      `json:"type"`
	Role      string      `json:"role"`
	Name      string      `json:"name"`
	Text      string      `json:"text"`
	Message   string      `json:"message"`
	Content   []codexPart `json:"content"`
	ID        string      `json:"id"`
	SessionID string      `json:"session_id"`
	CWD       string      `json:"cwd"`
	Item      *codexItem  `json:"item"`
}

// codexItem is the Codex desktop app's turn representation. The desktop app
// reports conversation turns as `item_completed` events rather than the CLI's
// `user_message`/`agent_message` events (ADR-039 per-vendor adapter fidelity).
type codexItem struct {
	Type    string      `json:"type"`
	Content []codexPart `json:"content"`
}

type codexPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (codexAdapter) name() string { return "codex" }

func (codexAdapter) meta(line []byte, session *Session) {
	var raw codexLine
	if json.Unmarshal(line, &raw) != nil || raw.Type != "session_meta" {
		return
	}
	var payload codexPayload
	if json.Unmarshal(raw.Payload, &payload) != nil {
		return
	}
	if id := payload.SessionID; id != "" {
		session.SessionID = id
	} else if payload.ID != "" {
		session.SessionID = payload.ID
	}
	// Codex records no title and no branch; the title is derived from the
	// opening request and the branch is read from the worktree itself.
}

func (codexAdapter) messages(line []byte) []Message {
	var raw codexLine
	if json.Unmarshal(line, &raw) != nil || len(raw.Payload) == 0 {
		return nil
	}
	var payload codexPayload
	if json.Unmarshal(raw.Payload, &payload) != nil {
		return nil
	}
	switch raw.Type {
	case "event_msg":
		// The conversation as Codex presented it to its own user.
		switch payload.Type {
		case "user_message":
			return single(KindUser, payload.Message, raw.Timestamp)
		case "agent_message":
			return single(KindAssistant, payload.Message, raw.Timestamp)
		case "agent_reasoning":
			// Surfaced reasoning, not the encrypted reasoning Codex withholds.
			return single(KindThinking, payload.Text, raw.Timestamp)
		case "item_completed":
			// The desktop app carries the same conversation in a different
			// shape. Only the two turn kinds are read; command execution, file
			// changes and reasoning items stay dropped, so raw tool output and
			// withheld reasoning still never become content.
			if payload.Item == nil {
				return nil
			}
			switch payload.Item.Type {
			case "UserMessage":
				return single(KindUser, itemText(payload.Item.Content), raw.Timestamp)
			case "AgentMessage":
				return single(KindAssistant, itemText(payload.Item.Content), raw.Timestamp)
			}
		}
		return nil
	case "response_item":
		switch payload.Type {
		case "function_call", "custom_tool_call":
			// A tool name is coordination metadata; its arguments are not.
			if tool := bounded(payload.Name, 64); tool != "" {
				return []Message{{Kind: KindTool, Tool: tool, At: raw.Timestamp}}
			}
		case "message":
			// Only the operating instructions; user and assistant turns come
			// from the event stream so injected context is never mistaken for
			// something a person wrote.
			if payload.Role == "developer" {
				parts := make([]string, 0, len(payload.Content))
				for _, part := range payload.Content {
					if part.Type == "input_text" && strings.TrimSpace(part.Text) != "" {
						parts = append(parts, part.Text)
					}
				}
				return single(KindSystem, strings.Join(parts, "\n\n"), raw.Timestamp)
			}
		}
		// reasoning, function_call_output, custom_tool_call_output and every
		// unknown item are dropped: encrypted reasoning and raw tool output must
		// never become content.
		return nil
	}
	return nil
}

// itemText joins the text parts of a desktop-app item, ignoring every other
// part type so non-text attachments never become content. The desktop app is
// inconsistent about the part-type case: `UserMessage` writes "text" while
// `AgentMessage` writes "Text", so the comparison folds case.
func itemText(parts []codexPart) string {
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.EqualFold(part.Type, "text") && strings.TrimSpace(part.Text) != "" {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, "\n\n")
}

func single(kind, text, at string) []Message {
	if body := bounded(text, MaxMessageBytes); body != "" {
		return []Message{{Kind: kind, Text: body, At: at}}
	}
	return nil
}

// LocateCodexRollout finds the rollout file for a Codex session. Codex does not
// pass a transcript path to its hooks, but it names every rollout after the
// session id, so the file is discoverable from the id the hook already sends.
// The id is used locally for this lookup only and is never published.
func LocateCodexRollout(home, sessionID string) string {
	if home == "" || !validCodexSessionID(sessionID) {
		return ""
	}
	suffix := "-" + sessionID + ".jsonl"
	for _, base := range []string{filepath.Join(home, ".codex", "sessions"), filepath.Join(home, ".codex", "archived_sessions")} {
		found := ""
		// Rollouts are nested under year/month/day; stop at the first match.
		_ = filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if found != "" {
				return filepath.SkipAll
			}
			if !entry.IsDir() && strings.HasPrefix(entry.Name(), "rollout-") && strings.HasSuffix(entry.Name(), suffix) {
				found = path
				return filepath.SkipAll
			}
			return nil
		})
		if found != "" {
			return found
		}
	}
	return ""
}

// validCodexSessionID accepts only a plain UUID so the id can never introduce a
// path separator or glob character into the lookup.
func validCodexSessionID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, character := range value {
		switch index {
		case 8, 13, 18, 23:
			if character != '-' {
				return false
			}
		default:
			isDigit := character >= '0' && character <= '9'
			isLower := character >= 'a' && character <= 'f'
			isUpper := character >= 'A' && character <= 'F'
			if !isDigit && !isLower && !isUpper {
				return false
			}
		}
	}
	return true
}
