import { useState } from "react";
import type { ReactNode } from "react";
import { Check, Monitor, Moon, Sun } from "lucide-react";
import { Screen, ScreenSection } from "./screen";
import { AIDefaultsSettings } from "./ai-defaults";
import { VendorMark } from "./vendor-marks";
import type { ThemeChoice } from "./theme";
import type { AdapterState, NativeOnboarding, OnboardingState } from "./native";
import type { AgentVendor } from "./model";

const tabs = ["you", "appearance", "integrations", "intelligence", "advanced"] as const;
type Tab = typeof tabs[number];
const tabLabel: Record<Tab, string> = { you: "You", appearance: "Appearance", integrations: "Agents", intelligence: "Intelligence", advanced: "Advanced" };

/**
 * Preferences for this Mac.
 *
 * Every section here used to be one control and a sentence: appearance was a
 * single button whose label was the *other* state, agents were a checkbox and a
 * button stacked inside the same paragraph, and the service was two lines of
 * prose. Each was accurate and none of them looked like a place, which is the
 * whole problem with a settings screen assembled one control at a time.
 *
 * What they now share is a row: what the thing is on the left, what it is set
 * to or what to do about it on the right, hairlines between. That is the same
 * shape the workroom's rows use, so this screen belongs to the same product.
 */
export function MacSettings({ api, state, projectId, onBack, refresh, theme = "system", onTheme, identity }: {
  api: NativeOnboarding;
  state: OnboardingState;
  projectId: string;
  onBack: () => void;
  refresh: () => Promise<unknown>;
  theme?: ThemeChoice;
  onTheme?: (choice: ThemeChoice) => void;
  /** The display-name control, supplied by the window that has a Project
   *  session to save it through. The standalone Projects window has none, so
   *  it reports the name this Mac holds instead of offering a dead field. */
  identity?: ReactNode;
}) {
  const [tab, setTab] = useState<Tab>("you");
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const [reconnect, setReconnect] = useState<AgentVendor | null>(null);
  const project = state.projects?.find((entry) => entry.projectId === projectId);
  const root = project?.repositoryRoot ?? state.repositoryRoot;
  const run = async (operation: () => Promise<unknown>) => { setPending(true); setError(""); try { await operation(); await refresh(); } catch (cause) { setError((cause as Error).message); } finally { setPending(false); } };

  return <Screen title="App settings" backLabel={project?.repositoryLabel ?? "Projects"} onBack={onBack} lede="Preferences for this Mac. Project access and sharing live in Project settings.">
    <nav className="settings-tabs" aria-label="App settings sections">
      {tabs.map((name) => <button key={name} className="text-button" aria-current={tab === name ? "page" : undefined} onClick={() => setTab(name)}>{tabLabel[name]}</button>)}
    </nav>

    {/* One name, not one per Project: nobody is called something different in
        each of them, and asking again in every Project's settings was the
        clearest sign that this preference was filed in the wrong place. */}
    {tab === "you" && (identity ?? <ScreenSection title="Your name" help="How teammates see you on live sessions and decisions.">
      <div className="fact-list"><div className="fact-row"><span className="fact-name">Display name</span><span className="fact-value">{state.memberName || state.deviceLabel}</span></div></div>
      <p className="settings-help">Open a Project to change it; the name is stored with your membership of each Project this Mac belongs to.</p>
    </ScreenSection>)}

    {tab === "appearance" && <ScreenSection title="Appearance" help="How Overgent looks on this Mac. Nothing here is shared with a Project or a teammate.">
      <AppearanceChoices theme={theme} onTheme={onTheme} />
    </ScreenSection>}

    {tab === "integrations" && <ScreenSection title="Coding agents" help="Agents detected on this Mac. Connecting installs Overgent’s hooks for that agent; it never changes what the agent is allowed to do in your code.">
      <div className="agent-list">
        {state.adapters.map((adapter) => <AgentRow
          key={adapter.name} adapter={adapter} pending={pending} canDisconnect={Boolean(api.disconnectAgent)} connectable={Boolean(root)}
          onConnect={(vendor) => void run(() => api.configureAdapters(root, vendor === "codex", vendor === "claude", vendor === "cursor"))}
          onDisconnect={(vendor) => void run(async () => { await api.disconnectAgent!(vendor); rememberAgent(vendor, false); })}
          onReconnect={setReconnect}
          onPreference={(vendor, on) => { if (!rememberAgent(vendor, on)) setError("This window could not save the preference."); }}
        />)}
      </div>
      {reconnect && <div className="reconnect-preview">
        <h3>Reconnect {reconnect}?</h3>
        <p>This replaces Overgent’s managed connection to another profile. Unrelated agent settings are preserved.</p>
        <button className="pill" onClick={() => setReconnect(null)}>Cancel</button>
        <button className="pill solid" disabled={pending} onClick={() => void run(async () => { await api.reconnectAdapter(root, reconnect); setReconnect(null); })}>Reconnect to this Project</button>
      </div>}
    </ScreenSection>}

    {/* Intelligence is a preference for this Mac, so it sits here beside the
        others. What a given Project actually runs stays on that Project's own
        settings screen, where the sharing consequences are stated. */}
    {tab === "intelligence" && (api.aiDefaults && api.putAIDefaults
      ? <AIDefaultsSettings api={{ aiDefaults: api.aiDefaults, putAIDefaults: api.putAIDefaults }} />
      : <ScreenSection title="Defaults for new Projects"><p className="settings-help">This version of the Overgent app cannot store defaults yet. Each Project’s intelligence settings are on its own settings screen.</p></ScreenSection>)}

    {tab === "advanced" && <ScreenSection title="Local service" help="The coordination service that runs on this Mac. It watches only the repositories you have registered, and a Project kept on this Mac keeps its database here too.">
      <div className="fact-list">
        <div className="fact-row">
          <span className="fact-name">Status</span>
          <span className={state.backend?.running ? "fact-value live" : "fact-value"}>
            {state.backend?.running ? "Running now" : state.backend?.present ? "Starts when you open a repository" : "Not installed in this build"}
          </span>
        </div>
        {state.backend?.running && state.backend.port !== undefined && <Fact name="Address" value={`127.0.0.1:${state.backend.port}`} mono />}
        {state.backend?.version && <Fact name="Version" value={state.backend.version} mono />}
        {state.backend?.sizeOnDisk !== undefined && <Fact name="Data on disk" value={formatSize(state.backend.sizeOnDisk)} mono />}
        <Fact name="This Mac" value={state.deviceLabel} />
      </div>
      {state.backend?.lastError && <p className="form-warning">{state.backend.lastError}</p>}
      <p className="settings-help">Self-hosted server addresses are chosen when you create a shared Project. They never change a Project kept on this Mac.</p>
    </ScreenSection>}

    {error && <p role="alert" className="form-error">{error}</p>}
  </Screen>;
}

/**
 * Light, dark, or whatever the Mac is set to.
 *
 * Exported because a window with no desktop bridge still has an appearance,
 * and the alternative was a second theme control that looked nothing like this
 * one.
 */
export function AppearanceChoices({ theme = "system", onTheme }: { theme?: ThemeChoice; onTheme?: (choice: ThemeChoice) => void }) {
  return <div className="theme-choices" role="radiogroup" aria-label="Appearance">
    {([
      { id: "light", label: "Light", detail: "Always light", Icon: Sun },
      { id: "dark", label: "Dark", detail: "Always dark", Icon: Moon },
      { id: "system", label: "System", detail: "Follows macOS", Icon: Monitor },
    ] as const).map(({ id, label, detail, Icon }) => <button
      key={id} type="button" role="radio" aria-checked={theme === id} className="theme-choice"
      onClick={() => onTheme?.(id)}
    >
      {/* A real miniature of each palette rather than an icon standing in for
          one: the two grounds and a hairline, in the literal token values, so
          the swatch cannot drift from what it promises. System shows both
          because that is what it does. */}
      <span className={`theme-swatch theme-swatch-${id}`} aria-hidden="true"><Icon size={15} /></span>
      <span className="theme-name"><strong>{label}</strong><small>{detail}</small></span>
      <span className="theme-check" aria-hidden="true">{theme === id && <Check size={14} />}</span>
    </button>)}
  </div>;
}

/**
 * One agent, as a row: its own mark, what it is called, what Overgent is
 * actually getting from it, and — on the trailing edge — the one action
 * available.
 *
 * The action used to sit in the middle of the text with a checkbox above it,
 * so three agents produced three differently-shaped paragraphs. Connect takes
 * `--live` and disconnect takes `--alert`: both are already this product's
 * colours for "true right now" and "destructive", and neither fills a
 * background.
 *
 * The status line never claims observation the vendor has not shown. An agent
 * whose hooks still need review says so, which is the case the tests pin.
 */
function AgentRow({ adapter, pending, canDisconnect, connectable, onConnect, onDisconnect, onReconnect, onPreference }: {
  adapter: AdapterState;
  pending: boolean;
  canDisconnect: boolean;
  connectable: boolean;
  onConnect: (vendor: AgentVendor) => void;
  onDisconnect: (vendor: AgentVendor) => void;
  onReconnect: (vendor: AgentVendor) => void;
  onPreference: (vendor: AgentVendor, on: boolean) => void;
}) {
  const vendor = vendorFor(adapter.name);
  const connectNow = connectable && adapter.installed && !adapter.configured && !adapter.reconnectAllowed && adapter.binding !== "drifted";
  return <div className={adapter.installed ? "agent-row" : "agent-row quiet"}>
    <span className="agent-mark" aria-hidden="true"><VendorMark vendor={vendor} size={20} /></span>
    <span className="agent-text">
      <strong>{adapter.name}</strong>
      <small className={adapter.runtimeVerified && !adapter.hooksNeedReview ? "live" : undefined}>{statusOf(adapter)}</small>
      {adapter.installed && <label className="agent-auto">
        <input type="checkbox" defaultChecked={readAgentPreference(vendor)} onChange={(event) => onPreference(vendor, event.target.checked)} />
        Connect in new Projects
      </label>}
    </span>
    <span className="agent-action">
      {connectNow && <button className="pill affirming" disabled={pending} onClick={() => onConnect(vendor)}>Connect</button>}
      {adapter.configured && canDisconnect && <button className="pill alerting" disabled={pending} onClick={() => onDisconnect(vendor)}>Disconnect</button>}
      {adapter.reconnectAllowed && <button className="pill" disabled={pending} onClick={() => onReconnect(vendor)}>Reconnect</button>}
    </span>
  </div>;
}

/** What this agent is giving Overgent, in the order the claims get weaker.
 *  Hooks awaiting review outrank a runtime claim, because until they run the
 *  agent is connected and reporting nothing. */
function statusOf(adapter: AdapterState): string {
  if (adapter.hooksNeedReview) return adapter.reviewGuidance || adapter.detail || "Hooks need review before sessions can be observed";
  if (adapter.runtimeVerified) return "Observing session activity";
  if (adapter.configured) return adapter.detail || "Connected · waiting for a session";
  if (adapter.installed) return adapter.detail || "Detected on this Mac";
  return adapter.detail || "Not detected on this Mac";
}

function Fact({ name, value, mono = false }: { name: string; value: string; mono?: boolean }) {
  return <div className="fact-row"><span className="fact-name">{name}</span><span className={mono ? "fact-value mono" : "fact-value"}>{value}</span></div>;
}

function formatSize(bytes: number): string {
  if (bytes < 1_000_000) return `${Math.max(1, Math.round(bytes / 1_000))} KB`;
  if (bytes < 1_000_000_000) return `${(bytes / 1_000_000).toFixed(bytes < 10_000_000 ? 1 : 0)} MB`;
  return `${(bytes / 1_000_000_000).toFixed(1)} GB`;
}

const vendorNames: Record<string, AgentVendor> = { Codex: "codex", "Claude Code": "claude", Cursor: "cursor" };
function vendorFor(name: string): AgentVendor { return vendorNames[name] ?? "codex"; }

export function readAgentPreference(vendor: string): boolean {
  try { return localStorage.getItem(`overgent.agent.${vendor}`) !== "off"; } catch { return true; }
}
/** Returns whether the preference could be stored; a window that cannot write
 *  it must say so rather than show a tick that means nothing. */
function rememberAgent(vendor: string, on: boolean): boolean {
  try { localStorage.setItem(`overgent.agent.${vendor}`, on ? "on" : "off"); return true; } catch { return false; }
}
