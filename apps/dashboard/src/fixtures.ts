import type { DashboardSession, ProjectSnapshot, ShellState } from "./model";

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
      },
    ],
    findings: [
      {
        id: "fnd_atlas_session",
        kind: "direct_collision",
        severity: "high",
        confidence: "deterministic",
        state: "open",
        title: "Codex and Claude are touching the session boundary",
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
        title: "Manifest work depends on the schema generator",
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
