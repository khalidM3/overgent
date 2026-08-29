# Stickguy dogfood — state of play

Written 2026-08-28 at the end of a long manual evaluation session, so a fresh session
can continue without rediscovering any of it.

## Read these first

- `validation/evidence/manual-dogfood-2026-08-27.md` — every finding, with `file:line`
  root causes, fixes applied, and per-round scorecards. The primary document.
- `validation/evidence/b22-research-brief.md` — the research brief for B22.
- `validation/evidence/b22-codex-file-read-observations.md` — the answer that came back.
- `validation/manual-evaluation-playbook.md` — the SG-01..SG-18 scenario definitions
  and the product gates.

## Harness

- Fixture repository: `~/stickguy-eval-fixture`, tag `eval-base-v2`. Reset between
  rounds with `git checkout -- . && git clean -fd`. **Never put the fixture under
  `/private/tmp`** — macOS purged it mid-session once already.
- Project `prj_d47289ed2ca9834aa42b8d7b94629105`, profile
  `~/Library/Application Support/Stickguy`.
- Inspect findings, briefs, workstreams and events straight out of the local Convex
  sqlite — far cheaper than driving the dashboard:
  `SINCE_MS=<unix_ms> python3 ~/.stickguy-eval/sgpeek.py all 14`
  (sections `ws`, `f`, `d`, `e`). Mark the start of each round with
  `python3 -c "import time;print(int(time.time()*1000))"`.
- A plain shell resolves Node 14; put Node 22 on PATH before any `pnpm` command:
  `export PATH="$HOME/.nvm/versions/node/v22.23.2/bin:$PATH"`.
- Restarting the Go service after a Go change:
  `kill $(lsof "$HOME/Library/Application Support/Stickguy/service.lock" | tail -1 | awk '{print $2}')`,
  wait for the lock to clear, then `./bin/stickguy service run` as a supervised
  background task. Do **not** launch it with `nohup … &` from a tool call; it dies with
  the calling shell.
- **Close sessions between rounds.** Claude: Ctrl+Esc. Codex desktop: **Archive the
  chat** — starting a new chat does not end the old session (B9), and a lingering
  session keeps live path claims that contaminate the next round.

## Fixed this session, each with a regression test verified to fail without the fix

| Bug | What it was |
|---|---|
| B7 | A deleted repository root aborted service boot for every Project on the device |
| B8 | MCP briefs used the workspace workstream; findings route to per-session workstreams, so `check_coordination` always returned empty |
| B10 | Read paths counted as write evidence, manufacturing false collisions |
| B11 | Workspace workstream absorbed agent intent (fell with B8) |
| B15 | Agents collided with themselves (fell with B8) |
| B16 | Codex desktop rollout shape unparsed, disabling semantic detection for Codex |
| B18 | Codex assistant turns dropped by a `"text"` vs `"Text"` casing mismatch |
| B21 | Browser activation screen was a dead end with a button that could never succeed |

B14 dissolved as a consequence of B10. B12 was an evaluation-method defect, resolved by
using a neutral follow-up prompt.

## Open, in the order I would take them

### 1. B22 — Codex read sets — DONE 2026-08-28 (ADR-052)

Resolved. Both halves shipped, with the ADR written first.

The framing above was stale by the time it was acted on: ADR-051 had already accepted
spawning a version-matched `codex app-server` stdio child, and `internal/codexappserver`
already did it for hook trust, so the first objection never needed re-litigating. The
second stands and is honoured — this evidence is stored as `vendor_inferred`.

Measured against the bundled `0.149.0-alpha.4.1` before implementing:

- The hook's `session_id` **is** the app-server `threadId`. No discovery heuristic.
  `thread/read` on a rollout UUID returned the task with status `notLoaded`.
- Spawn + `initialize` 39ms, `thread/read` 34ms, `thread/list` 41ms — cheap enough to
  spawn on demand at a turn boundary, so there is no long-lived child to supervise.
- In one 99-command thread: 31 items classified a `read`, 14 more were `unknown` while
  naming a reader tool, recovering 36 distinct source paths — roughly 69% of read-ish
  commands, and partial by measurement rather than only in principle.
- `thread/read` returned 1.6 MiB. Payload is the binding cost, which is why the refresh
  is debounced to `Stop`/`SessionEnd` rather than run per `PostToolUse`.

What landed: read-set entries carry a `fidelity` (`observed` / `vendor_inferred` /
`self_declared`) with a rank so the strongest evidence per path wins and a weaker later
report cannot downgrade it; `stale_assumption` steps its `confidenceBand` down with that
fidelity instead of hard-coding `deterministic`, and its wording follows; sessions carry
a `readCoverage` surfaced to the agent's operator in the inspector as **Contract drift**;
and `Client.ThreadReads` recovers Codex reads at turn boundaries, dropping anything
outside the registered repository and never decoding the command or its output.

**One finding worth carrying forward.** Codex has *two* independent gaps, not one. Beyond
having no read tool, it exports no session identity to MCP servers
(`internal/mcp/server.go` probes only `CLAUDE_CODE_SESSION_ID`), so paths declared at
`begin_work` are attributed to the workspace workstream while the session's own read set
stays empty. That is why self-declaration cannot substitute for the observer, and why any
future plan to lean on MCP declaration for Codex has to solve session identity first.

### 2. B9 — sessions stay live forever — DONE 2026-08-28

Resolved in the retention sweep, which already runs every five minutes.

An agent session that has not reported for thirty minutes is ended: status and
`agentStatus` go to `done`, `endedAt` records the last moment the session was actually
seen rather than the moment the sweep noticed, and its routed agent-path findings resolve
exactly as a real `SessionEnd` would have resolved them. Because the engine's liveness
test is `status !== "done"`, fixing it at the source covers every call site at once
instead of adding a staleness window to each.

Three properties worth keeping in mind when reading it:

- **Only agent sessions expire.** A workspace workstream or a manually reported intent
  has no vendor and no turn loop, so silence says nothing about whether it is finished,
  and completing one on the member's behalf would be a claim Stickguy cannot support.
- **Expiry is not final.** An event from a revived session sets its status straight back
  to active, so a wrongly-expired session costs a gap, never data.
- **The finding resolution cannot break the sweep.** `resolveAgentPathFindings` throws on
  a scope too large to enumerate, and an escaping error inside a scheduled mutation would
  roll the whole sweep back and keep rolling it back, silently stopping retention for
  every Project on the deployment. It is caught; ending the session is the part that
  matters, and findings expire on their own retention anyway.

The practical consequence for evaluation rounds: closing sessions between rounds is still
the clean thing to do, but forgetting to archive a Codex chat no longer contaminates
every subsequent round for the rest of the day.

### 3. B17 — re-test before fixing
Empty briefs were reported as `degraded: false`, and in round 4 that produced a false
all-clear. It was a symptom of B8, which is now fixed. **Re-run SG-06 and check whether
the false all-clear still occurs** before building the honest-`degraded` guard. The
guard is still probably worth having for the Codex path, which remains unidentified.

### 4. B20 — `redundant_work` never names the shared behaviour — DONE 2026-08-28

The hunch in the original note was right: a richer rendering existed and did not fire.
`deterministicJudgment` has a `duplicate_behavior` branch that calls
`sharedBehaviorTerms` and appends "Both describe X work, so one of them is probably
redundant", and `CONCEPT_GROUPS` is exported with a comment saying it exists so the
judgment layer "names the words a pair of workstreams actually shared". The wiring was
never the problem.

The vocabulary was. `sharedBehaviorTerms` only matched the eight curated concept groups,
while `redundant_work` fires on *overall* similarity — which two summaries reach easily
through ordinary shared words the vocabulary has no group for. A duplicated exporter, a
duplicated parser, a duplicated importer: strongly similar, no concept word in common,
`terms.length === 0`, and the branch falls through to the generic sentence. That is the
whole bug, and it is why the text looked like a missing feature rather than a narrow one.

`sharedBehaviorTerms` now falls back to the specific words the two summaries genuinely
share when no curated term matches. The curated vocabulary still wins when it applies, so
existing wording is unchanged; a stop list keeps words like "update", "change", and
"implement" from producing an explanation that sounds specific while saying nothing. Only
summaries that already passed the semantic text policy are read, and both summaries are
already shared coordination facts, so nothing is named that the receiving member could
not already see.

One thing I tried and backed out, worth not repeating: naming the terms in
`evaluatePair`'s base reason instead. An existing test asserts that
`deterministicJudgment` names the shared behaviour *whatever* reason it is handed, which
is the stronger invariant — moving the guarantee into the finding builder would let any
other caller bypass it.

### 5. Small items — ALL DONE 2026-08-28

**B13 — four approval prompts before any edit.** `setup claude` now pre-approves exactly
Stickguy's own coordination tools in the `.claude/settings.local.json` it already manages,
and `setup claude remove` withdraws precisely what it granted. The grant is narrow by
construction: the eight exact `mcp__stickguy__*` names, never a wildcard, never another
server's tools, and a member's own permissions are preserved untouched. A test in
`internal/mcp` asserts the registered tool list and the pre-approved list agree, so adding
a tool without pre-approving it fails in CI instead of reappearing as an unexplained
prompt at someone's keyboard.

*Codex has no equivalent.* I checked the installed binary for a per-tool allowlist key and
found none, so this is Claude-only and should be described that way rather than as
"approval prompts are fixed".

**B1 — `setup status` naming your own profile as "other".** The root cause is that
`Portable` is derived from whether `--config-root` was *passed*, not from which profile is
in use, so a development binding checked with the no-flag command is compared against the
portable binding a released install uses — and correctly, if uselessly, reported as
another profile. Both status replies now carry `checkedProfile` naming what the check was
made against, so `other_profile` can be read without guessing; `docs/development.md` shows
the `--development` form and explains what seeing your own profile there means.

**B5 — `create` could not add a second Project.** One per-user service keeps one device
identity across its Projects, and `create` was always taking the first-enrollment path,
which would mint a second credential and strand the first one's Projects. The CLI now
routes an already-enrolled profile to `CreateAdditional`, which the desktop app has done
all along, and refuses a repository that is already connected. `docs/development.md` now
documents both this and `workspace add --development --root … --project …` for adding
another local root to an existing Project.

**B19 — `SQLITE_BUSY` flake.** `SetMaxOpenConns(1)` serializes writers inside one process,
but the CLI, the service, and the tests all open the same file, and a second process
writing during a held transaction was refused immediately. `busy_timeout` is now set
through the DSN rather than as a one-off `PRAGMA`, because `database/sql` may open a fresh
connection at any time and a pragma applies only to the connection that ran it. The
regression test reproduces the original `database is locked (5) (SQLITE_BUSY)` exactly
when the timeout is removed. Worth noting this was never only a test flake: in production
the same race meant a lost observation rather than a delayed one.

## Scenario coverage

Run and passing: SG-01 (twice, second time with MCP pull working), SG-03 (twice,
second time attributable), SG-06 (after B16/B18), SG-07 (silence held), plus a
symmetric mixed-vendor negative round.

Never run: SG-02, SG-04, SG-05, SG-08, SG-09, SG-10, SG-11, SG-12, SG-13, SG-14,
SG-15, SG-16, SG-17, SG-18.

**Highest-value gaps.** Negative and silence scenarios are thin — only two data points
against a gate of at least 98% receiving no proactive injection. And **SG-03 has only
ever been run with Claude as the reader**; the role-reversed form, where Codex reads and
Claude changes the contract, is what exposed B22 and belongs in the corpus permanently.

## Before the two-person test

Every code item on the open list is done. What remains is **B17**, which is not a code
change: empty briefs were reported as `degraded: false` and produced a false all-clear in
round 4, but that was a symptom of B8, which is fixed. Re-run SG-06 and check whether the
false all-clear still happens before building the honest-`degraded` guard. Building it
first would be guessing.

Round 8 had already demonstrated the full loop with real agents: contract changed, finding
routed, brief delivered through the MCP pull path, agent checked for a resolution, then
halted and escalated to the operator rather than overwriting a peer's in-flight edit.

The two risks named here previously are both closed. B9 no longer lets phantom sessions
accumulate across a working day, and a Codex user is now told when their contract moved —
partially, at roughly 69% of read-ish commands, and labelled as inferred rather than
observed wherever it appears.

The honest gap that remains is coverage of the scenario corpus, not the engine: fourteen
of the eighteen scenarios have never been run, the negative and silence cases have only
two data points against a gate of at least 98% receiving no proactive injection, and the
role-reversed SG-03 — Codex reading while Claude changes the contract — still belongs in
the corpus permanently.
