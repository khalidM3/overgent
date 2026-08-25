import type { DashboardSession, ProjectSnapshot, ShellState } from "./model";

const atlas = {
  id: "prj_atlas",
  name: "Atlas launch",
  repositoryLabel: "stickguy/atlas",
  semanticStatus: "degraded",
} as const;

const orchard = {
  id: "prj_orchard",
  name: "Orchard mobile",
  repositoryLabel: "stickguy/orchard",
  semanticStatus: "disabled",
} as const;

export const fixtureSession: DashboardSession = {
  memberName: "Khalid",
  projects: [atlas, orchard],
  selectedProjectId: atlas.id,
};

export const fixtureSnapshots: Record<string, ProjectSnapshot> = {
  [atlas.id]: {
    project: atlas,
    contextRevision: 184,
    synchronizedAt: "12 seconds ago",
    workspacePaused: false,
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
        agent: { vendor: "codex", sessionAlias: "codex-a1b2c3", status: "active", tool: "apply_patch", subagents: [{ alias: "sub-a1b2c3", agentType: "reviewer", status: "active" }] },
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
        agent: { vendor: "claude", sessionAlias: "claude-d4e5f6", status: "waiting", tool: "Read", subagents: [] },
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
  return { memberName: fixtureSession.memberName, projects: [], selectedProjectId: "" };
}
