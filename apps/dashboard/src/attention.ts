import type { Finding, ProjectSnapshot, Workstream } from "./model";

/**
 * What the lead block answers, and how it decides.
 *
 * The workroom's first question is "is anything about to hit me". Until now the
 * only thing that could answer it was a coordination finding, so a session of
 * your own that was sitting on a permission prompt - blocked, costing time, and
 * already known to the service - rendered nowhere near the place built to be
 * looked at. Agent health belongs in the same block for the same reason
 * findings do: it is work that has converged on the member and stopped.
 *
 * Health is derived here rather than reported over the wire. `waiting` and
 * `error` are vendor-observed statuses the service already syncs; a stall is
 * arithmetic over event times the snapshot already carries. Nothing new is
 * collected and nothing new crosses the boundary.
 */

/**
 * A session is only called stalled after this long without a reported event.
 *
 * Fifteen minutes is deliberately generous. A test suite, a long build, or a
 * slow model turn can all hold a tool open for several minutes, and a stall
 * notice that fires during ordinary work would spend the one thing the
 * coordination engine is measured on. The retention sweep ends a silent session
 * at thirty minutes, so this sits well inside the window where the session is
 * still believed to be alive.
 */
export const STALL_SECONDS = 900;

export type HealthKind = "error" | "waiting" | "stalled";

export interface HealthSignal {
  kind: HealthKind;
  /** What is true, in a sentence a person wrote. */
  statement: string;
  /** The machine fact behind it, or undefined when there is none worth showing. */
  fact?: string;
  /** Seconds since this session last reported anything, when that is known. */
  silentSeconds?: number;
}

export type AttentionItem =
  | { kind: "finding"; id: string; finding: Finding }
  | { kind: "health"; id: string; session: Workstream; signal: HealthSignal };

/**
 * Whether a finding is interrupt-worthy, by the same rule the engine routes
 * briefs with. The judgment layer already decided where each finding belongs
 * (ADR-045/046); the workroom re-deriving admission from ownership alone was
 * the UI second-guessing its own engine. A record that predates a judged
 * verdict falls back to the engine's own severity rule (decideDelivery).
 */
export function interruptWorthy(finding: Finding): boolean {
  if (finding.delivery) return finding.delivery === "next_turn";
  return finding.severity === "high" || finding.severity === "critical";
}

const SEVERITY_ORDER = { critical: 0, high: 1, medium: 2, low: 3 } as const;
const HEALTH_ORDER: Record<HealthKind, number> = { error: 0, waiting: 1, stalled: 2 };

function eventTime(value: string | undefined): number | null {
  if (!value) return null;
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) ? parsed : null;
}

/** The most recent moment anything in this Project was observed, in epoch ms. */
export function newestEventTime(snapshot: ProjectSnapshot): number | null {
  let newest: number | null = null;
  for (const stream of snapshot.workstreams) {
    for (const item of stream.agent?.activity ?? []) {
      const at = eventTime(item.occurredAt);
      if (at !== null && (newest === null || at > newest)) newest = at;
    }
  }
  return newest;
}

/** Epoch ms this session last reported an event, or null if none is timestamped. */
export function lastReportedAt(session: Workstream): number | null {
  let newest: number | null = null;
  for (const item of session.agent?.activity ?? []) {
    const at = eventTime(item.occurredAt);
    if (at !== null && (newest === null || at > newest)) newest = at;
  }
  return newest;
}

/**
 * The health of one session, or null when there is nothing to say.
 *
 * Silence is only reported for a session Overgent can actually watch. A vendor
 * that does not report tool activity produces an empty event stream whether the
 * agent is working hard or has been wedged for an hour, and calling that second
 * case a stall would be inventing evidence. Absence of observation is not
 * absence of progress, exactly as an empty read set is not an all-clear.
 */
export function sessionHealth(session: Workstream, now: number): HealthSignal | null {
  const agent = session.agent;
  if (!agent) return null;
  if (session.presence === "paused") return null;
  if (agent.status === "done") return null;

  const latest = agent.activity?.[0];

  if (agent.status === "error") {
    return { kind: "error", statement: "This session reported a failure and has not recovered.", ...(latest?.action ? { fact: latest.action } : {}) };
  }

  if (agent.status === "waiting") {
    return { kind: "waiting", statement: "This session is waiting on you before it can continue.", ...(latest?.action ? { fact: latest.action } : {}) };
  }

  if (agent.status !== "active") return null;
  if (!agent.capabilities.observeToolActivity) return null;

  const last = lastReportedAt(session);
  if (last === null) return null;
  const silentSeconds = Math.floor((now - last) / 1_000);
  if (silentSeconds < STALL_SECONDS) return null;

  // The claim is the measurement, not a diagnosis. Overgent knows the session
  // has gone quiet; it does not know whether the agent is stuck, thinking, or
  // running something slow, and saying otherwise would overstate the evidence.
  return {
    kind: "stalled",
    statement: "This session is still open but has reported nothing for a while.",
    ...(latest?.action ? { fact: latest.action } : {}),
    silentSeconds,
  };
}

/**
 * Everything reaching the viewer, in the order it should be dealt with.
 *
 * Findings come before health because they threaten the correctness of work
 * already done, while a blocked session only costs time that has not been spent
 * yet. Within each group the ordering is the ordinary one: severity, then how
 * badly the session is stuck.
 */
export function attentionItems(snapshot: ProjectSnapshot, mineIds: ReadonlySet<string>, now: number): AttentionItem[] {
  const findings = snapshot.findings
    .filter((finding) => finding.state === "open" && interruptWorthy(finding) && finding.workstreamIds.some((id) => mineIds.has(id)))
    .sort((left, right) => SEVERITY_ORDER[left.severity] - SEVERITY_ORDER[right.severity])
    .map((finding): AttentionItem => ({ kind: "finding", id: finding.id, finding }));

  const health = snapshot.workstreams
    .filter((stream) => mineIds.has(stream.id))
    .flatMap((session) => {
      const signal = sessionHealth(session, now);
      return signal ? [{ kind: "health", id: `health-${session.id}`, session, signal } as AttentionItem] : [];
    })
    .sort((left, right) => {
      if (left.kind !== "health" || right.kind !== "health") return 0;
      return HEALTH_ORDER[left.signal.kind] - HEALTH_ORDER[right.signal.kind];
    });

  return [...findings, ...health];
}

/**
 * The order sessions are read in, and which of them fold away.
 *
 * A list was rendering in the order the snapshot happened to carry, so a
 * session started a minute ago sat below one from this morning and a finished
 * session sat above a live one. Teammates were already ranked by presence,
 * which made the inconsistency worse rather than hiding it: the half of the
 * screen showing other people was ordered and the half showing your own work
 * was not.
 *
 * Rank is by what the reader needs to do about it, then by recency inside each
 * band. Recency comes from the same elapsed label the row displays, so the
 * order always agrees with the clock next to it.
 */
const NEEDS_YOU = 0;
const RUNNING = 1;
const OPEN = 2;
const FINISHED = 3;

function sessionBand(session: Workstream, now: number): number {
  if (session.agent?.status === "done") return FINISHED;
  if (sessionHealth(session, now)) return NEEDS_YOU;
  if (session.agent?.status === "active") return RUNNING;
  return OPEN;
}

export interface OrderedSessions {
  /** Everything still worth a full row, most urgent and most recent first. */
  live: Workstream[];
  /** Finished sessions, newest first, folded behind a disclosure. */
  finished: Workstream[];
}

export function orderSessions(sessions: readonly Workstream[], now: number, recency: (session: Workstream) => number): OrderedSessions {
  const ranked = [...sessions].sort((left, right) => {
    const band = sessionBand(left, now) - sessionBand(right, now);
    return band !== 0 ? band : recency(left) - recency(right);
  });
  return {
    live: ranked.filter((session) => sessionBand(session, now) !== FINISHED),
    finished: ranked.filter((session) => sessionBand(session, now) === FINISHED),
  };
}
