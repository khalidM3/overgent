// Package localbackend supervises the open-source Convex backend that a
// local-mode Project runs on loopback.
//
// The coordination engine lives in Convex functions (ADR-072), so a Project
// that never leaves the Mac still needs a backend speaking the same /v1
// contract as Overgent Cloud. This package starts the bundled binary, deploys
// the release-time function bundle to it, and keeps it alive while the service
// runs. Everything above the wire is unchanged.
package localbackend

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/khalidM3/overgent/internal/config"
)

// convexClientVersion is the npm Convex CLI whose deploy2 request shape the
// replay below reproduces. The deploy2 endpoints are internal to Convex rather
// than a promised API (Lane 01 §2), so this constant, the backend release in
// scripts/backend-version.json, and the recorded push payload are one pin:
// changing any of them means regenerating the payload and re-running the
// release replay check. A test asserts this equals the manifest's cliVersion.
const convexClientVersion = "npm-cli-1.45.0"

const (
	// healthBudget is the cold-start allowance. Lane 01 measured 120 ms on a
	// new database; ten seconds is that number plus room for a cold page cache
	// and a Mac doing something else at login.
	defaultHealthBudget   = 10 * time.Second
	defaultHealthInterval = 100 * time.Millisecond
	// logCap rotates backend.log rather than letting a crash loop fill the disk.
	logCap = 5 << 20
	// restartWindow bounds "consecutive" for the restart limit: five failures
	// inside five minutes is a backend that is not going to start.
	restartWindow = 5 * time.Minute
	restartLimit  = 5
)

// portMovedError is what a member is told when the backend could not reclaim
// the port its Projects name. It is a standing condition, not a transient one.
const portMovedError = "backend came back on a new port; existing local Projects need overgent backend reset"

// Keychain accounts. The instance secret is per-install and the secrets key is
// the deployment secret Lane 04 reads from the deployment, never from Go.
const (
	instanceAccountPrefix = "overgent.local-backend."
	secretsKeyAccount     = "overgent.local-backend.secrets-key"
)

// CredentialStore is the macOS Keychain, or a fake in tests. It mirrors
// onboarding.CredentialStore rather than importing it, so neither package
// depends on the other.
type CredentialStore interface {
	Put(context.Context, string, string) error
	Get(context.Context, string) (string, error)
	Delete(context.Context, string) error
}

// Endpoint is where a client reaches this backend. Origin serves the admin and
// deploy2 routes; SiteOrigin serves the Convex HTTP actions, which is where
// Overgent's own /v1 contract lives, so that is the API base URL a local
// Project is created against.
type Endpoint struct {
	Origin     string `json:"origin"`
	SiteOrigin string `json:"siteOrigin"`
}

// State is <root>/backend/backend.json. It is deliberately a sibling of
// config.json, not a field inside it, so Lane 06 can reshape config.json
// without touching backend state (migration README rule 3).
type State struct {
	Version        string `json:"version"`
	BundleRevision string `json:"bundleRevision"`
	Port           int    `json:"port"`
	SitePort       int    `json:"sitePort"`
	InstanceName   string `json:"instanceName"`
	BinaryPath     string `json:"binaryPath"`
	BundlePath     string `json:"bundlePath"`
	PID            int    `json:"pid"`
}

// Status is what `health` and `overgent backend status` report. InstanceName is
// absent: it names the Keychain item holding the instance secret, and
// diagnostics carries backend.json minus that field.
type Status struct {
	Running        bool   `json:"running"`
	PID            int    `json:"pid,omitempty"`
	Port           int    `json:"port,omitempty"`
	SitePort       int    `json:"sitePort,omitempty"`
	Origin         string `json:"origin,omitempty"`
	SiteOrigin     string `json:"siteOrigin,omitempty"`
	Version        string `json:"version,omitempty"`
	BundleRevision string `json:"bundleRevision,omitempty"`
	DatabasePath   string `json:"databasePath,omitempty"`
	DatabaseBytes  int64  `json:"databaseBytes,omitempty"`
	LastError      string `json:"lastError,omitempty"`
	IdleSince      string `json:"idleSince,omitempty"`
}

// Manager owns one profile's backend process.
type Manager struct {
	root      string
	directory string
	statePath string
	dbPath    string
	storage   string
	logPath   string
	creds     CredentialStore
	logger    *slog.Logger

	// Test seams. Production keeps the defaults; unit tests shorten the health
	// budget and the backoff so a supervision test is not a wall-clock test.
	now            func() time.Time
	healthBudget   time.Duration
	healthInterval time.Duration
	restartBackoff func(attempt int) time.Duration
	// idleTimeout stops the backend after this long without activity. Lane 01
	// measured 56 MB idle RSS, well under the 300 MB threshold in the brief, so
	// production leaves this at zero: the backend runs while the service runs.
	idleTimeout time.Duration

	mu        sync.Mutex
	state     State
	command   *exec.Cmd
	adopted   int
	lastError string
	idleSince time.Time
	failures  int
	firstFail time.Time
	stopped   bool
	// portMoved records that this profile could not have the port its existing
	// Projects were created against.
	portMoved bool
	// watching is whether a supervisor goroutine is running. One manager has at
	// most one, and it exits when the backend is stopped or given up on.
	watching bool
}

// New opens the manager for a profile root. It does not start anything.
func New(root string, creds CredentialStore, logger *slog.Logger) (*Manager, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("local backend requires a profile root")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve profile root: %w", err)
	}
	if logger == nil {
		logger = slog.Default()
	}
	directory := filepath.Join(absolute, "backend")
	manager := &Manager{
		root:           absolute,
		directory:      directory,
		statePath:      filepath.Join(directory, "backend.json"),
		dbPath:         filepath.Join(directory, "state.sqlite3"),
		storage:        filepath.Join(directory, "storage"),
		logPath:        filepath.Join(directory, "backend.log"),
		creds:          creds,
		logger:         logger,
		now:            time.Now,
		healthBudget:   defaultHealthBudget,
		healthInterval: defaultHealthInterval,
		restartBackoff: exponentialBackoff,
	}
	state, err := manager.load()
	if err != nil {
		return nil, err
	}
	manager.state = state
	return manager, nil
}

// Configured reports whether this profile has a backend to manage at all. A
// team-mode profile has no backend.json and must never start one.
func Configured(root string) bool {
	if strings.TrimSpace(root) == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(root, "backend", "backend.json"))
	return err == nil && info.Mode().IsRegular()
}

// IsLoopbackOrigin reports whether this origin is served by a backend running
// on this Mac. The rule itself now lives in internal/config, because that is
// where a Project's backend kind is decided when the binding is written; this
// stays as the name every existing caller already asks it by.
func IsLoopbackOrigin(origin string) bool { return config.IsLoopbackOrigin(origin) }

// StatePath is where Install and the desktop write the bundle paths.
func StatePath(root string) string { return filepath.Join(root, "backend", "backend.json") }

// Install records where the backend binary and the release-time deploy payload
// live. The desktop calls it on every launch so an app update moves the paths;
// a CLI-only install calls `overgent backend install`.
func Install(root, binaryPath, bundlePath string) error {
	manager, err := New(root, nil, slog.Default())
	if err != nil {
		return err
	}
	return manager.SetArtifacts(binaryPath, bundlePath)
}

// SetArtifacts validates and persists the two absolute artifact paths.
func (m *Manager) SetArtifacts(binaryPath, bundlePath string) error {
	binary, err := existingFile(binaryPath, "backend binary")
	if err != nil {
		return err
	}
	bundle, err := existingFile(bundlePath, "backend deploy payload")
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.state
	state.BinaryPath = binary
	state.BundlePath = bundle
	if state.InstanceName == "" {
		name, nameErr := newInstanceName()
		if nameErr != nil {
			return nameErr
		}
		state.InstanceName = name
	}
	m.state = state
	return m.save(state)
}

func existingFile(path, what string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("%s path is required", what)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", what, err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", what, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", what)
	}
	return absolute, nil
}

// Touch marks coordination activity. It feeds the idle timer only; the backend
// is otherwise kept running for as long as the service runs.
func (m *Manager) Touch() {
	m.mu.Lock()
	m.idleSince = m.now()
	m.mu.Unlock()
}

func (m *Manager) load() (State, error) {
	body, err := os.ReadFile(m.statePath)
	if os.IsNotExist(err) {
		return State{}, nil
	}
	if err != nil {
		return State{}, fmt.Errorf("read backend state: %w", err)
	}
	var state State
	if err = json.Unmarshal(body, &state); err != nil {
		return State{}, fmt.Errorf("decode backend state: %w", err)
	}
	return state, nil
}

func (m *Manager) save(state State) error {
	if err := os.MkdirAll(m.directory, 0o700); err != nil {
		return fmt.Errorf("create backend directory: %w", err)
	}
	if err := os.Chmod(m.directory, 0o700); err != nil {
		return fmt.Errorf("secure backend directory: %w", err)
	}
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode backend state: %w", err)
	}
	temporary := m.statePath + ".tmp"
	if err = os.WriteFile(temporary, append(body, '\n'), 0o600); err != nil {
		return fmt.Errorf("write backend state: %w", err)
	}
	if err = os.Rename(temporary, m.statePath); err != nil {
		return fmt.Errorf("replace backend state: %w", err)
	}
	return os.Chmod(m.statePath, 0o600)
}

func newInstanceName() (string, error) {
	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		return "", fmt.Errorf("generate backend instance name: %w", err)
	}
	// Convex instance names are lowercase and dash-separated; keep the shape
	// the CLI produces so a member reading a log recognizes it.
	return "overgent-local-" + hex.EncodeToString(suffix), nil
}

func randomHex(bytes int) (string, error) {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}

func exponentialBackoff(attempt int) time.Duration {
	delay := time.Second << attempt
	if delay > 60*time.Second || delay <= 0 {
		return 60 * time.Second
	}
	return delay
}

// freePorts reserves two loopback ports by binding and releasing them. There is
// an unavoidable race between release and the backend's own bind, which the
// caller resolves by retrying with a fresh pair rather than by holding the
// listeners open (the backend cannot bind a port this process still owns).
func freePorts() (int, int, error) {
	first, err := reservePort()
	if err != nil {
		return 0, 0, err
	}
	second, err := reservePort()
	if err != nil {
		return 0, 0, err
	}
	if first == second {
		return 0, 0, errors.New("loopback port reservation returned one port twice")
	}
	return first, second, nil
}

// available reports whether this profile can still have the port it used last
// time. Binding and releasing is the only honest test; asking the OS what is
// free answers a different question.
func available(port int) bool {
	if port <= 0 {
		return false
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	return listener.Close() == nil
}

func reservePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("reserve loopback port: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err = listener.Close(); err != nil {
		return 0, fmt.Errorf("release loopback port: %w", err)
	}
	return port, nil
}

// arguments is the exact argument array the backend is spawned with. It is a
// function so a test can assert the loopback bind without starting anything.
func arguments(state State, instanceSecret, dbPath, storage string) []string {
	return []string{
		"--interface", "127.0.0.1",
		"--port", fmt.Sprint(state.Port),
		"--site-proxy-port", fmt.Sprint(state.SitePort),
		"--convex-origin", loopbackOrigin(state.Port),
		"--convex-site", loopbackOrigin(state.SitePort),
		"--instance-name", state.InstanceName,
		"--instance-secret", instanceSecret,
		"--local-storage", storage,
		// The backend is reachable only from this Mac and runs only Overgent's
		// own functions, whose outbound requests go to provider origins the
		// Project owner configured, so no SSRF-screening proxy is configured
		// (docs/security-privacy.md, "Local").
		"--disable-beacon",
		dbPath,
	}
}

func loopbackOrigin(port int) string  { return "http://" + loopbackAddress(port) }
func loopbackAddress(port int) string { return fmt.Sprintf("127.0.0.1:%d", port) }

// killStale terminates a backend this profile started and then lost track of.
// Only a live process that is this user's own convex-local-backend is signalled;
// a recycled pid belonging to something else is left alone.
func (m *Manager) killStale(pid int, binaryPath string) {
	if pid <= 0 || pid == os.Getpid() {
		return
	}
	if !processMatches(pid, filepath.Base(binaryPath)) {
		return
	}
	_ = syscall.Kill(pid, syscall.SIGTERM)
	deadline := m.now().Add(5 * time.Second)
	for m.now().Before(deadline) {
		if syscall.Kill(pid, 0) != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
}

// processMatches reports whether pid is a live process of this user running a
// command with this base name.
func processMatches(pid int, name string) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	out, err := exec.Command("/bin/ps", "-o", "uid=,comm=", "-p", fmt.Sprint(pid)).Output()
	if err != nil {
		return false
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) < 2 {
		return false
	}
	if fields[0] != fmt.Sprint(os.Getuid()) {
		return false
	}
	return filepath.Base(strings.Join(fields[1:], " ")) == name
}

func bundleRevision(path string) (string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read backend deploy payload: %w", err)
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:8]), nil
}

// rotateLog keeps backend.log bounded. One generation is enough: the log exists
// to explain a backend that will not start, and that evidence is in the newest
// lines.
func rotateLog(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < logCap {
		return
	}
	_ = os.Rename(path, path+".1")
}
