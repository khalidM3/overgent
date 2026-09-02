package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// ignoreCacheLimit bounds the memory the post-snapshot check-ignore cache can
// hold. Beyond it, answers are still correct — they just cost one git
// invocation each.
const ignoreCacheLimit = 4096

// Ignores answers whether Git ignores a path under one repository root, so the
// file watcher can prune ignored trees instead of watching them. On macOS the
// watcher's kqueue backend pins an open file descriptor for every watched
// directory and every file inside one for the life of the watch, so watching
// ignored build trees (node_modules and friends) exhausts the system file
// table. Pruning is therefore a resource-correctness requirement, not an
// optimization.
//
// The bulk of the answer is captured once from ls-files at construction. Paths
// that appear after that snapshot — a fresh node_modules materializing during
// an install — are resolved with check-ignore on first sight and cached.
type Ignores struct {
	root string
	// dirs and files hold the snapshot, as slash-separated repository-relative
	// paths. dirs are entirely-ignored directories; anything under one is
	// ignored too.
	dirs  map[string]bool
	files map[string]bool

	mu    sync.Mutex
	cache map[string]bool
}

// NewIgnores snapshots the ignored paths of the repository at root. root must
// be absolute, as the watcher hands absolute paths back for matching.
func NewIgnores(ctx context.Context, r Runner, root string) (*Ignores, error) {
	out, err := r.run(ctx, root, "ls-files", "-z", "--others", "--ignored", "--exclude-standard", "--directory")
	if err != nil {
		return nil, err
	}
	ig := &Ignores{root: root, dirs: map[string]bool{}, files: map[string]bool{}, cache: map[string]bool{}}
	for _, p := range split0(out) {
		if rest, isDir := strings.CutSuffix(p, "/"); isDir {
			ig.dirs[rest] = true
		} else {
			ig.files[p] = true
		}
	}
	return ig, nil
}

// relative maps an absolute path to its slash-separated repository-relative
// form. Paths outside the repository report ok=false and are never ignored.
func (ig *Ignores) relative(path string) (string, bool) {
	rel, err := filepath.Rel(ig.root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// Prune reports whether the snapshot recorded path as ignored. It never runs
// git, so it is safe to call once per directory of a large tree walk.
func (ig *Ignores) Prune(path string) bool {
	rel, ok := ig.relative(path)
	if !ok || rel == "." {
		return false
	}
	if ig.files[rel] {
		return true
	}
	for probe := rel; ; {
		if ig.dirs[probe] {
			return true
		}
		parent := pathDir(probe)
		if parent == probe {
			return false
		}
		probe = parent
	}
}

// Ignored reports whether Git ignores path, consulting the snapshot first and
// falling back to check-ignore for paths the snapshot has never seen. Use it
// for paths created after construction; the verdict is cached.
func (ig *Ignores) Ignored(ctx context.Context, path string) bool {
	if ig.Prune(path) {
		return true
	}
	rel, ok := ig.relative(path)
	if !ok || rel == "." {
		return false
	}
	ig.mu.Lock()
	verdict, cached := ig.cache[rel]
	ig.mu.Unlock()
	if cached {
		return verdict
	}
	verdict = checkIgnore(ctx, ig.root, rel)
	ig.mu.Lock()
	if len(ig.cache) < ignoreCacheLimit {
		ig.cache[rel] = verdict
	}
	ig.mu.Unlock()
	return verdict
}

// checkIgnore asks git whether one path is ignored. check-ignore -q exits 0
// for ignored, 1 for not ignored, and anything else for failure; failure
// degrades to "not ignored" so a broken repository never silently blinds the
// watcher to real paths.
func checkIgnore(ctx context.Context, root, rel string) bool {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "check-ignore", "-q", "--", rel)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	return cmd.Run() == nil
}

// pathDir is filepath.Dir for slash-separated relative paths.
func pathDir(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return p
}
