package codexappserver

import (
	"context"
	"testing"
	"time"
)

// threadCodex answers thread/read with the shape the installed app-server
// actually returns: turns carrying items, a commandExecution discriminated by
// `type`, and Codex's own best-effort commandActions. It deliberately includes
// the cases that must not become read evidence.
const threadCodex = `#!/usr/bin/env python3
import json, sys

THREAD = {
  "id": "01a04ac6-684c-7650-a8b4-311eb918f98a",
  "cwd": "/repo",
  "turns": [{"items": [
    {"id": "item_1", "type": "commandExecution", "status": "completed",
     "command": "sed -n 1,40p /repo/internal/session/rotate.go",
     "aggregatedOutput": "package session\nfunc Rotate() {}",
     "commandActions": [{"type": "read", "name": "rotate.go", "path": "/repo/internal/session/rotate.go"}]},
    {"id": "item_2", "type": "commandExecution", "status": "failed",
     "command": "cat /repo/internal/session/missing.go",
     "aggregatedOutput": "no such file",
     "commandActions": [{"type": "read", "name": "missing.go", "path": "/repo/internal/session/missing.go"}]},
    {"id": "item_3", "type": "commandExecution", "status": "completed",
     "command": "ls /repo && rg needle",
     "aggregatedOutput": "...",
     "commandActions": [{"type": "listFiles", "path": "/repo"},
                        {"type": "search", "query": "needle", "path": "/repo"},
                        {"type": "unknown"}]},
    {"id": "item_4", "type": "commandExecution", "status": "completed",
     "command": "cat ../outside/escape.go",
     "aggregatedOutput": "...",
     "commandActions": [{"type": "read", "name": "escape.go", "path": "/outside/escape.go"}]},
    {"id": "item_5", "type": "reasoning", "status": "completed"}
  ]}]
}

for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    message = json.loads(line)
    method, identifier = message.get("method"), message.get("id")
    if identifier is None:
        continue
    if method == "initialize":
        result = {"userAgent": "fake"}
    elif method == "thread/read":
        if message["params"]["threadId"] != THREAD["id"]:
            print(json.dumps({"jsonrpc": "2.0", "id": identifier,
                              "error": {"code": -32602, "message": "no such thread"}}), flush=True)
            continue
        result = {"thread": THREAD}
    else:
        print(json.dumps({"jsonrpc": "2.0", "id": identifier,
                          "error": {"code": -32601, "message": "unknown method"}}), flush=True)
        continue
    print(json.dumps({"jsonrpc": "2.0", "id": identifier, "result": result}), flush=True)
`

func TestThreadReadsKeepsOnlyCompletedReadActions(t *testing.T) {
	executable, _ := fakeCodex(t, threadCodex)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := Dial(ctx, Options{Executable: executable, CodexHome: t.TempDir(), ClientVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	cwd, reads, err := client.ThreadReads(ctx, "01a04ac6-684c-7650-a8b4-311eb918f98a")
	if err != nil {
		t.Fatal(err)
	}
	if cwd != "/repo" {
		t.Fatalf("cwd=%q", cwd)
	}
	// A failed command proves nothing was read; listFiles and search do not show
	// that any particular file's contract was examined; an unknown variant is
	// not guessed at. Only the completed read survives, and the out-of-repo one
	// is left for the caller's containment check.
	want := []ThreadRead{
		{ItemID: "item_1", Path: "/repo/internal/session/rotate.go"},
		{ItemID: "item_4", Path: "/outside/escape.go"},
	}
	if len(reads) != len(want) {
		t.Fatalf("reads=%#v, want %#v", reads, want)
	}
	for i, read := range reads {
		if read != want[i] {
			t.Fatalf("reads[%d]=%#v, want %#v", i, read, want[i])
		}
	}
}

func TestThreadReadsRejectsAThreadIdItWillNotSend(t *testing.T) {
	executable, _ := fakeCodex(t, threadCodex)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := Dial(ctx, Options{Executable: executable, CodexHome: t.TempDir(), ClientVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	for _, id := range []string{"", "../../etc/passwd", "01a04ac6684c7650a8b4311eb918f98a", "not-a-uuid"} {
		if _, _, err := client.ThreadReads(ctx, id); err == nil {
			t.Fatalf("thread id %q was accepted", id)
		}
	}
}
