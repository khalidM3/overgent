import type { DashboardSession, ProjectSnapshot, ScopeSnapshot, ScopeSnapshotFact, ScopeSnapshotField, ShellState } from "./model";

const declared = (text: string, facts: ScopeSnapshotFact[]): ScopeSnapshotField => ({ text, provenance: "declared", evidenceQuality: "high", facts });
const observed = (text: string, evidenceQuality: "high" | "medium", facts: ScopeSnapshotFact[]): ScopeSnapshotField => ({ text, provenance: "observed", evidenceQuality, facts });
const fallback = (text: string): ScopeSnapshotField => ({ text, provenance: "fallback", evidenceQuality: "low", facts: ["session.derivedTitle"] });
const unavailable = (text: string): ScopeSnapshotField => ({ text, provenance: "unavailable", evidenceQuality: "none", facts: [] });
function scopeSnapshot(revision: number, state: ScopeSnapshot["state"], fields: Partial<Omit<ScopeSnapshot, "revision" | "state">>): ScopeSnapshot {
  return {
    revision, state,
    goal: fields.goal ?? unavailable("No goal reported."),
    now: fields.now ?? unavailable("No current action reported."),
    done: fields.done ?? unavailable("No completed work reported."),
    waitingOn: fields.waitingOn ?? unavailable("Nothing reported."),
    verification: fields.verification ?? unavailable("No verification reported."),
    scope: fields.scope ?? unavailable("No scope reported."),
    priorGoals: fields.priorGoals ?? [],
    priorGoalsDropped: fields.priorGoalsDropped ?? 0,
  };
}

const atlas = {
  id: "prj_atlas",
  name: "Atlas launch",
  repositoryLabel: "stickguy/atlas",
  semanticStatus: "degraded",
  semanticMode: "managed_degraded",
} as const;

const orchard = {
  id: "prj_orchard",
  name: "Orchard mobile",
  repositoryLabel: "stickguy/orchard",
  semanticStatus: "disabled",
  semanticMode: "offline_fallback",
} as const;

export const fixtureSession: DashboardSession = {
  memberId: "mem_fixture_khalid",
  memberName: "Khalid",
  memberNameSource: "member",
  projects: [atlas, orchard],
  selectedProjectId: atlas.id,
};

export const fixtureSnapshots: Record<string, ProjectSnapshot> = {
  [atlas.id]: {
    project: atlas,
    contextRevision: 184,
    synchronizedAt: "12 seconds ago",
    workspacePaused: false,
    collaboration: {
      projectId: atlas.id,
      syncCards: [{ id: "syn_session", findingId: "fnd_atlas_session", title: "Choose the session rotation owner", summary: "Codex and Claude are changing the same boundary.", state: "open", revision: 1, comments: [], updatedAt: "2026-08-24T18:31:00Z" }],
      resolutions: [], cursor: "time:1787596260000",
    },
    workstreams: [
      {
        id: "wrk_agent_fixture_codex",
        memberName: "Khalid",
        initials: "KM",
        title: "Codex · codex-a1b2c3",
        outcome: "editing apps/dashboard/src/session.ts",
        presence: "online",
        fidelity: "hook",
        updatedLabel: "Now",
        pathCount: 1,
        paths: ["apps/dashboard/src/session.ts"],
        scopeSnapshot: scopeSnapshot(8, "implementing", {
          goal: fallback("Rotate the browser session boundary"),
          priorGoals: [
            { title: "Read how browser sessions are currently validated", endedAt: "2026-08-25T09:41:00Z" },
            { title: "Add a rotation helper to the session store", intendedOutcome: "Add a rotation helper to the session store.", endedAt: "2026-08-25T09:52:00Z" },
          ],
          now: observed("Edited apps/dashboard/src/session.ts · 1 parallel agent active", "medium", ["activity.currentAction", "activity.subagents"]),
          done: observed("Writes observed in apps/dashboard/src/session.ts.", "medium", ["activity.writes"]),
          scope: observed("Paths: apps/dashboard/src/session.ts.", "medium", ["activity.writes"]),
        }),
        agent: {
          vendor: "codex", sessionAlias: "codex-a1b2c3", sessionTitle: "Rotate the browser session boundary", branch: "feature/session-rotation", status: "active", tool: "apply_patch", startedAt: "2026-08-25T09:58:00Z", capabilities: { observeSession: true, observeToolActivity: true, observeSafePaths: true, readExistingSession: true, pollUpdates: true, deliverBrief: "mcp_pull", requestAttention: "unavailable", observeReadSet: "none" },
          subagents: [{ alias: "sub-a1b2c3", agentType: "reviewer", status: "active" }],
          activity: [
            { id: "codex-act-3", at: "Now", occurredAt: "2026-08-25T09:59:11Z", kind: "PostToolUse", status: "active", action: "Edited apps/dashboard/src/session.ts", tool: "apply_patch", paths: ["apps/dashboard/src/session.ts"] },
            { id: "codex-act-2", at: "1 min", occurredAt: "2026-08-25T09:59:09Z", kind: "PreToolUse", status: "active", action: "Started a repository edit", tool: "apply_patch", paths: ["apps/dashboard/src/session.ts"] },
            { id: "codex-act-1", at: "4 min", occurredAt: "2026-08-25T09:58:00Z", kind: "SessionStart", status: "active", action: "Session started", paths: [] },
          ],
          coordination: [{ id: "brief-fixture-1", routedAt: "2026-08-25T09:59:20Z", acknowledgedAt: "2026-08-25T09:59:31Z", summary: "Mina is reviewing the same session boundary in apps/dashboard/src/session.ts.", itemCount: 1, trigger: "user_prompt_submit" }],
        },
      },
      {
        id: "wst_atlas_session",
        memberName: "Mina",
        initials: "MN",
        title: "Claude Code · claude-d4e5f6",
        outcome: "reviewing apps/dashboard/src/session.ts before editing",
        presence: "online",
        fidelity: "hook",
        updatedLabel: "Now",
        pathCount: 7,
        paths: ["convex/auth/session.ts", "apps/dashboard/src/session.ts"],
        scopeSnapshot: scopeSnapshot(11, "waiting", {
          goal: declared("Audit session validity checks before changing the rotation boundary.", ["intent.intendedOutcome"]),
          now: declared("Read the current boundary, then review the proposed validity checks.", ["intent.approachSummary"]),
          waitingOn: observed("Waiting for approval to continue", "high", ["activity.currentAction"]),
          scope: declared("Components: dashboard sessions. Contracts: BrowserSession rotation.", ["intent.components", "intent.contracts"]),
        }),
        agent: {
          vendor: "claude", sessionAlias: "claude-d4e5f6", sessionTitle: "Audit session validity checks", branch: "main", status: "waiting", tool: "Read", startedAt: "2026-08-25T10:02:00Z", capabilities: { observeSession: true, observeToolActivity: true, observeSafePaths: true, readExistingSession: true, pollUpdates: true, deliverBrief: "mcp_pull", requestAttention: "unavailable", observeReadSet: "observed" }, subagents: [],
          activity: [
            { id: "claude-act-2", at: "Now", occurredAt: "2026-08-25T10:05:00Z", kind: "PermissionRequest", status: "waiting", action: "Waiting for approval to continue", tool: "Read", paths: ["apps/dashboard/src/session.ts"] },
            { id: "claude-act-1", at: "3 min", occurredAt: "2026-08-25T10:02:00Z", kind: "SessionStart", status: "active", action: "Session started", paths: [] },
          ],
          coordination: [],
        },
      },
      {
        // Silent long enough to be reported as quiet, and observable enough for
        // that silence to mean something: a vendor that reports no tool activity
        // would produce the same empty stream while working perfectly.
        id: "wrk_agent_fixture_claude_quiet",
        memberName: "Khalid",
        initials: "KM",
        title: "Claude Code · claude-77aa21",
        outcome: "running the protocol conformance suite",
        presence: "online",
        fidelity: "hook",
        updatedLabel: "21 min",
        pathCount: 3,
        paths: ["protocol/schemas/manifest.json"],
        scopeSnapshot: scopeSnapshot(14, "implementing", {
          goal: declared("Regenerate protocol types without contract drift.", ["intent.intendedOutcome"]),
          now: observed("Started pnpm protocol:check", "high", ["activity.currentAction"]),
          done: observed("Writes observed in protocol/schemas/manifest.json.", "high", ["activity.writes"]),
          verification: observed("Passed: Protocol conformance — no byte drift", "high", ["checkpoint.verification"]),
          scope: declared("Components: protocol generation. Contracts: manifest schema.", ["intent.components", "intent.contracts"]),
        }),
        agent: {
          vendor: "claude", sessionAlias: "claude-77aa21", sessionTitle: "Regenerate protocol types", branch: "feature/protocol-regen", status: "active", tool: "Bash", startedAt: "2026-08-25T09:31:00Z",
          capabilities: { observeSession: true, observeToolActivity: true, observeSafePaths: true, readExistingSession: true, pollUpdates: true, deliverBrief: "native_push", requestAttention: "advisory", observeReadSet: "observed" },
          subagents: [],
          activity: [
            { id: "quiet-act-2", at: "21 min", occurredAt: "2026-08-25T09:44:00Z", kind: "PreToolUse", status: "active", action: "Started pnpm protocol:check", tool: "Bash", paths: ["protocol/schemas/manifest.json"] },
            { id: "quiet-act-1", at: "34 min", occurredAt: "2026-08-25T09:31:00Z", kind: "SessionStart", status: "active", action: "Session started", paths: [] },
          ],
          coordination: [{ id: "brief-fixture-2", routedAt: "2026-08-25T09:43:10Z", summary: "Ravi activated manifest revision 42; the generated contract you are regenerating moved.", itemCount: 2, trigger: "session_start" }],
        },
      },
      {
        // A Cursor session, which is the highest-fidelity reader Stickguy has:
        // beforeReadFile names each file before it is read, so its read set is
        // observed rather than inferred, and a correction reaches it by push in
        // the same hook response rather than waiting to be pulled over MCP. It
        // also shows the one thing Cursor gives up — readExistingSession is
        // false, because Cursor publishes no session record this device can
        // parse, so the local "read my own session" view is unavailable.
        id: "wrk_agent_fixture_cursor",
        memberName: "Ravi",
        initials: "RV",
        title: "Cursor · cursor-b7c3d1",
        outcome: "reading backend/refresh.go",
        presence: "online",
        fidelity: "hook",
        updatedLabel: "2 min",
        pathCount: 2,
        paths: ["backend/refresh.go", "frontend/session.ts"],
        scopeSnapshot: scopeSnapshot(9, "implementing", {
          goal: declared("Implement the session view against Refresh.", ["intent.intendedOutcome"]),
          now: observed("editing frontend/session.ts", "high", ["activity.currentAction"]),
          done: observed("Writes observed in backend/refresh.go, frontend/session.ts. Contract fingerprints reported for backend/refresh.go.", "high", ["activity.writes", "contract.fingerprints"]),
          verification: observed("Running: Session view integration", "high", ["checkpoint.verification"]),
          scope: declared("Components: frontend session view. Contracts: Refresh.", ["intent.components", "intent.contracts"]),
        }),
        agent: {
          vendor: "cursor", sessionAlias: "cursor-b7c3d1", sessionTitle: "Implement the session view against Refresh", branch: "feature/session-view", status: "active", tool: "read", startedAt: "2026-08-25T10:00:00Z",
          capabilities: { observeSession: true, observeToolActivity: true, observeSafePaths: true, readExistingSession: false, pollUpdates: true, deliverBrief: "native_push", requestAttention: "unavailable", observeReadSet: "observed" },
          subagents: [],
          activity: [
            { id: "cursor-act-3", at: "2 min", occurredAt: "2026-08-25T10:04:00Z", kind: "PostToolUse", status: "active", action: "editing frontend/session.ts", tool: "edit", paths: ["frontend/session.ts"] },
            { id: "cursor-act-2", at: "5 min", occurredAt: "2026-08-25T10:01:00Z", kind: "PreToolUse", status: "active", action: "inspecting files backend/refresh.go", tool: "read", paths: ["backend/refresh.go"] },
            { id: "cursor-act-1", at: "6 min", occurredAt: "2026-08-25T10:00:00Z", kind: "SessionStart", status: "active", action: "Session started", paths: [] },
          ],
          coordination: [{ id: "brief-fixture-3", routedAt: "2026-08-25T10:04:40Z", acknowledgedAt: "2026-08-25T10:04:52Z", summary: "Refresh(userID string) became Refresh(sessionID string, policy Policy) after you read backend/refresh.go.", itemCount: 1, trigger: "user_prompt_submit" }],
        },
      },
      {
        id: "wst_atlas_manifest",
        memberName: "Ravi",
        initials: "RV",
        title: "Normalize manifest chunks",
        outcome: "Make large manifest activation atomic across retries.",
        presence: "idle",
        fidelity: "git",
        updatedLabel: "8 min",
        pathCount: 1000,
        paths: ["internal/manifest/chunks.go", "protocol/schemas/manifest.json"],
        scopeSnapshot: scopeSnapshot(42, "implementing", {
          goal: declared("Make large manifest activation atomic across retries.", ["intent.intendedOutcome"]),
          now: declared("Regenerate chunks and compare the activated revision.", ["intent.approachSummary"]),
          done: observed("1,000 reported paths changed.", "high", ["activity.writes"]),
          verification: observed("Passed: Manifest activation — atomic at revision 42", "high", ["checkpoint.verification"]),
          scope: declared("Components: manifest activation. Contracts: manifest schema v1.", ["intent.components", "intent.contracts"]),
        }),
        largeChange: {
          pathCount: 1000,
          summary: "Generated fixture paths; activation remains atomic at revision 42.",
          revision: 42,
        },
      },
      {
        id: "wst_atlas_docs",
        memberName: "June",
        initials: "JL",
        title: "Clarify onboarding copy",
        outcome: "Explain disclosed metadata without changing collection behavior.",
        presence: "offline",
        fidelity: "manual",
        updatedLabel: "31 min",
        pathCount: 2,
        paths: ["docs/privacy.md", "apps/dashboard/src/onboarding.tsx"],
        scopeSnapshot: scopeSnapshot(2, "implementing", {
          goal: declared("Explain disclosed metadata without changing collection behavior.", ["intent.intendedOutcome"]),
          now: declared("Clarify the onboarding copy and privacy explanation.", ["intent.approachSummary"]),
          done: observed("Writes observed in apps/dashboard/src/onboarding.tsx, docs/privacy.md.", "high", ["activity.writes"]),
          scope: declared("Components: onboarding, privacy documentation.", ["intent.components"]),
        }),
      },
    ],
    findings: [
      {
        id: "fnd_atlas_session",
        kind: "direct_collision",
        severity: "high",
        confidence: "deterministic",
        state: "open",
        title: "Two of Khalid's sessions are both changing apps/dashboard/src/session.ts",
        reason: "Two live agent sessions report the same dashboard session path.",
        workstreamIds: ["wrk_agent_fixture_codex", "wst_atlas_session"],
        evidence: [
          { kind: "path", label: "apps/dashboard/src/session.ts", source: "git" },
          { kind: "contract", label: "BrowserSession rotation", source: "mcp" },
        ],
        firstSeen: "12 min ago",
        lastSeen: "Now",
      },
      {
        id: "fnd_atlas_dependency",
        kind: "shared_dependency",
        severity: "medium",
        confidence: "high",
        state: "acknowledged",
        title: "Khalid and another session both depend on protocol manifest schema v1",
        reason: "Two reported changes reference the same generated manifest contract.",
        workstreamIds: ["wst_atlas_manifest"],
        evidence: [
          { kind: "dependency", label: "protocol manifest schema v1", source: "mcp" },
          { kind: "intent", label: "Atomic activation", source: "semantic_candidate" },
        ],
        firstSeen: "44 min ago",
        lastSeen: "8 min ago",
      },
    ],
    activity: [
      { id: "act_1", at: "Now", actor: "Mina", kind: "checkpoint", summary: "Reported session rotation boundary and verification passed.", fidelity: "mcp" },
      { id: "act_2", at: "2 min", actor: "Stickguy", kind: "finding", summary: "Updated direct-collision evidence for the session contract.", fidelity: "structural" },
      { id: "act_3", at: "8 min", actor: "Ravi", kind: "manifest", summary: "Activated manifest revision 42 with 1,000 paths.", fidelity: "git" },
      { id: "act_4", at: "31 min", actor: "June", kind: "intent", summary: "Set a manual onboarding-copy intent.", fidelity: "manual" },
    ],
    devices: [
      { id: "dev_atlas_mac", label: "Khalid’s MacBook", platform: "macOS arm64", status: "online", lastSeen: "12 seconds ago" },
      { id: "dev_atlas_linux", label: "Ravi’s runner", platform: "Linux amd64", status: "idle", lastSeen: "8 minutes ago" },
    ],
  },
  [orchard.id]: {
    project: orchard,
    contextRevision: 29,
    synchronizedAt: "1 minute ago",
    workspacePaused: true,
    collaboration: { projectId: orchard.id, syncCards: [], resolutions: [], cursor: "time:0" },
    workstreams: [
      {
        id: "wst_orchard_nav",
        memberName: "Ari",
        initials: "AR",
        title: "Simplify checkout navigation",
        outcome: "Reduce checkout to two explicit steps.",
        presence: "paused",
        fidelity: "hook_unverified",
        updatedLabel: "1 hr",
        pathCount: 4,
        paths: ["app/checkout/navigation.ts", "app/checkout/review.tsx"],
        scopeSnapshot: scopeSnapshot(4, "waiting", {
          goal: declared("Reduce checkout to two explicit steps.", ["intent.intendedOutcome"]),
          now: observed("Workspace sharing is paused.", "high", ["activity.currentAction"]),
          done: observed("Writes observed in app/checkout/navigation.ts, app/checkout/review.tsx.", "high", ["activity.writes"]),
          scope: declared("Components: checkout navigation.", ["intent.components"]),
        }),
      },
    ],
    findings: [],
    activity: [
      { id: "act_orchard_1", at: "1 hr", actor: "Ari", kind: "pause", summary: "Paused workspace metadata transmission.", fidelity: "manual" },
    ],
    devices: [
      { id: "dev_orchard_mac", label: "Ari’s MacBook", platform: "macOS arm64", status: "paused", lastSeen: "1 hour ago" },
    ],
  },
};

export function parseShellState(search: string): ShellState {
  const requested = new URLSearchParams(search).get("state");
  const supported: ShellState[] = ["activation", "loading", "ready", "empty", "offline", "unauthorized", "version_mismatch"];
  return supported.includes(requested as ShellState) ? (requested as ShellState) : "ready";
}

export function snapshotForProject(projectId: string): ProjectSnapshot {
  const snapshot = fixtureSnapshots[projectId];
  if (!snapshot) throw new Error("Project is not authorized for this fixture session.");
  return structuredClone(snapshot);
}

export function emptyFixtureSession(): DashboardSession {
  return { memberId: fixtureSession.memberId, memberName: fixtureSession.memberName, memberNameSource: fixtureSession.memberNameSource, projects: [], selectedProjectId: "" };
}
