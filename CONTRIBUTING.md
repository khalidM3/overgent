# Contributing to Stickguy

Stickguy is still establishing its first public implementation. Read
`AGENTS.md` and every document in `docs/README.md` order before changing code.

Use focused changes with tests. Data-flow, protocol, authentication, privacy,
installer, and update changes require explicit security review. Never use real
credentials, contributor state, customer data, source/diff content, transcripts,
prompts, environment values, or raw command/test output in fixtures.

Required checks are listed in `AGENTS.md`; do not document a no-op check. Use
temporary repositories and config roots in tests. Git subprocesses take argument
arrays, local servers bind only to loopback/current-user IPC, and generated
protocol code is changed only through `pnpm protocol:generate`.

Contributions use Developer Certificate of Origin sign-off (`Signed-off-by`) as
recommended by the reviewed open-source strategy. Contributions intentionally
submitted for inclusion are accepted under the repository's Apache-2.0 terms.
