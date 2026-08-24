import { snapshotForProject } from "./fixtures";
import type { FindingFeedback, FindingState, ProjectSnapshot } from "./model";

export class FixtureProjectSource {
  readonly live: boolean = false;
  private snapshots = new Map<string, ProjectSnapshot>();
  private listeners = new Map<string, Set<() => void>>();

  constructor(initial: ProjectSnapshot[] = []) {
    for (const snapshot of initial) this.snapshots.set(snapshot.project.id, structuredClone(snapshot));
  }

  get(projectId: string): ProjectSnapshot {
    const current = this.snapshots.get(projectId);
    if (current) return current;
    const snapshot = snapshotForProject(projectId);
    this.snapshots.set(projectId, snapshot);
    return snapshot;
  }

  subscribe(projectId: string, listener: () => void): () => void {
    const listeners = this.listeners.get(projectId) ?? new Set();
    listeners.add(listener);
    this.listeners.set(projectId, listeners);
    return () => listeners.delete(listener);
  }

  togglePause(projectId: string): void {
    this.update(projectId, (snapshot) => ({ ...snapshot, workspacePaused: !snapshot.workspacePaused }));
  }

  setFindingState(projectId: string, findingId: string, state: FindingState): void {
    this.update(projectId, (snapshot) => ({
      ...snapshot,
      findings: snapshot.findings.map((finding) => finding.id === findingId ? { ...finding, state } : finding),
    }));
  }

  async recordFindingFeedback(_findingId: string, _value: FindingFeedback): Promise<void> {
    return Promise.resolve();
  }

  publishSyntheticUpdate(projectId: string): void {
    this.update(projectId, (snapshot) => ({
      ...snapshot,
      contextRevision: snapshot.contextRevision + 1,
      synchronizedAt: "just now",
      workstreams: snapshot.workstreams.map((workstream, index) => index === 0
        ? { ...workstream, pathCount: workstream.pathCount + 1, updatedLabel: "Now" }
        : workstream),
      activity: [{
        id: `act_fixture_${snapshot.contextRevision + 1}`,
        at: "Now",
        actor: "Fixture device",
        kind: "manifest",
        summary: "Published one new path-only manifest revision.",
        fidelity: "git",
      }, ...snapshot.activity],
    }));
  }

  replace(snapshot: ProjectSnapshot): void {
    this.snapshots.set(snapshot.project.id, structuredClone(snapshot));
    for (const listener of this.listeners.get(snapshot.project.id) ?? []) listener();
  }

  private update(projectId: string, updater: (snapshot: ProjectSnapshot) => ProjectSnapshot): void {
    this.snapshots.set(projectId, updater(this.get(projectId)));
    for (const listener of this.listeners.get(projectId) ?? []) listener();
  }
}
