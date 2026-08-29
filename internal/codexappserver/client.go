// Package codexappserver speaks the Codex app-server JSON-RPC protocol to a
// private stdio child process.
//
// Codex refuses to run a non-managed lifecycle hook until the exact hook
// definition has been reviewed and trusted, recording that decision as a
// content hash under `hooks.state` in the user's `config.toml`. A hook that has
// never been trusted is parsed, listed, and silently skipped, so an installer
// that only writes `hooks.json` reports success for a binding that can never
// fire.
//
// The hash is derived from a normalized hook identity, not from the bytes on
// disk — Codex clamps a SessionEnd timeout before hashing, for one — so
// reproducing it locally would be wrong in ways that only surface as silence.
// This package asks Codex for the value instead: `hooks/list` returns each
// hook's `key`, `currentHash`, and `trustStatus`, and `config/batchWrite`
// persists trust through Codex's own writer under an optimistic-concurrency
// version. Nothing here parses or rewrites `config.toml`.
//
// The app-server CLI surface is marked experimental. Every call here is
// best-effort by contract: callers degrade to reporting an untrusted binding
// rather than failing setup (ADR-051).
package codexappserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ErrCodexNotFound reports that no Codex executable could be located. It is an
// ordinary condition on a machine that has never installed Codex, not a fault.
var ErrCodexNotFound = errors.New("no Codex executable found")

// Trust states reported by hooks/list.
const (
	TrustManaged   = "managed"
	TrustTrusted   = "trusted"
	TrustUntrusted = "untrusted"
	TrustModified  = "modified"
)

const (
	maxLineBytes    = 8 << 20
	maxSkippedLines = 2048
	stderrCaptured  = 4 << 10
)

// Hook is one entry from hooks/list. Only the fields Stickguy acts on are
// decoded; the protocol carries more and may add more.
type Hook struct {
	Key         string `json:"key"`
	EventName   string `json:"eventName"`
	HandlerType string `json:"handlerType"`
	Command     string `json:"command"`
	Source      string `json:"source"`
	SourcePath  string `json:"sourcePath"`
	IsManaged   bool   `json:"isManaged"`
	Enabled     bool   `json:"enabled"`
	CurrentHash string `json:"currentHash"`
	TrustStatus string `json:"trustStatus"`
}

// Trusted reports whether Codex will run this hook as configured. A managed
// hook is trusted by policy and must never be rewritten by Stickguy.
func (h Hook) Trusted() bool {
	return h.TrustStatus == TrustTrusted || h.TrustStatus == TrustManaged
}

// Client owns one `codex app-server` child process. It is not safe for
// concurrent use; calls are serialized by the caller or by mu.
type Client struct {
	command *exec.Cmd
	stdin   *bufio.Writer
	closer  func() error
	lines   chan []byte
	readErr chan error
	stderr  *boundedBuffer

	mu     sync.Mutex
	nextID int
	closed bool
}

// Home resolves the Codex home directory the same way Codex does. An explicit
// override wins so tests never touch the contributor's real Codex state.
func Home(override string) (string, error) {
	if override != "" {
		return filepath.Abs(override)
	}
	if env := os.Getenv("CODEX_HOME"); env != "" {
		return filepath.Abs(env)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	return filepath.Join(home, ".codex"), nil
}

// Locate finds a Codex executable. Codex ships both as a standalone CLI on
// PATH and bundled inside the desktop application, and only the bundle is
// present on a machine that installed Codex through ChatGPT.app.
func Locate() (string, error) {
	if override := strings.TrimSpace(os.Getenv("STICKGUY_CODEX_EXECUTABLE")); override != "" {
		if executableFile(override) {
			return filepath.Abs(override)
		}
		return "", fmt.Errorf("%w: STICKGUY_CODEX_EXECUTABLE is not executable", ErrCodexNotFound)
	}
	if path, err := exec.LookPath("codex"); err == nil {
		return path, nil
	}
	for _, candidate := range candidatePaths() {
		if executableFile(candidate) {
			return candidate, nil
		}
	}
	return "", ErrCodexNotFound
}

func candidatePaths() []string {
	if runtime.GOOS != "darwin" {
		return nil
	}
	paths := []string{
		"/Applications/ChatGPT.app/Contents/Resources/codex",
		"/opt/homebrew/bin/codex",
		"/usr/local/bin/codex",
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths,
			filepath.Join(home, "Applications", "ChatGPT.app", "Contents", "Resources", "codex"),
			filepath.Join(home, ".codex", "bin", "codex"),
		)
	}
	return paths
}

func executableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

// Options configure a Dial. Executable and CodexHome are resolved when empty.
type Options struct {
	Executable string
	CodexHome  string
	// ClientVersion is reported to Codex during initialize.
	ClientVersion string
}

// Dial starts a private app-server and completes the initialize handshake.
//
// This deliberately spawns its own stdio child rather than attaching to a
// shared daemon: the Codex desktop application runs its app-server as a private
// stdio child with no socket to join, and the shared daemon requires the
// standalone Codex installer. A private child observes nothing of the user's
// running sessions and only reads and writes configuration.
func Dial(ctx context.Context, options Options) (*Client, error) {
	executable := options.Executable
	if executable == "" {
		located, err := Locate()
		if err != nil {
			return nil, err
		}
		executable = located
	}
	command := exec.CommandContext(ctx, executable, "app-server")
	command.Env = os.Environ()
	if options.CodexHome != "" {
		home, err := filepath.Abs(options.CodexHome)
		if err != nil {
			return nil, fmt.Errorf("resolve Codex home: %w", err)
		}
		command.Env = append(command.Env, "CODEX_HOME="+home)
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open Codex app-server stdin: %w", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open Codex app-server stdout: %w", err)
	}
	captured := &boundedBuffer{limit: stderrCaptured}
	command.Stderr = captured
	if err = command.Start(); err != nil {
		return nil, fmt.Errorf("start Codex app-server: %w", err)
	}
	client := &Client{
		command: command,
		stdin:   bufio.NewWriter(stdin),
		closer:  stdin.Close,
		lines:   make(chan []byte, 16),
		readErr: make(chan error, 1),
		stderr:  captured,
		nextID:  1,
	}
	go client.read(stdout)

	version := options.ClientVersion
	if version == "" {
		version = "0.0.0"
	}
	initialize := map[string]any{"clientInfo": map[string]any{"name": "stickguy", "version": version}}
	if err = client.call(ctx, "initialize", initialize, nil); err != nil {
		client.Close()
		return nil, err
	}
	if err = client.notify("initialized", map[string]any{}); err != nil {
		client.Close()
		return nil, err
	}
	return client, nil
}

// Close shuts the child process down. It is safe to call more than once.
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	_ = c.closer()
	done := make(chan error, 1)
	go func() { done <- c.command.Wait() }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = c.command.Process.Kill()
		<-done
	}
	return nil
}

// ListHooks returns every hook Codex resolves for the given working
// directories. An empty cwds list asks Codex for its own default.
func (c *Client) ListHooks(ctx context.Context, cwds []string) ([]Hook, error) {
	if cwds == nil {
		cwds = []string{}
	}
	var response struct {
		Data []struct {
			CWD   string `json:"cwd"`
			Hooks []Hook `json:"hooks"`
		} `json:"data"`
	}
	if err := c.call(ctx, "hooks/list", map[string]any{"cwds": cwds}, &response); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var hooks []Hook
	for _, entry := range response.Data {
		for _, hook := range entry.Hooks {
			// The same user-level hook is reported once per requested cwd.
			if hook.Key == "" || seen[hook.Key] {
				continue
			}
			seen[hook.Key] = true
			hooks = append(hooks, hook)
		}
	}
	return hooks, nil
}

// UserConfigVersion returns the version of the user config layer, used as the
// optimistic-concurrency token for a write. An empty result means the layer was
// not reported and the write must proceed without a compare-and-swap.
func (c *Client) UserConfigVersion(ctx context.Context) (string, error) {
	var response struct {
		Origins map[string]struct {
			Name    map[string]any `json:"name"`
			Version string         `json:"version"`
		} `json:"origins"`
	}
	if err := c.call(ctx, "config/read", map[string]any{"includeLayers": true}, &response); err != nil {
		return "", err
	}
	for _, origin := range response.Origins {
		if kind, _ := origin.Name["type"].(string); kind == "user" {
			return origin.Version, nil
		}
	}
	return "", nil
}

// TrustEdit names one hook whose current hash should be recorded as trusted.
type TrustEdit struct {
	Key  string
	Hash string
}

// Trust records the given hooks as trusted in the user's config.toml through
// Codex's own configuration writer.
//
// Every edit is a narrow upsert of a single `hooks.state."<key>".trusted_hash`
// value. Stickguy never serializes the surrounding document, so a concurrent
// write by the Codex desktop application cannot be clobbered by this call, and
// expectedVersion turns a lost update into a returned error rather than
// silent damage.
func (c *Client) Trust(ctx context.Context, edits []TrustEdit, expectedVersion string) error {
	if len(edits) == 0 {
		return nil
	}
	encoded := make([]map[string]any, 0, len(edits))
	for _, edit := range edits {
		if edit.Key == "" || edit.Hash == "" {
			return errors.New("hook trust edit is missing a key or hash")
		}
		if strings.ContainsAny(edit.Key, "\"\\\r\n\x00") || strings.ContainsAny(edit.Hash, "\r\n\x00") {
			return errors.New("hook trust edit contains unsupported characters")
		}
		encoded = append(encoded, map[string]any{
			"keyPath":       fmt.Sprintf("hooks.state.%q.trusted_hash", edit.Key),
			"mergeStrategy": "upsert",
			"value":         edit.Hash,
		})
	}
	parameters := map[string]any{"edits": encoded, "reloadUserConfig": true}
	if expectedVersion != "" {
		parameters["expectedVersion"] = expectedVersion
	}
	return c.call(ctx, "config/batchWrite", parameters, nil)
}

func (c *Client) call(ctx context.Context, method string, params any, result any) error {
	c.mu.Lock()
	id := c.nextID
	c.nextID++
	c.mu.Unlock()

	request := map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}
	if err := c.write(request); err != nil {
		return fmt.Errorf("send Codex app-server %s: %w", method, err)
	}
	for skipped := 0; skipped < maxSkippedLines; skipped++ {
		line, err := c.next(ctx)
		if err != nil {
			return fmt.Errorf("read Codex app-server %s response: %w%s", method, err, c.stderrSuffix())
		}
		var envelope struct {
			ID     *json.Number    `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(line, &envelope) != nil || envelope.ID == nil {
			// Notifications and server-initiated requests share the stream.
			continue
		}
		if envelope.ID.String() != fmt.Sprint(id) {
			continue
		}
		if envelope.Error != nil {
			return fmt.Errorf("Codex app-server %s failed: %s (code %d)", method, envelope.Error.Message, envelope.Error.Code)
		}
		if result == nil || len(envelope.Result) == 0 {
			return nil
		}
		if err = json.Unmarshal(envelope.Result, result); err != nil {
			return fmt.Errorf("decode Codex app-server %s response: %w", method, err)
		}
		return nil
	}
	return fmt.Errorf("Codex app-server %s response was not delivered", method)
}

func (c *Client) notify(method string, params any) error {
	if err := c.write(map[string]any{"jsonrpc": "2.0", "method": method, "params": params}); err != nil {
		return fmt.Errorf("send Codex app-server %s: %w", method, err)
	}
	return nil
}

func (c *Client) write(message map[string]any) error {
	encoded, err := json.Marshal(message)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err = c.stdin.Write(append(encoded, '\n')); err != nil {
		return err
	}
	return c.stdin.Flush()
}

func (c *Client) next(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-c.readErr:
		return nil, err
	case line, ok := <-c.lines:
		if !ok {
			return nil, errors.New("Codex app-server closed its output stream")
		}
		return line, nil
	}
}

func (c *Client) read(stdout io.Reader) {
	defer close(c.lines)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64<<10), maxLineBytes)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		c.lines <- line
	}
	if err := scanner.Err(); err != nil {
		select {
		case c.readErr <- err:
		default:
		}
	}
}

func (c *Client) stderrSuffix() string {
	captured := strings.TrimSpace(c.stderr.String())
	if captured == "" {
		return ""
	}
	return "; Codex reported: " + captured
}

// boundedBuffer keeps the first bytes written to it and discards the rest, so a
// chatty child process cannot grow this in memory without bound.
type boundedBuffer struct {
	mu     sync.Mutex
	limit  int
	buffer bytes.Buffer
}

func (b *boundedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if remaining := b.limit - b.buffer.Len(); remaining > 0 {
		kept := data
		if len(kept) > remaining {
			kept = kept[:remaining]
		}
		b.buffer.Write(kept)
	}
	// Report the full length: discarding overflow is this buffer's purpose, not
	// a short write the child process should see as an error.
	return len(data), nil
}

func (b *boundedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}
