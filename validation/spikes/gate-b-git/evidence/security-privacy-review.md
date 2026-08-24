# Gate B security and privacy review

Result: pass for the bounded spike, with L0/L1 follow-ups recorded below.

- Git is executed directly with context cancellation and argument arrays. There is
  no shell concatenation. Full captured object IDs are ASCII-hex validated before
  use; paths are never command arguments in observation queries.
- All name streams use NUL delimiters and `--` terminators. Malicious filenames
  remain inert metadata.
- Repository-relative paths are cleaned, normalized to `/`, and checked against a
  canonicalized root. Existing symlinks and nonexistent terminals beneath symlink
  parents cannot escape the root.
- Remote userinfo, password/token, port, query, and fragment are discarded. Only
  normalized host/path identity inputs survive. Raw remotes must not enter logs or
  hosted payloads.
- The Git common directory is used locally to associate linked worktrees. Its local
  absolute path is sensitive and must not be uploaded; production emits only an
  opaque repository fingerprint bound to explicit registration.
- Fixtures are synthetic temporary repositories. Tests do not read real contributor
  config (`GIT_CONFIG_NOSYSTEM=1`) or mutate the workspace repository.
- Canonical fixtures contain path/status metadata only. Paths are sensitive metadata
  subject to project authorization/retention; prohibited V1 content is absent.

L0/L1 must additionally bound subprocess output/time, decide simultaneous
index/worktree status serialization, hash repository identity with a domain-separated
scheme, redact all error/log fields structurally, and test platform-specific path
rules on Windows and Linux.
