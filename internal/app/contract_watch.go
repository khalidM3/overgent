package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/stickguy/stickguy/internal/agentactivity"
	"github.com/stickguy/stickguy/internal/config"
	"github.com/stickguy/stickguy/internal/contract"
	git "github.com/stickguy/stickguy/internal/git"
	"github.com/stickguy/stickguy/internal/store"
)

// contractEntriesPerEvent and readSetEntriesPerEvent match the bounds the wire
// contract declares for one event, so a large scan chunks the same way manifest
// publication does instead of producing one oversized event.
const (
	contractEntriesPerEvent = 20
	readSetEntriesPerEvent  = 100
)

// readPathsPerEvent bounds the on-demand hashing done while an agent turn is in
// flight. Observation must never delay the coding agent (ADR-017).
const readPathsPerEvent = 20

// publishContractFingerprints derives the exported surface of the changed
// fingerprintable paths in a manifest and publishes only the files whose
// contract actually moved. A file that cannot be read or parsed simply has no
// fingerprint; extraction never fails manifest publication.
func (s *Service) publishContractFingerprints(ctx context.Context, workspace config.Workspace, entries []git.Entry) {
	observed := map[string]string{}
	files := map[string]contract.File{}
	for _, entry := range entries {
		if !contract.Fingerprintable(entry.Path) {
			continue
		}
		file, ok := s.fingerprint(workspace.Root, entry.Path)
		if !ok {
			continue
		}
		observed[entry.Path] = file.FileContractHash
		files[entry.Path] = file
	}
	changed, err := s.store.ChangedFingerprints(ctx, workspace.ID, observed, time.Now())
	if err != nil || len(changed) == 0 {
		return
	}
	for len(changed) > 0 {
		window := changed[:min(contractEntriesPerEvent, len(changed))]
		changed = changed[len(window):]
		payloadEntries := make([]contract.File, 0, len(window))
		for _, path := range window {
			payloadEntries = append(payloadEntries, files[path])
		}
		payload := map[string]any{"workspaceId": workspace.ID, "entries": payloadEntries}
		if err := s.store.EnqueueEvent(ctx, workspace.ID, newID("evt_"), "git", "workspace.contract_fingerprints_reported", payload); err != nil {
			return
		}
	}
}

// fingerprint extracts one repository-relative path. The path has already been
// validated as repository-relative by the manifest or activity pipeline; it is
// re-joined and re-checked here so a fingerprint can never escape the workspace.
func (s *Service) fingerprint(root, path string) (contract.File, bool) {
	if strings.Contains(path, "..") || filepath.IsAbs(path) {
		return contract.File{}, false
	}
	absolute := filepath.Join(root, filepath.FromSlash(path))
	if relative, err := filepath.Rel(root, absolute); err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return contract.File{}, false
	}
	info, err := os.Lstat(absolute)
	if err != nil || !info.Mode().IsRegular() || info.Size() > contract.MaxSourceBytes {
		return contract.File{}, false
	}
	source, err := os.ReadFile(absolute)
	if err != nil {
		return contract.File{}, false
	}
	return contract.Extract(path, source, agentactivity.ProhibitedContractSignature)
}

// repositoryRelative resolves an observed candidate to a safe
// repository-relative path, dropping anything that escapes the workspace or
// names a protected credential path. Callers whose paths are already
// normalized pass through it unchanged.
func repositoryRelative(root, candidate string) (string, bool) {
	if candidate == "" || strings.ContainsRune(candidate, '\x00') {
		return "", false
	}
	absolute := candidate
	if !filepath.IsAbs(absolute) {
		absolute = filepath.Join(root, filepath.FromSlash(candidate))
	}
	absolute = filepath.Clean(absolute)
	relative, err := filepath.Rel(root, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	relative = filepath.ToSlash(relative)
	if !agentactivity.SafeRepositoryPath(relative) {
		return "", false
	}
	return relative, true
}

// publishReadSet records which fingerprintable paths a session observed and the
// file contract hash current at that moment, then publishes only the entries
// that are new or whose hash moved. The hash is read from disk so it describes
// what the session actually saw, falling back to the last recorded fingerprint
// when the file is no longer readable.
func (s *Service) publishReadSet(ctx context.Context, workspace config.Workspace, sessionWorkstreamID string, candidates []string) {
	if sessionWorkstreamID == "" {
		return
	}
	observedAt := time.Now().UTC().Format(time.RFC3339Nano)
	entries := make([]store.ReadSetEntry, 0, len(candidates))
	for _, candidate := range candidates {
		path, ok := repositoryRelative(workspace.Root, candidate)
		if !ok || !contract.Fingerprintable(path) {
			continue
		}
		if len(entries) >= readPathsPerEvent {
			break
		}
		hash := ""
		if file, ok := s.fingerprint(workspace.Root, path); ok {
			hash = file.FileContractHash
		} else if cached, found, err := s.store.FingerprintHash(ctx, workspace.ID, path); err == nil && found {
			hash = cached
		}
		if hash == "" {
			continue
		}
		entries = append(entries, store.ReadSetEntry{Path: path, FileContractHashAtRead: hash, ObservedAt: observedAt})
	}
	changed, err := s.store.ChangedReadSet(ctx, workspace.ID, sessionWorkstreamID, entries)
	if err != nil || len(changed) == 0 {
		return
	}
	for len(changed) > 0 {
		window := changed[:min(readSetEntriesPerEvent, len(changed))]
		changed = changed[len(window):]
		payload := map[string]any{
			"workspaceId": workspace.ID, "sessionWorkstreamId": sessionWorkstreamID, "entries": window,
		}
		if err := s.store.EnqueueEvent(ctx, workspace.ID, newID("evt_"), "hook", "session.read_set_reported", payload); err != nil {
			return
		}
	}
}
