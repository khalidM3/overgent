# L-1 gate matrix

This matrix is the integration record for `docs/prebuild-validation.md`. A gate
is complete only when its evidence supports `pass`, `narrow`, `replace`, or
`block`; scaffolded code alone is not completion.

| Gate | Outcome | Evidence | Remaining boundary |
|---|---|---|---|
| A — Codex adapter | pending | `validation/spikes/gate-a-codex/` | pending |
| B — Git/worktree | pass | `validation/spikes/gate-b-git/evidence/` | L0 encodes simultaneous index/worktree state; L1 adds real watcher/platform coverage and SHA-256 repo fixture |
| C — Convex shared state/vector | pass | `validation/spikes/gate-c-convex/` | Hosted cost/load, authenticated server-derived scope, indexed cleanup, and real embedding dimensions remain L2/L6 work |
| D — install/local service | pass on macOS arm64; narrowed elsewhere | `validation/spikes/gate-d-service/` | Linux/Windows compile only; native service, credential, and IPC validation required before advertising support |
| E — intelligence seed | pass | `validation/spikes/gate-e-intelligence/evidence.md` | Production provider/threshold selection remains L6; proactive semantic alerts stay disabled |

Production L0 is prohibited until every `pending` cell is replaced by an honest
terminal outcome and the combined exit rule is reviewed.
