package sessiontranscript

// Cursor has no adapter in this package, and that absence is the finding rather
// than an omission.
//
// Claude Code and Codex both write a line-oriented session record — one JSON
// object per line — which is what Read parses and what ADR-036 lets a member
// read back for their own session. Cursor does not publish an equivalent file
// with a documented, stable, line-oriented shape that Stickguy can point at.
// Writing a speculative parser here would break the first rule in
// docs/adapter-development.md — do not share a guessed record format across
// vendors — and would fail in the worst available way: silently, producing an
// empty or wrong session view that reads as a working local transcript.
//
// The consequence is bounded and worth stating plainly. A Cursor session loses
// exactly one thing relative to Claude: the local "read my own session" view,
// and the transcript-derived title that view supplies. It loses nothing that
// coordination depends on. Lifecycle, tool activity, safe paths, and the read
// set all come from hooks, and Cursor's hooks are the most informative of the
// three vendors — beforeReadFile names the file being read before it is read,
// which is the strongest read evidence any supported vendor provides.
//
// The session title degrades honestly instead of disappearing: Cursor's
// beforeSubmitPrompt hook carries the submitted prompt, and
// agentactivity.ParseCursor runs it through ClassifyCoordinationTitle (ADR-042)
// so a short prompt becomes a bounded, classifier-approved title. A prompt over
// 160 characters yields no title at all rather than a truncation presented as a
// name.
//
// If Cursor later documents a readable session record, an adapter belongs beside
// claudeAdapter and codexAdapter, with detect() taught its shape.

// TranscriptAvailable reports whether a vendor writes a session record this
// package can parse. Callers use it to skip a read that can only fail, so an
// absent transcript is a known vendor limitation rather than a read error that
// looks like a broken file.
func TranscriptAvailable(vendor string) bool {
	switch vendor {
	case "claude", "codex":
		return true
	}
	return false
}
