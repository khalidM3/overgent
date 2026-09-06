# Overgent design system

Status: canonical UI specification for the dashboard and desktop shell
Owner: Khalid
Last updated: 2026-08-28

This document is binding for anyone — human or agent — building UI in
`apps/dashboard` or `apps/desktop`. The implementation lives in
[`apps/dashboard/src/style.css`](../apps/dashboard/src/style.css); this file
explains *why*, so the next screen looks like it belongs without needing to
reverse-engineer the CSS.

Precedence: security/privacy and honest-fidelity rules in
[`security-privacy.md`](security-privacy.md) and
[`overgent-v1-spec.md`](overgent-v1-spec.md) outrank anything here. If a design
rule would hide provenance or overstate confidence, the design rule loses.

## 1. What the interface is for

The workroom answers one question before any other:

> **Is anything about to hit me?**

Everything else — what teammates are doing, what happened earlier, what has been
decided — is context that supports that question. A member manages their own
sessions and the collisions that reach them. They are not an operator watching a
whole fleet, and two teammates colliding with each other in code the member
never touches is not their problem and should not render as if it were.

**Needs you** is the block that answers it, and a collision is not the only
thing that can. A session of the member's own that is waiting on a permission
prompt, has failed, or has gone quiet is blocked work converging on the same
person, so it belongs in the same block under the same rule (ADR-053). The same
test excludes a *teammate's* blocked session: it is not the viewer's to unblock.

**History** is the one screen that is a record rather than a windshield. It
answers what was raised, where it was delivered, and what was settled, and it is
deliberately colourless in everything but its identity marks — see Rule 2. It
replaced two tabs, Ledger and Decisions, that asked the same question in words
nobody uses (ADR-056).

This is a windshield, not a control room. When a design choice would add
surveillance value at the cost of calm, choose calm.

## 2. The rules

These are the rules the whole look depends on. The first four are the ones the
visual identity rests on; 5 to 8 are the ones its structure and behaviour rest
on. Breaking one does not produce a
slightly different design; it produces a different product.

### Rule 1 — Hairlines and space, never filled cards

Rows are separated by whitespace, a hairline, or nothing at all. Differentiation
comes from **type weight, colour value, indentation, and spacing**.

The only persistently tinted surface in the app is the inspector panel
(`--panel`), which earns it by being a different *kind* of thing. The sidebar
uses the same ground as the main column with a single `border-right` hairline.

Filled backgrounds are allowed only for:

- transient hover (`--hover`)
- a solid primary button (`.pill.solid`)
- the brand mark and the current project monogram

Anything openable says so the same way: the whole row or card takes `--hover`
and reveals a chevron on its trailing edge, and its heading button is stretched
across the block so the affordance is the block rather than one line of text
inside it (ADR-062). A converging card's chevron takes `--alert`, because a
glyph is where colour is allowed; the hover ground stays neutral, because a
tinted background is not.

Stacks of bordered, rounded, background-filled cards are the single most
recognisable signature of generated UI. Do not add them.

### Rule 2 — Colour is status, and there are exactly two status colours

Colour never decorates, never fills a background, and never becomes a badge. It
marks a fact, in text or in a glyph, and there are two facts worth marking.

**`--alert` — this is converging on you, or it is destructive.** The warning
glyph, the finding's headline sentence, the deterministic evidence row, the count
beside "Needs you", the warning glyph on an implicated session row, the headline
of a session of your own that a vendor has reported as waiting or failed, and
the heading of a destructive settings section.

Colour tracks the strength of the evidence, not the loudness the row deserves.
A `waiting` or `error` session is a fact its own vendor reported, so it takes
`--alert` like any other converging work. **A stalled session does not.**
Silence is arithmetic, it is true without being proof of a fault, and dressing a
measurement as an alarm is the overstatement the honest-fidelity rules exist to
prevent — so it reads at ordinary weight and lets the duration speak.

**Identity is a different colour system, and it lives only in marks.** A member
chip and a vendor logo say *who*, not *what is happening*, so they are not
governed by the two status colours (ADR-055). The bound is that identity colour
never appears in **text** — never a coloured name, never a tinted row, never a
badge behind a word — because what protects "one orange sentence is impossible
to miss" is that `--alert` is the only colour a sentence can be. Member hue is
derived from the display name so one person is one colour everywhere. The chip
itself is a **solid keycap**: a squircle at a confident saturation with
near-white initials, no inner border (ADR-064). The earlier pale-tint-plus-
outline circle was illegible at a glance, which is the one job an identity mark
has — minimal does not mean colourless, and the fix for a UI reading as
monochrome is contrast *within* the identity channel, never a new status
colour. Saturation and lightness stay fixed per theme by `--member-sat`,
`--member-bg-l` and `--member-ink-l`, and only hue varies. The ramp starts
past hue 40: `--alert` sits near hue 17 and a chip below that reads as a
warning. Vendor marks carry their own brand colour (ADR-057): Codex is a blue-violet
gradient near hue 230, 118 ΔE76 from `--alert`; Claude is its brand terracotta
`--claude-mark`, 28 ΔE76 from `--alert` and softer in tone. Identity being a
glyph and status being a sentence is what keeps those two apart.

**A record takes almost no colour.** History lists cases that have already
happened; state is a word in the monospace arc line, and the colour test
applies as usual. The one exception: a decided case whose conclusion has not
yet been considered by every affected agent is still converging on someone, so
that single step — `2 not yet considered` — takes `--alert` (ADR-064).

**`--live` — this is true right now.** The repository path an agent is inside at
this second on a session row, an adapter that has been verified by a real session
event, the ready mark on a connection line, and a change that has just been
saved. It is the answer to "is this happening", nothing else.

The test for adding colour is whether removing it loses a fact. A total, a
category, a name, or a heading loses nothing, so none of those take colour: the
file count beside a live path stays neutral while the path itself does not.

There is still no info blue and no severity rainbow. Presence and selection are
carried by type instead:

| State | How it reads |
|---|---|
| Teammate online | name at `--ink`, weight 600 |
| Teammate idle or paused | name at `--ink-2`, weight 500 |
| Teammate offline | name and intent at `--ink-3`, weight 500 |
| Selected session | title at `--ink` weight 650, chevron visible |
| Unselected session | title at `--ink-2` weight 500, chevron hidden |

On a healthy screen the only chromatic pixels are the paths agents are inside
right now — a handful of small mono words. That is what keeps one orange sentence
impossible to miss.

### Rule 3 — Live work is a moving clock and moving text, never an indicator light

**There are no pulsing status dots in this app.** They are the second most
recognisable signature of generated UI, and they carry less information than the
alternatives.

Running work reads as two things:

- **A clock that counts up.** `.clock`, monospace, `tabular-nums`, advancing once
  per second from a single shared interval (`useSecondTick` in `main.tsx`).
- **Text that moves.** `.livetext` sweeps a gradient across the glyphs of the
  current action and animates a trailing ellipsis. The live thing is the
  *content*, not an ornament beside it.

Both are disabled under `prefers-reduced-motion`, where `.livetext` falls back
to solid `--ink`. Nothing else on the screen animates, ever.

### Rule 4 — Elapsed time is `47s`, `12m 16s`, `1h 04m`

Never `mm:ss`. A colon reads as a wall-clock time, so `12:07` is understood as an
hour of the day rather than as time since something happened. Use
`formatElapsed` from [`elapsed.ts`](../apps/dashboard/src/elapsed.ts) for every
duration in the product, without exception.

The service currently sends prose labels (`"Now"`, `"8 min"`); `parseElapsedLabel`
converts what it can so the clock counts from a truthful start, and renders an
unparseable label verbatim rather than guessing. **Sending real timestamps from
the service is the outstanding follow-up** — see §8.

### Rule 5 — One gutter, one content edge, three type steps

Every row in the main column hangs off the same gutter — `--row-icon` (22px)
plus `--row-gap` (12px) — so the primary text of a finding, a session and a
teammate all begin at 34px. Section headings stay flush at 0: a heading is the
spine, its content hangs beneath it. Four components each inventing their own
gutter is what made one column read as four unrelated lists (ADR-058).

The ramp is three steps, and only three:

| Step | Form | Carries |
|---|---|---|
| primary | 14.5px / 600, `--ink` | what the row is about |
| secondary | 13.5px / 400, `--ink-2` | what it means |
| machine | 11.5px mono, `--ink-3` | facts the system measured |

A finding headline at 15.5px/650 in `--alert` is the one deliberate exception,
because it is the one thing on the screen that should outrank everything else.

Peer rows are separated by a hairline; blocks are separated by space. Machine
facts are consolidated onto one line rather than stacked into three — and a fact
already visible on the line above is not repeated below it.

### Rule 6 — Streams run down, lists rank up

A **stream** is chronological ascending and the interesting end is the bottom:
the session thread opens at its newest entry, follows the tail, and stops
following the moment the reader scrolls up, offering a control to return rather
than dragging them back.

A **list** is ranked, never in insertion order. Sessions rank by what the reader
would do about them — needs you, running, open, finished — then by recency
inside each band, measured with the same elapsed label the row displays so the
order always agrees with the clock beside it. Finished work folds behind one
`details.fold` line; it is worth keeping and not worth scrolling past.

### Rule 7 — One object, one place

What a session **is** right now is an attribute of the session and reads in its
header. What it **did** is an entry in its stream. Carrying the newest event in
both places is what made a strictly chronological feed read as out of order, and
it is the general failure: when a fact appears twice the reader checks both and
trusts neither.

### Rule 8 — Harness context is not conversation

What the vendor told the agent — sandbox rules, permissions, repository
instructions — is provenance. Consecutive blocks fold into one line and open on
demand. What the person and the agent said to each other is conversation and
renders in full. A disabled control is held to the same standard: it explains
itself in words or it is replaced by a sentence, because a button that cannot be
pressed and does not say why names neither an action nor a place.

The stronger version of that failure is a control that is *enabled* and still
cannot do what it says. The browser-activation screen offered "Activate secure
session", which that page can never do — only the Overgent app mints a ticket —
and it printed the recovery only after the first press failed, so the one
instruction that resolves the state was hidden behind the dead end it explains.
**A recovery is stated before the control, not after it fails**, and the control
is named for what it actually does.

## 3. Tokens

Defined once on `:root`, redefined for `prefers-color-scheme: dark` guarded by
`:root:not([data-theme="light"])`, and again on `:root[data-theme="dark"]` so an
explicit toggle wins in both directions. **Never declare a colour only inside a
media or `[data-theme]` block** — it will not apply in the un-stamped state.

| Token | Light | Dark | Use |
|---|---|---|---|
| `--bg` | `#ffffff` | `#0a0a0a` | Sidebar and main column ground |
| `--panel` | `#fafaf9` | `#111111` | Inspector only |
| `--hover` | `#f4f4f2` | `#1a1a1a` | Transient hover |
| `--line` | `#e8e8e5` | `#232323` | Every hairline |
| `--ink` | `#0e0e0e` | `#f5f5f5` | Primary text |
| `--ink-2` | `#5e5e5a` | `#a3a39f` | Secondary text, body copy |
| `--ink-3` | `#98988f` | `#6c6c69` | Metadata, mono facts, icons at rest |
| `--alert` | `#c43d14` | `#ff7a4d` | Converging work, and destructive actions. |
| `--live` | `#1c6b40` | `#5fcf93` | A fact that is true right now. Nothing else. |
| `--solid` / `--onsolid` | `#0e0e0e` / `#fff` | `#fff` / `#0a0a0a` | Primary button, brand mark |

Neutrals are warm, not cold slate. This is deliberate: cold grey reads as an
inherited default, warm grey reads as a chosen palette.

## 4. Type

Two families, three jobs.

```css
--sans: Figtree, "SF Pro Text", -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
--mono: "Geist Mono", "SFMono-Regular", ui-monospace, Consolas, "Liberation Mono", monospace;
```

**Monospace means a machine produced it.** Paths, branches, session aliases,
revisions, elapsed clocks, fidelity and capability values, evidence labels.
**Sans means a person wrote or decided it.** Titles, intents, findings in plain
language, decisions, body copy, every control.

This mapping is not decoration. `overgent-v1-spec.md` requires honest fidelity —
labelling whether information came from Git, manual input, MCP, or a hook. The
typeface carries part of that signal, so a reader registers provenance without
reading a label.

Sizes, and there are only these:

| px | Weight | Use |
|---|---|---|
| 25 | 700, `-0.03em` | Project name, screen title |
| 15.5 | 650 | Inspector title, finding headline |
| 14.5 | 500 / 650 | Session row title (unselected / selected) |
| 13.5 | 500–600 | Body, nav, intents, controls |
| 13 | 400 | Bullets, secondary copy |
| 16 | 700, `-0.02em` | Section headings (`Needs you`, `Devices & security`) |
| 11–11.5 | 400–500 | All monospace metadata |

Do not introduce a size between these. The previous stylesheet had ten sizes
below `1rem` and the fuzz was the main reason it read as undesigned.

**Section headings are sentence case at full ink.** They were 12px grey capitals
with `0.07em` tracking, which is the one place the type scale actively fought the
layout: uppercase shrinks the x-height and tracking pulls the word apart, so the
label that is supposed to separate two blocks of content ended up quieter than
the content itself. A heading is the spine of the column and reads like one —
16px/700 at `--ink`, sentence case, no tracking. The same treatment covers the
workroom block heads, screen section titles, and the command palette's groups.
Uppercase survives only on the `.eyebrow` line above an `h1` in the centred state
cards, where it labels the page rather than dividing it.

Fonts are named with system fallbacks and **not loaded from a CDN**. A dashboard
that phones a font host would leak member IPs to a third party, which
contradicts the trust model. Self-hosting Figtree and Geist Mono as woff2 under
`apps/dashboard/public/fonts` is the outstanding follow-up (§8); until then the
system sans is the honest fallback.

## 5. Layout

Three floor-to-ceiling panels. **There is no global top bar** — each panel owns
its own height and its own header. This is the shape people recognise.

```
┌──────────────┬────────────────────────────┬──────────────────────┐
│ side  212px  │ main       minmax(0, 1fr)  │ inspector      436px │
│              │                            │                      │
│ Projects     │ toolbar (its own, not      │ its own header + ×   │
│ same ground  │ spanning the other panels) │ --panel ground       │
│ as main      │ ────────────────────────── │                      │
│ + hairline   │ column, max-width 680px    │ scrolls itself       │
└──────────────┴────────────────────────────┴──────────────────────┘
```

The inspector is **wider than the sidebar**, because the sidebar is navigation
and the inspector is where the reading happens.

The sidebar is a `nav[aria-label="Projects"]` holding brand, search, the Project
list with its add control, and App settings — **and nothing above the list**.
Both shells share that shape: the entry shell in `desktop-onboarding.tsx` and
the workroom in `main.tsx` present the same list in the same place, so switching
between them is not a change of navigation.

A **screen** — Settings, People, Add a Project — takes the main and inspector
columns together (`.workroom-shell.screen-open`) and keeps the sidebar, so the
member is never stranded on a surface with no way out except a close button. It
reuses the workroom's shape exactly: its own toolbar, its own scroll, one 680px
column, an `h1` at 25px.

`grid-template-rows: minmax(0, 1fr)` plus `min-height: 0` on each panel is load
bearing: without it the grid row sizes to its tallest content and the panels
overflow the viewport instead of scrolling internally.

The project header carries identity and freshness only: name, repository,
`rev N`, and `synced <elapsed> ago`. Nothing else earns a place there.

Directly beneath it, a **view tab row** (`.view-tabs`, `nav[aria-label="Project
views"]`) chooses between the Project's two views — Workroom and History — so
the reading order is *which Project*, then *which view of it*. It reuses the
`.settings-tabs` shape rather than introducing a second way to switch between
sections of one screen, and marks the open view with `aria-current="page"`.
Workroom carries the "Needs you" count when there is one; History carries no
count, because a running total of everything ever caught is not a thing to act
on. Both used to be root items in the sidebar, above the Project list, which
put the open Project on screen three ways at once (ADR-078).

### The main column, in fixed order

1. **Needs you** — interrupt-worthy findings touching your sessions, then your
   own sessions that have stopped (waiting, failed, or quiet). Never reorders
   below the others, and shows an explicit empty state rather than
   disappearing. Admission follows the engine's own routing verdict: a finding
   the judgment layer marked `next_turn` (ADR-045/046) is interrupt-worthy;
   one it routed to the dashboard is not, even when it touches your work, and
   renders in Elsewhere instead. The workroom re-deriving admission from
   ownership alone was the UI second-guessing its own engine (ADR-064).
   Directly beneath the heading sits the fidelity caveat (`.fidelity-note`),
   rendered **only** when semantic processing is degraded or disabled. Healthy
   processing is the expected state and says nothing.
2. **Sessions** — everyone's, in one block, grouped by the area of the product
   being touched (contract → component → deepest shared directory). Splitting
   your sessions from teammates' made the grouping unable to do its one job:
   "who else is in my lane" was answered only by reading two lists and
   matching their headings (ADR-064). The self/other distinction survives as
   row richness — your rows carry live **activity** (current file, subagents,
   elapsed clock), a teammate's row carries **intent** in one line. When an
   open finding spans two or more sessions of one area, the area heading
   itself carries the warning glyph and `· colliding`: the group label *is*
   the collision, shown as structure. Finished sessions fold behind one
   `details.fold` line. With no teammates the block states a fact rather than
   naming an absent setup step (ADR-054).
3. **Elsewhere in the Project** — open findings the lead block did not admit:
   other people's, and dashboard-verdict findings on yours. Rendered `.quiet`;
   present so nothing is hidden, styled so it never competes.

**History** is the Project's second view, reached from the tab row above the
column rather than from the sidebar, and it is not a block in this column: it
answers a different question — what has already been handled, rather than what
is happening — about the same Project. It names itself once, on its tab, and
carries no heading of its own beneath it. It is a **case log** (ADR-064): one
entry per finding carrying its whole lifecycle on a single mono arc line —
raised → decided or dismissed → sent to N sessions → considered — under day
dividers, newest movement first, with a one-row filter (All / Open / Settled /
Dismissed) and the raw event stream folded at the bottom. Decisions that never had a finding are
cases too. It stops at consideration: Overgent knows a decision was routed and
whether the agent acknowledged reading it, and does not know whether the agent
then did the right thing. Wording that implied otherwise would fail the
honest-fidelity rule this document is subordinate to.

## 6. Components

### Session row (`.session-row`)

No fill, no border, no rail. Grid of `26px | 1fr | auto`:

```
[vendor]  Rotate the browser session boundary        ⚠  1m 08s  ›
          Codex · codex-a1b2c3 · feature/session-rotation
          Editing token rotation…                     ← .livetext when active
          apps/dashboard/src/session.ts      4 files   ← own line, mono
          └ reviewer subagent · active                 ← indented, elbow rule
```

The file line is **its own line**, never appended to the action. Agents move
through files constantly; the current path comes from the newest activity entry
(`currentPath`) and is keyed by path so it re-mounts and fades when it changes.
The right-hand count is `pathCount` for the whole session.

Selection is `aria-current="true"` plus weight and a visible chevron — no
background.

### Converging block (`.converge`)

No tint and no panel. An alert glyph, the finding sentence in `--alert`, then
**both sides of the collision** as compact `.mini` rows — member chip, name,
session title, elapsed clock, chevron. Each opens that session in the
inspector. The reason line renders **only when it says something the headline
has not** (`reasonAddsFacts`): on live data both derive from the same
evidence, and a card repeating itself in two registers was most of its bulk.
The mono meta line carries severity (only at high or critical) and first-seen.

The stretched headline is the only opener — a second button opening the same
panel was two controls for one intent (ADR-064). Evidence, confidence, and the
decision live in the inspector (ADR-060). A `dependency_ready` finding — the
one kind that is good news — renders `.quiet` with a neutral check glyph,
never as an alarm.

The block is a `<section>`, not a button, because it contains buttons; the
heading carries the click target and the accessible name.

### Inspector

Session detail opens directly into one chronological live feed. A compact
header identifies the task, member, vendor, and branch; session aliases belong
in technical details. The hairline strip beneath the header says what an active
or waiting agent is doing now; it disappears when the session completes. A
completed session says **Complete** in the header and ends with one **Session
ended** boundary in the feed. User and assistant messages, bounded tool
activity, session start/end, permission waits, parallel-agent lifecycle, and
coordination delivery share one stream in timestamp order. Consecutive tool
calls coalesce into one row while retaining every tool name. Thinking is
collapsed by default. CommonMark/GFM renders structurally; raw HTML and remote
images never render. There are no reading-mode tabs between the member and the
session.

Context delivery is shown honestly. A delivered brief item reads
**Coordination routed** with a route glyph and the bounded reason. A later
acknowledgement reads **Agent considered coordination**. Never call either
event "steered," "corrected," or "followed": acknowledgement proves
consideration, not compliance, behavioral adjustment, or correctness. On the
viewer's own session, the routed event may use `--alert` because it is work
converging on them; the acknowledgement and every teammate event stay neutral.

Technical provenance is supporting detail, not permanent navigation. Fidelity,
safe-path coverage, brief delivery, attention behavior, large-change scope, and
the file inventory live behind an **info** control at the top-right of the
session header. It opens an anchored, dismissible Session details popover
without replacing or filtering the feed. Combine related capabilities into
plain-language Source and Coordination rows instead of repeating each wire
value. Show the branch only in the header and keep files last. The info control
and popover render for every workstream, including Git-observed work without an
agent. Use an info glyph, not a settings gear: this surface explains provenance
and does not configure the external agent.

Codex and Claude Code use their official product marks. Render them in neutral
ink (including a grayscale Codex app icon) so vendor branding never introduces
a second status colour or competes with `--alert`.

Reported subagents appear as compact inline parallel-agent entries in the same
feed, and only when they exist. The feed describes their role and state in human
terms; machine aliases live in Session details. Separate subagent conversation
output must not be invented; add it inline only when an adapter actually
exposes it.

### The finding inspector, and the three exits (ADR-064)

A finding and its decision are **one object at two ages**; there is no
separate resolve tab, and the umbrella word is **finding** — "collision" is
one kind of eight and mislabels the rest. Top to bottom:

- **What is true**: headline, reason, and the branch statement.
- **Sessions**: a two-party finding renders both sides as **twin panes** —
  member, goal, now, from the scope snapshot — so comparing them never means
  opening one, going back, and holding it in your head. Each pane still
  drills into the full session.
- **What changed**: contract drift renders its structured divergence — the
  file, who moved it, read/changed times, and each symbol as `was` / `now`
  signature lines. `was` takes `--alert` and `now` takes `--live` **in text**,
  never as filled rows (rule 2). Prose evidence keeps the mono list.
- **Decision**: one always-present composer. Suggested outcomes are chips
  phrased per finding kind that *prefill* the text — the member always sees
  and can edit the exact words before anything is sent — and "Settled outside
  Overgent" is one of them, because a conclusion reached in Slack still has to
  reach the agents. The send control names its targets ("Send to both
  sessions"); the routing note names them fully. There is no create-card
  pre-step: the sync card is plumbing, created when the first decision lands.
- **After sending**: the decision note becomes a per-session delivery
  tracker — queued for its next turn → delivered → considered — fed by
  `decisionDeliveries`. Considered records that the agent read it, never that
  it complied. This is the loop closing where the decision was made.
- **Dismiss** is the only other exit, and it names its reason inline — not
  related / already coordinated — which *is* the feedback that trains the
  engine. Acknowledge and the standalone Useful/Not-related row are gone:
  "read but unhandled" is the ambiguity the product exists to remove, and a
  survey row beside real controls was input with no downstream effect.

There are **no comments**. Discussion already has homes; the panel's only
inputs are ones with an observable effect on the affected agents.

### Screens, and the one dialog

**A modal is for a decision that must be finished before returning. Nothing in
the Project lifecycle is one, so none of them are modals.** Settings, People and
Add a Project are screens: they replace the main and inspector columns, keep the
sidebar, and are entered and left like any other place in the app. A modal over a
workroom you can still see, holding a form you might want to leave and come back
to, was always the wrong container.

`Screen` in [`screen.tsx`](../apps/dashboard/src/screen.tsx) is the shared shell:
a back control that **names where back goes**, the `h1`, an optional lede, and
`ScreenSection` for each group. It carries `aria-labelledby` on the `<main>`, so
the region still answers to "Settings" the way the dialog did. Escape still goes
back, because the dialogs these replaced closed that way.

Screens are a stack, not a flag each. People is reachable from the toolbar *and*
from inside Settings; entered from the toolbar it is top level and back returns
to the Project, entered from Settings it stacks and back returns to Settings.
Anything less makes the back control lie.

- **`NewProjectScreen`** — enrollment runs through the native bridge, which only
  exists inside the desktop shell. **While the bridge is being probed the screen
  says so.** Rendering the form during the probe and swapping it for the hand-off
  a moment later meant a member could start filling in a form that vanished under
  them and be told to open the app they were already looking at — the live
  workroom is served from the hosted origin *inside* that same desktop window.

  When the bridge is out of reach the screen asks **where it is** before it says
  anything, because the two situations need opposite answers and the wrong one
  is inert:

  | Where | What it does | Why the other one fails |
  |---|---|---|
  | Hosted page in the desktop window | Navigates this window to the shell's own origin, saying "continuing on this Mac" | This WKWebView has no navigation policy delegate, so it never hands `overgent://` to the system: the link does nothing at all |
  | A real browser | Opens `overgent://new-project` | The shell's origin is not reachable from outside the app |

  `isDesktopShell` answers that question from the webview's user agent, which
  `apps/desktop` stamps with `desktopUserAgentName`. It decides copy and
  destination only — **no capability is ever granted because of what a user
  agent claims**, and the native bridge stays unreachable from every origin but
  the shell's own.

  Inside the shell the hand-off happens on arrival rather than behind a button:
  the member already asked for this screen, and an interstitial offering one
  control is asking the same question twice. Both variants name a fallback that
  does not depend on the hand-off working — **Add a project…** in the menu bar,
  which is the one route back to the setup screen from a window showing a live
  Project — and `overgent create` remains the last resort. A hand-off control
  that silently does nothing is the exact failure this screen exists to prevent.
- **`DesktopOnboarding`, first run** — three steps, not one form: **what this is
  and what is already on this Mac** → **which repository** → **what to connect
  and what that shares**. A `.step-track` names the step in mono above three
  hairline segments. The order is the order a member can answer in, and the two
  things most worth reading — the sharing boundary and the sentence saying a
  background service is about to start — now sit directly above the button that
  does it, rather than below four fields and a tablist.

  The first step **states what was detected before asking for anything**.
  Finding nothing is a real answer and reads as one: neutral ink, a plain glyph,
  and a sentence saying the Project can still be created and sessions appear
  when an agent is installed. It is not `--alert`, because nothing needs fixing,
  and the "found" line is not `--live` either — removing the colour would lose
  no fact (rule 2), and the ready mark is earned later by a real session event.
  Detected agents arrive ticked in **both** places a Project is set up; the
  checkbox group is one shared component (`AgentOptions`) for that reason.
- **`PeopleSections`** — invite, members, open invites. One implementation with
  two ways in: the `PeopleScreen` reached from the workroom toolbar, and the
  People tab of Project settings. Adding a teammate should never require hunting
  through Settings, and Settings should not be missing the thing it is about;
  a row that left for another screen satisfied neither.
- **`SettingsScreen`** — one Project, in four tabs: **Project**, **People**,
  **Intelligence**, **Data**. It was a single column of seven sections that were
  not all about the same subject, and the one subject it never covered was the
  Project itself.

  - **Project** answers what this Project *is* — repository, the folder on this
    Mac (copyable, because the next thing anyone does with a path is paste it),
    whether coordination lives on this Mac or on a server, which server, and
    what you are in it. Then the signpost to App settings, then the destructive
    section last, separated by a hairline rather than a tinted panel, its
    heading in `--alert`. Deleting or leaving calls `onRemoved` so the shell
    drops the Project and moves to the next one; queuing the request and leaving
    the member inside a Project they no longer belong to is the failure mode
    that exists to prevent.
  - **People** carries the shared sections plus devices, because who can reach
    this Project and what can reach it are the same question.
  - **Data** carries the privacy statement and the export.

- **Identity is not a Project setting.** The display name used to be the first
  section of every Project's settings, which asked a question nobody answers
  differently twice — people are not called one thing in one Project and
  something else in the next. It lives in App settings under **You**, with the
  member chip rendered beside the field so the initials and hue are visible
  before saving rather than discovered later on a session row. The name is still
  *stored* per Project, because that is where a member row lives, so one save
  writes it to every Project this device belongs to and says how many that was;
  a Project that could not be reached is named rather than silently skipped.

**The command palette is the only dialog left**, because it genuinely is modal:
it is a transient overlay you dismiss. It closes three ways — Escape, a backdrop
click, and a visible control. Its `esc` label is a real `<button>` for exactly
that reason; a keycap that looks pressable and is not was a bug, not a
decoration.

### App settings, and why each section is a row

Every section of App settings was once a single control and a sentence:
appearance was one button whose label named the *other* state, an agent was a
checkbox and a button stacked inside the same paragraph, and the local service
was two lines of prose. Each was accurate, none of them looked like a place,
and together they read as a form somebody had assembled one control at a time.

They share a shape now, and it is the workroom's: **what the thing is on the
left, what it is set to or what to do about it on the trailing edge, hairlines
between.** No section invents its own layout.

- **Appearance is three choices, and each shows what it means.** Light, dark,
  and following macOS — the third being the state the old boolean could not
  express, which is why a Mac that switches at sunset used to leave Overgent
  behind. Each swatch is a literal miniature of that palette, written with the
  same token values the theme uses; system shows both halves. These are the only
  fixed colours in the app, because a preview of the other theme cannot be drawn
  in the current one.
- **An agent is a row with one action.** Its own vendor mark, its name, what
  Overgent is actually getting from it, and — on the trailing edge — Connect or
  Disconnect. Connect takes `--live` and disconnect takes `--alert` as outlined
  pills (`.pill.affirming`, `.pill.alerting`): the two colours already mean "true
  right now" and "destructive", so nothing new is introduced and no background is
  filled. The status line never claims observation a vendor has not shown —
  hooks awaiting review outrank a runtime claim, because until they run the agent
  is connected and reporting nothing.
- **The local service reports facts, not prose.** Status, address, version, data
  on disk, in a two-column list: the name in sans because a person wrote it, the
  value in mono where a machine measured it. Running now takes `--live`.

### The intelligence form is the pipeline, in three levels

Two screens configure providers — this Mac's defaults for new Projects, and a
Project's own settings — and they are one component (`intelligence-form.tsx`)
with two sets of sentences. The fields can be shared because the two write
shapes are identical; the sentences cannot, because a key saved on this Mac
stays on it and a key saved on a shared Project is uploaded to a server other
members' sessions spend it from (ADR-073, ADR-077).

The first version of this screen named the codebase's internals — "Assess
coordination", "Judgment provider" — and left the member to work out what any
of it caught. Nobody arrives here wanting to configure a judgment provider.
They arrive wanting to know what Overgent notices and what it costs to notice
more. So the form **is** the detection pipeline, in the order it runs
(`coordination-intelligence.md` §4):

| Level | What it catches | What it needs |
|---|---|---|
| 1 · Overlapping code | Same file, same symbol, a contract that moved under a session that read it | Nothing. Always on |
| 2 · Related work | Work that overlaps in meaning without overlapping in files | Built-in matching, or an embedding provider |
| 3 · Judgment | What a candidate means, how certain that is, and whether it interrupts | A model provider |

Level one has no controls and is not a lesser version of the two below it:
saying plainly that it always runs is what makes the other two optional rather
than load-bearing. Levels two and three carry one picker each and open into
fields only once switched on, so a member who wants none of it reads three
sentences and leaves.

The rest of the rules the form keeps:

- **A provider is a preset, never a new enum value.** The schema has three
  judgment providers and two embedding providers and is not going to grow: every
  other vendor is `openai-compatible` with a base URL, and
  `intelligence-catalog.ts` holds the addresses. The stored value is an
  **origin** — the client appends `/v1/chat/completions` itself — so a preset
  holding the documented endpoint would 404 in a way that reads as a bad key.
  **Only vendors that have actually been run are offered**, because an option in
  a picker is a claim that it works; the rest sit in a comment in that file with
  their verified addresses, one line away from being offered.
- **A model is chosen from a list, and typed when it is not on one.** The list
  is what each provider's documentation named on `PRESETS_CHECKED`, and the date
  is on screen. "Other model…" is a first-class choice rather than an escape
  hatch, and a saved ID the list has never heard of opens the field already typed
  in rather than being silently dropped.
- **The endpoint is on screen.** One mono line spells out the full URL the key is
  about to be spent at — the fact that settles whether the member meant this
  provider. It is hidden only while a custom address is empty, where printing the
  fallback would name a destination nobody chose.
- **A key belongs to one destination.** Changing provider or address drops the
  saved key, and when it is dropped *because* the destination moved there is no
  undo — offering to keep it would send a key issued for one host to another. A
  removal the member asked for keeps its undo, because that failure is a
  mis-click rather than a leak.
- **There is no dimensions control.** Embeddings are fixed at 1024 and changing
  the model migrates every vector already stored, so the form says that instead
  of offering a number.
- **A held save says what it is waiting on**, beside itself, per Rule 8: a
  missing model, a missing address, or an unticked agreement to upload a key.

### Failure states that the member can act on

A failure the member could fix must never surface as a raw error string. The
rule: **name the cause, say what is safe, offer the one action that recovers.**

The worked example is a rejected device credential. The hosted API returns HTTP
401 for two different situations, and they need different copy:

| Code | Means | Recovery |
|---|---|---|
| `credential_revoked` | An owner removed this device | Needs a fresh invite from an owner |
| `unauthorized` | The deployment has no record of the credential | Re-enroll from the app |

`hosted.ClassifyCredentialError` maps both onto a `CredentialStatus`, which
`OnboardingService.State()` reports as `state.credential` so the desktop shell
shows the recovery screen **on open**, not only after an action fails.

Three rules this establishes for any similar state:

1. **Never offer a destructive recovery for a failure you could not verify.**
   `uncertain` (offline, timeout, server fault) gets its own screen with a retry
   and no reset button. `ResetEnrollment` also re-checks server-side and refuses
   unless the credential is genuinely rejected - being offline is not being
   locked out, and erasing a working enrollment is unrecoverable.
2. **Say what is safe before asking for confirmation.** The screen states that
   repositories and code are untouched, and the confirm step lists what is
   removed, what is kept, and what happens next.
3. **The recovery is an in-app action.** If the only way out is a terminal
   command, it is not a solution - most members will never run one.

The check and the safety gate live in `internal/onboarding`
(`Service.CredentialState` and `Service.Reset`), shared by the desktop app's
**Reconnect this Mac** and by `overgent reset` for headless and support use.
`overgent reset --force` skips the gate for an operator who has already
established the enrollment is dead; nothing in the UI can reach that flag.

### Buttons

`.pill` for everything, `999px` radius. `.pill.solid` is the single primary
action in any context. `.text-button` for rare, quiet actions. `.icon-button` at
`30px` with a `9px` radius for toolbar icons.

Label controls with the decision the user is making — "Talk to Mina about this",
not "Create sync card".

## 7. Accessibility and motion

- Every interactive row is a real `<button>` with an `aria-label` that names the
  action and its subject. Labels must be **unique on screen**: a teammate can
  appear in Nearby and inside a converging block at once, which is why those
  carry distinct names (`Open Mina's side of this collision`).
- Section labels are real headings (`h2` for block heads, `h3` in the inspector)
  so the panels can be navigated structurally.
- Focus is always visible: `2px solid var(--ink-3)` at `2px` offset.
- All motion sits behind `@media (prefers-reduced-motion: no-preference)`.
- Never reorder a list under the pointer. Sorting is computed on snapshot
  change, not on hover.

## 8. Known follow-ups

1. ~~Self-host the fonts.~~ Done: Figtree and Geist Mono ship as latin-subset
   variable woff2 in `apps/dashboard/public/fonts`, declared at the top of
   `style.css`. No font host is contacted.
2. **Finish the timestamp migration.** Workstreams now carry `updatedAt` and
   findings `firstSeenAt`/`lastSeenAt` as ISO ground truth beside the prose
   labels; History orders and day-divides by them. Still prose-only:
   `ActivityItem.at` and the row clocks, which still count from parsed labels.
3. **History search.** The case log filters by outcome (All / Open / Settled /
   Dismissed); free-text search and kind/person filters wait until a real
   Project's log is long enough to need them.
4. **Narrow viewports.** The shell has `min-width: 1240px` and scrolls
   horizontally below that. A collapsed-inspector breakpoint exists in CSS
   (`.no-inspector`) but is not yet wired to a control.
5. **Cross-Project switching in one browser session.** Switching between
   Projects already enrolled on this Mac now works in place: the session lists
   every Project the device belongs to, the sidebar switches between them, and
   the live poll follows the Project on screen (ADR-076). The sidebar is now the
   top level of the interface, with no root items above the list (ADR-078), so
   that switch is the primary navigation rather than a control below two others.

   What is still one Project at a time is **liveness**. `LiveApp` starts exactly
   one `LiveProjectSource.start(projectId)`, so every other Project's snapshot is
   frozen at page load — which is why the collision count on a sidebar row is
   fresh only for the Project on screen, and why there is no all-Projects view.
   `timers` is already keyed per Project, so N polls is a small change; the two
   things that are not are that `onStatus` is a single global setter (one
   unreachable team backend would report the whole app offline while the visible
   Project is healthy), and that `scopeKey(projectId, repoFingerprint)` scopes
   every finding, contract fingerprint and semantic object, so nothing
   coordinates across Projects and a cross-Project view could only ever list.
   Both are stated in ADR-078.

   *Creating* a Project is still a hand-off — the sidebar opens the native
   Project-creation screen, which reuses the enrolled device plus the one
   running local service, and opening the newly created Project performs a
   fresh one-time native activation, which is why "Open Project" still passes
   through an activation confirmation. The remaining seam is that the hosted
   workroom cannot reach the local service at all, so adding a Project from it
   is a hand-off to the desktop shell rather than something that happens in
   place. The hand-off is now reliable and
   correctly worded in both directions (see `NewProjectScreen` above), but it is
   still a hand-off: the window leaves the live Project view to add one, and
   comes back through a fresh activation. Closing that would mean either
   exposing a narrow authenticated loopback endpoint to the hosted origin, or
   keeping the desktop window on its own origin and embedding the Project view,
   and neither has been designed yet.
6. **Desktop title bar.** `apps/desktop/main_darwin.go` uses
   `MacTitleBarDefault`, which guarantees the macOS traffic lights cannot
   overlap the sidebar brand mark. The frameless alternative
   (`MacTitleBarHiddenInset` plus reserved space and a drag region at the top of
   the sidebar) would suit the floor-to-ceiling panels better, but it needs a
   desktop build to verify and has not been attempted.

## 9. Checklist before adding UI

- [ ] No new filled card, tinted panel, or coloured badge.
- [ ] No new colour; status still means `--alert` or `--live` and nothing else,
      and removing the colour would lose a fact.
- [ ] Section headings are sentence case at `--ink`, never grey capitals.
- [ ] No pulsing dot, spinner, or indicator light for "running".
- [ ] Every duration goes through `formatElapsed`.
- [ ] Machine facts are monospace; human statements are sans.
- [ ] New text sizes come from the table in §4.
- [ ] Interactive rows are buttons with unique, action-shaped `aria-label`s.
- [ ] Motion is behind `prefers-reduced-motion` and is the only thing moving.
- [ ] Provenance and confidence are still visible; nothing reads as more certain
      than the evidence supports.
- [ ] A new surface is a screen unless it is genuinely modal, its back control
      names where back goes, and a destructive action tells the shell what
      changed.
- [ ] Any failure the member could fix names its cause, says what is safe, and
      offers an in-app action - never a terminal command, and never a
      destructive recovery for a failure that could not be verified.
