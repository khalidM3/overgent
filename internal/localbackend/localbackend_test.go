package localbackend

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// fakeBinary builds the stand-in backend once per test binary. It is named
// convex-local-backend on purpose: the stale-process check matches on the
// command name, so a differently named helper would not exercise it.
var (
	fakeOnce sync.Once
	fakePath string
	fakeErr  error
)

func fakeBackend(t *testing.T) string {
	t.Helper()
	fakeOnce.Do(func() {
		directory, err := os.MkdirTemp("", "overgent-fake-backend-")
		if err != nil {
			fakeErr = err
			return
		}
		fakePath = filepath.Join(directory, "convex-local-backend")
		build := exec.Command("go", "build", "-o", fakePath, "./testdata/fakebackend")
		build.Stderr = os.Stderr
		fakeErr = build.Run()
	})
	if fakeErr != nil {
		t.Fatalf("build fake backend: %v", fakeErr)
	}
	return fakePath
}

// memoryCredentials is the Keychain's stand-in. Tests never touch the real one.
type memoryCredentials struct {
	mu     sync.Mutex
	values map[string]string
}

func newCredentials() *memoryCredentials { return &memoryCredentials{values: map[string]string{}} }

func (store *memoryCredentials) Put(_ context.Context, account, secret string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.values[account] = secret
	return nil
}

func (store *memoryCredentials) Get(_ context.Context, account string) (string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if secret, ok := store.values[account]; ok {
		return secret, nil
	}
	return "", os.ErrNotExist
}

func (store *memoryCredentials) Delete(_ context.Context, account string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	delete(store.values, account)
	return nil
}

// newManager returns a manager over a temp profile with the fake backend and a
// one-module deploy payload installed, and stops whatever it started.
func newManager(t *testing.T, payload string) (*Manager, *memoryCredentials) {
	t.Helper()
	root := t.TempDir()
	bundle := filepath.Join(root, "backend-push.json")
	if payload == "" {
		payload = `{"adminKey":"__OVERGENT_ADMIN_KEY__","dryRun":false,"functions":"functions/"}`
	}
	if err := os.WriteFile(bundle, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	credentials := newCredentials()
	manager, err := New(root, credentials, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = manager.SetArtifacts(fakeBackend(t), bundle); err != nil {
		t.Fatal(err)
	}
	manager.healthBudget = 8 * time.Second
	manager.healthInterval = 20 * time.Millisecond
	t.Cleanup(func() { _ = manager.Stop(context.Background()) })
	return manager, credentials
}

func TestEnsureStartsDeploysAndIsIdempotent(t *testing.T) {
	manager, credentials := newManager(t, "")
	endpoint, err := manager.Ensure(context.Background())
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if !strings.HasPrefix(endpoint.Origin, "http://127.0.0.1:") || !strings.HasPrefix(endpoint.SiteOrigin, "http://127.0.0.1:") {
		t.Fatalf("endpoint is not loopback: %+v", endpoint)
	}
	if endpoint.Origin == endpoint.SiteOrigin {
		t.Fatal("cloud and site origins must differ")
	}
	status := manager.Status(context.Background())
	if !status.Running || status.BundleRevision == "" {
		t.Fatalf("status after Ensure: %+v", status)
	}
	// The instance secret and the deployment secrets key live in the credential
	// store, not in any file under the profile root.
	if _, err = credentials.Get(context.Background(), secretsKeyAccount); err != nil {
		t.Fatal("secrets key was not stored in the credential store")
	}
	state, err := manager.load()
	if err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(manager.statePath)
	if strings.Contains(string(body), state.InstanceName+"|") {
		t.Fatal("backend.json must not contain an admin key")
	}

	// A second Ensure adopts the running backend rather than starting another.
	first := status.PID
	if _, err = manager.Ensure(context.Background()); err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if again := manager.Status(context.Background()); again.PID != first {
		t.Fatalf("second Ensure restarted the backend: %d then %d", first, again.PID)
	}
}

func TestEnsureSkipsPushWhenTheBundleIsUnchanged(t *testing.T) {
	manager, _ := newManager(t, "")
	if _, err := manager.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	revision := manager.state.BundleRevision
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	// A restart with the same payload keeps the same revision, so nothing is
	// pushed. Changing the payload has to change it.
	if _, err := manager.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if manager.state.BundleRevision != revision {
		t.Fatal("an unchanged bundle changed its revision")
	}
	if err := os.WriteFile(manager.state.BundlePath, []byte(`{"adminKey":"x","dryRun":false,"functions":"functions/","extra":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if manager.state.BundleRevision == revision {
		t.Fatal("a changed bundle kept its revision")
	}
}

func TestIncompatibleSchemaKeepsServingAndSaysSo(t *testing.T) {
	manager, _ := newManager(t, "")
	t.Setenv("FAKE_BACKEND_MODE", "incompatible")
	endpoint, err := manager.Ensure(context.Background())
	if err == nil || !strings.Contains(err.Error(), schemaIncompatible) {
		t.Fatalf("expected a data-migration error, got %v", err)
	}
	// The backend is still up and answering: a rejected push is a failed
	// update, not an outage.
	if endpoint.SiteOrigin == "" {
		t.Fatal("a rejected push must still report the running endpoint")
	}
	status := manager.Status(context.Background())
	if !status.Running || !strings.Contains(status.LastError, schemaIncompatible) {
		t.Fatalf("status after a rejected push: %+v", status)
	}
	if manager.state.BundleRevision != "" {
		t.Fatal("a rejected push must not record the new revision")
	}
}

func TestHealthTimeoutFailsRatherThanHanging(t *testing.T) {
	manager, _ := newManager(t, "")
	manager.healthBudget = 300 * time.Millisecond
	t.Setenv("FAKE_BACKEND_MODE", "silent")
	started := time.Now()
	if _, err := manager.Ensure(context.Background()); err == nil {
		t.Fatal("a backend that never answers must fail Ensure")
	}
	// Three attempts at 300 ms each, plus process churn.
	if elapsed := time.Since(started); elapsed > 15*time.Second {
		t.Fatalf("health timeout took %s", elapsed)
	}
}

func TestRestartBackoffGivesUpAfterFiveFailures(t *testing.T) {
	manager, _ := newManager(t, "")
	manager.restartBackoff = func(int) time.Duration { return time.Millisecond }
	manager.healthBudget = 200 * time.Millisecond
	if _, err := manager.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Every restart from here on starts a process that exits immediately, so
	// the supervisor must stop rather than loop.
	t.Setenv("FAKE_BACKEND_MODE", "exit")
	manager.mu.Lock()
	command := manager.command
	manager.mu.Unlock()
	_ = command.Process.Signal(syscall.SIGKILL)

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		manager.mu.Lock()
		failures, lastError := manager.failures, manager.lastError
		manager.mu.Unlock()
		if failures > restartLimit && strings.Contains(lastError, "not restarted") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the supervisor never gave up on a backend that will not start")
}

func TestStaleProcessIsCleanedUpOnStart(t *testing.T) {
	manager, _ := newManager(t, "")
	if _, err := manager.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	stale := manager.state.PID
	// Forget the child the way a killed service would, leaving only the pid in
	// backend.json, then start again.
	manager.mu.Lock()
	manager.command = nil
	state := manager.state
	state.Port, state.SitePort = 0, 0
	manager.state = state
	manager.mu.Unlock()
	if _, err := manager.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if manager.state.PID == stale {
		t.Fatal("a fresh start reused the stale pid")
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(stale, 0) != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("the stale backend %d was left running", stale)
}

func TestIdleStopIsOffByDefaultAndWorksWhenEnabled(t *testing.T) {
	manager, _ := newManager(t, "")
	if _, err := manager.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Lane 01 measured 56 MB idle, far below the 300 MB threshold, so the
	// shipped setting keeps the backend running while the service runs.
	if manager.idleTimeout != 0 {
		t.Fatal("idle shutdown must be off by default")
	}
	if manager.idleStop(context.Background()) {
		t.Fatal("idle shutdown fired with the shipped setting")
	}
	manager.idleTimeout = time.Nanosecond
	manager.Touch()
	time.Sleep(time.Millisecond)
	if !manager.idleStop(context.Background()) {
		t.Fatal("idle shutdown did not fire once enabled")
	}
	if manager.Status(context.Background()).Running {
		t.Fatal("the backend is still running after an idle stop")
	}
}

func TestResetClearsTheDatabaseAndForcesARePush(t *testing.T) {
	manager, _ := newManager(t, "")
	if _, err := manager.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manager.dbPath, []byte("rows"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := manager.Reset(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(manager.dbPath); !os.IsNotExist(err) {
		t.Fatal("reset left the database in place")
	}
	if manager.state.BundleRevision != "" {
		t.Fatal("reset kept the deployed revision, so a fresh database would have no functions")
	}
	// The artifact paths survive so the next Ensure can start again.
	if manager.state.BinaryPath == "" || manager.state.BundlePath == "" {
		t.Fatal("reset discarded the artifact paths")
	}
}

func TestArgumentsBindLoopbackOnly(t *testing.T) {
	state := State{Port: 1234, SitePort: 5678, InstanceName: "overgent-local-test"}
	arguments := arguments(state, "secret", "/tmp/state.sqlite3", "/tmp/storage")
	position := -1
	for index, argument := range arguments {
		if argument == "--interface" {
			position = index
		}
		// The whole privacy claim of local mode rests on this: no argument may
		// name an address that is not loopback.
		if strings.Contains(argument, "0.0.0.0") || strings.Contains(argument, "://") && !strings.Contains(argument, "127.0.0.1") {
			t.Fatalf("argument %q is not loopback", argument)
		}
	}
	if position < 0 || arguments[position+1] != "127.0.0.1" {
		t.Fatalf("--interface 127.0.0.1 is missing from %v", arguments)
	}
	if arguments[len(arguments)-1] != "/tmp/state.sqlite3" {
		t.Fatal("the SQLite path must be the positional argument")
	}
}

func TestStateFileIsOwnerOnly(t *testing.T) {
	manager, _ := newManager(t, "")
	info, err := os.Stat(manager.statePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("backend.json mode is %v", info.Mode().Perm())
	}
	directory, err := os.Stat(manager.directory)
	if err != nil {
		t.Fatal(err)
	}
	if directory.Mode().Perm() != 0o700 {
		t.Fatalf("backend directory mode is %v", directory.Mode().Perm())
	}
}

func TestConfiguredOnlyReportsAProfileWithABackend(t *testing.T) {
	root := t.TempDir()
	if Configured(root) {
		t.Fatal("an empty profile reported a backend")
	}
	manager, _ := newManager(t, "")
	if !Configured(manager.root) {
		t.Fatal("an installed profile reported no backend")
	}
}

// The deploy2 replay is only safe while the backend release, the CLI version
// that recorded the payload, and this code are one pin. This is that check.
func TestPinMatchesThePackagingManifest(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "scripts", "backend-version.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Version    string `json:"version"`
		CLIVersion string `json:"cliVersion"`
	}
	if err = json.Unmarshal(body, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != backendRelease {
		t.Fatalf("backend release drifted: manifest %q, Go %q", manifest.Version, backendRelease)
	}
	if manifest.CLIVersion != backendCLI {
		t.Fatalf("Convex CLI version drifted: manifest %q, Go %q", manifest.CLIVersion, backendCLI)
	}
	if convexClientVersion != "npm-cli-"+backendCLI {
		t.Fatalf("Convex-Client header %q does not name the pinned CLI", convexClientVersion)
	}
}

// The backend answers start_push with objects carrying a repeated "type" key,
// and its own deserializer then refuses a body that still has both. jq and
// JavaScript drop all but the last, which is why the shell replay and the CLI
// never saw it; passing the bytes back verbatim produced HTTP 400. This is the
// regression test for that.
func TestNormalizeCollapsesRepeatedKeysLikeJSONParse(t *testing.T) {
	document, err := normalize([]byte(`{"a":{"type":"SerializedDeveloperIndexConfig","type":"database"},"big":9007199254740993}`))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	if strings.Contains(text, "SerializedDeveloperIndexConfig") {
		t.Fatalf("the shadowed key survived: %s", text)
	}
	if !strings.Contains(text, `"type":"database"`) {
		t.Fatalf("the last value did not win: %s", text)
	}
	// Integers must survive exactly. Decoding into float64 would round this to
	// 9007199254740992 and change a document the backend echoes back.
	if !strings.Contains(text, "9007199254740993") {
		t.Fatalf("an integer lost precision: %s", text)
	}
}

// A local Project stores the backend's site origin as the API base URL it was
// created against, so the port has to survive a restart. This is that
// guarantee: without it every relaunch strands every existing local Project.
func TestPortsSurviveARestart(t *testing.T) {
	manager, _ := newManager(t, "")
	if _, err := manager.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	port, sitePort := manager.state.Port, manager.state.SitePort
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	endpoint, err := manager.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if manager.state.Port != port || manager.state.SitePort != sitePort {
		t.Fatalf("ports moved across a restart: %d/%d became %d/%d", port, sitePort, manager.state.Port, manager.state.SitePort)
	}
	if endpoint.SiteOrigin != loopbackOrigin(sitePort) {
		t.Fatalf("endpoint after restart=%s", endpoint.SiteOrigin)
	}
}

func TestATakenPortIsReplacedAndSaidOutLoud(t *testing.T) {
	manager, _ := newManager(t, "")
	if _, err := manager.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	sitePort := manager.state.SitePort
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Something else takes this profile's cloud port while it is stopped.
	squatter, err := net.Listen("tcp", loopbackAddress(manager.state.Port))
	if err != nil {
		t.Fatal(err)
	}
	defer squatter.Close()
	if _, err = manager.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if manager.state.SitePort == sitePort {
		t.Fatal("the backend kept a port it could not bind")
	}
	// Existing Projects still name the old origin, so this has to be visible
	// rather than showing up later as an unexplained offline queue.
	if status := manager.Status(context.Background()); status.LastError != portMovedError {
		t.Fatalf("a moved port was not reported: %+v", status)
	}
}

// Two managers share one profile: the service supervising the process, and a
// CLI command acting on it. A service that shuts down after `overgent backend
// reset` must not write its stale copy back over the reset.
func TestStopDoesNotResurrectAResetFromAnotherManager(t *testing.T) {
	service, credentials := newManager(t, "")
	if _, err := service.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	// A second manager over the same profile, as `overgent backend reset` has.
	cli, err := New(service.root, credentials, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = cli.Reset(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = service.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	state, err := service.load()
	if err != nil {
		t.Fatal(err)
	}
	if state.BundleRevision != "" || state.Port != 0 {
		t.Fatalf("the service's shutdown undid the reset: %+v", state)
	}
	if state.BinaryPath == "" {
		t.Fatal("the artifact paths were lost")
	}
}
