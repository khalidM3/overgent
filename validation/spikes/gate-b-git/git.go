package gatebgit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type Runner struct{}

func (Runner) Git(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %q: %w: %s", args, err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

type BaselineState string

const (
	BaselineAncestor BaselineState = "ancestor"
	BaselineDiverged BaselineState = "diverged_non_ancestor"
	BaselineMissing  BaselineState = "missing"
	BaselineUnborn   BaselineState = "unborn_head"
)

func CaptureBaseline(ctx context.Context, r Runner, root string) (string, error) {
	out, err := r.Git(ctx, root, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return "", fmt.Errorf("capture workstream baseline: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func ClassifyBaseline(ctx context.Context, r Runner, root, baseline string) (BaselineState, error) {
	if !validObjectID(baseline) {
		return "", fmt.Errorf("classify baseline: invalid full object ID")
	}
	if _, err := r.Git(ctx, root, "rev-parse", "--verify", "HEAD^{commit}"); err != nil {
		return BaselineUnborn, nil
	}
	if _, err := r.Git(ctx, root, "cat-file", "-e", baseline+"^{commit}"); err != nil {
		return BaselineMissing, nil
	}
	cmd := exec.CommandContext(ctx, "git", "-C", root, "merge-base", "--is-ancestor", baseline, "HEAD")
	cmd.Env = append(os.Environ(), "LC_ALL=C", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	err := cmd.Run()
	if err == nil {
		return BaselineAncestor, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return BaselineDiverged, nil
	}
	return "", fmt.Errorf("classify baseline ancestry: %w", err)
}

func validObjectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, r := range value {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

type Entry struct {
	Path    string `json:"path"`
	Status  string `json:"status"`
	OldPath string `json:"oldPath,omitempty"`
}

type Manifest struct {
	Baseline      string        `json:"baseline"`
	Head          string        `json:"head"`
	BaselineState BaselineState `json:"baselineState"`
	Entries       []Entry       `json:"entries"`
}

func Observe(ctx context.Context, r Runner, root, baseline string) (Manifest, error) {
	state, err := ClassifyBaseline(ctx, r, root, baseline)
	if err != nil {
		return Manifest{}, err
	}
	headOut, err := r.Git(ctx, root, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return Manifest{}, fmt.Errorf("resolve current head: %w", err)
	}
	manifest := Manifest{Baseline: baseline, Head: strings.TrimSpace(string(headOut)), BaselineState: state}
	byPath := map[string]Entry{}
	if state != BaselineMissing && state != BaselineUnborn {
		out, err := r.Git(ctx, root, "diff", "--name-status", "-z", "-M", baseline, "HEAD", "--")
		if err != nil {
			return Manifest{}, fmt.Errorf("observe committed baseline delta: %w", err)
		}
		if err := mergeNameStatus(root, byPath, out); err != nil {
			return Manifest{}, err
		}
	}
	for _, args := range [][]string{
		{"diff", "--name-status", "-z", "-M", "--"},
		{"diff", "--cached", "--name-status", "-z", "-M", "--"},
	} {
		out, err := r.Git(ctx, root, args...)
		if err != nil {
			return Manifest{}, fmt.Errorf("observe worktree/index delta: %w", err)
		}
		if err := mergeNameStatus(root, byPath, out); err != nil {
			return Manifest{}, err
		}
	}
	untracked, err := r.Git(ctx, root, "ls-files", "--others", "--exclude-standard", "-z", "--")
	if err != nil {
		return Manifest{}, fmt.Errorf("observe untracked paths: %w", err)
	}
	for _, raw := range splitNUL(untracked) {
		path, err := NormalizeObservedPath(root, raw)
		if err != nil {
			return Manifest{}, fmt.Errorf("normalize untracked path: %w", err)
		}
		byPath[path] = Entry{Path: path, Status: "untracked"}
	}
	for _, entry := range byPath {
		manifest.Entries = append(manifest.Entries, entry)
	}
	sort.Slice(manifest.Entries, func(i, j int) bool { return manifest.Entries[i].Path < manifest.Entries[j].Path })
	return manifest, nil
}

func mergeNameStatus(root string, dst map[string]Entry, data []byte) error {
	fields := splitNUL(data)
	for i := 0; i < len(fields); {
		code := fields[i]
		i++
		if code == "" || i >= len(fields) {
			return fmt.Errorf("malformed git name-status stream")
		}
		status := statusName(code[0])
		if code[0] == 'R' || code[0] == 'C' {
			if i+1 >= len(fields) {
				return fmt.Errorf("malformed git rename/copy stream")
			}
			oldPath, err := NormalizeObservedPath(root, fields[i])
			if err != nil {
				return err
			}
			path, err := NormalizeObservedPath(root, fields[i+1])
			if err != nil {
				return err
			}
			i += 2
			dst[path] = Entry{Path: path, Status: status, OldPath: oldPath}
			continue
		}
		path, err := NormalizeObservedPath(root, fields[i])
		if err != nil {
			return err
		}
		i++
		dst[path] = Entry{Path: path, Status: status}
	}
	return nil
}

func statusName(code byte) string {
	switch code {
	case 'A':
		return "added"
	case 'D':
		return "deleted"
	case 'R':
		return "renamed"
	case 'C':
		return "copied"
	case 'T':
		return "type_changed"
	default:
		return "modified"
	}
}

func splitNUL(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	parts := bytes.Split(data, []byte{0})
	if len(parts[len(parts)-1]) == 0 {
		parts = parts[:len(parts)-1]
	}
	out := make([]string, len(parts))
	for i := range parts {
		out[i] = string(parts[i])
	}
	return out
}

func NormalizeObservedPath(root, raw string) (string, error) {
	if raw == "" || strings.IndexByte(raw, 0) >= 0 || filepath.IsAbs(raw) {
		return "", fmt.Errorf("invalid repository-relative path")
	}
	clean := filepath.Clean(filepath.FromSlash(raw))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes repository root")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("canonicalize repository root: %w", err)
	}
	if resolvedRoot, resolveErr := filepath.EvalSymlinks(absRoot); resolveErr == nil {
		absRoot = resolvedRoot
	}
	joined := filepath.Join(absRoot, clean)
	existing := joined
	for {
		resolved, resolveErr := filepath.EvalSymlinks(existing)
		if resolveErr == nil {
			rel, relErr := filepath.Rel(absRoot, resolved)
			if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return "", fmt.Errorf("symlink escapes repository root")
			}
			break
		}
		if !os.IsNotExist(resolveErr) {
			return "", fmt.Errorf("resolve observed path: %w", resolveErr)
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			break
		}
		existing = parent
	}
	return filepath.ToSlash(clean), nil
}

type RepositoryIdentity struct {
	CommonDir      string   `json:"commonDir"`
	RemoteIdentity string   `json:"remoteIdentity,omitempty"`
	Classification string   `json:"classification"`
	Remotes        []string `json:"remotes,omitempty"`
}

func IdentifyRepository(ctx context.Context, r Runner, root string) (RepositoryIdentity, error) {
	common, err := r.Git(ctx, root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return RepositoryIdentity{}, fmt.Errorf("resolve Git common directory: %w", err)
	}
	remoteNames, err := r.Git(ctx, root, "remote")
	if err != nil {
		return RepositoryIdentity{}, fmt.Errorf("list remotes: %w", err)
	}
	id := RepositoryIdentity{CommonDir: filepath.Clean(strings.TrimSpace(string(common)))}
	for _, name := range strings.Fields(string(remoteNames)) {
		raw, err := r.Git(ctx, root, "remote", "get-url", name)
		if err != nil {
			return RepositoryIdentity{}, fmt.Errorf("read remote %q: %w", name, err)
		}
		normalized, err := NormalizeRemote(strings.TrimSpace(string(raw)))
		if err != nil {
			return RepositoryIdentity{}, fmt.Errorf("normalize remote %q: %w", name, err)
		}
		id.Remotes = append(id.Remotes, normalized)
	}
	sort.Strings(id.Remotes)
	id.Remotes = unique(id.Remotes)
	switch len(id.Remotes) {
	case 0:
		id.Classification = "no_remote_requires_registration"
	case 1:
		id.Classification = "remote_identity"
		id.RemoteIdentity = id.Remotes[0]
	default:
		id.Classification = "multiple_distinct_remotes_require_registration"
	}
	return id, nil
}

func NormalizeRemote(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("empty remote")
	}
	if !strings.Contains(raw, "://") {
		if at := strings.LastIndex(raw, "@"); at >= 0 {
			raw = raw[at+1:]
		}
		parts := strings.SplitN(raw, ":", 2)
		if len(parts) != 2 {
			return "", fmt.Errorf("unsupported remote form")
		}
		return strings.ToLower(parts[0]) + "/" + normalizeRepoPath(parts[1]), nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "", fmt.Errorf("invalid remote URL")
	}
	return strings.ToLower(u.Hostname()) + "/" + normalizeRepoPath(u.Path), nil
}

func normalizeRepoPath(path string) string {
	path = strings.Trim(strings.ReplaceAll(path, "\\", "/"), "/")
	return strings.TrimSuffix(path, ".git")
}

func unique(in []string) []string {
	out := in[:0]
	for _, item := range in {
		if len(out) == 0 || out[len(out)-1] != item {
			out = append(out, item)
		}
	}
	return out
}

func HashEntries(entries []Entry) string {
	h := sha256.New()
	for _, e := range entries {
		fmt.Fprintf(h, "%s\x00%s\x00%s\x00", e.Path, e.Status, e.OldPath)
	}
	return hex.EncodeToString(h.Sum(nil))
}
