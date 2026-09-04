import type { AgentVendor } from "./model";

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
  enableCursor: boolean;
}

export interface NativeSessionDetail {
  available: boolean;
  title?: string;
  branch?: string;
  messages: Array<{ kind: string; text?: string; tool?: string; at?: string }>;
}

export interface NativeSessionOpenResult {
  vendor: AgentVendor;
  opened: boolean;
  /** Plain-language outcome. Handler absence is a supported, visible state. */
  detail: string;
  /** A local fallback the member can copy when native opening is unavailable. */
  fallbackCommand?: string;
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

/**
 * Wails reaches the webview under a custom scheme on macOS and a loopback host
 * elsewhere, so both spellings mean "this is the desktop app". Everything that
 * branches on being native has to agree on that: when the bootstrap in main.tsx
 * only checked the scheme, a webview served from wails.localhost fell through
 * to the public marketing page instead of the dashboard.
 */
export const isDesktopWebview = window.location.protocol === "wails:" || window.location.hostname === "wails.localhost";

/**
 * Whether this page is being rendered inside the Overgent desktop window, on
 * any origin.
 *
 * `isDesktopWebview` answers a narrower question - is this page served by the
 * desktop shell's own asset handler - and the live Project view is not: the
 * desktop window navigates to the hosted origin to open a Project, so from that
 * point on the app is running in the desktop window with no native bridge. The
 * shell stamps its own name into the webview's user agent so a hosted page can
 * still tell where it is, which is the difference between "continue on this
 * Mac" and telling somebody to open the app they are already looking at.
 *
 * This never grants a capability. It only decides what the screen says.
 */
export const isDesktopShell = isDesktopWebview || / OvergentDesktop\//.test(navigator.userAgent);

/** The scheme the desktop shell registers, so a browser can hand off to it. */
export const desktopScheme = import.meta.env.DEV ? "overgent-dev" : "overgent";

/** The desktop shell's own route for adding a Project, on its own origin. */
const shellAddProjectRoute = "/?desktop=onboarding&add=project";

/**
 * Where "continue on this Mac" should send this window, from wherever it is.
 *
 * Three cases, and the difference matters because two of the three targets do
 * nothing in the other's situation:
 *
 * - Already on the shell's own origin: an ordinary route change.
 * - A hosted page inside the desktop window: the shell's origin, addressed in
 *   full. A `overgent://` link is the wrong instrument here - WKWebView has no
 *   navigation policy delegate in this build, so it never hands a custom scheme
 *   to the system and the link silently does nothing. That is the bug that made
 *   the old hand-off screen a dead end: the one place its button was offered
 *   was the one place it could not work.
 * - A real browser: the registered scheme, which is exactly what it is for.
 */
export function desktopHandoffURL(): string {
  if (isDesktopWebview) return shellAddProjectRoute;
  if (isDesktopShell) {
    // The shell serves its assets from wails://localhost, and in development
    // proxies the dev server on the same port this page is served from.
    const port = import.meta.env.DEV && window.location.port ? `:${window.location.port}` : "";
    return `wails://localhost${port}${shellAddProjectRoute}`;
  }
  return `${desktopScheme}://new-project`;
}

const nativeRuntimeReady = isDesktopWebview
  ? import(/* @vite-ignore */ runtimeModulePath).then((runtime: { Call?: WailsCall }) => { importedCall = runtime.Call; })
  : Promise.resolve();

async function call<T>(method: string, ...args: unknown[]): Promise<T> {
  await nativeRuntimeReady;
  const bridge = importedCall ?? globalThis.wails?.Call;
  if (!bridge?.ByName) return Promise.reject(new Error("The native Overgent bridge is unavailable. Open this flow in the Overgent desktop app."));
  return bridge.ByName<T>(`main.OnboardingService.${method}`, ...args);
}

export const nativeOnboarding = {
  state: () => call<OnboardingState>("State"),
  chooseRepository: () => call<string>("ChooseRepository"),
  createProject: (request: EnrollmentRequest) => call<EnrollmentResult>("CreateProject", request),
  createAdditionalProject: (request: EnrollmentRequest) => call<EnrollmentResult>("CreateAdditionalProject", request),
  joinProject: (request: EnrollmentRequest) => call<EnrollmentResult>("JoinProject", request),
  // Accepting an invite on a Mac that is already enrolled. Distinct from
  // joinProject, which mints a device identity and is only correct on a Mac
  // that has none.
  joinAdditionalProject: (request: EnrollmentRequest) => call<EnrollmentResult>("JoinAdditionalProject", request),
  configureAdapters: (root: string, codex: boolean, claude: boolean, cursor: boolean) => call<AdapterState[]>("ConfigureAdapters", root, codex, claude, cursor),
  reconnectAdapter: (root: string, agent: AgentVendor) => call<AdapterState>("ReconnectAdapter", root, agent),
  connectAgentWorktree: (root: string, agent: AgentVendor) => call<AdapterState>("ConnectAgentWorktree", root, agent),
  openLiveProject: (projectId: string) => call<string>("OpenLiveProject", projectId),
  resetEnrollment: () => call<OnboardingState>("ResetEnrollment"),
  sessionDetail: (workstreamId: string) => call<NativeSessionDetail>("SessionDetail", workstreamId),
  openOwningSession: (workstreamId: string, prompt: string, target: "vendor" | "vscode" = "vendor") => call<NativeSessionOpenResult>("OpenOwningSession", workstreamId, prompt, target),
  setProjectPaused: (projectId: string, paused: boolean) => call<void>("SetProjectPaused", projectId, paused),
  sessionFocus: (workstreamId: string) => call<NativeSessionFocus>("SessionFocus", workstreamId),
  /** Minutes of quiet; zero or less lets the session hear again immediately. */
  setSessionFocus: (workstreamId: string, minutes: number) => call<NativeSessionFocus>("SetSessionFocus", workstreamId, minutes),
};

// Optional on the interface so older signed desktop shells degrade by omitting
// the action instead of making the rest of onboarding unusable during update.
export type NativeOnboarding = Omit<typeof nativeOnboarding, "openOwningSession"> & {
  openOwningSession?: typeof nativeOnboarding.openOwningSession;
};
