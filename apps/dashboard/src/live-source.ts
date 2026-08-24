import { FixtureProjectSource } from "./fixture-source";
import type { DashboardSession, FindingFeedback, ProjectSnapshot } from "./model";

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
  return request<ProjectSnapshot>(`/dashboard/projects/${encodeURIComponent(projectId)}`);
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
}
