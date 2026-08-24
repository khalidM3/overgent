export interface AdapterState {
  name: string;
  installed: boolean;
  configured: boolean;
  fidelity: string;
  detail: string;
}

export interface OnboardingState {
  available: boolean;
  enrolled: boolean;
  projectId: string;
  repositoryRoot: string;
  repositoryLabel: string;
  deviceLabel: string;
  apiBaseUrl: string;
  adapters: AdapterState[];
  limitation: string;
}

export interface EnrollmentRequest {
  repositoryRoot: string;
  projectLabel: string;
  deviceLabel: string;
  joinCode: string;
  enableCodex: boolean;
  enableClaude: boolean;
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

function call<T>(method: string, ...args: unknown[]): Promise<T> {
  if (!globalThis.wails?.Call?.ByName) return Promise.reject(new Error("The native Stickguy bridge is unavailable. Open this flow in Stickguy Dev.app."));
  return globalThis.wails.Call.ByName<T>(`main.OnboardingService.${method}`, ...args);
}

export const nativeOnboarding = {
  state: () => call<OnboardingState>("State"),
  chooseRepository: () => call<string>("ChooseRepository"),
  createProject: (request: EnrollmentRequest) => call<EnrollmentResult>("CreateProject", request),
  joinProject: (request: EnrollmentRequest) => call<EnrollmentResult>("JoinProject", request),
  configureAdapters: (root: string, codex: boolean, claude: boolean) => call<AdapterState[]>("ConfigureAdapters", root, codex, claude),
  connectAgentWorktree: (root: string, agent: "codex" | "claude") => call<AdapterState>("ConnectAgentWorktree", root, agent),
  openLiveProject: (projectId: string) => call<string>("OpenLiveProject", projectId),
};

export type NativeOnboarding = typeof nativeOnboarding;
