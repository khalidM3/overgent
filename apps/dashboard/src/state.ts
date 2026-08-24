export type DashboardState = "loading" | "ready" | "offline" | "unauthorized" | "version_mismatch";

export const stateMessage = (state: DashboardState): string => ({
  loading: "Loading Project coordination…",
  ready: "Project coordination is ready.",
  offline: "Offline. Showing the last synchronized revision.",
  unauthorized: "This device is not authorized for the Project.",
  version_mismatch: "Upgrade Stickguy to continue.",
})[state];
