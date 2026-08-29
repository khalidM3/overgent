export interface AdapterState {
  name: string;
  installed: boolean;
  configured: boolean;
  fidelity: string;
  detail: string;
  binding: "not_configured" | "current" | "partial" | "other_profile" | "drifted";
  previousProfile?: string;
  currentProfile: string;
  runtimeVerified: boolean;
  restartRequired: boolean;
  reconnectAllowed: boolean;
  /** Codex installed the hooks but will not run them until the member trusts them. */
  hooksNeedReview: boolean;
  reviewGuidance?: string;
}

export interface OnboardingState {
  available: boolean;
  development: boolean;
  enrolled: boolean;
  projectId: string;
  repositoryRoot: string;
  repositoryLabel: string;
  deviceLabel: string;
  apiBaseUrl: string;
  adapters: AdapterState[];
  limitation: string;
  /**
   * Whether this Mac's stored credential still authenticates. "revoked" and
   * "unknown" both arrive from the hosted API as 401 but need different copy;
   * "uncertain" means the check could not complete and must never be shown as a
   * reason to erase an enrollment.
   */
  credential?: "ok" | "revoked" | "unknown" | "uncertain";
}

export interface EnrollmentRequest {
  repositoryRoot: string;
  projectLabel: string;
  deviceLabel: string;
  /** Member-chosen live-work identity. Empty means the member is asked to choose one later. */
  displayName: string;
  joinCode: string;
  enableCodex: boolean;
  enableClaude: boolean;
}

export interface NativeSessionDetail {
  available: boolean;
  title?: string;
  branch?: string;
  messages: Array<{ kind: string; text?: string; tool?: string; at?: string }>;
}

/**
 * The quiet period on one of this device's own agent sessions.
 *
 * Focus is local state that never crosses the wire. It stops coordination
 * being injected into this agent's turns and changes nothing about what this
 * device publishes, so a teammate sees no difference and loses no visibility.
 */
export interface NativeSessionFocus {
  sessionId: string;
  focused: boolean;
  /** RFC 3339 instant the quiet period lapses. Always present while focused. */
  until?: string;
}

export interface EnrollmentResult {
  projectId: string;
  joinCode: string;
  warnings: string[];
}

interface WailsCall {
  ByName<T>(name: string, ...args: unknown[]): Promise<T>;
}

declare global {
  // Wails injects this only inside the signed desktop webview.
  var wails: { Call?: WailsCall } | undefined;
}

const runtimeModulePath = "/wails/runtime.js";
let importedCall: WailsCall | undefined;
const nativeRuntimeReady = window.location.protocol === "wails:" || window.location.hostname === "wails.localhost"
  ? import(/* @vite-ignore */ runtimeModulePath).then((runtime: { Call?: WailsCall }) => { importedCall = runtime.Call; })
  : Promise.resolve();

async function call<T>(method: string, ...args: unknown[]): Promise<T> {
  await nativeRuntimeReady;
  const bridge = importedCall ?? globalThis.wails?.Call;
  if (!bridge?.ByName) return Promise.reject(new Error("The native Stickguy bridge is unavailable. Open this flow in the Stickguy desktop app."));
  return bridge.ByName<T>(`main.OnboardingService.${method}`, ...args);
}

export const nativeOnboarding = {
  state: () => call<OnboardingState>("State"),
  chooseRepository: () => call<string>("ChooseRepository"),
  createProject: (request: EnrollmentRequest) => call<EnrollmentResult>("CreateProject", request),
  createAdditionalProject: (request: EnrollmentRequest) => call<EnrollmentResult>("CreateAdditionalProject", request),
  joinProject: (request: EnrollmentRequest) => call<EnrollmentResult>("JoinProject", request),
  configureAdapters: (root: string, codex: boolean, claude: boolean) => call<AdapterState[]>("ConfigureAdapters", root, codex, claude),
  reconnectAdapter: (root: string, agent: "codex" | "claude") => call<AdapterState>("ReconnectAdapter", root, agent),
  connectAgentWorktree: (root: string, agent: "codex" | "claude") => call<AdapterState>("ConnectAgentWorktree", root, agent),
  openLiveProject: (projectId: string) => call<string>("OpenLiveProject", projectId),
  resetEnrollment: () => call<OnboardingState>("ResetEnrollment"),
  sessionDetail: (workstreamId: string) => call<NativeSessionDetail>("SessionDetail", workstreamId),
  setProjectPaused: (projectId: string, paused: boolean) => call<void>("SetProjectPaused", projectId, paused),
  sessionFocus: (workstreamId: string) => call<NativeSessionFocus>("SessionFocus", workstreamId),
  /** Minutes of quiet; zero or less lets the session hear again immediately. */
  setSessionFocus: (workstreamId: string, minutes: number) => call<NativeSessionFocus>("SetSessionFocus", workstreamId, minutes),
};

export type NativeOnboarding = typeof nativeOnboarding;
