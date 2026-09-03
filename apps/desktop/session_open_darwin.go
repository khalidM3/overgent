//go:build darwin

package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/khalidM3/overgent/internal/agentactivity"
	"github.com/khalidM3/overgent/internal/config"
)

const (
	maxSessionFiles        = 50_000
	maxSessionMetadataLine = 256 << 10
	maxOpenPromptRunes     = 4_000
)

var (
	workstreamIDPattern = regexp.MustCompile(`^wrk_agent_[0-9a-f]{32}$`)
	codexSessionPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	codexFilenameID     = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
)

// SessionOpenResult is intentionally a desktop-only result. The raw vendor
// session id and repository root are consumed below and never enter a hosted
// dashboard request or snapshot.
type SessionOpenResult struct {
	Vendor          string `json:"vendor"`
	Opened          bool   `json:"opened"`
	Detail          string `json:"detail"`
	FallbackCommand string `json:"fallbackCommand,omitempty"`
}

type owningSession struct {
	vendor, vendorSessionID, cwd string
	modified                     int64
}

// OpenOwningSession resolves the raw vendor identity from local session files,
// validates it against a repository enrolled on this Mac, then invokes either
// a URL handler or executable with an argument array. No shell is involved.
//
// Claude's handler always starts a fresh session carrying the finding prompt;
// it never resumes the existing history. Codex's exact continuation surface is
// `codex continue <id>` -- `resume` is deliberately not used because it opens a
// picker rather than the owning session.
func (service *OnboardingService) OpenOwningSession(workstreamID, prompt, target string) (SessionOpenResult, error) {
	if !workstreamIDPattern.MatchString(workstreamID) {
		return SessionOpenResult{}, errors.New("session id is invalid")
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" || !utf8.ValidString(prompt) || strings.ContainsRune(prompt, '\x00') || utf8.RuneCountInString(prompt) > maxOpenPromptRunes {
		return SessionOpenResult{}, errors.New("finding prompt is empty, invalid, or too large")
	}
	if target != "vendor" && target != "vscode" {
		return SessionOpenResult{}, errors.New("session open target is invalid")
	}

	paths, err := config.Resolve(service.configRoot)
	if err != nil {
		return SessionOpenResult{}, err
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return SessionOpenResult{}, err
	}
	if len(cfg.Workspaces) == 0 {
		return SessionOpenResult{}, errors.New("no local Project workspace is registered")
	}
	home := service.homeDirectory
	if home == "" {
		home, err = os.UserHomeDir()
		if err != nil {
			return SessionOpenResult{}, errors.New("local session records are unavailable")
		}
	}
	session, err := resolveOwningSession(home, workstreamID, cfg.Workspaces)
	if err != nil {
		return SessionOpenResult{}, err
	}

	if session.vendor == "codex" {
		if target != "vendor" {
			return SessionOpenResult{}, errors.New("VS Code opening is available only for Claude Code sessions")
		}
		if !codexSessionPattern.MatchString(session.vendorSessionID) {
			return SessionOpenResult{}, errors.New("Codex session identity is invalid")
		}
		fallback := "codex continue " + session.vendorSessionID
		executable, ok := agentExecutable("codex")
		if !ok {
			return SessionOpenResult{Vendor: "codex", Detail: "Codex is not available to continue this session. Copy the exact continuation command instead.", FallbackCommand: fallback}, nil
		}
		starter := service.startSessionCommand
		if starter == nil {
			starter = startDetachedCommand
		}
		if err = starter(executable, []string{"continue", session.vendorSessionID}, session.cwd); err != nil {
			return SessionOpenResult{Vendor: "codex", Detail: "Codex could not be started. Copy the exact continuation command instead.", FallbackCommand: fallback}, nil
		}
		return SessionOpenResult{Vendor: "codex", Opened: true, Detail: "Continued the exact Codex session in its repository."}, nil
	}

	opener := service.openSessionURL
	if opener == nil {
		opener = openURLWithSystemHandler
	}
	if target == "vscode" {
		const vscodeURL = "vscode://anthropic.claude-code/open"
		if err = opener(vscodeURL); err != nil {
			return SessionOpenResult{Vendor: "claude", Detail: "VS Code's Claude Code handler is unavailable on this Mac.", FallbackCommand: "open " + shellQuote(vscodeURL)}, nil
		}
		return SessionOpenResult{Vendor: "claude", Opened: true, Detail: "Opened Claude Code in VS Code."}, nil
	}

	query := url.Values{}
	query.Set("cwd", session.cwd)
	query.Set("q", prompt)
	openURL := "claude-cli://open?" + query.Encode()
	fallback := "claude " + shellQuote(prompt)
	if err = opener(openURL); err != nil {
		return SessionOpenResult{
			Vendor: "claude", Detail: "Claude Code's open handler is not registered. This can happen before the first interactive prompt, or when an organization disables it. Copy the command and run it from the repository instead.", FallbackCommand: fallback,
		}, nil
	}
	return SessionOpenResult{Vendor: "claude", Opened: true, Detail: "Opened a fresh Claude Code session with the finding pre-filled. The existing session was not resumed."}, nil
}

func startDetachedCommand(executable string, arguments []string, cwd string) error {
	command := exec.Command(executable, arguments...)
	command.Dir = cwd
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}

func openURLWithSystemHandler(value string) error {
	// macOS `open` returns an error when Launch Services has no registered
	// handler. Waiting for that result is what makes handler absence visible.
	return exec.Command("open", value).Run()
}

func resolveOwningSession(home, workstreamID string, workspaces []config.Workspace) (owningSession, error) {
	roots := []struct {
		vendor, root string
	}{
		{vendor: "codex", root: filepath.Join(home, ".codex", "sessions")},
		{vendor: "claude", root: filepath.Join(home, ".claude", "projects")},
	}
	var best owningSession
	seen := 0
	for _, source := range roots {
		_ = filepath.WalkDir(source.root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil || seen >= maxSessionFiles {
				if seen >= maxSessionFiles {
					return filepath.SkipAll
				}
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
				return nil
			}
			seen++
			// Both supported vendors name a session record with its session id.
			// Hashing that filename first means resolution does not read hundreds
			// of unrelated transcripts merely to find one local identity.
			candidateID := sessionIDFromFilename(entry.Name(), source.vendor)
			derived, _, ok := agentactivity.WorkstreamIDFor(source.vendor, candidateID)
			if !ok || derived != workstreamID {
				return nil
			}
			id, cwd := readSessionIdentity(path, source.vendor)
			if id != candidateID {
				return nil
			}
			canonical, ok := registeredSessionCWD(cwd, workspaces)
			if !ok {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return nil
			}
			candidate := owningSession{vendor: source.vendor, vendorSessionID: id, cwd: canonical, modified: info.ModTime().UnixNano()}
			if best.vendor == "" || candidate.modified > best.modified {
				best = candidate
			}
			return nil
		})
	}
	if best.vendor == "" {
		return owningSession{}, errors.New("the owning vendor session is not available on this Mac")
	}
	return best, nil
}

func sessionIDFromFilename(name, vendor string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	switch vendor {
	case "claude":
		if base != "" && len(base) <= 512 && !strings.ContainsRune(base, '\x00') {
			return base
		}
	case "codex":
		return codexFilenameID.FindString(base)
	}
	return ""
}

func readSessionIdentity(path, vendor string) (string, string) {
	file, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer file.Close()
	reader := bufio.NewReaderSize(io.LimitReader(file, 2<<20), 64<<10)
	for range 64 {
		line, readErr := reader.ReadSlice('\n')
		if errors.Is(readErr, bufio.ErrBufferFull) || len(line) > maxSessionMetadataLine {
			return "", ""
		}
		if len(line) > 0 {
			var raw struct {
				Type      string `json:"type"`
				SessionID string `json:"sessionId"`
				CWD       string `json:"cwd"`
				Payload   struct {
					ID        string `json:"id"`
					SessionID string `json:"session_id"`
					CWD       string `json:"cwd"`
				} `json:"payload"`
			}
			if json.Unmarshal(line, &raw) == nil {
				if vendor == "claude" && raw.SessionID != "" && raw.CWD != "" {
					return raw.SessionID, raw.CWD
				}
				if vendor == "codex" && raw.Type == "session_meta" && raw.Payload.CWD != "" {
					id := raw.Payload.SessionID
					if id == "" {
						id = raw.Payload.ID
					}
					return id, raw.Payload.CWD
				}
			}
		}
		if readErr != nil {
			return "", ""
		}
	}
	return "", ""
}

func registeredSessionCWD(value string, workspaces []config.Workspace) (string, bool) {
	if value == "" || !filepath.IsAbs(value) || strings.ContainsRune(value, '\x00') {
		return "", false
	}
	canonical, err := filepath.EvalSymlinks(filepath.Clean(value))
	if err != nil {
		return "", false
	}
	for _, workspace := range workspaces {
		root, rootErr := filepath.EvalSymlinks(workspace.Root)
		if rootErr != nil {
			continue
		}
		relative, relErr := filepath.Rel(root, canonical)
		if relErr == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative) {
			return canonical, true
		}
	}
	return "", false
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
