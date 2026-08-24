# L-1 gate matrix

This matrix is the integration record for `docs/prebuild-validation.md`. A gate
is complete only when its evidence supports `pass`, `narrow`, `replace`, or
`block`; scaffolded code alone is not completion.

| Gate | Outcome | Evidence | Remaining boundary |
|---|---|---|---|
| A — Codex adapter | narrow | `validation/spikes/gate-a-codex/` | Official-SDK stdio MCP accepted for Codex 0.148.0-alpha.15; hooks remain `available_but_unverified`, desktop GUI and structured config merge/removal require L5 proof |
| B — Git/worktree | pass | `validation/spikes/gate-b-git/evidence/` | L0 encodes simultaneous index/worktree state; L1 adds real watcher/platform coverage and SHA-256 repo fixture |
| C — Convex shared state/vector | pass | `validation/spikes/gate-c-convex/` | Hosted cost/load, authenticated server-derived scope, indexed cleanup, and real embedding dimensions remain L2/L6 work |
| D — install/local service | pass on macOS arm64; narrowed elsewhere | `validation/spikes/gate-d-service/` | Linux/Windows compile only; native service, credential, and IPC validation required before advertising support |
| E — intelligence seed | pass | `validation/spikes/gate-e-intelligence/evidence.md` | Production provider/threshold selection remains L6; proactive semantic alerts stay disabled |

All cells now have an honest terminal outcome. The combined exit rule passes:
no result requires replacement of Go, the Stickguy protocol, the manifest model,
Project isolation, or the coordination-harness lifecycle. This completes L-1;
it does not start production L0.
