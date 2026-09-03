# Overgent — Pre-build Validation Gates

Status: required before production implementation  
Last updated: 2026-08-23

## 1. Purpose

Run focused, disposable spikes only for assumptions that could invalidate the architecture or V1 product promise. Do not build production UI, abstractions, or generalized frameworks inside a spike. Preserve fixtures, commands, observed wire formats, and a short ADR/result; delete or isolate throwaway code after the decision.

Passing this level means the planned surfaces are feasible. It does not mean every vendor adapter is complete or every product behavior is already reliable.

### Bootstrap prerequisite

Before Gate A, initialize the workspace as a Git repository and preserve the reviewed documentation as the baseline commit. Verify the contributor toolchain rather than assuming it: the primary machine needs the documented Go 1.26 baseline, Git, Node/Corepack/pnpm for the hosted layers, and an installed Codex build for Gate A. Do not begin parallel agent lanes while the specification is unversioned or the Go compiler is unavailable.

After bootstrap, Gates A, B, D, and E may run in parallel in isolated spike directories/worktrees. Gate C may run alongside them once its synthetic scope/fixture vocabulary is fixed. One integrator owns shared fixture schemas, ADR acceptance, and cleanup; spike agents must not redefine production contracts independently.

## 2. Gate A — Codex coordination adapter

Use the locally installed Codex build and official current documentation. Prove:

1. A minimal Overgent Go stdio MCP server can be configured at project scope and discovered by Codex CLI and the desktop app.
2. MCP initialization instructions are read, the first 512 characters contain the critical workflow, and Codex can call fixture versions of `begin_work`, `check_coordination`, `report_checkpoint`, and `finish_work`.
3. MCP exit does not own/terminate the per-user service; two Codex clients can connect without duplicating observers or workstream events.
4. Workspace/workstream resolution from `cwd` is correct and ambiguous registrations fail explicitly.
5. `SessionStart` can inject a bounded fixture `CoordinationBrief` on startup/resume/compaction through an explicitly reviewed project hook.
6. `SubagentStart` feasibility is recorded so subagents either receive relevant context or are honestly labeled unsupported.
7. Hook failure, service absence, stale brief, timeout, and oversized output degrade visibly without blocking Codex unexpectedly.
8. Setup/status/removal is idempotent and never overwrites unrelated user/project MCP or hook configuration.

Use `codex exec --json` only in the test harness to assert lifecycle/tool-call events. Do not make JSONL monitoring, transcript parsing, or Codex App Server ownership part of the installed V1 architecture.

### Privacy/control constraint

V1 may use `SessionStart`/`SubagentStart` for context delivery because their useful routing fields do not require reading repository content or prompts. Do not install `UserPromptSubmit`, transcript-reading, `PreToolUse`, `PostToolUse`, `Stop`, or `SessionEnd` collection by default during this gate.

Current Codex tool/stop hooks can expose prompt, tool arguments/results, command lines, patches, transcripts, or assistant messages and may block/continue execution. Any production use beyond bounded context delivery requires a separate adapter privacy/control ADR, explicit consent, public collection code, fixture-only testing, and proof that prohibited fields are neither retained nor transmitted. V1 remains advisory.

Exit evidence: supported Codex version/range, exact project config/hook shape, captured redacted MCP/JSONL events, automated fixture assertion, install/remove diff, privacy review, and ADR accepting or narrowing the Codex adapter.

## 3. Gate B — Git/worktree observation

Build one narrow Go spike around the real Git CLI. Without changing repository state, prove the manifest model across:

- uncommitted, staged, untracked, renamed, deleted, and ignored paths;
- commits made after a captured workstream baseline, including a clean worktree afterward;
- branch switch, rebase/non-ancestor baseline, detached `HEAD`, worktrees, no remote, and multiple remotes;
- 1,000-path fixture chunking/hash/atomic replacement;
- watcher overflow/full rescan and rapid edit coalescing; and
- path normalization, malicious names, symlink escape, and repository identity.

Record commands, latency/resource measurements, failure classifications, and canonical fixture outputs. Exit only when the V1 manifest contract can represent locally committed work before push without source/diff upload.

## 4. Gate C — Convex shared-state and semantic feasibility

Use a disposable development deployment and synthetic data. Prove:

- two simulated devices publish and subscribe to project state with expected realtime behavior;
- transactional event deduplication, manifest chunk activation, and monotonic repository-scope context revision;
- mandatory composite project/repository vector scope plus post-retrieval authorization/current-state reload;
- vector insertion, immediate retrieval, update/supersession, deletion, and embedding-model version migration;
- bounded retry or structural fallback when semantic retrieval races a state change;
- retention/deletion removes objects, vectors, findings, and delivery receipts; and
- rate/size limits support the 1,000-path fixture without a single oversized mutation.

Use synthetic summaries only. Exit with a cost/limit note, isolation tests, failure behavior, and an ADR accepting Convex or selecting the already-defined portable fallback boundary.

## 5. Gate D — Installation and local-service feasibility

Build a skeletal Go executable, not the application. Prove on the primary development OS first:

- one executable can run CLI, service, and stdio MCP modes;
- single-instance lock plus health-checked current-user IPC;
- pure-Go SQLite persistence and restart recovery;
- OS credential-store access with no silent plaintext fallback;
- user service install/start/status/stop/remove without a language runtime; and
- release cross-compilation succeeds for the planned targets, with unsupported platform-specific behavior identified.

Signing, updater UX, and full installers remain later implementation work. This gate decides whether any promised one-download/runtime-free behavior needs an early ADR adjustment.

## 6. Gate E — Intelligence evaluation seed

Before enabling semantic notifications, create a small versioned synthetic corpus containing:

- same capability under unrelated paths/names;
- incompatible intent before edits;
- shared schema/API/package impact;
- large independent mechanical changes;
- semantically related but non-conflicting work;
- stale/completed workstreams; and
- an unrelated fourth workstream that must receive no context.

The gate does not choose a winner from intuition. Record baseline structural/lexical results, embedding candidates, expected finding/routing labels, and false positives. This seed becomes the permanent public eval harness expanded during L6.

## 7. Go/no-go rule

After each gate:

- **Pass:** record evidence and continue immediately.
- **Narrow:** update the adapter capability/fidelity contract and continue without pretending support.
- **Replace:** write a superseding ADR at the portable boundary, then continue.
- **Block:** only when the V1 promise cannot be met through an existing fallback boundary.

Do not wait for every editor/agent vendor. Codex is the first high-fidelity adapter; Git/manual fidelity and the public MCP contract keep the core build unblocked.
