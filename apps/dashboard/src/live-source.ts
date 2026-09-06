import { FixtureProjectSource } from "./fixture-source";
import { isDesktopWebview, nativeOnboarding } from "./native";
import type { CollaborationSnapshot, DashboardSession, FindingFeedback, FindingState, LocalSessionDetail, MemberNameSource, ProjectAccess, ProjectMember, ProjectSnapshot, SessionFocus, SessionMessagesSnapshot } from "./model";

const prefix = import.meta.env.VITE_OVERGENT_API_PREFIX ?? "/api/v1";

const objectProjects = new Map<string, string>();
async function request<T>(path: string, init?: RequestInit, projectId?: string): Promise<T> {
  if (isDesktopWebview) {
    const parts = path.split("/");
    const scope = projectId ?? (parts[1] === "projects" ? parts[2] : parts[1] === "dashboard" && parts[2] === "projects" ? parts[3] : objectProjects.get(parts[2] ?? ""));
    if (!scope) throw new Error("This operation has no Project context.");
    const result = await nativeOnboarding.dashboardRequest(scope, init?.method ?? "GET", path, typeof init?.body === "string" ? init.body : "");
    if (result.status < 200 || result.status >= 300) throw Object.assign(new Error("Project request could not be completed."), { status: result.status });
    return (result.body ? JSON.parse(result.body) : undefined) as T;
  }
  const response = await fetch(`${prefix}${path}`, { ...init, credentials: "include", headers: { "content-type": "application/json", ...init?.headers } });
  if (!response.ok) {
    const error = new Error(`dashboard API ${response.status}`) as Error & { status: number };
    error.status = response.status;
    throw error;
  }
  // An accepted-but-empty response is normal for administration and data-rights
  // routes, which answer 202/204 with no body. Parsing unconditionally turned a
  // successful delete into "Unexpected end of JSON input".
  const body = await response.text();
  return (body ? JSON.parse(body) : undefined) as T;
}

export async function loadSession(): Promise<DashboardSession> {
  if (!isDesktopWebview) return request<DashboardSession>("/dashboard/session");
  const state = await nativeOnboarding.state();
  const projects = state.projects ?? [];
  if (!projects.length) return { projects: [], selectedProjectId: "", memberId: "", memberName: "You", memberNameSource: "device" };
  const origins = new Map(projects.map((project) => [project.backendId, project.projectId]));
  const results = await Promise.allSettled([...origins.values()].map((id) => request<DashboardSession>("/dashboard/session", undefined, id)));
  const sessions = results.flatMap((result) => result.status === "fulfilled" ? [result.value] : []);
  if (!sessions.length) throw new Error("Your Project servers are unavailable. Return to Projects to check connections.");
  const registered = new Set(projects.map((project) => project.projectId));
  const summaries = sessions.flatMap((session) => session.projects.filter((project) => registered.has(project.id)));
  const requested = new URLSearchParams(window.location.search).get("project");
  const selected = summaries.find((project) => project.id === requested) ?? summaries[0];
  const session = sessions.find((value) => value.projects.some((project) => project.id === selected?.id)) ?? sessions[0]!;
  return { ...session, projects: summaries, selectedProjectId: selected?.id ?? "" };
}

export async function loadSnapshot(projectId: string): Promise<ProjectSnapshot> {
  const [snapshot, collaboration] = await Promise.all([
    request<Omit<ProjectSnapshot, "collaboration">>(`/dashboard/projects/${encodeURIComponent(projectId)}`),
    request<CollaborationSnapshot>(`/projects/${encodeURIComponent(projectId)}/collaboration`),
  ]);
  for (const object of [...(snapshot.workstreams ?? []), ...(snapshot.findings ?? []), ...(snapshot.devices ?? []), ...(collaboration.syncCards ?? [])]) objectProjects.set(object.id, projectId);
  return { ...snapshot, collaboration };
}

export class LiveProjectSource extends FixtureProjectSource {
  override readonly live = true;
  private timers = new Map<string, number>();
  private readonly onStatus: (status: "ready" | "offline" | "unauthorized" | "version_mismatch") => void;

  constructor(initial: ProjectSnapshot[], onStatus: (status: "ready" | "offline" | "unauthorized" | "version_mismatch") => void = () => undefined) {
    super(initial);
    this.onStatus = onStatus;
  }

  start(projectId: string): () => void {
    if (!this.timers.has(projectId)) {
      const refresh = () => void loadSnapshot(projectId).then((snapshot) => { this.replace(snapshot); this.onStatus("ready"); }).catch((error: { status?: number }) => {
        this.onStatus(error.status === 401 || error.status === 403 ? "unauthorized" : error.status === 409 ? "version_mismatch" : "offline");
      });
      this.timers.set(projectId, window.setInterval(refresh, 2_000));
    }
    return () => {
      const timer = this.timers.get(projectId);
      if (timer !== undefined) window.clearInterval(timer);
      this.timers.delete(projectId);
    };
  }

  override async recordFindingFeedback(findingId: string, value: FindingFeedback): Promise<void> {
    await request<void>(`/findings/${encodeURIComponent(findingId)}/feedback`, { method: "POST", body: JSON.stringify({ value }) });
  }

  /**
   * Pause and focus are local-service state, so they are reachable only from
   * the desktop shell. A browser tab genuinely cannot set them, and probing
   * once is what lets the workroom offer a control that works or an
   * instruction that does - never a button that turns out to be inert.
   */
  override async localControl(): Promise<boolean> {
    try {
      return (await nativeOnboarding.state()).available;
    } catch {
      return false;
    }
  }

  override async setProjectPaused(projectId: string, paused: boolean): Promise<void> {
    await nativeOnboarding.setProjectPaused(projectId, paused);
    // Pause takes effect in the service before it is reported, so re-reading is
    // what makes the rendered state the service's rather than this page's.
    this.replace(await loadSnapshot(projectId));
  }

  override async getSessionFocus(workstreamId: string): Promise<SessionFocus> {
    try {
      return await nativeOnboarding.sessionFocus(workstreamId);
    } catch {
      return { sessionId: workstreamId, focused: false };
    }
  }

  override async setSessionFocus(workstreamId: string, minutes: number): Promise<SessionFocus> {
    return nativeOnboarding.setSessionFocus(workstreamId, minutes);
  }

  /**
   * Acknowledging and dismissing are the only states set from here; resolution
   * follows the recorded decision (ADR-061). The poll in start() replaces the
   * snapshot every two seconds, so a local-only patch would be wiped - re-read
   * instead, exactly as the sync card writes do.
   */
  override async setFindingState(projectId: string, findingId: string, state: FindingState): Promise<void> {
    await request<void>(`/findings/${encodeURIComponent(findingId)}/state`, { method: "POST", body: JSON.stringify({ state }) });
    this.replace(await loadSnapshot(projectId));
  }


  override async createSyncCard(projectId: string, findingId: string | undefined, title: string, summary: string): Promise<{ id: string; revision: number }> {
    const card = await request<{ id: string; revision: number }>(`/projects/${encodeURIComponent(projectId)}/sync-cards`, { method: "POST", body: JSON.stringify({ ...(findingId ? { findingId } : {}), title, summary }) });
    this.replace(await loadSnapshot(projectId));
    return { id: card.id, revision: card.revision };
  }

  override async commentSyncCard(projectId: string, cardId: string, body: string): Promise<void> {
    await request(`/sync-cards/${encodeURIComponent(cardId)}/comments`, { method: "POST", body: JSON.stringify({ body }) });
    this.replace(await loadSnapshot(projectId));
  }

  override async resolveSyncCard(projectId: string, cardId: string, expectedRevision: number, summary: string, affectedWorkstreamIds: string[]): Promise<void> {
    await request(`/sync-cards/${encodeURIComponent(cardId)}/resolve`, { method: "POST", body: JSON.stringify({ expectedRevision, summary, affectedMemberIds: [], affectedWorkstreamIds }) });
    this.replace(await loadSnapshot(projectId));
  }

  override async listMembers(projectId: string): Promise<ProjectMember[]> {
    return (await request<{ members: ProjectMember[] }>(`/projects/${encodeURIComponent(projectId)}/members`)).members;
  }

  override async getProjectAccess(projectId: string): Promise<ProjectAccess> {
    return request<ProjectAccess>(`/projects/${encodeURIComponent(projectId)}/access`);
  }

  override async createInvite(projectId: string): Promise<{ code: string }> {
    const invite = await request<{ id: string; secret: string }>(`/projects/${encodeURIComponent(projectId)}/invites`, { method: "POST", // Seven days, one use, revocable below - an invite is a link that must
    // survive until the recipient sits down, not a synchronous exchange.
    body: JSON.stringify({ expiresInSeconds: 604_800, maxUses: 1 }) });
    return { code: `${invite.id}.${invite.secret}` };
  }

  override async revokeInvite(projectId: string, inviteId: string): Promise<void> {
    await request<void>(`/projects/${encodeURIComponent(projectId)}/invites/${encodeURIComponent(inviteId)}/revoke`, { method: "POST" });
  }

  override async revokeDevice(_projectId: string, deviceId: string): Promise<void> {
    await request<void>(`/devices/${encodeURIComponent(deviceId)}/revoke`, { method: "POST" });
  }

  override async removeMember(projectId: string, memberId: string): Promise<void> {
    await request<void>(`/projects/${encodeURIComponent(projectId)}/members/${encodeURIComponent(memberId)}/remove`, { method: "POST" });
  }

  override exportURL(projectId: string): string { return `${prefix}/projects/${encodeURIComponent(projectId)}/export`; }

  override async deleteProject(projectId: string): Promise<void> {
    await request<void>(`/projects/${encodeURIComponent(projectId)}`, { method: "DELETE" });
  }

  override async deleteOwnProjectData(projectId: string): Promise<void> {
    await request<void>(`/projects/${encodeURIComponent(projectId)}/member`, { method: "DELETE" });
  }

  override async updateDisplayName(projectId: string, displayName: string): Promise<{ memberName: string; memberNameSource: MemberNameSource }> {
    const result = await request<{ memberId: string; memberName: string; memberNameSource: MemberNameSource }>(`/projects/${encodeURIComponent(projectId)}/member`, {
      method: "PATCH", body: JSON.stringify({ displayName }),
    });
    // A renamed member changes every rendered attribution, so re-read rather than patching locally.
    this.replace(await loadSnapshot(projectId));
    return { memberName: result.memberName, memberNameSource: result.memberNameSource };
  }

  /**
   * Own-session content comes from the local service through the native bridge,
   * never from the hosted API, so it is visible without any sharing (ADR-036).
   */
  override async getLocalSession(workstreamId: string): Promise<LocalSessionDetail> {
    try {
      const detail = await nativeOnboarding.sessionDetail(workstreamId);
      return {
        available: detail.available,
        ...(detail.title ? { title: detail.title } : {}),
        ...(detail.branch ? { branch: detail.branch } : {}),
        messages: (detail.messages ?? []).filter((message) => ["user", "assistant", "thinking", "tool"].includes(message.kind)) as LocalSessionDetail["messages"],
      };
    } catch {
      // Outside the desktop shell there is no local service to ask.
      return { available: false, messages: [] };
    }
  }

  override async getSessionMessages(workstreamId: string): Promise<SessionMessagesSnapshot> {
    return request<SessionMessagesSnapshot>(`/workstreams/${encodeURIComponent(workstreamId)}/session-sharing`);
  }
}
