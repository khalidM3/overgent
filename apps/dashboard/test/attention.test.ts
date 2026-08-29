import { describe, expect, it } from "vitest";
import { STALL_SECONDS, attentionItems, newestEventTime, orderSessions, sessionHealth } from "../src/attention";
import { snapshotForProject } from "../src/fixtures";
import type { ProjectSnapshot, Workstream } from "../src/model";

const atlas = (): ProjectSnapshot => snapshotForProject("prj_atlas");
const find = (snapshot: ProjectSnapshot, id: string): Workstream => {
  const stream = snapshot.workstreams.find((entry) => entry.id === id);
  if (!stream) throw new Error(`fixture workstream ${id} is missing`);
  return stream;
};

describe("agent health", () => {
  it("reports a session that has gone quiet, as measured silence rather than a diagnosis", () => {
    const snapshot = atlas();
    const now = newestEventTime(snapshot);
    if (now === null) throw new Error("fixture has no timestamped events");
    const signal = sessionHealth(find(snapshot, "wrk_agent_fixture_claude_quiet"), now);
    expect(signal?.kind).toBe("stalled");
    expect(signal?.silentSeconds).toBeGreaterThanOrEqual(STALL_SECONDS);
    expect(signal?.statement).toContain("reported nothing");
  });

  it("never calls a session quiet when the vendor does not report tool activity", () => {
    const snapshot = atlas();
    const now = newestEventTime(snapshot) ?? 0;
    const session = find(snapshot, "wrk_agent_fixture_claude_quiet");
    session.agent!.capabilities.observeToolActivity = false;
    // Absence of observation is not absence of progress. The same empty stream
    // is produced by an agent working perfectly and by one that is wedged.
    expect(sessionHealth(session, now)).toBeNull();
  });

  it("says nothing about a session that has only just gone quiet", () => {
    const snapshot = atlas();
    const session = find(snapshot, "wrk_agent_fixture_claude_quiet");
    const last = Date.parse(session.agent!.activity![0]!.occurredAt!);
    expect(sessionHealth(session, last + (STALL_SECONDS - 1) * 1_000)).toBeNull();
  });

  it("reports a vendor-declared block without waiting for any silence", () => {
    const snapshot = atlas();
    const session = find(snapshot, "wrk_agent_fixture_codex");
    session.agent!.status = "waiting";
    const signal = sessionHealth(session, Date.parse(session.agent!.activity![0]!.occurredAt!));
    expect(signal?.kind).toBe("waiting");
  });

  it("stays quiet about paused and finished sessions", () => {
    const snapshot = atlas();
    const session = find(snapshot, "wrk_agent_fixture_claude_quiet");
    const now = newestEventTime(snapshot) ?? 0;
    session.presence = "paused";
    expect(sessionHealth(session, now)).toBeNull();
    session.presence = "online";
    session.agent!.status = "done";
    expect(sessionHealth(session, now)).toBeNull();
  });
});

describe("what needs the member", () => {
  it("carries findings and health in one list, findings first", () => {
    const snapshot = atlas();
    const now = newestEventTime(snapshot) ?? 0;
    const mine = new Set(snapshot.workstreams.filter((stream) => stream.memberName === "Khalid").map((stream) => stream.id));
    const items = attentionItems(snapshot, mine, now);
    expect(items.map((item) => item.kind)).toEqual(["finding", "health"]);
    // Correctness before convenience: a collision can invalidate work already
    // done, while a stopped session has only failed to spend more time.
    expect(items[0]!.kind === "finding" && items[0]!.finding.id).toBe("fnd_atlas_session");
  });

  it("leaves a teammate's stopped session out of the member's block", () => {
    const snapshot = atlas();
    const now = newestEventTime(snapshot) ?? 0;
    const mine = new Set(snapshot.workstreams.filter((stream) => stream.memberName === "Khalid").map((stream) => stream.id));
    // Mina's session is waiting, and that is Mina's problem to act on.
    expect(find(snapshot, "wst_atlas_session").agent?.status).toBe("waiting");
    expect(attentionItems(snapshot, mine, now).some((item) => item.kind === "health" && item.session.memberName === "Mina")).toBe(false);
  });
});

describe("the order sessions are read in", () => {
  const recency = (session: Workstream): number => ({ "22 min": 1320, "2 min": 120, Now: 0 } as Record<string, number>)[session.updatedLabel] ?? 600;

  it("ranks what needs you above what is merely running, and folds finished work away", () => {
    const snapshot = atlas();
    const now = newestEventTime(snapshot) ?? 0;
    const mine = snapshot.workstreams.filter((stream) => stream.memberName === "Khalid");
    const quiet = find(snapshot, "wrk_agent_fixture_claude_quiet");
    quiet.updatedLabel = "22 min";

    const ordered = orderSessions(mine, now, recency);
    // The quiet session is older than the active one and still comes first:
    // rank is what you would do about it, and only then how recent it is.
    expect(ordered.live[0]!.id).toBe("wrk_agent_fixture_claude_quiet");
    expect(ordered.finished).toHaveLength(0);
  });

  it("moves a finished session out of the list rather than leaving it in the way", () => {
    const snapshot = atlas();
    const now = newestEventTime(snapshot) ?? 0;
    const codex = find(snapshot, "wrk_agent_fixture_codex");
    codex.agent!.status = "done";
    const ordered = orderSessions(snapshot.workstreams.filter((stream) => stream.memberName === "Khalid"), now, recency);
    expect(ordered.finished.map((session) => session.id)).toEqual(["wrk_agent_fixture_codex"]);
    expect(ordered.live.some((session) => session.id === "wrk_agent_fixture_codex")).toBe(false);
  });

  it("puts the most recent first inside one band", () => {
    const snapshot = atlas();
    const now = newestEventTime(snapshot) ?? 0;
    const quiet = find(snapshot, "wrk_agent_fixture_claude_quiet");
    // Take the health signal away so both sessions sit in the running band.
    quiet.agent!.capabilities.observeToolActivity = false;
    quiet.updatedLabel = "22 min";
    find(snapshot, "wrk_agent_fixture_codex").updatedLabel = "Now";
    const ordered = orderSessions(snapshot.workstreams.filter((stream) => stream.memberName === "Khalid"), now, recency);
    expect(ordered.live[0]!.id).toBe("wrk_agent_fixture_codex");
  });
});
