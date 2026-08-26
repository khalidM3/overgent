import { FixtureProjectSource } from "./fixture-source";
import { nativeOnboarding } from "./native";
import type { CollaborationSnapshot, DashboardSession, FindingFeedback, LocalSessionDetail, MemberNameSource, ProjectMember, ProjectSnapshot, SessionMessageKind, SessionSharingSnapshot } from "./model";

const prefix = import.meta.env.VITE_STICKGUY_API_PREFIX ?? "/api/v1";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${prefix}${path}`, { ...init, credentials: "include", headers: { "content-type": "application/json", ...init?.headers } });
  if (!response.ok) {
    const error = new Error(`dashboard API ${response.status}`) as Error & { status: number };
    error.status = response.status;
    throw error;
  }
  return response.status === 204 ? undefined as T : await response.json() as T;
}

export async function loadSession(): Promise<DashboardSession> {
  return request<DashboardSession>("/dashboard/session");
}

export async function loadSnapshot(projectId: string): Promise<ProjectSnapshot> {
  const [snapshot, collaboration] = await Promise.all([
    request<Omit<ProjectSnapshot, "collaboration">>(`/dashboard/projects/${encodeURIComponent(projectId)}`),
    request<CollaborationSnapshot>(`/projects/${encodeURIComponent(projectId)}/collaboration`),
  ]);
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





  override async createSyncCard(projectId: string, findingId: string | undefined, title: string, summary: string): Promise<void> {
    await request(`/projects/${encodeURIComponent(projectId)}/sync-cards`, { method: "POST", body: JSON.stringify({ ...(findingId ? { findingId } : {}), title, summary }) });
    this.replace(await loadSnapshot(projectId));
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

  override async getSessionSharing(workstreamId: string): Promise<SessionSharingSnapshot> {
    return request<SessionSharingSnapshot>(`/workstreams/${encodeURIComponent(workstreamId)}/session-sharing`);
  }

  override async updateSessionSharing(workstreamId: string, audience: "self" | "project", allowedKinds: SessionMessageKind[]): Promise<SessionSharingSnapshot> {
    return request<SessionSharingSnapshot>(`/workstreams/${encodeURIComponent(workstreamId)}/session-sharing`, {
      method: "PUT",
      body: JSON.stringify({ profile: "conversation", audience, consentVersion: "session-share/v1", allowedKinds, expiresInSeconds: 604800 }),
    });
  }

  override async deleteSessionSharing(workstreamId: string): Promise<SessionSharingSnapshot> {
    await request<void>(`/workstreams/${encodeURIComponent(workstreamId)}/session-sharing`, { method: "DELETE" });
    return this.getSessionSharing(workstreamId);
  }
}
