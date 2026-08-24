# Stickguy

Stickguy is a persistent coordination harness for teams building software with coding agents. It acts as air traffic control around existing Codex, Claude, Cursor, and other coding harnesses: combining live Git evidence, reported intent, and semantic coordination intelligence, then routing only relevant findings and decisions to each workstream before merge time.

The repository is at specification/scaffold stage. Start with [`docs/README.md`](docs/README.md) and [`AGENTS.md`](AGENTS.md).

Core decisions: persistent Projects; standalone Go local core; one service per user; React dashboard and Convex backend; deterministic evidence plus V1 semantic coordination over bounded summaries; no raw transcript, system-prompt, diff, or source-content collection in V1. The intended trust model publishes all installed/collection code and core hosted coordination code while isolating private cloud operations in a separate repository.

Implementation follows [`docs/implementation-plan.md`](docs/implementation-plan.md).
