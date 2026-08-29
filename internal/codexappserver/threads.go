package codexappserver

import (
	"context"
	"errors"
	"regexp"
)

// ThreadRead is one file read that Codex's own classifier attributed to a
// command it ran. It is vendor-inferred evidence, not an observation of the
// filesystem: OpenAI describes command actions as a best-effort understanding
// of what a command will do, and a compound command that genuinely reads files
// can still classify as `unknown` (ADR-052).
type ThreadRead struct {
	// ItemID is the app-server item this read came from. Callers deduplicate on
	// it so re-reading a thread republishes nothing.
	ItemID string
	// Path is absolute, as Codex reports it. It is meaningless until the caller
	// has checked it against a registered repository root.
	Path string
}

// maxThreadReads bounds one thread read. A long session accumulates hundreds of
// commands, and a read set is not improved by an unbounded tail.
const maxThreadReads = 512

// threadID is the id Codex uses for a task, which is also the session id its
// hooks report. It is validated before it is sent so nothing else can be
// smuggled into a request.
var threadID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ThreadReads returns the working directory of a stored Codex task and the file
// reads its own classifier attributed to completed commands.
//
// This reads a task without resuming or taking ownership of it: `thread/read`
// is documented as reading a stored task by id, and a separate app-server
// process observing a task the desktop application is still running reports it
// as `notLoaded`. Stickguy never issues `thread/start`, `thread/resume`,
// `turn/start`, or any approval, so it observes the member's session without
// participating in it (ADR-051, ADR-052).
//
// The decoded shape deliberately has no field for `command` or
// `aggregatedOutput`. Those cross the wire from Codex and are dropped during
// decoding rather than held and discarded later, so a raw command string or
// captured output never reaches a Stickguy structure at all.
func (c *Client) ThreadReads(ctx context.Context, id string) (cwd string, reads []ThreadRead, err error) {
	if !threadID.MatchString(id) {
		return "", nil, errors.New("codex thread id is invalid")
	}
	var result struct {
		Thread struct {
			ID    string `json:"id"`
			CWD   string `json:"cwd"`
			Turns []struct {
				Items []struct {
					ID     string `json:"id"`
					Type   string `json:"type"`
					Status string `json:"status"`
					// Only the classification is decoded, never the command it
					// classified.
					CommandActions []struct {
						Type string `json:"type"`
						Path string `json:"path"`
					} `json:"commandActions"`
				} `json:"items"`
			} `json:"turns"`
		} `json:"thread"`
	}
	if err = c.call(ctx, "thread/read", map[string]any{"threadId": id, "includeTurns": true}, &result); err != nil {
		return "", nil, err
	}
	for _, turn := range result.Thread.Turns {
		for _, item := range turn.Items {
			// A failed command may not have read what its classification says,
			// and an in-flight one can still be reclassified.
			if item.Type != "commandExecution" || item.Status != "completed" || item.ID == "" {
				continue
			}
			for _, action := range item.CommandActions {
				// `listFiles` and `search` are not read evidence: neither shows
				// that a particular file's contract was examined. An unknown
				// variant is ignored rather than guessed at.
				if action.Type != "read" || action.Path == "" {
					continue
				}
				if len(reads) >= maxThreadReads {
					return result.Thread.CWD, reads, nil
				}
				reads = append(reads, ThreadRead{ItemID: item.ID, Path: action.Path})
			}
		}
	}
	return result.Thread.CWD, reads, nil
}
