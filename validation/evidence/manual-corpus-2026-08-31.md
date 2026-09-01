# Stickguy scenario corpus — 2026-08-31
Fixture: ~/stickguy-eval-fixture @ eval-base-v2
Project: prj_49b778cd43b1a6dd7cb47facf051e69b (re-enrolled; old prj_d47289ed… lost when the
closed-test work wiped the default profile config)
Profile: ~/Library/Application Support/Stickguy | backend: local convex 127.0.0.1:3210
Helper: ~/.stickguy-eval/round.sh start|peek <n>
Models (held FIXED for the whole corpus): Claude = Sonnet 5 (Claude Code CLI);
Codex = OpenAI 5.6 Terra (ChatGPT desktop, codex-cli 0.149.0-alpha.4.1).
Rounds 1-9 recorded no model at all, so today's rounds set the baseline rather than
matching one. React-column results are only comparable within this model pair.

## Target: the 14 never-run scenarios + 2 re-runs
| R  | Scenario | Kind | Status |
|----|----------|------|--------|
| 10 | SG-06 re-run (B17 false all-clear) | positive | **RUN — detect/route/deliver/react PASS; wording FAIL (B25)** |
| 11 | SG-02 same file, unrelated regions | silence | **RUN — FAIL (B26); B28 found** |
| 12 | SG-04 body-only change after read | silence | **RUN — PASS** |
| 13 | SG-05 new unrelated export (reader=Codex) | silence | **RUN — PASS (strongest negative)** |
| 14 | SG-08 identifier false neighbour | silence | **RUN — PASS** |
| 15 | SG-11 old brief + irrelevant activity | silence | **RUN — PASS** |
| 16 | SG-03 role-reversed (Codex reads) | positive | |
| 17 | SG-09 dependency stable→ready | positive | |
| 18 | SG-10 provisional contract change | severity | |
| 19 | SG-13 workstream changes scope | staleness | |
| 20 | SG-12 fan-out to exactly 3 of 5 | routing | |
| 21 | SG-14 unsupported/disconnected adapter | fidelity | **RUN — FAIL (B29)** |
| 22 | SG-15 semantic provider outage | degradation | **PASS (ambient condition — see note)** |
| 23 | SG-16 pause is a wire boundary | privacy | **RUN — PASS** |
| 24 | SG-17 backend outage + recovery | reliability | **RUN — PARTIAL (3/4)** |
| 25 | SG-18 resolution reaches sessions once | delivery | **BLOCKED — needs interactive agents** |

## Bugs found today
| # | Issue | Sev | Status |
|---|-------|-----|--------|
| B23 | Re-enrolling a profile against an existing state.db crash-loops the service forever with a raw `UNIQUE constraint failed: workspaces.root (2067)`. No remediation text, service never boots, `doctor` only shows connection refused. | HIGH | open |
| B25 | `sharedBehaviorTerms` (`packages/coordination/src/intelligence.ts:182`) requires **the same literal word** in both summaries: `behaviorWord(word).test(left) && behaviorWord(word).test(right)`. CONCEPT_GROUPS is a set of **synonym groups**, but it is consumed as a flat allowlist, so cross-synonym matches never fire — "credentials" vs "login sessions" (group 0) and "role" vs "privilege" (group 1) both miss. The canonical SG-06 pair therefore yields `[]` and `redundant_work` falls back to the generic "appear to implement the same behavior under different paths", which names nothing actionable. Verified by executing the function on the four real field pairings: all `[]`. B20 fixed the *fallback*; the primary vocabulary path was broken all along and every existing test only ever pairs summaries that literally share the same word (`judgment.test.ts:130,155`), so nothing covered a cross-synonym pair. Fix: match at group level and name the group's shared concept. | **NOT A NEW BUG** | **Already fixed by Khalid in commit `458a8aa` (Aug 31 17:16) before this session.** The working tree used for round 10 was behind HEAD for `intelligence.ts` and `judgment.test.ts` only — verified: the old loop was present and the new test failed before my edit, and my edit converged byte-identical to HEAD. Round 10's generic sentence was therefore measured against stale local code, not shipped code. No action required. Open question for Khalid: what rolled those two files back behind HEAD. |
| B26 | Path granularity is **file-level**. `agent-path/v1` raised `direct_collision` / **high** / **next_turn** / `confidenceBand: deterministic` for two agents editing *different single lines* of `shared/settings.ts` (a label and a retry count). Evidence is only "Both active agent sessions reported work on shared/settings.ts" — there is no region or line awareness, so any two sessions touching one file collide at top severity. SG-02's oracle allows a **quiet** structural warning and forbids a `coordination_required` interruption; this is neither quiet nor low. Directly attacks the >=90% next-turn precision gate and is the "trains people to ignore it" mode from the product decision rule. | HIGH | open |
| B28 | "Next turn" means **the next time the human types**, not the next turn boundary. `internal/app/app.go:445` gates injection to `SessionStart` / `UserPromptSubmit` only. In round 11 the collision was raised at 15:29:47; the Claude session then ran 16 further hook boundaries including `Stop` and finished its edit at 15:30:15 — 28s later — and was **never** proactively injected. It learned of the peer only because it voluntarily called MCP `begin_work`. For an agent working autonomously through a long turn (the normal case for both vendors) a mid-turn finding cannot reach it before the work lands, which is exactly what "material collisions are found before the affected work is finished" claims. Not obviously a defect - possibly a deliberate limit - but it is unrecorded, and it makes the headline next-turn recall gate unmeasurable as written. | HIGH | open, needs a decision |
| B29 | A member **without a supported adapter cannot participate in collision detection at all**. `agent-path/v1` pairs on hook-reported paths held in `safePaths` on *agent session* workstreams; a manual member emits no hooks, and the workspace workstream carries `intendedOutcome`/`approachSummary`/`currentManifestId` but **no `safePaths` field**. Proven twice in round 21: (a) agent A and the manual member both edited `shared/settings.ts` and **no `direct_collision` fired**; (b) the manual member then edited `backend/audit.go`, which no agent touched — the manifest recorded it and the contract fingerprint updated to `066f8970`, and it was attributed to **no workstream at all**. The git evidence exists at workspace level and is never promoted to a coordination participant. Degrades honestly (nothing false is claimed) but degrades to silence, while the member still appears active. | HIGH | open |
| B30 | The `sharedBehaviorTerms` **fallback** names path fragments as shared behaviour. Round 24 produced "Both describe **shared, settings and rename** work" — "shared" and "settings" are simply the components of `shared/settings.ts` echoed in both prompts. `UNINFORMATIVE_WORDS` stops generic verbs (`update`, `change`, `implement`) but not path components, so the explanation reads specific while naming nothing behavioural — the exact failure the stop list exists to prevent. Pre-existing (B20's fallback), not caused by the B25 fix, but the fix makes it far more visible. Fix: stop-list the tokens of any path already named in the finding, or refuse to fall back to words that appear in a shared path. | MED | open |
| B31 | **Headless `claude -p` never ends its session.** The process exits emitting `Stop` but no `SessionEnd`, so the workstream stays `status: idle` / `endedAt: null` and remains **live** for the full 30 minutes until B9's retention sweep. Proven in round 24: five workstreams from rounds 21/23/24 were all still eligible and paired into **five spurious `redundant_work` findings** across unrelated rounds, while 37 real `SessionEnd` events from interactive sessions fired normally the same day. Claude Code headless is a normal production mode (CI, scripts, SDK), so any such run leaves a phantom collaborator generating collisions against unrelated later work. Also contaminates this harness: automated Claude rounds must either wait out the sweep or expect cross-round pairing. | HIGH | open |
| B32 | **Judgment budget exhaustion is silent.** Every other adjudication failure path calls `recordJudgmentDegraded`, but `convex/functions/intelligence.ts:224` returns `mode: "budget"` on a refused `claimJudgmentBudget` **without** recording degradation. Findings then fall back to `deterministicJudgment` with no `semanticDegraded` signal anywhere, so the product silently stops being LLM-adjudicated under exactly the load where coordination matters most. The cap is load-bearing: `needsManagedAdjudication` accepts any non-silent candidate with >=2 workstreams, and candidate pairs grow O(N^2) in concurrent agents (5 agents = 10 pairs per evaluation cycle, re-evaluated per revision), so 60/hour/project is reachable by a genuinely busy team. Same blind-spot family as B24 and the SG-17 honesty failure. | MED | open |
| B33 | **Switching a project between embedding providers silently mixes incompatible vector spaces.** `semanticEmbeddings` declares `vectorIndex("by_vector", { dimensions: 1024, filterFields: ["scopeKey"] })` — `modelVersion` is **not** a filter field, so `ctx.vectorSearch` cannot exclude vectors produced by a different provider. A `by_scope_model` index on `["scopeKey","modelVersion"]` already exists, so the intent was there; the vector path just cannot use it. The local backend already holds two incompatible populations under one provider string — `stickguy-concepts/v1` at **32 dims** (126 rows) and at **1024 dims** (39 rows) — from an un-migrated `CONCEPT_DIMENSIONS` change. Adding `openai/text-embedding-3-large` makes a third, and cosine similarity across unrelated embedding spaces is noise, not a weak signal. **Live risk:** the hosted dogfood deployment has `OPENAI_API_KEY` set now — if it ever ran without it, its scopes hold mixed vectors today. Fix: add `modelVersion` to `filterFields` and filter every search on the active model, plus a migration that drops embeddings whose `modelVersion` is not current. | HIGH | open |
| B34 | **Systemic: degradation is recorded without a reason.** Three independent sites discard the cause and keep only a boolean/counter — `flush()` (`internal/app/app.go:219`, B24), `claimJudgmentBudget` (`intelligence.ts:224`, B32, records nothing at all), and `embedSemanticObject` (`intelligence.ts:102`, `catch { recordOpenAIEmbeddingFailure }`). In round 26 an OpenAI embedding succeeded at 01:41:16 and the scope went degraded at 01:42:18 with **no recoverable explanation** in the database or `convex logs`. This is the direct blocker for the honest-status banner: `{level, reason}` cannot be derived because `reason` is thrown away at every site that knows it. Fix the three sites together with the banner, not separately. | HIGH | open |
| B35 | **Self-collision via `self_declared` reads, at interrupt severity.** Round 26: `wrk_agent_7932e24a` created `backend/security.go`, then received its own `stale_assumption` — high / `next_turn` / `contract-watch/v1` — reading *"backend/security.go: ErrInvalidUserID changed after this session said it expected to read it"*. The session's declared intent registered a self-declared read of a path it then authored, and the contract watcher treated its own write as drift against itself. B15 (agents colliding with themselves) was closed for the hook-path read set; the `self_declared` MCP path reopens it, and unlike B15 this one delivers at next_turn rather than the dashboard. | HIGH | open |
| B24 | A `wrk_agent_*` publicId is a pure function of (vendor, sessionID) with **no project scoping**, so an agent session that outlives a re-enrollment (or a device moving between Projects) derives a workstream id the backend already holds bound to the *old* project. `service.ts:1393` correctly refuses it — `workstream.projectId !== project._id` → **403** — but `flush()` (`internal/app/app.go:219`) discards the error, the batch is all-or-nothing, and the queue head-of-line blocks **forever**. Every surface still reports healthy: `doctor` `{"ok":true,"status":"ok"}`, `diagnostics` clean, only a quietly rising `pending`. Nothing publishes again, ever. | CRIT | open |

## Automation probe (2026-08-31)
Headless `claude -p` in the fixture emits the **complete** hook stream — session workstream,
PreToolUse/PostToolUse, `session.read_set_reported` at `fidelity: observed`, contract
fingerprints, conversation, and Stop carrying `readCoverage`. So the Claude half of any
scenario can be driven with no keyboard. Codex has no equivalent: it is the ChatGPT desktop
GUI, its session lifecycle needs a manual Archive, and the ADR-052 read-set path is bound to
the desktop app-server.

## LIMIT OF SOLO AUTOMATION (found round 27)
Headless `claude -p` sessions finish in ~2 minutes and their semantic objects are
deactivated as the session wraps up. In both managed rounds only one of two agents held an
active, embedded semantic object at evaluation time, so the semantic pair never existed.
**Semantic scenarios (SG-06, SG-08, SG-13) cannot be validated with two fast headless
agents** - the round looks like it ran and silently measures nothing. They need interactive
agents whose intent overlaps, which is how round 10 succeeded. Same family as B31: the
automation appears to test something it does not.

## SG-18 is not automatable — why
Its oracle is "the resolution reaches every affected active session exactly once per
revision". Delivery requires `SessionStart`/`UserPromptSubmit` on the *same* workstream
(B28), and a headless `claude -p` session cannot be handed a further user prompt (B31), so
the deliverable half cannot be exercised without interactive agents. Recording the
resolution is also dashboard-only: there is no CLI verb, and the workroom mints a same-site
session from a ticket the app opens in the **default** browser, which an automated browser
cannot join. Needs Khalid at the keyboard with two live sessions.

## Vendor-role coverage (added round 13)
Role assignment is not cosmetic: Claude reads are `observed`, Codex reads are
`vendor_inferred` at ~69% recovery (ADR-052), and `contractConfidenceBand` steps
severity down with fidelity - so an asymmetric scenario is a *different test* per
direction. Codex also exports no MCP session identity (B8), so it frequently has no
declared intent, only a title.

- **Asymmetric (reader/producer) - run BOTH directions:** SG-03, SG-04, SG-05, SG-09,
  SG-10, SG-12. Reader=Codex is the higher-value direction and the under-tested one.
- **Symmetric - one direction is enough:** SG-02, SG-06, SG-07, SG-08, SG-11, SG-13.

**Vacuous-pass trap:** a negative scenario with Codex as reader passes for the wrong
reason if the read was never captured. Every reader=Codex round must verify the read
set landed first; an unverified read makes the round **void**, not a pass.

| Scenario | reader=Claude | reader=Codex |
|---|---|---|
| SG-04 | R12 PASS | not run |
| SG-05 | not run | R13 PASS |
| SG-03 | rounds 1-9 | R16 |

## Engine versions
Rounds 10 and earlier ran the pre-B25 judgment rendering. **Rounds 11+ run the B25 fix**
(`sharedBehaviorTerms` group-level matching, applied 15:22, hot-redeployed to the local
backend at 15:22:19). Rendering-only: it cannot move detection, routing or delivery, so
those columns stay comparable across the whole corpus; only `redundant_work` *wording* is.

### B25 re-derivation (see correction above), and what it did not fix
Literal shared vocabulary words still win and are all still named, so every prior output is
unchanged; the group-level match only fires when a group has no literal match. Regression
test `names a concept two summaries reached through different synonyms` uses the real round-10
summaries and was verified to fail before the fix. 41/41 coordination + 50/50 convex tests pass.

Two follow-ups deliberately not taken, to keep the fix minimal:
- **B25a** — the canonical label is `group[0]`, a category name, so the round-10 pair now reads
  "Both describe **auth and member** work" rather than naming what either member wrote. Honest,
  but still not the vocabulary a receiving agent would recognise.
- **B25b** — `behaviorWord("invalidate")` is `/\binvalidate[a-z]*\b/`, which does not match
  "invalidation". The revoke/invalidate group is the *strongest* shared signal in the canonical
  SG-06 pair and is missed entirely on a stem mismatch.

## Cost model (measured 2026-09-01)
Adjudication is `claude-sonnet-5` ($2.00/1M in, $10.00/1M out), `max_tokens: 1024`.
Measured candidate prompt = 634 chars (~160 tokens); with tool schema and instructions
~700 input tokens, ~200 output tokens typical.
- **~$0.0034 per adjudication.**
- Hard cap `JUDGMENT_BUDGET_PER_PROJECT = 60` per rolling hour per project -> **$0.20/hour**
  absolute worst case, ~$1.63 per saturated 8-hour day.
- Today's full 10-round corpus would have cost roughly **$0.05** in adjudication.
- Embeddings: 1181 semantic objects, mean 67 chars (~17 tokens), 1644 revisions ~= 28K
  tokens across the entire database history - well under a cent.

**Verdict: always-on is comfortably viable.** The layering works. The risk is not price per
call, it is B32 - the cap is silent when it binds.

## COVERAGE GAP: the managed-provider path is untested
Every semantic result in this corpus came from the deterministic fallback, because
`OPENAI_API_KEY` is unset on the local deployment. The `providerCompatible` branch - a
different threshold (`semantic >= 0.86`) and different wording - has never been exercised
manually. **If the hosted dogfood deployment sets `OPENAI_API_KEY`, the closed test is
running a path with no manual evidence behind it.** Verify that deployment's env before
treating any of today's semantic results (including the B25 fix and the SG-08 negative) as
representative of what friends are seeing.

## Silence battery result (rounds 11-15)
**3 PASS / 1 FAIL.** The single failure is SG-02 (B26, file-level path granularity). The three
passes were each verified non-vacuous — a real contract movement or a verified concurrency
window — rather than accepted as bare absence of findings. Negative-scenario evidence goes
from 2 data points to 6; the >=98% gate needs far more, and the playbook's own rule is to
publish counts, not percentages.

## Rounds
| # | Scenario | Detect | Route | Deliver | React | Notes |
|---|----------|--------|-------|---------|-------|-------|
| 27 | SG-06 managed, retry with logging | **MISS (unmeasurable)** | n/a | n/a | n/a | Added `console.error("openai_embedding_failed", ...)` to the swallowing catch first (B34 slice), then re-ran. **No embedding error occurred** — embeddings that were attempted succeeded as `openai/text-embedding-3-large` at the correct revision. The real cause: only the `active` semantic object per workstream is embedded, and across rounds 26+27 **only one of the two agents ever held an active object** (2 active/OpenAI, 4 inactive/unembedded), so no pair existed to compare. Fired instead: `likely_collision` medium/dashboard (lexical `components` branch, weaker than round 10's `redundant_work`), plus **three false-positive `stale_assumption` at high/next_turn** — two targeting round 26's phantom session, one self-targeted (B35 reproduced). Logs also showed `semanticSearchContext` throwing `E:not_found` repeatedly — briefs requested before the workstream is projected. **Attribution withheld:** this is not evidence that managed embeddings underperform the fallback; round 10 succeeded with interactive agents holding simultaneous active intent. Two fast headless sessions likely deactivate B's objects before pair evaluation. |
| 26 | SG-06 on the **fully managed** path | **MISS** | n/a | n/a | n/a | First run ever with `OPENAI_API_KEY` + `ANTHROPIC_API_KEY` set. Three semantic objects created: A's intent embedded as `openai/text-embedding-3-large` (01:41:16); **B's two objects were never embedded**, and the scope flipped `degraded` at 01:42:18. With B unembedded there was nothing to match, so **no `redundant_work`** — the same pair the keyword fallback caught in round 10. Attribution is an embedding failure, not model quality, and the cause is unrecoverable (B34). Round also produced a false-positive self-targeted `stale_assumption` at high/next_turn (B35). Anthropic adjudication could not be confirmed to have run at all — no finding records a judgment provider. |
| 24 | SG-17 backend outage + recovery | PARTIAL | PASS | PASS | n/a | Backend killed (port unreachable, process gone), then two concurrent headless agents edited `shared/settings.ts`. **Both completed unblocked** (exit 0, both edits landed); `pending` climbed to 41. On restore the queue drained 41 -> 0 and produced **exactly one** `direct_collision` for the real overlap — dedup worked — with **no** context deliveries, so no duplicated injection. Oracle 3/4. **Fails the honesty clause:** with the backend completely unreachable, `doctor` *and* `diagnostics` both reported `"status":"ok"` with no connectivity or degraded field; the only signal was a climbing `pending`, which B24 proved is ambiguous. Same root cause as B24 — `flush()` swallows the error. Round also surfaced B31 (five spurious cross-round `redundant_work` findings from phantom headless sessions) and B30 (path fragments named as behaviour). |
| 22 | SG-15 semantic provider outage | PASS | n/a | n/a | n/a | **Not induced — it was the ambient condition all day.** `OPENAI_API_KEY` is unset locally, so `convex/functions/intelligence.ts:88` short-circuits to `recordOpenAIEmbeddingFailure` and returns `mode: "fallback"`. Round 10 therefore *was* SG-15 (the SG-06 pair with embeddings down). Oracle: briefs continue (`redundant_work` fired and delivered) / degradation visible (`semanticStatus: degraded`, `semanticMode: managed_degraded`, surfaced at `http.ts:463`) / queues never blocked (`pending: 0`) / no managed fidelity claimed (evidence `stickguy-concepts/v1`, band `medium`, non-provider wording). The deterministic fallback found SG-06 unaided, which the playbook scores separately and is the stronger result. |
| 23 | SG-16 pause is a wire boundary | PASS | PASS | PASS | n/a | Baseline accepted event first, then paused. While paused: `pending` climbed to 18 and exactly **three** backend documents were touched — device heartbeat `lastSeenAt`, project `contextRevision`, and the workspace recording `paused: true`. **No paths, file names, code, conversation text or read sets crossed the wire**, which is the "minimal paused health" the oracle permits. On resume all **18** queued events delivered (`pending` -> 0) through the normal `hook`/`git` sources and ordinary types, nothing bypassing the classifier. Queued-in equals delivered-out. |
| 21 | SG-14 unsupported adapter | **FAIL** | n/a | n/a | n/a | Agent A = headless `claude -p` (supported, hook-reported `safePaths: [shared/settings.ts]`). Member B = `stickguy intent` + plain `sed` edits, no adapter. Both touched `shared/settings.ts`: **no finding**. B then edited `backend/audit.go` alone: manifest chunk listed it, fingerprint recomputed to `066f8970`, attributed to nobody. Oracle scored in three parts — git evidence *available*: partial (manifest only, not on the workstream); B *labeled lower fidelity*: fail (no evidence to label); *no fabricated coverage*: pass. See B29. |
| 15 | SG-11 irrelevant activity advances revision | PASS | n/a | n/a | n/a | **Scenario adapted, deliberately.** A (Claude `wrk_agent_87d33663`) read `frontend/theme.ts` while exploring — the very file the literal SG-11 prompt gives B — so B was pointed at `backend/audit.go` instead, a file A demonstrably never read, renaming exported `AuditCategory` to `SessionAuditCategory`. That is a *stronger* SG-11 than the original: a colour-value edit would not have moved a contract hash at all. Result: `audit.go` contract recomputed to `30b81e7a` at 23:38:18 — a genuine contract movement — while A stayed live (`idle`, not `done`) with its `backend/refresh.go` assumption at `e7684bca` still matching the live fingerprint. **Zero findings.** The engine saw a real contract move, found no reader of that path, and stayed quiet. |
| 14 | SG-08 identifier false neighbour | PASS | n/a | n/a | n/a | Zero findings. Concurrency verified rather than assumed (operator flagged a Claude permission prompt that could have staggered them): Claude `wrk_agent_bef8648f` 23:30:28-23:31:50, Codex `wrk_agent_26acc5a5` 23:31:04-23:31:49 — Codex nested entirely inside Claude's window, **45s** of overlap across multiple scan cycles, both with declared titles and paths. `User` vs `UserAvatar` in different files drew no false `redundant_work`. Notably this ran on the **post-B25** matcher, which is broader, so it is a harder negative than the same round this morning. No concept group covers "user", so the pair stayed below the non-provider `semantic >= 0.50 && lexical >= 0.05` threshold. |
| 13 | SG-05 new export, reader=Codex | PASS | n/a | n/a | n/a | **Non-vacuous negative, the strongest of the day.** Codex `wrk_agent_97d68980` recorded `shared/user.ts` at contract hash `fa889e11` / `vendor_inferred` (barrier verified before B started). Claude added `formatUserInitials` only, touching no existing export. The contract fingerprint was recomputed at 23:28:39 and **changed to `b466954e`** — so the engine saw a real contract movement against a live reader's recorded assumption and still correctly raised **no `stale_assumption`**. Holds on the degraded read path, not just `observed`. No B26 (Codex wrote `frontend/session.ts`, Claude wrote `shared/user.ts`). Claude's own session read `user.ts` before editing without self-colliding — B15 stays closed. |
| 12 | SG-04 body-only change after read | PASS | n/a | n/a | n/a | Barrier verified before B started: `wrk_agent_af9fcca2` (claude) read `backend/refresh.go` at `fidelity: observed`, hash `e7684bca`. Codex hoisted a `sessionPrefix` const and used it in the body, leaving `func Refresh(userID string) string` byte-identical. **Zero findings.** Silence is meaningful, not vacuous: `file_contract_hash` hashes the contract rather than the bytes, so it stayed `e7684bca` and there was nothing to drift from; the fingerprint sweep demonstrably ran during B's edit (new entry for `backend/refresh_test.go` at 23:17:11). B26 correctly quiet — A wrote `frontend/session.ts`, B wrote `backend/refresh.go`, no real overlap. A's three MCP pulls all returned size 0, correctly. Caveat: unchanged contracts are not re-reported, so the re-comparison of `refresh.go` was not directly observed. |
| 11 | SG-02 same file, unrelated regions | **FAIL** | PASS | n/a | n/a | Agents' edits were fully compatible — final file carries both changes, 2 lines. Got `direct_collision`/high/next_turn/deterministic (B26) where the oracle allows only a quiet structural warning. Routing correct: exactly `wrk_agent_c5953b66` (claude) + `wrk_agent_09d1bdbf` (codex), no unrelated session. **No duplicate delivery** — the three content briefs (`begin`, `before_broad_edit`, `finish`, all `renderedSize` 76, all item revision 1) were MCP *pulls* by Claude itself and went to one workstream; the local claim ledger `injection_deliveries` is **empty**, so nothing was ever injected. Every hook-path `refresh` brief was size 0. Codex received nothing and had no eligible turn (idle from 15:29:44, 3s before the finding). React not measured by design. |
| 10 | SG-06 semantic duplicate | PASS | PASS | PASS | PASS | `redundant_work`/medium/dashboard, `confidenceBand: medium`, evidence `semantic` via `stickguy-concepts/v1`. Routed to exactly the two agent workstreams (`wrk_agent_acd1c9` claude + `wrk_agent_3b6a7f` codex) — no workspace workstream, no unrelated session. Delivered at `begin` rev46 size198 carrying the finding + a semantic candidate. Claude changed behaviour: scoped to one new file and explicitly avoided `sessions.go`/`refresh.go`/`audit.go`, and escalated reconciliation to the operator — it named the peer's *intent*, which only the brief could supply. **Wording fails the oracle**: the playbook requires the explanation name the shared behaviour; it is the generic sentence (B25). Ran on the **non-provider** deterministic path (`providerCompatible` false), so the managed-embedding path is still untested. **B17 did not reproduce** — two later empty briefs (`before_broad_edit`, `checkpoint`) were correctly read as "no new findings", not an all-clear; but the dangerous case (empty brief while an *undelivered* finding is open) was not exercised, and no `degraded` field exists on the delivery record at all, so the guard is still unbuilt. Open question: finding went `state: resolved` 17s after first seen while both sessions were still active — cause not attributed. |

## Fix sprint — 2026-09-01 (afternoon)
Codex's three handoff tasks (B33/B34/B35) verified via stash-revert: every acceptance test
fails on the old code for the stated reason and passes on the new. The B35 hypothesis is now
empirically confirmed: old attribution returned the workspace workstream (`wrk_local_…`) for
an agent-authored path.

| # | Fix | Where | Proof |
|---|-----|-------|-------|
| B23 | Re-enrollment over an existing root replaces the dead workspace instead of crash-looping on UNIQUE(root) | `store.UpsertWorkspace` | `TestReenrollmentReplacesWorkspaceWithSameRoot` (reproduced the raw 2067 first) |
| B24 (wedge) | Permanent rejections retry individually, quarantine the refused, `doctor` reports `quarantined` + `lastPublishError: rejected`. Root cause (unscoped session identity) split into its own task. | `app.flush`, `store.Quarantine` | `TestPermanentRejectionQuarantinesInsteadOfWedging` |
| B25a | Cross-synonym pairs name each member's own word ("credential/login"), never the category label | `sharedBehaviorTerms` | judgment.test.ts |
| B25b | Behavior-word stems drop a trailing "e" (len>=6): "invalidate" now matches "invalidation"; "role" stays intact | `behaviorWord` | judgment.test.ts |
| B26 | Bare same-file overlap → medium/dashboard; escalates to high/next_turn only when the path's contract moved while both sessions were live (escalates in place, never de-escalates) | `upsertAgentPathFindings` | intelligence.test.ts |
| B28 | PostToolUse is a delivery boundary (claude only): daemon throttles to one fetch/20s/workstream, mid-turn payloads restricted to coordination_required | `handleAgentInjection` + hook CLI | `TestMidTurnInjectionDeliversUrgentFindingsOnly` |
| B29 | Manifest paths unclaimed by live agent sessions become `residualPaths` on the workspace workstream; agents pair against them at medium/medium-band/dashboard, evidence fidelity "residual". Same-path-same-checkout stays honestly undetectable. | manifest_completed + `upsertAgentPathFindings` | intelligence.test.ts |
| B30 | Fallback wording stop-lists tokens of any path either summary names | `sharedBehaviorTerms` | judgment.test.ts |
| B31 | Idle (post-Stop) sessions expire after 10 min (`SESSION_STOP_TIMEOUT_MS`); active keep 30. Sweep index bound matches per-status. Revival stays automatic. | `sessionHasGoneQuiet` + sweep | domain.test.ts |
| B36 (new) | Codex's B33 migration deleted the other population's rows on any provider flip — an env blip wiped recall. Convergence now re-embeds in place (fallback: synchronous conceptVector; managed: scheduled re-embed), deletes only orphans, and runs only on an actual model switch. | `convergeForeignEmbeddings` | intelligence.test.ts |
| B37 (new) | Fourth reason-less degradation site: `recordSemanticHealth` degraded path now records a reason; healthy clears it. | `recordSemanticHealth`, `searchSemantic` | intelligence.test.ts |

Suites after sprint: coordination 44/44, convex 60/60, `go test ./internal/... ./cmd/...` clean,
`pnpm typecheck` clean. Local backend hot-redeployed throughout (convex dev was running);
hosted dogfood NOT deployed — deploy deliberately.

Semantics changed under the corpus oracles: B26 re-run (SG-02) should now yield the quiet
structural warning it demands; B31 shortens the phantom window to 10 min (headless rounds
still need the wait or an interactive agent); SG-14 re-run should now produce the residual
finding. The banner state (B34) is complete server-side: `{semantic,judgment} x {DegradedAt,
DegradedReason, ProviderName}` + `judgmentRecoversAt` + local `lastPublishError`/`quarantined`.
