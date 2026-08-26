# V2 task briefs — orchestration guide

These briefs implement the V2 reboot (ADR-044 … ADR-048, implementation plan
M1–M6). They are written so an implementing agent needs **zero architectural
judgment calls**: every contract, boundary, and acceptance criterion is stated.
If a brief is ambiguous or conflicts with the code you find, STOP and report
the conflict in your handoff instead of deciding yourself.

## Division of labor

| Lane | Brief | Suggested executor | Depends on |
|------|-------|--------------------|------------|
| A | `lane-a-eval-harness.md` (M1) | Opus | nothing |
| B | `lane-b-contract-watch.md` (M2) | Opus | schema section below |
| C | `lane-c-brief-injection.md` (M3) | Opus | nothing (integrates with B at the end) |
| D | `lane-d-simplify-sharing.md` (M6) | Codex | nothing |

M4 (LLM judgment) and M5 (dependency readiness) are follow-on work after
A–D land; do not start them from these briefs.

## Rules for every lane

1. **Worktree isolation.** Each lane works in its own linked worktree and
   branch (`git worktree add ../stickguy-lane-a -b v2/lane-a-eval-harness`).
   Never commit to `main`.
2. **Protocol ownership.** Only Lane B may modify `protocol/openapi.yaml`,
   `protocol/schemas/`, or run `pnpm protocol:generate`. Every other lane that
   needs a new wire shape must use the shapes defined in Lane B's brief and
   rebase on Lane B once it lands. Do not hand-edit anything in
   `protocol/generated/`.
3. **Read before writing.** Each brief lists files to read first. Read them.
   Match existing package conventions, error handling, and test style.
4. **Verification before handoff.** `go test ./...`, `go vet ./...`,
   `pnpm typecheck`, `pnpm test`, `pnpm build` must pass in your worktree.
   If your lane touched protocol: `pnpm protocol:check`.
5. **Handoff format.** End with a summary containing: behavior delivered vs.
   the brief's acceptance criteria; files/contracts changed; commands run and
   their results; known limitations; anything the brief got wrong about the
   codebase.
6. **Scope discipline.** Do not refactor beyond the brief. Do not "improve"
   neighboring code. Flag it in the handoff instead.

## Integration order

Lane D (deletion) and Lane A (new isolated suite) merge first — they are low
conflict. Lane B merges next (owns protocol). Lane C rebases on B and merges
last. The integrator (Fable session) reviews merges and runs the M1 suite as
the final gate.
