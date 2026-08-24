import type { Fidelity, SemanticStatus, ShellState } from "./model";

export const stateMessage = (state: ShellState): string => ({
  activation: "Finish activating this browser session.",
  loading: "Loading Project coordination…",
  ready: "Project coordination is ready.",
  empty: "No Projects are available for this member yet.",
  offline: "Offline. Showing the last synchronized revision.",
  unauthorized: "This device is not authorized for the Project.",
  version_mismatch: "Upgrade Stickguy to continue.",
})[state];

export const fidelityLabel = (fidelity: Fidelity): string => ({
  mcp: "MCP reported",
  git: "Git observed",
  manual: "Manual intent",
  hook: "Live agent",
  hook_unverified: "Hook unverified",
})[fidelity];

export const semanticMessage = (status: SemanticStatus): string => ({
  enabled: "Semantic coordination is available.",
  degraded: "Semantic processing is delayed. Structural findings remain live.",
  disabled: "Semantic processing is off. Structural findings remain available.",
})[status];
