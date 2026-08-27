# Stickguy design system

Status: canonical UI specification for the dashboard and desktop shell
Owner: Khalid
Last updated: 2026-08-26

This document is binding for anyone — human or agent — building UI in
`apps/dashboard` or `apps/desktop`. The implementation lives in
[`apps/dashboard/src/style.css`](../apps/dashboard/src/style.css); this file
explains *why*, so the next screen looks like it belongs without needing to
reverse-engineer the CSS.

Precedence: security/privacy and honest-fidelity rules in
[`security-privacy.md`](security-privacy.md) and
[`stickguy-v1-spec.md`](stickguy-v1-spec.md) outrank anything here. If a design
rule would hide provenance or overstate confidence, the design rule loses.

## 1. What the interface is for

The workroom answers one question before any other:

> **Is anything about to hit me?**

Everything else — what teammates are doing, what happened earlier, what has been
decided — is context that supports that question. A member manages their own
sessions and the collisions that reach them. They are not an operator watching a
whole fleet, and two teammates colliding with each other in code the member
never touches is not their problem and should not render as if it were.

This is a windshield, not a control room. When a design choice would add
surveillance value at the cost of calm, choose calm.

## 2. The four rules

These are the rules the whole look depends on. Breaking one does not produce a
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

Stacks of bordered, rounded, background-filled cards are the single most
recognisable signature of generated UI. Do not add them.

### Rule 2 — Colour is status, and there is one colour

`--alert` is the entire chromatic palette. It marks work converging on you and
nothing else: the warning glyph, the finding's headline sentence, the
deterministic evidence row, the count beside "Converging on you", and the small
warning glyph on an implicated session row.

There is no success green, no info blue, no severity rainbow. Presence, liveness
and selection are carried by type instead:

| State | How it reads |
|---|---|
| Teammate online | name at `--ink`, weight 600 |
| Teammate idle or paused | name at `--ink-2`, weight 500 |
| Teammate offline | name and intent at `--ink-3`, weight 500 |
| Selected session | title at `--ink` weight 650, chevron visible |
| Unselected session | title at `--ink-2` weight 500, chevron hidden |

On a healthy screen, the number of chromatic pixels is zero. That is what makes
one orange sentence impossible to miss.

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
| `--alert` | `#c43d14` | `#ff7a4d` | Converging work. Nothing else. |
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

This mapping is not decoration. `stickguy-v1-spec.md` requires honest fidelity —
labelling whether information came from Git, manual input, MCP, or a hook. The
typeface carries part of that signal, so a reader registers provenance without
reading a label.

Sizes, and there are only these:

| px | Weight | Use |
|---|---|---|
| 25 | 700, `-0.03em` | Project name |
| 15.5 | 650 | Inspector title, finding headline |
| 14.5 | 500 / 650 | Session row title (unselected / selected) |
| 13.5 | 500–600 | Body, nav, intents, controls |
| 13 | 400 | Bullets, secondary copy |
| 12 | 700, `0.07em`, uppercase | Block headings (`Converging on you`) |
| 11–11.5 | 400–500 | All monospace metadata |

Do not introduce a size between these. The previous stylesheet had ten sizes
below `1rem` and the fuzz was the main reason it read as undesigned.

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
│ nav only     │ toolbar (its own, not      │ its own header + ×   │
│ same ground  │ spanning the other panels) │ --panel ground       │
│ as main      │ ────────────────────────── │                      │
│ + hairline   │ column, max-width 680px    │ scrolls itself       │
└──────────────┴────────────────────────────┴──────────────────────┘
```

The inspector is **wider than the sidebar**, because the sidebar is navigation
and the inspector is where the reading happens.

`grid-template-rows: minmax(0, 1fr)` plus `min-height: 0` on each panel is load
bearing: without it the grid row sizes to its tallest content and the panels
overflow the viewport instead of scrolling internally.

The project header carries identity and freshness only: name, repository,
`rev N`, and `synced <elapsed> ago`. Nothing else earns a place there.

### The main column, in fixed order

1. **Converging on you** — findings touching your sessions. Never reorders below
   the others, and shows an explicit empty state rather than disappearing.
   Directly beneath the heading sits the fidelity caveat (`.fidelity-note`),
   rendered **only** when semantic processing is degraded or disabled. Healthy
   processing is the expected state and says nothing. The caveat belongs here,
   not in the project header, because it changes how the findings below it
   should be read — it is not a property of the project.
2. **Your sessions** — full detail, live.
3. **Nearby** — teammates, one line each.
4. **Elsewhere in the Project** — open findings that do not touch your work,
   rendered `.quiet` (neutral icon, smaller heading, no alert colour). Present so
   nothing is hidden; styled so it never competes.
5. **Recent** — the activity ledger.

### Your sessions vs. Nearby: different facts for different subjects

For **your own** sessions show **activity** — what the agent is doing right now,
the file it is touching, its subagents, its elapsed clock.

For **teammates** show **intent** — what they are about to do. That is the fact
you want when checking on someone before you start work, and it comes from
`Workstream.outcome`. Their activity is one click away in the inspector.

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

No tint and no panel. An alert glyph, the finding sentence in `--alert`, the
plain-language reason, then **both sides of the collision** as compact
`.mini` rows — vendor mark, member, session title, what it is doing right now,
elapsed clock, chevron. Each opens that session in the inspector, so you can
move between their side and yours.

Evidence is a two-column mono grid with the kind and source spelled out
(`same file · git`, `same contract · mcp`) rather than bare tags. A `git` source
is the deterministic one and is the only evidence row that takes `--alert`.

The primary action names **the other people**, never you (`otherNames`).

The block is a `<section>`, not a button, because it contains buttons; the
heading carries the click target and the accessible name.

### Inspector

Session sections, in order: **Intent** (with reported status), **Activity**
(phases; the newest is `.now` and shimmers when live, older are `.done`),
**Large change**, **Files this session** (the touched file is `.hot`),
**Subagents**, **How we know**, **Session** (the classifier-passing transcript).

"How we know" renders for every workstream, not only agent-backed ones — a
Git-observed teammate has provenance too. It shows `fidelity`, `branch`,
`paths`, `briefs`, and `attention`, drawn from `HarnessCapabilities`. That last
row is the honest answer to "why didn't it interrupt my agent?" and must not be
dropped.

A finding and its sync card are **one object at two ages**. The conversation and
the decision live in the collision inspector under "Work it out"; there is no
separate resolve tab.

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

1. **Self-host the fonts.** Figtree and Geist Mono as woff2 in
   `apps/dashboard/public/fonts` with `@font-face`, so the design renders as
   specified without a CDN request. Until then the system sans is used.
2. **Send timestamps, not prose.** `Workstream.updatedLabel` and
   `ActivityItem.at` are strings like `"8 min"`. An ISO timestamp alongside them
   would let `formatElapsed` count from ground truth instead of a parsed label.
3. **Decisions view is thin.** It lists resolutions and resolved sync cards; it
   has no search, filter, or per-session view yet.
4. **Narrow viewports.** The shell has `min-width: 1240px` and scrolls
   horizontally below that. A collapsed-inspector breakpoint exists in CSS
   (`.no-inspector`) but is not yet wired to a control.
5. **Creating a Project from the dashboard.** There is no service endpoint for
   it — creation is `stickguy create` in the CLI or the desktop app. The sidebar
   "New project" row therefore opens the command palette rather than pretending
   to create one. Wire it to a real flow, or drop the row, once an endpoint
   exists. Never leave a control that looks actionable and is not: the toolbar
   overflow button was removed for exactly this reason.

## 9. Checklist before adding UI

- [ ] No new filled card, tinted panel, or coloured badge.
- [ ] No new colour; status still means `--alert` and only `--alert`.
- [ ] No pulsing dot, spinner, or indicator light for "running".
- [ ] Every duration goes through `formatElapsed`.
- [ ] Machine facts are monospace; human statements are sans.
- [ ] New text sizes come from the table in §4.
- [ ] Interactive rows are buttons with unique, action-shaped `aria-label`s.
- [ ] Motion is behind `prefers-reduced-motion` and is the only thing moving.
- [ ] Provenance and confidence are still visible; nothing reads as more certain
      than the evidence supports.
