package gatebgit

import (
	"fmt"
	"sort"
)

func Chunk(entries []Entry, size int) [][]Entry {
	if size <= 0 {
		panic("chunk size must be positive")
	}
	var chunks [][]Entry
	for len(entries) > 0 {
		n := min(size, len(entries))
		chunks = append(chunks, append([]Entry(nil), entries[:n]...))
		entries = entries[n:]
	}
	return chunks
}

type Assembler struct {
	activeRevision int
	active         []Entry
	staged         map[int]map[int][]Entry
}

func NewAssembler() *Assembler { return &Assembler{staged: make(map[int]map[int][]Entry)} }

func (a *Assembler) Stage(revision, index int, entries []Entry) {
	if a.staged[revision] == nil {
		a.staged[revision] = make(map[int][]Entry)
	}
	a.staged[revision][index] = append([]Entry(nil), entries...)
}

func (a *Assembler) Activate(revision, count int, expectedHash string) error {
	staged := a.staged[revision]
	if len(staged) != count {
		return fmt.Errorf("revision %d incomplete: have %d chunks, want %d", revision, len(staged), count)
	}
	var entries []Entry
	for i := range count {
		chunk, ok := staged[i]
		if !ok {
			return fmt.Errorf("revision %d missing chunk %d", revision, i)
		}
		entries = append(entries, chunk...)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	if got := HashEntries(entries); got != expectedHash {
		return fmt.Errorf("revision %d hash mismatch", revision)
	}
	a.activeRevision = revision
	a.active = entries
	delete(a.staged, revision)
	return nil
}

func (a *Assembler) Active() (int, []Entry) {
	return a.activeRevision, append([]Entry(nil), a.active...)
}

type EventKind int

const (
	EventEdit EventKind = iota
	EventOverflow
)

type ScanRequest struct {
	Full    bool
	Reasons int
}

func Coalesce(events []EventKind) ScanRequest {
	request := ScanRequest{Reasons: len(events)}
	for _, event := range events {
		if event == EventOverflow {
			request.Full = true
		}
	}
	return request
}
