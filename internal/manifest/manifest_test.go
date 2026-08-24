package manifest

import (
	"fmt"
	"testing"

	git "github.com/stickguy/stickguy/internal/git"
)

func TestChunkPreservesThousandEntries(t *testing.T) {
	entries := make([]git.Entry, 1000)
	for i := range entries {
		entries[i] = git.Entry{Path: fmt.Sprintf("bulk/%04d.txt", i), States: git.States{Baseline: &git.Change{Status: "added"}}}
	}
	chunks := Chunk(entries, 100)
	if len(chunks) != 10 {
		t.Fatalf("chunks=%d", len(chunks))
	}
	var rebuilt []git.Entry
	for _, chunk := range chunks {
		if len(chunk) > 100 {
			t.Fatalf("oversized chunk=%d", len(chunk))
		}
		rebuilt = append(rebuilt, chunk...)
	}
	if len(rebuilt) != 1000 || git.Hash(rebuilt) != git.Hash(entries) {
		t.Fatal("chunk reconstruction changed manifest")
	}
}
