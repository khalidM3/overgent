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

/**
 * One Project on this Mac and the backend it lives on. After ADR-074 a local
 * Project and a team Project sit side by side, so "where does this Project's
 * data live" is answered per Project rather than per Mac.
 */
export interface ProjectState {
  projectId: string;
  repositoryRoot: string;
  repositoryLabel: string;
  backendId: string;
  kind: "local" | "team" | "";
  apiBaseUrl: string;
  credential?: "ok" | "revoked" | "unknown" | "uncertain";
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
  /** The backend the selected Project lives on, so a reset names one enrollment. */
  backendId?: string;
  projects?: ProjectState[];
  adapters: AdapterState[];
  limitation: string;
  /**
   * Whether the credential this Mac holds for the selected Project's backend
   * still authenticates. "revoked" and "unknown" both arrive as 401 but need
   * different copy; "uncertain" means the check could not complete and must
   * never be shown as a reason to erase an enrollment. It is per backend: one
   * revoked team Project says nothing about the local Project beside it.
   */
  credential?: "ok" | "revoked" | "unknown" | "uncertain";
  /** This build carries a backend, so "Use on this Mac" is a real choice. */
  localAvailable?: boolean;
  /**
   * The name this Mac knows the member by. It seeds a new Project's member row
   * so identity does not fall back to the device hostname; it never overrides
   * the name a Project already holds, which is that Project's own to change.
   */
  memberName?: string;
  /** The bundled backend's state, shown beside service health. */
  backend?: {
    present: boolean;
    running: boolean;
    port?: number;
    sitePort?: number;
    version?: string;
    lastError?: string;
    sizeOnDisk?: number;
  };
}

export interface EnrollmentRequest {
  repositoryRoot: string;
  projectLabel: string;
  deviceLabel: string;
  /** Member-chosen live-work identity. Empty means the member is asked to choose one later. */
  displayName: string;
  joinCode: string;
  /**
   * "Advanced: connect to a different server". Empty means this build's
   * default. Validated with the same rule as `overgent create --api`.
   */
  serverOrigin: string;
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

export interface AISettings {
  judgment: { provider: "anthropic" | "openai-compatible" | "none"; model: string; baseUrl: string | null; keyConfigured: boolean; keyHint: string | null };
  embeddings: { provider: "openai" | "deterministic"; model: string; dimensions: 1024; baseUrl: string | null; keyConfigured: boolean; keyHint: string | null };
  effective: { judgment: "project" | "operator" | "none"; embeddings: "project" | "operator" | "deterministic" };
  revision: number;
  updatedAt: string;
}

export interface AISettingsWrite {
  judgment: { provider: AISettings["judgment"]["provider"]; model: string; baseUrl?: string; apiKey?: string; removeKey?: boolean };
  embeddings: { provider: AISettings["embeddings"]["provider"]; model: string; dimensions: 1024; baseUrl?: string; apiKey?: string; removeKey?: boolean };
}

/**
 * What new Projects on this Mac start from.
 *
 * One tier above AISettings, which stays the only thing that runs: a Project's
 * settings live in that Project's backend, encrypted there (ADR-073). These are
 * a preference on this Mac, so intelligence is configured once instead of
 * re-entered per Project. Keys are held in the login Keychain, never in this
 * shape and never in a readable file, which is why only `keyStored` crosses.
 */
export interface AIDefaults {
  judgment: { provider: AISettings["judgment"]["provider"]; model: string; baseUrl: string; keyStored: boolean };
  embeddings: { provider: AISettings["embeddings"]["provider"]; model: string; dimensions: 1024; baseUrl: string; keyStored: boolean };
}

export interface AIDefaultsWrite {
  judgment: { provider: AISettings["judgment"]["provider"]; model: string; baseUrl?: string; apiKey?: string; removeKey?: boolean };
  embeddings: { provider: AISettings["embeddings"]["provider"]; model: string; dimensions: 1024; baseUrl?: string; apiKey?: string; removeKey?: boolean };
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
 *
 * `returnProjectId` names the Project this window was reading when the member
 * asked to add another, so the setup screen has somewhere to send them back to.
 * Adding a Project navigates the whole window away from the workroom, and
 * without it "Back" could only ever reach the setup screen's own home — which
 * is how pressing Back inside a Project ended up somewhere the member had never
 * been. It is carried only on a navigation this window makes to itself, never
 * on the `overgent://` deep link, which stays a fixed route with nothing from
 * the URL interpolated into it (see desktopDeepLinkTarget). The receiving side
 * checks it against the Projects actually enrolled on this Mac before acting on
 * it, so the worst a crafted value can do is name a Project the member has.
 */
export function desktopHandoffURL(returnProjectId = ""): string {
  const from = returnProjectId ? `&from=${encodeURIComponent(returnProjectId)}` : "";
  if (isDesktopWebview) return `${shellAddProjectRoute}${from}`;
  if (isDesktopShell) {
    // The shell serves its assets from wails://localhost, and in development
    // proxies the dev server on the same port this page is served from.
    const port = import.meta.env.DEV && window.location.port ? `:${window.location.port}` : "";
    return `wails://localhost${port}${shellAddProjectRoute}${from}`;
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
  dashboardRequest: (projectId: string, method: string, path: string, body: string) => call<{ status: number; body: string }>("DashboardRequest", projectId, method, path, body),
  state: () => call<OnboardingState>("State"),
  // "Check again" means "the cached answer is wrong", so it must not be served
  // one. State() caches credential health for fifteen seconds; this drops it.
  recheckState: () => call<OnboardingState>("RecheckState"),
  chooseRepository: () => call<string>("ChooseRepository"),
  createProject: (request: EnrollmentRequest) => call<EnrollmentResult>("CreateProject", request),
  createLocalProject: (request: EnrollmentRequest) => call<EnrollmentResult>("CreateLocalProject", request),
  createAdditionalProject: (request: EnrollmentRequest) => call<EnrollmentResult>("CreateAdditionalProject", request),
  joinProject: (request: EnrollmentRequest) => call<EnrollmentResult>("JoinProject", request),
  // Accepting an invite from the "Add a Project" screen. It reaches the same
  // flow as joinProject; whether this Mac already has a device identity is a
  // question about the invite's backend, and the flow answers it.
  joinAdditionalProject: (request: EnrollmentRequest) => call<EnrollmentResult>("JoinAdditionalProject", request),
  exportProject: (projectId: string) => call<void>("ExportProject", projectId),
  disconnectAgent: (vendor: AgentVendor) => call<void>("DisconnectAgent", vendor),
  configureAdapters: (root: string, codex: boolean, claude: boolean, cursor: boolean) => call<AdapterState[]>("ConfigureAdapters", root, codex, claude, cursor),
  reconnectAdapter: (root: string, agent: AgentVendor) => call<AdapterState>("ReconnectAdapter", root, agent),
  connectAgentWorktree: (root: string, agent: AgentVendor) => call<AdapterState>("ConnectAgentWorktree", root, agent),
  openLiveProject: (projectId: string) => call<string>("OpenLiveProject", projectId),
  // Scoped to one backend. An empty id forgets every backend on this Mac,
  // which is the "start over completely" form.
  resetEnrollment: (backendId = "") => call<OnboardingState>("ResetEnrollment", backendId),
  sessionDetail: (workstreamId: string) => call<NativeSessionDetail>("SessionDetail", workstreamId),
  openOwningSession: (workstreamId: string, prompt: string, target: "vendor" | "vscode" = "vendor") => call<NativeSessionOpenResult>("OpenOwningSession", workstreamId, prompt, target),
  setProjectPaused: (projectId: string, paused: boolean) => call<void>("SetProjectPaused", projectId, paused),
  sessionFocus: (workstreamId: string) => call<NativeSessionFocus>("SessionFocus", workstreamId),
  /** Minutes of quiet; zero or less lets the session hear again immediately. */
  setSessionFocus: (workstreamId: string, minutes: number) => call<NativeSessionFocus>("SetSessionFocus", workstreamId, minutes),
  aiSettings: (projectId: string) => call<AISettings>("AISettings", projectId),
  putAISettings: (projectId: string, write: AISettingsWrite) => call<AISettings>("PutAISettings", projectId, write),
  aiDefaults: () => call<AIDefaults>("AIDefaults"),
  putAIDefaults: (write: AIDefaultsWrite) => call<AIDefaults>("PutAIDefaults", write),
};

// Optional on the interface so older signed desktop shells degrade by omitting
// the action instead of making the rest of onboarding unusable during update.
export type NativeOnboarding = Omit<typeof nativeOnboarding, "exportProject" | "disconnectAgent" | "dashboardRequest" | "openOwningSession" | "aiSettings" | "putAISettings" | "aiDefaults" | "putAIDefaults" | "recheckState"> & {
  exportProject?: typeof nativeOnboarding.exportProject;
  disconnectAgent?: typeof nativeOnboarding.disconnectAgent;
  dashboardRequest?: typeof nativeOnboarding.dashboardRequest;
  openOwningSession?: typeof nativeOnboarding.openOwningSession;
  aiSettings?: typeof nativeOnboarding.aiSettings;
  putAISettings?: typeof nativeOnboarding.putAISettings;
  aiDefaults?: typeof nativeOnboarding.aiDefaults;
  putAIDefaults?: typeof nativeOnboarding.putAIDefaults;
  recheckState?: typeof nativeOnboarding.recheckState;
};
