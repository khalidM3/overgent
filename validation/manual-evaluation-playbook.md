# Overgent evaluation and dogfood playbook

Status: working evaluation guide  
Audience: product owner and evaluation contributors  
Scope: collision coordination, context routing, and usefulness testing

## 1. Recommendation: automate correctness, test usefulness with people

Overgent needs both automated and active testing. They answer different
questions.

| Method | Best for | Cannot prove by itself |
|---|---|---|
| Automated scripted scenarios | Repeatability, regression detection, finding precision/recall, routing, deduplication, latency, privacy, failure fallback | That a real agent understands the warning or that a person finds it worthwhile |
| Real Codex/Claude sessions | Whether context changes agent behavior, whether wording is actionable, integration fidelity, realistic timing | Stable accuracy estimates without repetition and fixed ground truth |
| Two-person dogfood | Trust, interruption tolerance, workflow fit, repeat use, perceived time saved | Exact detection-stage attribution or reproducible threshold comparisons |
| External benchmarks | Outcome comparison on unfamiliar repositories and tasks | Overgent-specific ground truth for routing, privacy, stale assumptions, or degradation |

The working order is:

1. Run a new scenario manually three to five times with real agents.
2. Decide its ground truth before examining Overgent's output.
3. Fix unclear prompts or timing barriers.
4. Automate the scenario once the expected event, recipients, delivery mode,
   required facts, and forbidden behavior are unambiguous.
5. Keep a smaller rotating set of manual scenarios so the product is not
   optimized only for its own fixtures.

The existing `pnpm eval:coordination` suite remains the fast M1 regression gate.
This playbook adds a broader accuracy corpus and real-agent outcome testing; it
does not replace M1.

## 2. Claims to evaluate

Evaluate the complete coordination loop in separate stages:

```text
observe evidence
      ↓
retrieve a candidate
      ↓
classify the relationship and severity
      ↓
route it to affected sessions only
      ↓
deliver it at the appropriate surface/turn
      ↓
agent or person changes behavior
      ↓
joint work finishes with less failure or rework
```

A miss must be assigned to the earliest failing stage. For example, if a
contract change was observed and a correct finding existed but the consumer
never received it, record a routing or delivery failure, not a detection miss.

Do not use one undifferentiated “accuracy” score. At minimum, report detection,
routing, interruption, delivery, and outcome separately.

## 3. Test environments

### 3.1 Synthetic fixture repository

Create a small, separate Git repository for repeatable tests. Do not use the
Overgent repository as the task under test. A useful shape is:

```text
overgent-eval-fixture/
├── backend/
│   ├── refresh.go
│   ├── sessions.go
│   └── audit.go
├── frontend/
│   ├── session.ts
│   └── dashboard.ts
├── shared/
│   ├── settings.ts
│   └── user.ts
├── tests/
├── run_tests.sh
└── README.md
```

The base fixture should compile and pass tests. Tag immutable starting states,
for example `eval-base-v1`. Create fresh linked worktrees or fresh temporary
clones for every run. The evaluator creates and removes these; Overgent never
creates, switches, resets, or removes worktrees.

### 3.2 Real-project transfer set

Maintain three to five scenarios drawn from real repositories that were not
used to tune thresholds. Replace private names and content with a synthetic
equivalent before adding anything to this public repository. These runs test
whether fixture-specific vocabulary is inflating performance.

### 3.3 Conditions

Use paired conditions. Hold the base commit, agent product/version, model,
prompt, tool permissions, token budget, and time limit fixed.

| Condition | Purpose |
|---|---|
| `control` | Overgent absent or disconnected; measures ordinary parallel-agent behavior |
| `observe` | Overgent observes and renders the dashboard, but no context is injected; measures visibility value |
| `full` | Normal findings, routing, dashboard, and supported next-turn injection |

`observe` is an evaluation mode to add to the harness if it is not available;
do not weaken production delivery or privacy behavior to simulate it.

Run `control` and `full` first. Randomize their order, because agents and the
operator may learn the scenario. Add `observe` when measuring which part of the
product caused the improvement.

## 4. Standard run protocol

For every run:

1. Allocate a run ID such as `stale-contract-001/full/run-03`.
2. Materialize clean worktrees from the scenario's pinned base commit.
3. Start the selected Overgent condition using an isolated local profile and
   Project.
4. Start new agent sessions after adapter installation so their configuration
   is current and runtime delivery can be verified.
5. Give each agent only its own prompt. Do not tell it the expected collision,
   expected finding, or evaluation label.
6. Follow the scenario schedule exactly. Timing barriers such as “A has read
   the contract” are part of the ground truth.
7. Stop when both agents finish, the scenario time limit expires, or an
   unrecoverable execution error occurs.
8. Run the fixture's objective integration tests against the combined result.
9. Record Overgent objects and aggregate timing only. Never add raw private
   transcripts, source, diffs, tool results, commands, output, environment
   values, or credentials to public evaluation evidence.
10. Complete the scorecard before changing a threshold.

Use at least three repetitions while developing a scenario and five or more
when comparing product conditions. Report the number of runs and uncertainty;
do not present one stochastic run as an accuracy rate.

## 5. Scenario specification

Store automated scenarios as data rather than embedding all expectations in
runner code. A proposed representation is:

```yaml
schemaVersion: overgent-scenario/v1
id: stale-contract-001
family: stale_assumption
baseRef: eval-base-v1
agents:
  a:
    prompt: >-
      Read backend/refresh.go, then implement frontend/session.ts against the
      current Refresh interface. Do not modify backend code.
  b:
    prompt: >-
      Change Refresh from Refresh(userID string) to
      Refresh(sessionID string, policy Policy). Update backend tests.
schedule:
  - start: a
  - waitForRead: backend/refresh.go
  - start: b
  - waitForContractChange: backend/refresh.go#Refresh
  - nextTurn: a
oracle:
  findings:
    - kind: stale_assumption
      targets: [a]
      delivery: next_turn
      requiredFacts: [Refresh, userID, sessionID, Policy]
  forbiddenTargets: [b, observer]
  maxEligibleDeliveryMillis: 2000
outcome:
  testCommandId: fixture-integration
  agentAMustAdapt: true
```

`testCommandId` references a reviewed command stored by the fixture. Do not put
arbitrary scenario text into a shell command.

## 6. Manual scenario pack

The prompts below are starting points. Preserve the intent while changing names
and phrasing to create paraphrase variants.

### SG-01 — incompatible edits to one exported contract

Agent A:

> Extend `backend.Refresh` with a TTL argument and update its callers and tests.

Agent B:

> Change `backend.Refresh` to accept a session ID and policy instead of a user
> ID. Update its callers and tests.

Schedule: start both at the same base commit.

Oracle: a high-confidence contract/direct-collision relationship involving
both sessions; both receive the relevant competing contract shape. The brief
must name the symbol and the two incompatible directions.

Outcome question: do the agents converge on one compatible signature rather
than independently landing incompatible versions?

### SG-02 — same file, unrelated regions

Agent A:

> In `shared/settings.ts`, rename only `navigationLabel` from “Work” to
> “Sessions”. Do not change retry behavior.

Agent B:

> In `shared/settings.ts`, increase only `retryLimit` from 3 to 5. Do not change
> presentation labels.

Oracle: a quiet same-path structural warning is allowed; no stale-contract
finding and no `coordination_required` next-turn injection.

Outcome question: does Overgent preserve awareness without distracting either
agent from compatible work?

### SG-03 — contract changes after a consumer reads it

Agent A:

> Read `backend/refresh.go`, then implement `frontend/session.ts` against the
> current `Refresh` interface. Do not modify backend code.

Agent B, sent only after A has read the file:

> Replace `Refresh(userID string)` with
> `Refresh(sessionID string, policy Policy)` and update backend tests.

Oracle: one `stale_assumption` finding targeted to A. It must include the old
and new `Refresh` signatures and reach A's next supported turn. B and unrelated
sessions receive no corrective brief.

Outcome question: does A re-read/adapt before finishing, and do combined tests
pass more often than in the control condition?

### SG-04 — body-only change after a read

Agent A:

> Read `backend/refresh.go` and implement a frontend consumer of `Refresh`.

Agent B, after A reads:

> Optimize the body of `Refresh` and add tests, but preserve its exported
> declaration exactly.

Oracle: no contract-drift or stale-assumption finding. A low-level same-path
signal may exist only if actual work overlaps in that path; it must not be
presented as a changed contract.

### SG-05 — new unrelated export

Agent A:

> Read the existing exports in `shared/user.ts` and use `User` in the session
> view.

Agent B, after A reads:

> Add a new exported `formatUserInitials` helper to `shared/user.ts` without
> changing any existing export.

Oracle: no stale-assumption finding. Adding an export must not imply that an
existing reader's assumptions were invalidated.

### SG-06 — semantic duplicate under different paths and names

Agent A:

> Implement server-side revocation of active browser credentials whenever a
> member's role changes. Put it in `backend/revoke.go`.

Agent B:

> Implement invalidation of all current login sessions after a privilege
> change. Put it in `backend/security.go`.

Oracle: `redundant_work` involving both sessions, with an explanation naming
the shared behavior. Initial delivery should recommend review/coordination; it
must not claim that vector similarity proves duplication.

Outcome question: do the agents consolidate the work or establish distinct
responsibilities instead of landing duplicate mechanisms?

### SG-07 — lexical false neighbor: “refresh”

Agent A:

> Refresh browser session tokens after authentication renewal. Work only in
> `backend/sessions.go`.

Agent B:

> Refresh the dashboard's visual layout when its data changes. Work only in
> `frontend/dashboard.ts`.

Oracle: silence. No finding, routed brief, or injected context.

### SG-08 — identifier false neighbor: User versus UserAvatar

Agent A:

> Add stable JSON serialization for `User` in `shared/user.ts`.

Agent B:

> Add image-size normalization for `UserAvatar` in
> `frontend/user_avatar.ts`.

Oracle: silence unless the agents introduce an actual shared contract or path
dependency not present in the scenario.

### SG-09 — dependency becomes stable and then verified

Agent A:

> Implement the frontend session loader, but wait for an exported `SessionAPI`
> contract before completing the integration. Declare that dependency through
> the available Overgent lifecycle interface.

Agent B:

> Add the exported `SessionAPI` contract in `backend/session_api.go`. First
> report a checkpoint before running tests. Then run the fixture tests and
> report the passing verification state.

Oracle: A first receives `dependency_ready` with `stable_wip`; the same finding
ID later upgrades to `ready` at a higher revision. B and unrelated sessions do
not receive the unblock notice.

### SG-10 — provisional contract change

Agent A:

> Read and consume the current `Refresh` contract in the frontend.

Agent B, after A reads:

> Prototype a new `Refresh` signature. Report it as work in progress and do not
> run verification yet.

Oracle: the contract movement is visible, but the explanation must say it is
unverified/provisional and remain below the verified-change severity. It must
not instruct A to treat the prototype as settled truth.

### SG-11 — old brief plus irrelevant repository activity

Agent A:

> Begin frontend session work and continue from the initial coordination brief.

Agent B:

> Change only the dashboard accent color in `frontend/theme.ts`.

Oracle: no stale-assumption finding for A merely because the repository context
revision advanced.

### SG-12 — precise fan-out to three consumers

Agents A, B, and C:

> Read `SessionAPI` and implement a distinct consumer of it in your assigned
> file.

Agent D:

> Improve the README installation prose. Do not inspect or modify `SessionAPI`.

Producer E, after A–C read:

> Change the exported `SessionAPI` response contract and update backend tests.

Oracle: route the stale-contract correction to exactly A, B, and C. D and E do
not receive a consumer correction. Each brief stays within its requested budget
and names the changed contract.

### SG-13 — workstream changes scope

Agent A initial prompt:

> Implement session rotation after role changes.

After A reports its initial intent, tell A:

> Stop the session-rotation work. Switch scope to exporting audit events and
> update your intent before editing.

Agent B:

> Implement session rotation after role changes.

Oracle: any duplication candidate based only on A's superseded trajectory is
resolved, silenced, or clearly marked stale promptly. A current
`redundant_work` finding must not persist as if A were still doing that work.

### SG-14 — unsupported or disconnected adapter

Run A through a supported adapter and B through Git/manual intent only, or
disconnect B's adapter after the session starts. Ask both to modify the same
safe path.

Oracle: deterministic Git/path evidence remains available. B is labeled with
lower fidelity; Overgent must not claim read-set, checkpoint, acknowledgement,
or next-turn delivery coverage that it did not observe.

### SG-15 — semantic provider outage

Use the SG-06 prompt pair with managed embeddings and adjudication unavailable.

Oracle: structural observation and briefs continue, semantic processing is
visibly degraded, queues do not block agent work, and no managed-model fidelity
is claimed. Whether the deterministic fallback finds SG-06 is scored separately
from the requirement that the product degrades honestly.

### SG-16 — pause is a wire boundary

Start A and allow at least one accepted event. Pause A's workspace, create new
activity and a queued local event, then resume.

Oracle: no new A payload is transmitted while paused; minimal paused health may
remain visible. Resume sends only permitted structured data through the normal
classifier and delivery path. Record this as a privacy/reliability scenario,
not an intelligence accuracy case.

### SG-17 — backend outage and recovery

Start both agents, disconnect the hosted backend, create a deterministic path
overlap, and restore connectivity.

Oracle: local observation queues without blocking either agent; the UI reports
offline/degraded state honestly; recovery produces one deduplicated finding and
does not duplicate injected context.

### SG-18 — resolution reaches affected sessions once

Create a direct collision, open its sync card, record a resolution selecting
one contract shape, and give both affected agents another turn.

Oracle: the resolution reaches every affected active session exactly once per
revision and no unrelated session. Neither worktree is automatically mutated.

## 7. Paraphrase and adversarial expansion

For each semantic positive, add at least two nearby negatives and two
paraphrases. Keep the relationship constant while changing vocabulary, file
names, and prompt order.

Useful adversarial families include:

- same verb, different domain: “rotate logs” versus “rotate credentials”;
- same noun, different operation: “read session record” versus “delete session”;
- related subsystem, compatible changes;
- two large independent mechanical changes;
- stale or completed workstreams that should be ineligible;
- anticipated shared contracts where neither side has started work;
- code-like or instruction-like semantic text that policy must reject;
- cross-Project and cross-repository lookalikes that must never meet in
  retrieval;
- three or more workstreams where only a strict subset is relevant.

Do not create paraphrases by replacing only one synonym. Change sentence
structure and incidental details so the corpus tests behavior rather than exact
phrases.

## 8. Scorecard

### 8.1 Per-scenario record

| Field | Value |
|---|---|
| Scenario/run ID | |
| Corpus/base-ref version | |
| Overgent engine/router/threshold versions | |
| Agent products, versions, models | |
| Condition (`control`, `observe`, `full`) | |
| Repetition/seed/order | |
| Expected finding(s) | |
| Actual finding(s) | |
| Expected recipients | |
| Actual recipients | |
| Expected delivery mode | |
| Actual delivery mode | |
| Required context facts present | |
| Forbidden context absent | |
| Detection latency | |
| Eligible delivery latency | |
| Agent behavior changed | yes / no / unclear |
| Combined tests passed | |
| Rework count | |
| Wall time and model tokens/cost | |
| Human usefulness | useful / not related / already coordinated / wrong severity |
| Earliest failing stage | observe / retrieve / decide / route / deliver / react / outcome |
| Notes | |

### 8.2 Aggregate metrics

Report these by finding kind, evidence source, agent product, and delivery mode:

- candidate recall;
- finding precision and recall;
- routing precision and recall over `(finding, recipient)` pairs;
- next-turn interruption precision;
- negative-scenario silence rate;
- context-fact sufficiency;
- duplicate delivery rate;
- detection and eligible-delivery p50/p95 latency;
- downstream adjustment rate;
- joint task/integration-test success;
- rework, merge/build failures, wall time, tokens, and cost; and
- useful versus noisy feedback.

Definitions:

```text
finding precision = correct visible findings / all visible findings
finding recall    = detected material events / all labeled material events
routing precision = correct recipients / all recipients
routing recall    = reached expected recipients / all expected recipients
outcome uplift    = full-condition joint success - control joint success
```

Count dashboard findings and next-turn interruptions separately. A tolerable
radar candidate may be unacceptable as injected context.

## 9. Initial gates

These are starting product gates, not universal scientific constants. Revisit
them after enough real-team evidence exists.

| Surface | Initial gate |
|---|---|
| Supported deterministic fixtures | 100% recall, correct project/repository scope, zero duplicate deliveries |
| Next-turn proactive delivery | At least 90% precision and at least 75% recall for labeled material events |
| Quiet dashboard radar | At least 70% precision while preserving at least 80% candidate recall |
| Negative scenarios | At least 98% receive no proactive injection |
| Routing | At least 95% precision and recall over expected recipients |
| Context | All oracle-required critical facts present; no forbidden unrelated item |
| Delivery | p95 under two seconds from eligible finding to an available supported turn boundary, reported separately from human think time |
| Outcome | Positive joint-success or rework improvement without a material token/time regression |

Do not claim a percentage gate from fewer than 100 eligible examples. Include a
confidence interval once the corpus is large enough. Until then, publish counts
such as `18/20`, not an over-precise percentage.

## 10. Threshold-tuning procedure

Split scenarios by family, not randomly by individual paraphrase:

- 60% development;
- 20% tuning;
- 20% frozen holdout.

Keeping paraphrases of the same scenario in different partitions leaks the
answer and inflates accuracy.

For every proposed threshold or judgment change:

1. Assign a new engine, threshold-set, prompt, and corpus version.
2. Run the development partition and classify errors by earliest failing stage.
3. Select thresholds using only the tuning partition.
4. Run the frozen holdout once for the release decision.
5. Compare precision/recall curves separately for dashboard and next-turn
   delivery.
6. Reject a change that improves aggregate recall by creating unacceptable
   proactive false positives.
7. Run M1 and all privacy/isolation/failure scenarios before merging.
8. Add production misses and false positives only to the next corpus version;
   never rewrite a frozen result retroactively.

Thresholds should move into a versioned evaluation policy rather than remain
anonymous numeric literals. Store the selected threshold-set version on every
finding and delivery so regressions are reproducible.

## 11. Automation cadence

| Cadence | Suite |
|---|---|
| Every relevant pull request | Unit/conformance tests plus existing seven-scenario M1 gate |
| Nightly | Full synthetic labeled corpus, paraphrases, negative cases, provider-off fallback, repeated latency runs |
| Weekly while tuning | Five to ten real Codex/Claude paired runs, including at least one previously unseen scenario |
| Before beta candidate | Frozen holdout, shared two-Mac dogfood, privacy/failure suite, and external benchmark subset |
| During beta | Feedback review and replay of every consented, safely reconstructed false positive/miss as synthetic evidence |

Keep at least 20–30% of evaluation effort manual while product behavior and
agent adapters are changing quickly. As the system stabilizes, automate more of
the known corpus but retain unseen manual transfer cases and two-person
dogfood.

## 12. CooperBench use

CooperBench is an external outcome benchmark, not the source of truth for
Overgent finding accuracy. Use a curated subset only after the internal
scenario corpus can explain failures stage by stage.

Compare:

1. cooperative agents without Overgent and without a separate messaging
   channel;
2. the benchmark's normal cooperative messaging condition;
3. cooperative agents with Overgent and no separate messaging channel; and
4. optionally, Overgent plus the normal messaging channel.

Start with 10 task pairs, then expand to 20–50 if the integration is stable.
Record objective feature/integration tests, conflicts, rework, wall time,
tokens, and Overgent findings/routing. Do not run all available tasks merely to
produce a large number before adapter fidelity and condition isolation are
proven.

CooperBench does not replace SG-03, SG-09, SG-12, SG-15, SG-16, or SG-17: its
task success oracle does not label Overgent-specific stale contracts,
dependency readiness, routing recipients, provider degradation, pause, or
offline recovery.

## 13. Product decision rule

Overgent is demonstrating value when all of the following are true:

- material collisions are found before the affected work is finished;
- next-turn context is rarely irrelevant;
- unaffected sessions remain quiet;
- agents measurably adapt to delivered facts;
- paired runs finish more reliably or with less rework than controls;
- failure modes retain deterministic evidence and honest fidelity; and
- real users voluntarily keep Overgent enabled for a second collaborative
  session.

A passing regression suite alone is insufficient. A product that catches many
events but trains people to ignore it has failed the usefulness test.
