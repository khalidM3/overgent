# Codex task specs — 2026-09-01

Written so implementation can happen without a strong-model diagnosis loop. Every task
below has a **falsifiable acceptance test**: it must fail before the change and pass after.
If a task's test cannot be made to fail first, stop and report that — it means the defect
is not where this spec says it is.

Evidence for all of these: `validation/evidence/manual-corpus-2026-08-31.md`.

---

## Standing system prompt for Codex on this repository

> You are working in the Stickguy repository, a multi-agent coordination tool. Go service
> in `internal/`, hosted backend in `convex/functions/` and `convex/src/`, shared
> coordination logic in `packages/coordination/`.
>
> Rules for every task:
> 1. **Write the failing test first.** Before changing behaviour, add a test that fails for
>    the reason described in the task. Show me the failure output. If it passes before your
>    change, stop — the diagnosis is wrong and I need to know.
> 2. **Do not widen scope.** Change only what the task names. If you find an adjacent bug,
>    write it down and keep going; do not fix it.
> 3. **`convex/functions/**` is not covered by `pnpm typecheck`** (only `src/**` and
>    `test/**` are). Do not assume a clean typecheck means your change compiles there.
> 4. **Editing `packages/coordination/` hot-redeploys the live backend**, because
>    `convex/functions` imports it from source and `convex dev` watches it. Never edit it
>    while an evaluation round is running.
> 5. State plainly what you verified and what you did not. Do not report success on the
>    basis of a test you did not run.
>
> Run tests with:
> - `cd packages/coordination && ./node_modules/.bin/vitest run`
> - `cd convex && ./node_modules/.bin/vitest run`
> - `go test ./internal/...`
>
> Node 22 is required: `export PATH="$HOME/.nvm/versions/node/v22.23.2/bin:$PATH"`.

---

## Task 1 — B33: vector search cannot exclude a foreign embedding model

**Severity: HIGH. Mechanical. Start here.**

`convex/functions/schema.ts` declares:

```ts
.vectorIndex("by_vector", { vectorField: "vector", dimensions: 1024, filterFields: ["scopeKey"] })
```

`modelVersion` is not a filter field, so `ctx.vectorSearch` in
`convex/functions/intelligence.ts` (`searchSemantic`) cannot exclude vectors produced by a
different embedding provider. A `by_scope_model` index on `["scopeKey","modelVersion"]`
already exists, so the intent was there — the vector path just cannot use it.

Measured on the local backend: three incompatible populations coexist —
`stickguy-concepts/v1` at **32 dims** (126 rows), `stickguy-concepts/v1` at **1024 dims**
(39 rows), and `openai/text-embedding-3-large` at 1024 dims. Cosine similarity across
unrelated embedding spaces is noise, not a weak signal.

**Change:**
1. Add `modelVersion` to `filterFields` on the `by_vector` vector index.
2. In `searchSemantic`, filter on both `scopeKey` **and** the `modelVersion` of the query
   vector (the querying object's own embedding, from `loadContext`). Return the query
   vector's `modelVersion` from `loadContext` alongside the vector.
3. Add a retention/migration step that deletes `semanticEmbeddings` rows whose
   `modelVersion` differs from the currently-configured provider for that scope.

**Acceptance test** (`convex/test/`): seed one scope with two embeddings that have the same
`scopeKey` but different `modelVersion`, run the search path, and assert the result never
contains the foreign-model row. Verify it fails before step 1–2.

---

## Task 2 — B34/B32: degradation is recorded without a reason

**Severity: HIGH. This is the prerequisite for the status banner.**

Three sites discard the cause of a degradation and keep only a boolean or nothing:

| site | what is lost |
|---|---|
| `internal/app/app.go:219` `flush()` | why the event queue stopped publishing |
| `convex/functions/intelligence.ts` `claimJudgmentBudget` caller (~:224) | that the hourly judgment budget bound — records **nothing** |
| `convex/functions/intelligence.ts` `embedSemanticObject` catch (~:102) | why an embedding failed |

The third has already been given a `console.error("openai_embedding_failed", …)` probe —
keep it, but promote it to stored state as below.

**Change:**
1. Add to `repositoryScopes`: `semanticDegradedReason` and `judgmentDegradedAt` /
   `judgmentDegradedReason` / `judgmentProviderName`. Judgment state must be **separate**
   from embedding state — today `recordJudgmentDegraded` writes to the same
   `semanticDegradedAt` field, so an Anthropic failure is indistinguishable from an OpenAI
   one.
2. Record a reason at each site, from this closed set:
   `not_configured | quota | provider_error | offline | paused`.
3. On budget exhaustion specifically, record `quota` **and** a `recoversAt` derived from
   the existing rate-limit window (`JUDGMENT_BUDGET_WINDOW`), so the UI can say when it
   returns.
4. In `internal/app/app.go`, `flush()` must not discard the send error — log it and keep a
   last-error string reachable from `doctor`/`diagnostics`.

**Acceptance test:** a convex test that exhausts the judgment budget and asserts the scope
records `quota` with a future `recoversAt`; and a Go test asserting `doctor` surfaces a
non-empty last-publish-error after a failing send. Both must fail first.

**Do not build the banner UI in this task.** Only the state.

---

## Task 3 — B35: sessions are warned about contract changes they made themselves

**Severity: HIGH. Investigate before changing — the obvious fix is already present.**

Observed twice (rounds 26 and 27): an agent session created `backend/security.go` /
`backend/revoke.go` and then received its own `stale_assumption` at **high / next_turn**:

> `backend/revoke.go: AuditEntry changed after this session said it expected to read it`

`convex/functions/service.ts:1688` **already** excludes the changer:

```ts
if (reader.workstreamPublicId === change.changedByWorkstreamPublicId) continue;
```

So the exclusion did not fire, which means `change.changedByWorkstreamPublicId` was a
different workstream from the agent that actually wrote the file. **Hypothesis (not yet
confirmed): git-observed contract changes are attributed to the workspace/git workstream
rather than to the agent session whose edit produced them.** That is the same root cause as
B29, where a member without an adapter gets no path attribution at all.

**Step 1 (required first):** determine what `changedByWorkstreamPublicId` actually holds for
one of these findings, and report it before changing anything. Trace where contract
fingerprint changes are attributed on the way in.

**Step 2:** if the hypothesis holds, attribute a contract change to the agent session that
wrote the path when one is known, falling back to the workspace workstream only when no
agent session claimed that path. Do not simply widen the exclusion at line 1688 — that
hides the attribution bug instead of fixing it, and B29 depends on the same attribution.

**Acceptance test:** a convex test where one agent session both declares an anticipated read
of a path and authors the change to it, asserting **no** `stale_assumption` targets that
session. Must fail first.

---

## Verification protocol (run these yourself after Codex reports done)

For each task, in order:

1. `git diff --stat` — confirm only the files the task names were touched.
2. Re-run the acceptance test **with the change reverted** (`git stash`) and confirm it
   fails; unstash and confirm it passes. This is the step that catches a test written to
   pass rather than to prove.
3. `cd packages/coordination && ./node_modules/.bin/vitest run` — 41 tests must pass.
4. `cd convex && ./node_modules/.bin/vitest run` — 50 tests must pass (plus new ones).
5. `go test ./internal/...` for Task 2.
6. `./bin/stickguy doctor` — must return `{"ok":true,...}` with `pending: 0`.

If step 2 cannot be made to fail, the change is not doing what the task claims. Report that
rather than accepting it.
