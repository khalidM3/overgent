package config

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Workspace struct {
	ID           string `json:"id"`
	ProjectID    string `json:"projectId"`
	WorkstreamID string `json:"workstreamId"`
	Root         string `json:"root"`
	Baseline     string `json:"baseline"`
	Fingerprint  string `json:"repositoryFingerprint"`
	MemberID     string `json:"memberId"`
	SessionID    string `json:"sessionId"`
}

// Backend kinds. A Project's kind follows from the origin it lives on, and is
// stored so the desktop and CLI can name it without re-deriving the rule.
const (
	KindLocal = "local"
	KindTeam  = "team"
)

// Version is the configuration version this build writes. Load migrates
// version 1 in memory and Save always writes this one.
const Version = 2

// Backend is one server implementing the /v1 contract, together with the
// device identity this profile uses with it. One Mac has one device identity
// per backend (ADR-069), so the two travel together.
type Backend struct {
	ID         string `json:"id"`
	APIBaseURL string `json:"apiBaseUrl"`
	DeviceID   string `json:"deviceId"`
	Kind       string `json:"kind"`
}

// Project binds one Project to the backend that holds it (ADR-074). Workspaces
// keep naming a Project, and resolve their backend through it.
type Project struct {
	ID        string `json:"id"`
	BackendID string `json:"backendId"`
}

type Config struct {
	Version    int         `json:"version"`
	Backends   []Backend   `json:"backends"`
	Projects   []Project   `json:"projects"`
	Workspaces []Workspace `json:"workspaces"`
}

// storedConfig is what actually sits on disk. It carries the version 1 fields
// so Load can migrate them; nothing else reads them, and Save never writes
// them back.
type storedConfig struct {
	Version    int         `json:"version"`
	APIBaseURL string      `json:"apiBaseUrl,omitempty"`
	DeviceID   string      `json:"deviceId,omitempty"`
	Backends   []Backend   `json:"backends,omitempty"`
	Projects   []Project   `json:"projects,omitempty"`
	Workspaces []Workspace `json:"workspaces,omitempty"`
}

type Paths struct{ Root, Config, DB, Lock, Socket string }

// IsLoopbackOrigin reports whether this origin is served by a backend running
// on this Mac. It is the single loopback rule: localbackend.IsLoopbackOrigin
// delegates here so the service, the CLI, and the desktop cannot disagree
// about which Projects are local.
func IsLoopbackOrigin(origin string) bool {
	parsed, err := url.Parse(strings.TrimSpace(origin))
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" {
		return false
	}
	host := parsed.Hostname()
	if host == "localhost" {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

// BackendKind is the kind stored on a Backend record, derived once at write
// time from the origin.
func BackendKind(origin string) string {
	if IsLoopbackOrigin(origin) {
		return KindLocal
	}
	return KindTeam
}

// BackendID is the opaque, stable identifier for a backend origin. It is
// derived from the origin alone, so re-enrolling against the same server -
// with a new device identity - updates that backend rather than growing a
// second entry for the same server.
func BackendID(origin string) string {
	sum := sha256.Sum256([]byte("overgent.backend.v1\x00" + canonicalOrigin(origin)))
	return fmt.Sprintf("bk_%x", sum[:16])
}

func canonicalOrigin(origin string) string {
	return strings.TrimRight(strings.TrimSpace(origin), "/")
}

// Single is the version 2 configuration a single backend can express: one
// backend, and every Project in workspaces bound to it. It is what Load's
// migration of a version 1 profile produces.
func Single(apiBaseURL, deviceID string, workspaces []Workspace) Config {
	cfg := Config{Version: Version, Workspaces: workspaces}
	if canonicalOrigin(apiBaseURL) == "" || deviceID == "" {
		return cfg
	}
	backend := newBackend(apiBaseURL, deviceID)
	cfg.Backends = []Backend{backend}
	for _, workspace := range workspaces {
		cfg = cfg.BindProject(workspace.ProjectID, backend.ID)
	}
	return cfg
}

func newBackend(apiBaseURL, deviceID string) Backend {
	origin := canonicalOrigin(apiBaseURL)
	return Backend{ID: BackendID(origin), APIBaseURL: origin, DeviceID: deviceID, Kind: BackendKind(origin)}
}

// BackendByID returns the backend with this identifier.
func (c Config) BackendByID(id string) (Backend, bool) {
	for _, backend := range c.Backends {
		if backend.ID == id {
			return backend, true
		}
	}
	return Backend{}, false
}

// BackendForOrigin returns the backend this profile already uses for a server.
func (c Config) BackendForOrigin(origin string) (Backend, bool) {
	return c.BackendByID(BackendID(origin))
}

// BackendTarget is the backend record this profile would use for a server:
// the one it already has, or an unenrolled record for a server it has never
// used. It is what an onboarding flow is pointed at, so the flow can tell
// "mint a device identity here" from "reuse the one this profile has".
func (c Config) BackendTarget(apiBaseURL string) Backend {
	origin := canonicalOrigin(apiBaseURL)
	if backend, known := c.BackendForOrigin(origin); known {
		return backend
	}
	return Backend{ID: BackendID(origin), APIBaseURL: origin, Kind: BackendKind(origin)}
}

// BackendForProject resolves the backend one Project lives on.
func (c Config) BackendForProject(projectID string) (Backend, bool) {
	for _, project := range c.Projects {
		if project.ID == projectID {
			return c.BackendByID(project.BackendID)
		}
	}
	return Backend{}, false
}

// BackendForWorkspace resolves the backend one registered repository publishes
// to, through the Project it belongs to. A workspace whose Project has no
// backend is an orphan: the caller reports it and carries on with the rest.
func (c Config) BackendForWorkspace(workspace Workspace) (Backend, bool) {
	return c.BackendForProject(workspace.ProjectID)
}

// ProjectsForBackend lists the Projects bound to one backend.
func (c Config) ProjectsForBackend(backendID string) []string {
	var ids []string
	for _, project := range c.Projects {
		if project.BackendID == backendID {
			ids = append(ids, project.ID)
		}
	}
	return ids
}

// UpsertBackend records the backend at this origin, or updates the device
// identity of the one already there.
//
// A second device identity for a backend the profile already knows is refused
// rather than merged: the Projects on that backend belong to the identity that
// is already stored, and quietly replacing it would strand every one of them.
func (c Config) UpsertBackend(apiBaseURL, deviceID string) (Config, Backend, error) {
	origin := canonicalOrigin(apiBaseURL)
	if origin == "" {
		return c, Backend{}, fmt.Errorf("backend API base URL is required")
	}
	if deviceID == "" {
		return c, Backend{}, fmt.Errorf("backend device ID is required")
	}
	next := newBackend(origin, deviceID)
	backends := append([]Backend(nil), c.Backends...)
	for index, existing := range backends {
		if existing.ID != next.ID {
			continue
		}
		if existing.DeviceID != "" && existing.DeviceID != deviceID {
			return c, Backend{}, fmt.Errorf("device ID does not match the identity registered for this backend")
		}
		backends[index] = next
		c.Backends = backends
		return c, next, nil
	}
	c.Backends = append(backends, next)
	return c, next, nil
}

// BindProject records which backend holds a Project. Re-binding an existing
// Project to the same backend is a no-op; nothing here moves a Project between
// backends.
func (c Config) BindProject(projectID, backendID string) Config {
	if projectID == "" || backendID == "" {
		return c
	}
	projects := append([]Project(nil), c.Projects...)
	for index, project := range projects {
		if project.ID == projectID {
			projects[index].BackendID = backendID
			c.Projects = projects
			return c
		}
	}
	c.Projects = append(projects, Project{ID: projectID, BackendID: backendID})
	return c
}

// RemoveBackend forgets one backend, the Projects on it, and the repositories
// registered to those Projects. It returns the workspaces it cleared, which is
// what a reset reports.
func (c Config) RemoveBackend(backendID string) (Config, int) {
	removed := map[string]bool{}
	var projects []Project
	for _, project := range c.Projects {
		if project.BackendID == backendID {
			removed[project.ID] = true
			continue
		}
		projects = append(projects, project)
	}
	var backends []Backend
	for _, backend := range c.Backends {
		if backend.ID != backendID {
			backends = append(backends, backend)
		}
	}
	var workspaces []Workspace
	cleared := 0
	for _, workspace := range c.Workspaces {
		if removed[workspace.ProjectID] {
			cleared++
			continue
		}
		workspaces = append(workspaces, workspace)
	}
	c.Backends, c.Projects, c.Workspaces = backends, projects, workspaces
	return c, cleared
}

func DefaultRoot() (string, error) {
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("unsupported platform %s: local service validated only on macOS", runtime.GOOS)
	}
	d, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(d, "Overgent"), nil
}

func Resolve(root string) (Paths, error) {
	if runtime.GOOS != "darwin" {
		return Paths{}, fmt.Errorf("unsupported platform %s: local service validated only on macOS", runtime.GOOS)
	}
	if root == "" {
		return Paths{}, fmt.Errorf("config root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return Paths{}, fmt.Errorf("resolve config root: %w", err)
	}
	return Paths{Root: abs, Config: filepath.Join(abs, "config.json"), DB: filepath.Join(abs, "state.db"), Lock: filepath.Join(abs, "service.lock"), Socket: filepath.Join(abs, "service.sock")}, nil
}

func Load(paths Paths) (Config, error) {
	b, err := os.ReadFile(paths.Config)
	if os.IsNotExist(err) {
		return Config{Version: Version}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var stored storedConfig
	if err := json.Unmarshal(b, &stored); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	switch stored.Version {
	case 1:
		// One backend from the profile-wide origin and device, and one Project
		// per registered repository pointing at it. The device identity is
		// unchanged, so the Keychain entry it names is still the right one and
		// nothing has to be renamed.
		return Single(stored.APIBaseURL, stored.DeviceID, stored.Workspaces), nil
	case Version:
		return Config{Version: Version, Backends: stored.Backends, Projects: stored.Projects, Workspaces: stored.Workspaces}, nil
	default:
		return Config{}, fmt.Errorf("unsupported config version %d", stored.Version)
	}
}

func Save(paths Paths, c Config) error {
	if err := os.MkdirAll(paths.Root, 0o700); err != nil {
		return fmt.Errorf("create config root: %w", err)
	}
	if err := os.Chmod(paths.Root, 0o700); err != nil {
		return fmt.Errorf("secure config root: %w", err)
	}
	b, err := json.MarshalIndent(storedConfig{Version: Version, Backends: c.Backends, Projects: c.Projects, Workspaces: c.Workspaces}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	tmp := paths.Config + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmp, paths.Config); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	if err := os.Chmod(paths.Config, 0o600); err != nil {
		return fmt.Errorf("secure config: %w", err)
	}
	return nil
}
