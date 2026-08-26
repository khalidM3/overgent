package sessiontranscript

import "encoding/json"

// claudeAdapter reads Claude Code's transcript, which writes one message record
// per line with the session's own fields at the top level.
type claudeAdapter struct{}

var claudeRecordTypes = map[string]bool{
	"user": true, "assistant": true, "ai-title": true, "custom-title": true,
}

type claudeLine struct {
	Type       string          `json:"type"`
	SessionID  string          `json:"sessionId"`
	Timestamp  string          `json:"timestamp"`
	GitBranch  string          `json:"gitBranch"`
	AITitle    string          `json:"aiTitle"`
	CustomText string          `json:"customTitle"`
	Message    json.RawMessage `json:"message"`
	// Present only on records that carry a tool result; never content.
	ToolUseResult json.RawMessage `json:"toolUseResult"`
}

type claudeMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type claudePart struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Thinking string `json:"thinking"`
	Name     string `json:"name"`
}

func (claudeAdapter) name() string { return "claude" }

func (claudeAdapter) meta(line []byte, session *Session) {
	var raw claudeLine
	if json.Unmarshal(line, &raw) != nil {
		return
	}
	if raw.SessionID != "" {
		session.SessionID = raw.SessionID
	}
	if raw.GitBranch != "" {
		session.Branch = raw.GitBranch
	}
	switch raw.Type {
	case "custom-title":
		// A member-set title always wins over the generated one.
		if title := bounded(raw.CustomText, 160); title != "" {
			session.Title = title
		}
	case "ai-title":
		if session.Title == "" {
			session.Title = bounded(raw.AITitle, 160)
		}
	}
}

func (claudeAdapter) messages(line []byte) []Message {
	var raw claudeLine
	if json.Unmarshal(line, &raw) != nil {
		return nil
	}
	if raw.Type != "user" && raw.Type != "assistant" {
		return nil
	}
	if len(raw.ToolUseResult) > 0 || len(raw.Message) == 0 {
		return nil
	}
	var message claudeMessage
	if json.Unmarshal(raw.Message, &message) != nil {
		return nil
	}
	role := KindUser
	if message.Role == "assistant" {
		role = KindAssistant
	}
	if text := ""; json.Unmarshal(message.Content, &text) == nil {
		if body := bounded(text, MaxMessageBytes); body != "" {
			return []Message{{Kind: role, Text: body, At: raw.Timestamp}}
		}
		return nil
	}
	var parts []claudePart
	if json.Unmarshal(message.Content, &parts) != nil {
		return nil
	}
	out := make([]Message, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case "text":
			if body := bounded(part.Text, MaxMessageBytes); body != "" {
				out = append(out, Message{Kind: role, Text: body, At: raw.Timestamp})
			}
		case "thinking":
			if body := bounded(part.Thinking, MaxMessageBytes); body != "" {
				out = append(out, Message{Kind: KindThinking, Text: body, At: raw.Timestamp})
			}
		case "tool_use":
			// The tool name is coordination metadata; its input is not.
			if tool := bounded(part.Name, 64); tool != "" {
				out = append(out, Message{Kind: KindTool, Tool: tool, At: raw.Timestamp})
			}
		default:
			// tool_result and any unknown part type are dropped entirely.
		}
	}
	return out
}
