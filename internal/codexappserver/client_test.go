package codexappserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeCodex writes an executable that speaks enough of the app-server protocol
// to exercise this client: a handshake, a hooks listing that flips to trusted
// once a write lands, and a configuration write that records what it received.
// Real Codex is never started by a unit test.
func fakeCodex(t *testing.T, script string) (string, string) {
	t.Helper()
	directory := t.TempDir()
	recorded := filepath.Join(directory, "written.json")
	path := filepath.Join(directory, "codex")
	body := strings.ReplaceAll(script, "@@RECORDED@@", recorded)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path, recorded
}

const cooperativeCodex = `#!/usr/bin/env python3
import json, os, sys
RECORDED = "@@RECORDED@@"

def hook(event, key, trusted):
    return {"key": key, "eventName": event, "handlerType": "command",
            "command": "'sg' agent-hook --vendor codex", "matcher": None,
            "sourcePath": "/codex/hooks.json", "source": "user", "pluginId": None,
            "displayOrder": 0, "enabled": True, "isManaged": False,
            "currentHash": "sha256:" + key, "trustStatus": "trusted" if trusted else "untrusted"}

for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    message = json.loads(line)
    method, identifier = message.get("method"), message.get("id")
    if identifier is None:
        continue
    # An unsolicited notification shares the stream and must be skipped.
    print(json.dumps({"jsonrpc": "2.0", "method": "thread/started", "params": {}}), flush=True)
    if method == "initialize":
        result = {"userAgent": "fake"}
    elif method == "hooks/list":
        trusted = os.path.exists(RECORDED)
        entries = [{"cwd": cwd, "hooks": [hook("sessionStart", "k1", trusted), hook("stop", "k2", trusted)]}
                   for cwd in (message["params"]["cwds"] or ["/default"])]
        result = {"data": entries}
    elif method == "config/read":
        result = {"config": {}, "origins": {"user": {"name": {"type": "user", "file": "/codex/config.toml"}, "version": "v7"}}}
    elif method == "config/batchWrite":
        open(RECORDED, "w").write(json.dumps(message["params"]))
        result = {"filePath": "/codex/config.toml", "status": "ok", "version": "v8"}
    else:
        print(json.dumps({"jsonrpc": "2.0", "id": identifier,
                          "error": {"code": -32601, "message": "unknown method " + str(method)}}), flush=True)
        continue
    print(json.dumps({"jsonrpc": "2.0", "id": identifier, "result": result}), flush=True)
`

func TestListHooksDeduplicatesAcrossWorkingDirectoriesAndTrustWritesNarrowKeyPaths(t *testing.T) {
	executable, recorded := fakeCodex(t, cooperativeCodex)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := Dial(ctx, Options{Executable: executable, CodexHome: t.TempDir(), ClientVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	hooks, err := client.ListHooks(ctx, []string{"/one", "/two"})
	if err != nil {
		t.Fatal(err)
	}
	// The same user-level hook is reported once per requested directory; a
	// caller must not be asked to trust it twice.
	if len(hooks) != 2 {
		t.Fatalf("expected 2 deduplicated hooks, got %d: %#v", len(hooks), hooks)
	}
	for _, hook := range hooks {
		if hook.Trusted() {
			t.Fatalf("hook %s should start untrusted", hook.Key)
		}
	}
	version, err := client.UserConfigVersion(ctx)
	if err != nil || version != "v7" {
		t.Fatalf("version=%q err=%v", version, err)
	}
	edits := []TrustEdit{{Key: hooks[0].Key, Hash: hooks[0].CurrentHash}, {Key: hooks[1].Key, Hash: hooks[1].CurrentHash}}
	if err = client.Trust(ctx, edits, version); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(recorded)
	if err != nil {
		t.Fatal(err)
	}
	var parameters struct {
		Edits []struct {
			KeyPath       string `json:"keyPath"`
			MergeStrategy string `json:"mergeStrategy"`
			Value         string `json:"value"`
		} `json:"edits"`
		ExpectedVersion string `json:"expectedVersion"`
	}
	if err = json.Unmarshal(written, &parameters); err != nil {
		t.Fatal(err)
	}
	if parameters.ExpectedVersion != "v7" {
		t.Fatalf("write dropped its concurrency guard: %q", parameters.ExpectedVersion)
	}
	if len(parameters.Edits) != 2 {
		t.Fatalf("expected 2 edits, got %#v", parameters.Edits)
	}
	for _, edit := range parameters.Edits {
		// Every edit must address exactly one hook's trusted_hash. Anything
		// broader would let Stickguy replace configuration it does not own.
		if !strings.HasPrefix(edit.KeyPath, `hooks.state."`) || !strings.HasSuffix(edit.KeyPath, `".trusted_hash`) {
			t.Fatalf("edit is not a narrow trust key path: %q", edit.KeyPath)
		}
		if edit.MergeStrategy != "upsert" {
			t.Fatalf("edit must upsert, got %q", edit.MergeStrategy)
		}
	}
	after, err := client.ListHooks(ctx, []string{"/one"})
	if err != nil {
		t.Fatal(err)
	}
	for _, hook := range after {
		if !hook.Trusted() {
			t.Fatalf("hook %s still untrusted after write", hook.Key)
		}
	}
}

func TestCallSurfacesProtocolErrorsAndRejectsUnsafeEdits(t *testing.T) {
	executable, _ := fakeCodex(t, cooperativeCodex)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := Dial(ctx, Options{Executable: executable, CodexHome: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// An unknown method must surface as an error rather than hang or be read as
	// success; that distinction is what lets callers fall back honestly.
	if err = client.call(ctx, "does/notExist", map[string]any{}, nil); err == nil ||
		!strings.Contains(err.Error(), "unknown method") {
		t.Fatalf("expected a protocol error, got %v", err)
	}
	if err = client.Trust(ctx, []TrustEdit{{Key: "has\"quote", Hash: "sha256:x"}}, ""); err == nil {
		t.Fatal("a key that would escape its TOML string was accepted")
	}
	if err = client.Trust(ctx, []TrustEdit{{Key: "k", Hash: ""}}, ""); err == nil {
		t.Fatal("an empty hash was accepted")
	}
	if err = client.Trust(ctx, nil, ""); err != nil {
		t.Fatalf("an empty edit set should be a no-op: %v", err)
	}
}

func TestDialFailsCleanlyWhenCodexIsUnusable(t *testing.T) {
	executable, _ := fakeCodex(t, "#!/bin/sh\nexit 3\n")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := Dial(ctx, Options{Executable: executable, CodexHome: t.TempDir()}); err == nil {
		t.Fatal("dial to a failing Codex reported success")
	}
	if _, err := Dial(ctx, Options{Executable: filepath.Join(t.TempDir(), "absent"), CodexHome: t.TempDir()}); err == nil {
		t.Fatal("dial to a missing executable reported success")
	}
}

func TestHomeResolvesOverrideThenEnvironment(t *testing.T) {
	override := t.TempDir()
	if home, err := Home(override); err != nil || home != override {
		t.Fatalf("home=%q err=%v", home, err)
	}
	environment := t.TempDir()
	t.Setenv("CODEX_HOME", environment)
	if home, err := Home(""); err != nil || home != environment {
		t.Fatalf("home=%q err=%v", home, err)
	}
}
