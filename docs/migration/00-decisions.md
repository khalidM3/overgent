# Phase 0 — Decisions the owner must accept before any lane starts

Status: draft ADR text for owner review
Last updated: 2026-09-04

The four ADRs below are written in the register of `docs/decisions.md`. The
owner edits or rejects them here. Once accepted, the executing agent appends
them verbatim to `docs/decisions.md` after ADR-070, updates the
"Supersedes" cross-references named in each, and runs the doc sweep listed at
the bottom. Nothing in Phases 1–3 may contradict an accepted ADR here.

Read first: `docs/decisions.md` ADR-031, ADR-035, ADR-040, ADR-041, ADR-045,
ADR-047, ADR-054, ADR-066, ADR-067, ADR-069, ADR-070;
`docs/open-source-strategy.md`; `docs/public-repository-boundary.md`.

---

## ADR-071: Overgent is an open-source application with a hosted default

Supersedes ADR-067. The source repository, including the Convex backend,
dashboard, desktop app, installers, and release workflows, becomes public under
Apache-2.0 as `docs/open-source-strategy.md` recommended. The owner does not
form a company around Overgent now. The hosted deployment at
`api.overgent.com` is run from this public code as the default team backend so
that the first five minutes never require deploying anything; self-hosting is a
documented, supported escape hatch, not the onboarding path.

What stays out of the repository is unchanged from
`docs/public-repository-boundary.md`: production secrets, account identifiers,
private runbooks, incident records, customer data, and abuse-detection
specifics. Future commercial team features, if any, are added on the hosted
side without closing the source (the open-core boundary in
`docs/open-source-strategy.md` §9).

Rationale: the category is contested by funded, closed competitors and by the
agent vendors themselves; a solo, part-time startup racing them is the weakest
version of this project. A well-distributed open-source tool with a
zero-friction hosted default is the version that fits the owner's constraints
and forecloses nothing. Release trust (signed, notarized artifacts discovered
only through `releases.overgent.com`) is unchanged from ADR-066/067.

Consequences: Lane 02 purges the tracked `stickguy` binary and rewrites
history before the repository is public; the Go module path moves to the
public organization; `SECURITY.md` gains a real channel (GitHub private
vulnerability reporting) before visibility flips.

## ADR-072: Local-first by default; the hosted backend is opt-in per Project

Extends ADR-031 and ADR-054; narrows ADR-041. A fresh install creates a
Project against a **local backend**: the open-source `convex-local-backend`
binary, bundled with the desktop release, bound to loopback only, storing its
SQLite database under the profile root. The Go service supervises that process
(start, health, restart with backoff, stop on idle) and the desktop shows it as
ordinary service health. No account, invite, or network access is required, and
nothing about a local Project leaves the device.

The Go core, dashboard, and desktop keep speaking only the `/v1` contract; the
local backend is simply another origin that `internal/hosted.New` already
accepts (loopback HTTP). The Convex functions in `convex/functions/` are the
single coordination engine for every mode; there is no second engine.

A member who wants remote teammates creates or joins a **team Project** on
Overgent Cloud or a self-hosted backend. Mode is a property of the Project, not
of the install (ADR-074). "Sign up" means creating a team Project and receiving
an invite link under the existing ADR-035/ADR-069 device-and-invite identity
model; there are no email accounts, passwords, or SSO in this decision.

Rationale: coordination metadata is tiny and Convex free tiers are generous,
so the hosted default costs almost nothing, but people who only run parallel
sessions on one Mac should not have their coordination facts stored remotely by
default. The development profile has already run this exact loopback shape
since ADR-031, so the risk is packaging, not architecture.

Consequences: the desktop bundle grows by the backend binary (about 160 MB
today; Lane 01 measures the real number and idle memory). The bundled
`convex-backend` is licensed under FSL-1.1-Apache-2.0 and is redistributed,
not modified; `NOTICE` gains a third-party entry. Source builds fetch the
pinned backend release by checksum.

## ADR-073: Semantic providers are configured per Project with the Project's own keys

Supersedes the "hosted deployment secret only" clauses of ADR-040 and ADR-045;
keeps their degradation rules. Each Project carries an AI settings record:
judgment provider (`anthropic`, `openai-compatible`, or `none`), model, optional
base URL, and key; embedding provider (`openai`, or the deterministic fallback),
model, dimensions, optional base URL, and key. The backend resolves the provider
for a Project in this order: the Project's own settings; the deployment's
operator keys, only when the deployment explicitly enables them; otherwise
`none`, which degrades to deterministic evidence with visible fidelity exactly
as today.

Keys are written through a dedicated authenticated `/v1` operation by a Project
owner, encrypted at rest with a deployment secret, never returned by any read
(only a configured flag and a four-character hint), never logged, and never
part of any synced coordination object. This is a deliberate, member-initiated
exception to "credentials never cross the wire": the member sends their own key
to the backend they chose, over TLS or loopback, to be used for that Project's
judgment calls. The secret classifier remains a gate on every other payload.

Overgent Cloud ships with operator keys disabled. The owner may enable them
later behind the ADR-070 abuse ceilings; that is an operational choice, not a
product default.

Rationale: the variable cost of the hosted service is LLM and embedding calls,
not Convex. Bring-your-own-key by default makes hosting a few hundred users
close to free and gives members the tweakability of choosing model and
provider, including OpenAI-compatible local servers.

Consequences: Lane 04 owns the protocol change; `packages/coordination`
providers take model and base URL as parameters instead of constants;
`intelligence.ts` stops reading `process.env` directly for provider keys.

## ADR-074: A profile binds each Project to its own backend

Supersedes the profile-level `apiBaseUrl` in `internal/config.Config` version 1
and ADR-041's "separate profile per backend" workaround. Configuration version
2 records, per Project, the backend origin and the device identity used with
it; workspaces resolve their backend through their Project. One Go service
holds one hosted client per distinct backend, one credential per
(backend, device), one event queue partition per workspace as today, and one
heartbeat loop per backend. Migration from version 1 assigns every existing
Project the old global origin. Hooks and MCP bindings still name the profile
(ADR-043) and need no change.

Rationale: without this, "local by default, team opt-in" is really "switch the
whole app between two profiles", which contradicts ADR-072's per-Project
framing and makes joining a friend's team Project disruptive.

Consequences: Lane 06; largest refactor of the migration; may ship after the
first public release if the README states the profile-switch limitation.

---

## Doc sweep after acceptance (executor: Haiku 4.5)

Update these files so no sentence contradicts ADR-071…074. Keep edits minimal
and factual; do not restructure documents.

- `AGENTS.md`: the rule "Go calls hosted backend only through versioned
  Overgent HTTP contracts" stays; add "the backend may be the bundled loopback
  backend". Replace "a hosted LLM ... called only from the hosted service" in
  the mission paragraph with a pointer to ADR-073.
- `README.md`: rewritten by Lane 07, not here. Only remove the sentence that
  says publication requires owner gates if ADR-071 is accepted.
- `docs/README.md`: the pointer to `migration/README.md` already exists;
  add a `self-hosting.md` placeholder line for Lane 05.
- `docs/architecture.md` §1 and §10: note the backend origin may be loopback;
  §6 "The initial semantic index is hosted" becomes "runs in the Project's
  backend".
- `docs/security-privacy.md` "Hosted" section: add the ADR-073 key-storage
  control (encrypted at rest, never returned, owner-only write).
- `docs/openai-embeddings.md`: rewrite the first paragraph; keys are Project
  settings, not deployment secrets, with deployment env as optional operator
  fallback.
- `docs/open-source-strategy.md`: status line becomes "adopted by ADR-071".
- `docs/public-repository-boundary.md`: delete the ADR-067 paragraph; add one
  sentence citing ADR-071.
- `docs/beta-release.md`: replace "invited beta" and "owner gates" wording with
  "public release"; keep every technical gate (signing, notarization,
  clean-machine evidence) as is.
- `docs/development.md` "Shared two-Mac dogfood": note ADR-074 will make the
  separate shared profile unnecessary; do not delete the section until Lane 06
  lands.

Verification: `git grep -n "ADR-067"` shows only `decisions.md` (the original
entry and the ADR-071 "Supersedes" line) and this file.
