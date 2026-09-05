package localbackend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Ensure brings the backend up and the deployed bundle in step with the one
// shipped in the app, and returns where clients should reach it.
//
// It is idempotent and adoption-based: a healthy backend already listening on
// the recorded ports under this profile's instance name is used as it is,
// whether this process started it or a previous one did. That is what lets the
// desktop ask the CLI to start the backend before the service exists, and the
// service then take over supervision without a restart.
func (m *Manager) Ensure(ctx context.Context) (Endpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = false
	endpoint, err := m.ensureRunningLocked(ctx)
	if err != nil {
		m.lastError = err.Error()
		return Endpoint{}, err
	}
	if err = m.ensureDeployedLocked(ctx, endpoint); err != nil {
		// A rejected push leaves the previous bundle serving. Say so and keep
		// running rather than taking coordination down over an upgrade.
		m.lastError = err.Error()
		m.logger.Warn("local backend bundle was not deployed", "reason", err)
		return endpoint, err
	}
	m.lastError = ""
	if m.portMoved {
		m.lastError = portMovedError
	}
	m.idleSince = m.now()
	m.startSupervisorLocked()
	return endpoint, nil
}

func (m *Manager) ensureRunningLocked(ctx context.Context) (Endpoint, error) {
	state := m.state
	if state.BinaryPath == "" || state.BundlePath == "" {
		return Endpoint{}, errors.New("local backend is not installed for this profile; run overgent backend install")
	}
	if state.InstanceName == "" {
		name, err := newInstanceName()
		if err != nil {
			return Endpoint{}, err
		}
		state.InstanceName = name
		m.state = state
		if err = m.save(state); err != nil {
			return Endpoint{}, err
		}
	}
	if state.Port != 0 && m.healthy(ctx, state.Port, state.InstanceName) {
		if m.command == nil {
			m.adopted = state.PID
		}
		return endpointFor(state), nil
	}
	return m.startLocked(ctx)
}

func (m *Manager) startLocked(ctx context.Context) (Endpoint, error) {
	state := m.state
	m.killStale(state.PID, state.BinaryPath)
	secret, err := m.instanceSecret(ctx, state.InstanceName)
	if err != nil {
		return Endpoint{}, err
	}
	if err = os.MkdirAll(m.storage, 0o700); err != nil {
		return Endpoint{}, fmt.Errorf("create backend storage directory: %w", err)
	}
	previous := state.SitePort
	var lastErr error
	// Three attempts covers the reserve-then-bind race; a fourth failure is a
	// real problem with the binary, not a port that was taken twice in a row.
	for attempt := 0; attempt < 3; attempt++ {
		port, sitePort := state.Port, state.SitePort
		// The ports this profile used last time are reused whenever they are
		// still free. They are not an implementation detail: a local Project
		// stores its site origin as the API base URL it was created against, so
		// a backend that came back on a different port would leave every
		// existing Project on this profile publishing into a closed socket.
		if attempt > 0 || !available(port) || !available(sitePort) {
			fresh, freshSite, portErr := freePorts()
			if portErr != nil {
				return Endpoint{}, portErr
			}
			port, sitePort = fresh, freshSite
		}
		state.Port, state.SitePort = port, sitePort
		command, startErr := m.spawn(ctx, state, secret)
		if startErr != nil {
			return Endpoint{}, startErr
		}
		state.PID = command.Process.Pid
		if m.waitHealthy(ctx, state.Port, state.InstanceName) {
			m.command = command
			m.adopted = 0
			m.state = state
			if err = m.save(state); err != nil {
				return Endpoint{}, err
			}
			// Something else has this profile's port. Existing Projects still
			// name the old origin, so this is a standing condition rather than
			// a one-off log line: it outlives the successful start below and is
			// what explains the offline queue that follows.
			m.portMoved = previous != 0 && previous != state.SitePort
			if m.portMoved {
				m.logger.Warn("local backend port changed", "previous", previous, "current", state.SitePort)
			}
			return endpointFor(state), nil
		}
		lastErr = fmt.Errorf("local backend did not become healthy within %s", m.healthBudget)
		m.terminate(command)
	}
	return Endpoint{}, lastErr
}

func (m *Manager) spawn(ctx context.Context, state State, secret string) (*exec.Cmd, error) {
	rotateLog(m.logPath)
	logFile, err := os.OpenFile(m.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open backend log: %w", err)
	}
	command := exec.CommandContext(ctx, state.BinaryPath, arguments(state, secret, m.dbPath, m.storage)...)
	command.Stdout = logFile
	command.Stderr = logFile
	// The child gets its own process group so an interrupt aimed at the CLI or
	// the development harness does not take the backend down with it; shutdown
	// is an explicit signal, below.
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Cancel is SIGTERM rather than the default SIGKILL, and WaitDelay is the
	// five seconds after which the process is killed anyway.
	command.Cancel = func() error { return command.Process.Signal(syscall.SIGTERM) }
	command.WaitDelay = 5 * time.Second
	if err = command.Start(); err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("start local backend: %w", err)
	}
	go func() { _ = logFile.Close() }()
	return command, nil
}

func endpointFor(state State) Endpoint {
	return Endpoint{Origin: loopbackOrigin(state.Port), SiteOrigin: loopbackOrigin(state.SitePort)}
}

func (m *Manager) waitHealthy(ctx context.Context, port int, instance string) bool {
	deadline := m.now().Add(m.healthBudget)
	for m.now().Before(deadline) {
		if ctx.Err() != nil {
			return false
		}
		if m.healthy(ctx, port, instance) {
			return true
		}
		time.Sleep(m.healthInterval)
	}
	return false
}

// healthy asks two questions the brief keeps separate: is something answering
// (Lane 01's /version), and is it *this* profile's backend (/instance_name).
// Liveness alone would let a stale process on a recycled port pass as ours.
func (m *Manager) healthy(ctx context.Context, port int, instance string) bool {
	if get(ctx, loopbackOrigin(port)+"/version") == "" {
		return false
	}
	return get(ctx, loopbackOrigin(port)+"/instance_name") == instance
}

func get(ctx context.Context, url string) string {
	requestCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return ""
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 4096))
	if err != nil {
		return ""
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		// /version answers with a body; an empty one is not a healthy reply.
		return ""
	}
	return text
}

func (m *Manager) instanceSecret(ctx context.Context, instance string) (string, error) {
	if m.creds == nil {
		return "", errors.New("local backend requires a credential store")
	}
	account := instanceAccountPrefix + instance
	if secret, err := m.creds.Get(ctx, account); err == nil && secret != "" {
		return secret, nil
	}
	secret, err := randomHex(32)
	if err != nil {
		return "", err
	}
	if err = m.creds.Put(ctx, account, secret); err != nil {
		return "", fmt.Errorf("store backend instance secret: %w", err)
	}
	return secret, nil
}

// Stop asks the backend to exit, waits, and kills it if it does not.
func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = true
	if m.command != nil {
		m.terminate(m.command)
		m.command = nil
	} else if m.adopted > 0 {
		m.killStale(m.adopted, m.state.BinaryPath)
		m.adopted = 0
	} else if m.state.PID > 0 {
		m.killStale(m.state.PID, m.state.BinaryPath)
	}
	// Only the pid is cleared, and it is cleared against whatever is on disk
	// rather than against this manager's copy. Two managers share one profile -
	// the service supervising the process, and a CLI command acting on it - and
	// writing a whole in-memory record here is how a long-running service
	// undoes an `overgent backend reset` that a CLI already applied, simply by
	// shutting down afterwards.
	state, err := m.load()
	if err != nil {
		return err
	}
	state.PID = 0
	m.state = state
	return m.save(state)
}

func (m *Manager) terminate(command *exec.Cmd) {
	if command == nil || command.Process == nil {
		return
	}
	_ = command.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() { _, _ = command.Process.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = command.Process.Kill()
		<-done
	}
}

// Status reports what the menu, `health`, and `overgent backend status` show.
func (m *Manager) Status(ctx context.Context) Status {
	m.mu.Lock()
	state := m.state
	lastError := m.lastError
	idleSince := m.idleSince
	m.mu.Unlock()
	status := Status{
		Port: state.Port, SitePort: state.SitePort, PID: state.PID,
		Version: state.Version, BundleRevision: state.BundleRevision,
		DatabasePath: m.dbPath, LastError: lastError,
	}
	if state.Port != 0 {
		status.Origin = loopbackOrigin(state.Port)
		status.SiteOrigin = loopbackOrigin(state.SitePort)
		status.Running = m.healthy(ctx, state.Port, state.InstanceName)
	}
	if !status.Running {
		status.PID = 0
	}
	if info, err := os.Stat(m.dbPath); err == nil {
		status.DatabaseBytes = info.Size()
	}
	if !idleSince.IsZero() {
		status.IdleSince = idleSince.UTC().Format(time.RFC3339)
	}
	return status
}

// Reset stops the backend and deletes its database and file storage, keeping
// the recorded artifact paths so the next Ensure starts a fresh instance.
func (m *Manager) Reset(ctx context.Context) error {
	if err := m.Stop(ctx); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, path := range []string{m.dbPath, m.dbPath + "-shm", m.dbPath + "-wal"} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove backend database: %w", err)
		}
	}
	if err := os.RemoveAll(m.storage); err != nil {
		return fmt.Errorf("remove backend storage: %w", err)
	}
	state := m.state
	// The deployed bundle went with the database, so the next Ensure must push
	// again; keeping the revision would leave a fresh database with no
	// functions on it.
	state.BundleRevision = ""
	state.Port, state.SitePort, state.PID = 0, 0, 0
	m.state = state
	m.lastError = ""
	return m.save(state)
}

// startSupervisorLocked runs one watcher for the lifetime of this manager. It
// restarts a backend that exits unexpectedly, with the backoff the brief fixes,
// and gives up loudly rather than looping forever.
func (m *Manager) startSupervisorLocked() {
	if m.command == nil || m.watching {
		return
	}
	m.watching = true
	// The process is handed over rather than read back from the manager. A
	// watcher that re-reads m.command can find it already cleared, wait on
	// nothing, and leave the real process unreaped - which is both a leaked
	// zombie and a restart that was never earned.
	go m.supervise(m.command)
}

// supervise waits on one backend process and restarts it when it exits.
//
// A failed restart is itself a failure, so the loop keeps counting rather than
// parking on a signal that will not come: that is the difference between five
// attempts and one. After five failures inside five minutes it stops and leaves
// the reason in health, because a sixth attempt at a backend that will not
// start is noise, not recovery.
func (m *Manager) supervise(command *exec.Cmd) {
	for {
		_ = command.Wait()
		m.mu.Lock()
		if m.stopped {
			m.watching = false
			m.mu.Unlock()
			return
		}
		if m.command != nil && m.command != command {
			// Something already replaced this process - a manual Ensure, or a
			// reset. Follow the new one instead of restarting the old.
			command = m.command
			m.mu.Unlock()
			continue
		}
		m.command = nil
		if m.firstFail.IsZero() || m.now().Sub(m.firstFail) > restartWindow {
			m.firstFail = m.now()
			m.failures = 0
		}
		m.failures++
		failures := m.failures
		if failures > restartLimit {
			m.lastError = "backend exited repeatedly and was not restarted"
			m.watching = false
			m.mu.Unlock()
			m.logger.Error("local backend exited repeatedly; not restarting", "failures", failures)
			return
		}
		delay := m.restartBackoff(failures - 1)
		m.lastError = "backend exited; restarting"
		m.mu.Unlock()
		m.logger.Warn("local backend exited; restarting", "attempt", failures, "delay", delay)
		time.Sleep(delay)
		_, err := m.Ensure(context.Background())
		m.mu.Lock()
		next := m.command
		m.mu.Unlock()
		if err == nil && next != nil {
			command = next
			continue
		}
		m.logger.Warn("local backend restart failed", "error", err)
		// command is the process that already exited, so the next Wait returns
		// at once and the pass above counts this failed restart.
	}
}

// idleStop is the shutdown path the brief makes conditional on Lane 01's idle
// RSS measurement. The measurement (56 MB) is far below the 300 MB threshold,
// so idleTimeout is zero in production and this never fires; it exists so the
// decision can be reversed by setting one field rather than by writing the
// lifecycle again.
func (m *Manager) idleStop(ctx context.Context) bool {
	m.mu.Lock()
	timeout, idleSince := m.idleTimeout, m.idleSince
	m.mu.Unlock()
	if timeout <= 0 || idleSince.IsZero() || m.now().Sub(idleSince) < timeout {
		return false
	}
	_ = m.Stop(ctx)
	return true
}

// Export copies the stopped database out of the profile. It is deliberately
// minimal: the portable, backend-independent export is the /v1 owner export,
// which works against any backend; this is the "give me the file" answer.
func (m *Manager) Export(directory string) (string, error) {
	if strings.TrimSpace(directory) == "" {
		return "", errors.New("backend export requires an output directory")
	}
	target, err := filepath.Abs(directory)
	if err != nil {
		return "", fmt.Errorf("resolve export directory: %w", err)
	}
	if err = os.MkdirAll(target, 0o700); err != nil {
		return "", fmt.Errorf("create export directory: %w", err)
	}
	source, err := os.Open(m.dbPath)
	if err != nil {
		return "", fmt.Errorf("read backend database: %w", err)
	}
	defer source.Close()
	destination := filepath.Join(target, "state.sqlite3")
	copied, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", fmt.Errorf("create export file: %w", err)
	}
	defer copied.Close()
	if _, err = io.Copy(copied, source); err != nil {
		return "", fmt.Errorf("copy backend database: %w", err)
	}
	return destination, nil
}

// probe returns the status code of a GET, or 0 when nothing answered.
func probe(ctx context.Context, url string) int {
	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, url, nil)
	if err != nil {
		return 0
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return 0
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	return response.StatusCode
}
