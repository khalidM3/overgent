# Phase 4 — Post-launch product work (not part of the migration)

Status: notes for later briefs
Last updated: 2026-09-04

These are the two product properties the outside advice singled out as
deciding whether people keep Overgent running, plus the one business option
that stays open. None of them blocks launch. Each becomes its own brief in
`docs/tasks/` when started; the notes here only fix the framing.

## A. False positives are the kill switch

Framing: one wrong interruption and a member turns injection off. The
precision gate in ADR-045 (M1 eval harness) already exists for enabling
proactive interruption; what is missing is runtime control by the member.

Candidate scope for a brief:
- Per-Project, per-finding-kind delivery policy: `inject`, `dashboard-only`,
  `off`, with `stale_assumption` and `collision` defaulting to inject and
  semantic kinds defaulting to dashboard-only until the Project has recorded
  N confirmations through the existing `findingFeedback` table.
- Feedback from the injected brief itself: the hook payload already carries
  item ids; a one-token acknowledgement (`overgent intent ... --dismiss <id>`
  or a follow-up hook signal) records `findingFeedback` without a dashboard
  visit.
- A visible precision number per kind on the Project view, computed from
  `findingFeedback`, in the honest-fidelity vocabulary of ADR-064.
- Eval: extend `validation/evals/coordination` with a negative-scenario set
  and make the precision number part of `pnpm eval:coordination` output.

## B. Show what it caught

Framing: "prevented 3 overlapping edits this week" is the retention hook.

Candidate scope:
- A weekly tally per Project derived from `findings` (state resolved or
  acknowledged, by kind) joined with `findingFeedback` confirmations; never
  count dismissed findings.
- Surface: one line in the desktop menu and at the top of the Project view,
  monospace numbers, sans prose, no chart.
- Local mode gets the same tally from the local backend; nothing is sent
  anywhere.

## C. The hosted team tier stays a back-pocket option

ADR-071 keeps the open-core boundary from `docs/open-source-strategy.md` §9.
If Cloud Projects and retention show a real signal, the candidates that do
not widen client collection are: longer history retention, admin and audit
views, SSO, and priority support. Anything touching what the client collects
or sends stays public and default-off. Do not start this without the numbers
from `07-launch-checklist.md` §7.
