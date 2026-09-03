package manifest

import git "github.com/khalidM3/overgent/internal/git"

func Chunk(entries []git.Entry, size int) [][]git.Entry {
	if size <= 0 {
		panic("positive chunk size required")
	}
	var out [][]git.Entry
	for len(entries) > 0 {
		n := min(size, len(entries))
		out = append(out, append([]git.Entry(nil), entries[:n]...))
		entries = entries[n:]
	}
	return out
}
