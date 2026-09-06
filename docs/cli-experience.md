# Overgent CLI experience

Status: canonical product and implementation contract  
Owner: Khalid  
Last updated: 2026-09-06

## 1. Purpose

The Overgent CLI is the terminal companion to the Project workroom. It is not
a coding-agent shell and does not own a model loop, repository edits, command
execution, or agent permissions. Its first job is to answer:

> Is anything about to hit me?

The CLI is human-readable and Project-aware by default, scriptable on every
path, interactive only when attached to a terminal, and honest about evidence,
delivery, degradation, and privacy.

## 2. Interaction modes

One command surface supports four explicit modes:

1. **Glance.** `overgent` and `overgent status` show the current Project,
   service/sharing state, and the smallest useful next action.
2. **Guided.** Enrollment, adapter connection, recovery, resolution, and
   destructive actions may prompt when stdin is a TTY. Every answer also has a
   flag, and `--no-input` disables prompting.
3. **Live.** `overgent status --watch` (equivalently `overgent watch`) follows
   meaningful Project changes. It appends rather than repainting: no screen
   clear, no alternate screen, so scrollback stays the record. A refresh is
   drawn only when the fingerprint of what is displayed — sessions, open
   findings, coverage gaps — actually changes, which is what keeps a long watch
   free of heartbeat noise. Equal-severity findings are ordered by id so the
   list moves only when the Project moved. It stops on Ctrl-C and supports JSON
   Lines for programs.

   Because a quiet Project prints nothing between changes, liveness is carried
   by one footer line rewritten in place at the bottom of the stream. Following
   design-system rule 3, that line is a clock and not an indicator light: it
   counts how long the Project has been quiet, advances once per second
   independently of the poll interval, and states plainly when the last poll
   could not see everything. A spinner is specifically rejected — it reassures
   at exactly the same rate whether or not the service is reachable, which is
   the condition this command exists to surface. The footer writes no newline,
   so frames above it are never disturbed; it is suppressed entirely on a pipe,
   a file, or a dumb terminal, where the frames alone remain a complete and
   correct result.
4. **Machine.** `--json` and `--jsonl` produce versioned stable records on
   stdout. Progress and diagnostics use stderr. Non-TTY execution never emits
   ANSI escapes or waits for a prompt.

A full alternate-screen TUI is deferred until measured headless/watch use
demonstrates demand. The desktop/browser workroom remains the dense visual
surface.

## 3. Context selection

Commands resolve a Project in this order:

1. an explicit `--project`;
2. the registered workspace with the longest canonical root containing the
   current directory;
3. one available Project for read-only commands, while naming that choice;
4. an interactive picker when more than one Project remains.

Non-interactive ambiguity fails with a recovery that names `--project`. A
mutating command never relies on a hidden global active Project. Repository
roots are canonicalized and symlink-safe using the same config/Git boundaries
as the service.

## 4. Public command surface

The root help groups commands by the job a member is doing:

```text
Daily
  status                  Show whether anything needs you
  watch                   Follow agents and findings until Ctrl-C
  open                    Open the Project in the app, or a browser
  pause | resume          Control outbound sharing

Projects
  init                    Create or join a Project, guided
  create | join           Create or join directly
  projects                List registered Projects
  workspace               Manage registered checkouts

Agents
  setup                   Connect, inspect, repair, or remove adapters
  focus | unfocus         Temporarily quiet inbound coordination
  intent | scan           Report workstream intent, refresh evidence

Configuration
  ai                      Configure optional Project intelligence
  privacy                 Explain what may sync and what stays local

Maintenance
  doctor | diagnostics    Inspect health with privacy-safe output
  update                  Update or roll back the executable
  service | backend       Advanced local process/storage controls
  reset                   Clear enrollment for one or all backends
  version | help | completion
```

`open` is the single destination command (ADR-080). It routes to the desktop
app, falls back to the browser dashboard when the app is not installed and says
so on stderr, and takes `--web` to force the browser when that is the surface a
member wants. `--web` is not a headless mechanism: both paths shell out to
`/usr/bin/open`, which needs a GUI session, and there is no flag that prints the
activation URL instead of opening it.
`dashboard` survives as an alias that implies `--web`, routed through the same
dispatch case rather than a second implementation.

`mcp`, `agent-hook`, deprecated aliases, release verification, and development
registration remain supported integration/maintainer surfaces. They are marked `Internal` in the
command catalogue: root help omits them, while `overgent help NAME` and shell
completion still resolve them, so a support note or hook trace is never a dead
end. Every catalogue flag carries a one-line description; a flag without one is
treated as an incomplete command entry and fails the surface tests.
Existing command names and flags stay compatible. New noun-group aliases may
be added only when they route to the same implementation rather than creating
two behaviors.

## 5. Root behavior

- With no enrolled Project on an interactive terminal, `overgent` explains the
  product and offers one front door, `overgent init`, with the direct
  `--local`, `--team`, and `--join` forms beside it. Every first-run recovery
  across the CLI names `init`; a device with nothing registered is never sent to
  `--project ID`, which would name an id that cannot exist. It makes no change
  before a choice.
- With an enrolled Project in the current workspace, `overgent` behaves as
  `overgent status`.
- Outside registered workspaces, it lists Projects and explains how to select
  one; it never combines Projects into a cross-Project workroom (ADR-078).
- With redirected output, no arguments print concise help and exit without
  prompting.

## 6. Information hierarchy

Status is one Project at a time and renders, in order:

1. Project name, mode (`local` or `team`), repository, and branch when known.
2. **Needs you:** findings routed `next_turn` to the member's sessions, then the
   member's vendor-reported waiting/error sessions, then qualifying quiet-time
   measurements in neutral styling.
3. **Sessions:** everyone in one area-grouped block, ranked needs-you, running,
   open/idle, finished.
4. **Elsewhere:** review-recommended Project findings that do not converge on
   the viewer.
5. Only relevant service, sync, adapter, or semantic degradation.

If a backing API cannot supply one of these facts, the CLI omits it or labels
the coverage unavailable. It never derives an all-clear from missing evidence.

Needs you is evaluated once, in `cmd/overgent/needsyou.go`, and rendered by both
`status` and `watch`; a second implementation would eventually disagree with the
first. Its state is four-valued (ADR-081):

- `attention` — routed findings or waiting/error sessions converge on the viewer;
- `clear` — every evidence source answered and none of it lands here;
- `partial` — some evidence was reachable and some was not;
- `unavailable` — no evidence source answered at all.

`clear` is the only state that prints "Nothing needs you", and it is legal only
when both the findings source and the local session source actually answered.
A finding is Needs you when its `delivery` is `next_turn` and its
`workstreamIds` name a workstream this member owns for the Project on this
device — every such workstream, not only the current checkout. A finding on this
member's work carrying no `delivery` verdict is reported as missing coverage,
never read as `silent`. A finding naming no workstream is Elsewhere, not work
for whoever is looking. `status` renders Elsewhere as a count; `watch` has room
to list it.

## 7. Language and visual grammar

- `Project` is the only user-facing top-level container.
- Human statements lead; opaque IDs appear only for an action or under
  `--verbose`.
- Alert color marks only work converging on the viewer or a destructive action.
- Live color marks only a fact true now. All output remains understandable with
  color removed.
- Identity color, when terminal capability supports it, stays in a small vendor
  or member mark and never colors prose.
- No severity rainbow, pulsing dot, permanent spinner, invented percentage, or
  decorative banner. Running work reads as a clock that counts up, never as an
  ornament beside it; `watch`'s footer is the only animated element in the CLI,
  and it animates a fact.
- Durations use `47s`, `12m 16s`, and `1h 04m`; never `mm:ss`.
- Lists rank actionable work first; streams append chronologically.
- A finding summary carries its plain-language reason. Detail carries evidence,
  provenance, confidence, timestamps, and delivery state.
- `delivered` and `considered` never mean `followed`, `fixed`, or `correct`.

## 8. Human and machine output

Every public read command supports `--json`. Streaming commands support
`--jsonl`. JSON records include `schemaVersion`; field removals, renames, or
meaning changes require a new output schema version.

Global presentation flags:

```text
--json      emit one JSON document                         implemented, per command
--jsonl     emit one JSON record per streamed event        implemented, per command
--no-color  never emit ANSI color                          implemented, global
--no-input  never prompt                                   implemented, global
--quiet     suppress successful human output               not yet built
--verbose   include IDs, exact paths, timestamps, fidelity not yet built
```

`--no-color` and `--no-input` are global: they are parsed once, before dispatch,
and every command reads the same decision rather than re-deriving it from the
environment. `--no-input` is checked ahead of terminal detection, so an attached
TTY never overrides an explicit refusal to be asked. `--json` and `--jsonl` are
per-command, because which of the two applies depends on whether the command
streams; asking a streaming command for `--json` returns the other spelling
rather than a flag-parser usage dump.

Rows marked *not yet built* are commitments, not descriptions. Nothing in the
implementation advertises them until they exist.

`NO_COLOR`, `TERM=dumb`, redirected output, and terminal width are honored.
Unicode status marks degrade to ASCII. JSON and quiet modes never contain ANSI
escapes.

Ordinary `status` exits zero when it successfully reports a finding, and exits
non-zero only when it could not report at all. `--fail-on
needs-you|degraded|unavailable` is a planned automation opt-in and is not yet
built.

## 9. Guided operations

### Enrollment

Local creation is the first and recommended choice. Before team enrollment,
the CLI explains that Project membership shares bounded coordination facts and
that raw source, raw diffs, Git objects, environment values, credentials,
prompts, transcript files, and command/tool output do not cross the wire.
Connecting an adapter is an explicit choice during or after enrollment and
states the exact managed configuration it will change. AI setup is never an
onboarding prerequisite.

### Findings

A finding can be inspected, resolved with an outcome routed to every affected
session, or dismissed with the existing feedback vocabulary. The exact outcome
and delivery targets are visible before send. There is no standalone
acknowledge action or comment thread (ADR-064).

### Pause and focus

Pause is outbound and scoped to the named workspace or Project on this device.
Focus is inbound, per session, local, and always expires. Copy always explains
the asymmetry so quieting oneself cannot be mistaken for hiding work.

### Recovery

An actionable failure names the cause, says what remains safe, and gives one
recovery. An uncertain backend/credential failure never offers destructive
reset. Repair touches only structurally recognized Overgent-owned state and
preserves unrelated agent configuration.

## 10. Privacy and diagnostics

Human status names whether the Project is local, Overgent Cloud, or another
server. Privacy/status output distinguishes:

- bounded coordination facts that may sync;
- local raw material used to derive them;
- content prohibited from the wire;
- adapter fidelity and runtime verification;
- optional provider processing and degradation;
- pause state and retention.

Diagnostics retain the existing allowlist. They never contain Project IDs,
repository paths, database contents, environment values, credentials, raw
errors, events, tool results, or command output. No support bundle uploads
itself.

## 11. Performance and accessibility

- Help and version do not load config or contact a service.
- A warm local status targets less than one second.
- All I/O and watch operations honor context cancellation.
- Layout snapshots cover 40, 80, and 120 columns.
- Color is never the only signal; `NO_COLOR` and ASCII output are first-class.
- Interactive choices expose ordinary numbered input in terminals that cannot
  support cursor control.
- Prompts state how to cancel and never trap closed stdin.

## 12. Verification

CLI changes require focused unit/golden tests plus the repository gates. Tests
use temporary profiles/repositories and injected readers/writers; they never
read contributor state. Required scenarios include first run, one and multiple
Projects/backends, current-directory resolution, offline service/backend,
paused workspace, optional AI unavailable, configured-but-unverified adapters,
TTY/non-TTY, JSON stability, narrow widths, cancellation, and prohibited-data
absence in diagnostics.

