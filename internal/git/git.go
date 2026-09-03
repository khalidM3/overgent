package git

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
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

type Runner struct{}

func (Runner) run(ctx context.Context, root string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	var er bytes.Buffer
	cmd.Stderr = &er
	out, e := cmd.Output()
	if e != nil {
		return nil, fmt.Errorf("git operation: %w: %s", e, strings.TrimSpace(er.String()))
	}
	return out, nil
}

type Change struct {
	Status  string `json:"status"`
	OldPath string `json:"oldPath,omitempty"`
}
type States struct {
	Baseline *Change `json:"baseline,omitempty"`
	Index    *Change `json:"index,omitempty"`
	Worktree *Change `json:"worktree,omitempty"`
}
type Entry struct {
	Path   string `json:"path"`
	States States `json:"states"`
}
type Manifest struct {
	Baseline, Head, BaselineState string
	Entries                       []Entry
}

func CaptureBaseline(ctx context.Context, r Runner, root string) (string, error) {
	b, e := r.run(ctx, root, "rev-parse", "--verify", "HEAD^{commit}")
	if e != nil {
		return "", fmt.Errorf("capture baseline: %w", e)
	}
	return strings.TrimSpace(string(b)), nil
}

// CurrentBranch reports the checked-out branch name for a worktree. A detached
// HEAD has no branch, which is reported as an empty name rather than an error so
// session observation keeps working. The name is bounded and validated because
// it becomes shared coordination metadata.
func CurrentBranch(ctx context.Context, r Runner, root string) (string, error) {
	out, err := r.run(ctx, root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		// Detached HEAD exits non-zero with --quiet; that is not a failure.
		return "", nil
	}
	branch := strings.TrimSpace(string(out))
	if !validBranchName(branch) {
		return "", nil
	}
	return branch, nil
}

// validBranchName keeps only names Git itself accepts and that are safe to
// render as shared metadata; anything unusual degrades to no branch.
func validBranchName(name string) bool {
	if name == "" || len(name) > 255 || strings.HasPrefix(name, "-") {
		return false
	}
	for _, r := range name {
		if r < 0x21 || r == 0x7f || strings.ContainsRune(" ~^:?*[\\", r) {
			return false
		}
	}
	return !strings.Contains(name, "..") && !strings.Contains(name, "@{") && !strings.HasSuffix(name, ".lock")
}

func Observe(ctx context.Context, r Runner, root, baseline string) (Manifest, error) {
	if !validOID(baseline) {
		return Manifest{}, errors.New("invalid baseline object ID")
	}
	head, e := r.run(ctx, root, "rev-parse", "--verify", "HEAD^{commit}")
	if e != nil {
		return Manifest{}, e
	}
	m := Manifest{Baseline: baseline, Head: strings.TrimSpace(string(head)), BaselineState: "ancestor"}
	if _, e = r.run(ctx, root, "cat-file", "-e", baseline+"^{commit}"); e != nil {
		m.BaselineState = "missing"
	}
	if m.BaselineState != "missing" {
		cmd := exec.CommandContext(ctx, "git", "-C", root, "merge-base", "--is-ancestor", baseline, "HEAD")
		cmd.Env = append(os.Environ(), "LC_ALL=C", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
		if e := cmd.Run(); e != nil {
			m.BaselineState = "diverged_non_ancestor"
		}
	}
	by := map[string]Entry{}
	if m.BaselineState != "missing" {
		b, e := r.run(ctx, root, "diff", "--name-status", "-z", "-M", baseline, "HEAD", "--")
		if e != nil {
			return m, e
		}
		if e = merge(root, by, b, "baseline"); e != nil {
			return m, e
		}
	}
	for _, layer := range []struct {
		name string
		args []string
	}{{"worktree", []string{"diff", "--name-status", "-z", "-M", "--"}}, {"index", []string{"diff", "--cached", "--name-status", "-z", "-M", "--"}}} {
		b, e := r.run(ctx, root, layer.args...)
		if e != nil {
			return m, e
		}
		if e = merge(root, by, b, layer.name); e != nil {
			return m, e
		}
	}
	b, e := r.run(ctx, root, "ls-files", "--others", "--exclude-standard", "-z", "--")
	if e != nil {
		return m, e
	}
	for _, p := range split0(b) {
		p, e = normalize(root, p)
		if e != nil {
			return m, e
		}
		entry := by[p]
		entry.Path = p
		entry.States.Worktree = &Change{Status: "untracked"}
		by[p] = entry
	}
	for _, v := range by {
		m.Entries = append(m.Entries, v)
	}
	sort.Slice(m.Entries, func(i, j int) bool { return m.Entries[i].Path < m.Entries[j].Path })
	return m, nil
}
func merge(root string, d map[string]Entry, b []byte, layer string) error {
	f := split0(b)
	for i := 0; i < len(f); {
		c := f[i]
		i++
		if c == "" || i >= len(f) {
			return errors.New("malformed git status")
		}
		change := Change{Status: status(c[0])}
		var p string
		var e error
		if c[0] == 'R' || c[0] == 'C' {
			if i+1 >= len(f) {
				return errors.New("malformed rename")
			}
			old, e := normalize(root, f[i])
			if e != nil {
				return e
			}
			p, e = normalize(root, f[i+1])
			if e != nil {
				return e
			}
			i += 2
			change.OldPath = old
		} else {
			p, e = normalize(root, f[i])
			if e != nil {
				return e
			}
			i++
		}
		entry := d[p]
		entry.Path = p
		switch layer {
		case "baseline":
			entry.States.Baseline = &change
		case "index":
			entry.States.Index = &change
		case "worktree":
			entry.States.Worktree = &change
		default:
			return errors.New("invalid change layer")
		}
		d[p] = entry
	}
	return nil
}
func split0(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	p := bytes.Split(b, []byte{0})
	if len(p[len(p)-1]) == 0 {
		p = p[:len(p)-1]
	}
	o := make([]string, len(p))
	for i := range p {
		o[i] = string(p[i])
	}
	return o
}
func status(c byte) string {
	switch c {
	case 'A':
		return "added"
	case 'D':
		return "deleted"
	case 'R':
		return "renamed"
	case 'C':
		return "copied"
	default:
		return "modified"
	}
}
func normalize(root, raw string) (string, error) {
	if raw == "" || !utf8.ValidString(raw) || filepath.IsAbs(raw) || strings.IndexByte(raw, 0) >= 0 {
		return "", errors.New("invalid relative path")
	}
	clean := filepath.Clean(filepath.FromSlash(raw))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("path escape")
	}
	abs, e := filepath.Abs(root)
	if e != nil {
		return "", fmt.Errorf("resolve repository root: %w", e)
	}
	if resolved, e := filepath.EvalSymlinks(abs); e == nil {
		abs = resolved
	}
	joined := filepath.Join(abs, clean)
	existing := joined
	for {
		resolved, e := filepath.EvalSymlinks(existing)
		if e == nil {
			rel, relErr := filepath.Rel(abs, resolved)
			if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return "", errors.New("symlink escape")
			}
			break
		}
		if !os.IsNotExist(e) {
			return "", e
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			break
		}
		existing = parent
	}
	normalized := filepath.ToSlash(clean)
	if utf8.RuneCountInString(normalized) > 512 {
		return "", errors.New("path exceeds manifest limit")
	}
	return normalized, nil
}
func validOID(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}
func Hash(entries []Entry) (string, error) {
	h := sha256.New()
	for i, e := range entries {
		if i > 0 && entries[i-1].Path >= e.Path {
			return "", errors.New("manifest paths must be strictly ordered and unique")
		}
		fmt.Fprintf(h, "%s\x00", e.Path)
		for _, layer := range []struct {
			name  string
			state *Change
		}{{"baseline", e.States.Baseline}, {"index", e.States.Index}, {"worktree", e.States.Worktree}} {
			status, oldPath := "", ""
			if layer.state != nil {
				status, oldPath = layer.state.Status, layer.state.OldPath
			}
			fmt.Fprintf(h, "%s\x00%s\x00%s\x00", layer.name, status, oldPath)
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func Fingerprint(ctx context.Context, r Runner, root, projectID string) (string, error) {
	if !regexp.MustCompile(`^prj_[a-z0-9_]{1,80}$`).MatchString(projectID) {
		return "", errors.New("invalid Project ID")
	}
	names, e := r.run(ctx, root, "remote")
	if e != nil {
		return "", e
	}
	var rem []string
	for _, n := range strings.Fields(string(names)) {
		b, e := r.run(ctx, root, "remote", "get-url", n)
		if e != nil {
			return "", e
		}
		v, e := normalizeRemote(strings.TrimSpace(string(b)))
		if e != nil {
			return "", e
		}
		rem = append(rem, v)
	}
	sort.Strings(rem)
	rem = uniqueStrings(rem)
	if len(rem) == 0 {
		return "", errors.New("repository registration requires exactly one distinct normalized remote; found none")
	}
	if len(rem) != 1 {
		return "", errors.New("repository registration requires explicit selection from multiple distinct remotes")
	}
	raw := "overgent.repository-fingerprint.v1\x00" + projectID + "\x00" + rem[0]
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:]), nil
}

func CommonDir(ctx context.Context, r Runner, root string) (string, error) {
	b, e := r.run(ctx, root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if e != nil {
		return "", e
	}
	return filepath.Clean(strings.TrimSpace(string(b))), nil
}

func uniqueStrings(in []string) []string {
	out := in[:0]
	for _, value := range in {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}
func normalizeRemote(raw string) (string, error) {
	if !strings.Contains(raw, "://") {
		if at := strings.LastIndex(raw, "@"); at >= 0 {
			raw = raw[at+1:]
		}
		p := strings.SplitN(raw, ":", 2)
		if len(p) != 2 {
			return "", errors.New("unsupported remote")
		}
		return strings.ToLower(p[0]) + "/" + strings.TrimSuffix(strings.Trim(p[1], "/"), ".git"), nil
	}
	u, e := url.Parse(raw)
	if e != nil || u.Hostname() == "" {
		return "", errors.New("invalid remote")
	}
	return strings.ToLower(u.Hostname()) + "/" + strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git"), nil
}
