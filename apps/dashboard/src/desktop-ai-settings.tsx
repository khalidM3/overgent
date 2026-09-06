import { useEffect, useState } from "react";
import { IntelligenceForm, type IntelligenceWrite } from "./intelligence-form";
import type { AIDefaults, AISettings, AISettingsWrite } from "./native";

type AISettingsAPI = {
  aiSettings: (projectId: string) => Promise<AISettings>;
  putAISettings: (projectId: string, write: AISettingsWrite) => Promise<AISettings>;
  /** This Mac's defaults, when the native bridge offers them. Present only so
   *  the member can copy the non-secret half into this Project; nothing here
   *  can read a key out of the Keychain, and the backend never consults the
   *  defaults tier (ADR-077). */
  aiDefaults?: () => Promise<AIDefaults>;
};

const editable = (value: AISettings): AISettingsWrite => ({
  judgment: { provider: value.judgment.provider, model: value.judgment.model, ...(value.judgment.baseUrl ? { baseUrl: value.judgment.baseUrl } : {}) },
  embeddings: { provider: value.embeddings.provider, model: value.embeddings.model, dimensions: 1024, ...(value.embeddings.baseUrl ? { baseUrl: value.embeddings.baseUrl } : {}) },
});

/**
 * The providers this Project actually runs on.
 *
 * This is the tier that executes (ADR-073): what is saved here lives in this
 * Project's own backend, and for a shared Project that backend is a server
 * other members' sessions spend the key from. The checkbox that gates saving a
 * key there is not ceremony — it is the one place a member is told that their
 * key is about to leave this Mac, so it appears exactly when a key is in the
 * form and never as a permanent piece of furniture.
 *
 * The fields are shared with the defaults screen. What is not shared is every
 * sentence about destination, and the sentence differs because the destination
 * does.
 */
export function DesktopAISettings({ api, projectId, local = false }: { api: AISettingsAPI; projectId: string; local?: boolean }) {
  const [settings, setSettings] = useState<AISettings | null>(null);
  const [form, setForm] = useState<AISettingsWrite | null>(null);
  const [defaults, setDefaults] = useState<AIDefaults | null>(null);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  const [approved, setApproved] = useState(false);
  const [attempt, setAttempt] = useState(0);
  // Depend on stable methods, not an inline wrapper object. Drop late responses
  // and all secret drafts when changing Project.
  useEffect(() => {
    let active = true; setForm(null); setSettings(null); setError(""); setSaved(false); setApproved(false);
    void api.aiSettings(projectId).then((value) => { if (active) { setSettings(value); setForm(editable(value)); } }).catch((cause: Error) => { if (active) setError(cause.message); });
    return () => { active = false; };
  }, [api.aiSettings, projectId, attempt]);
  // Defaults are a convenience and never a requirement: a bridge that cannot
  // produce them leaves the form exactly as it was.
  useEffect(() => {
    let active = true; setDefaults(null);
    if (!api.aiDefaults) return;
    void api.aiDefaults().then((value) => { if (active) setDefaults(value); }).catch(() => { if (active) setDefaults(null); });
    return () => { active = false; };
  }, [api.aiDefaults, projectId]);

  const change = (value: IntelligenceWrite) => { setForm(value); setSaved(false); setApproved(false); };
  const save = async () => {
    if (!form) return;
    setPending(true); setError(""); setSaved(false);
    try { const updated = await api.putAISettings(projectId, form); setSettings(updated); setForm(editable(updated)); setSaved(true); setApproved(false); }
    catch { setError("The settings could not be saved. Check your connection and Project owner access, then try again."); }
    finally { setPending(false); }
  };
  if (!form) return <section aria-label="Intelligence"><h2>Intelligence</h2>{error ? <><p className="form-error" role="alert">Provider settings could not be loaded.</p><button className="pill" onClick={() => setAttempt((value) => value + 1)}>Try again</button></> : <p role="status" className="field-note">Loading provider settings…</p>}</section>;

  const sendsKey = Boolean(form.judgment.apiKey || form.embeddings.apiKey);
  const fromDefaults = settings && defaults ? applyDefaults(form, settings, defaults) : null;
  return <section className="intelligence-settings" aria-label="Intelligence">
    <h2>Intelligence</h2>
    <p className="settings-help sharp">{local ? "Keys are encrypted in this Project’s database on your Mac. Configuring an external provider sends it approved coordination summaries." : "Intelligence runs on this shared Project’s server for every member. Keys you save here are sent to that server and encrypted there. Other members cannot read them."}</p>
    {fromDefaults && <div className="defaults-offer">
      <button className="text-button" onClick={() => change(fromDefaults)}>Use this Mac’s defaults</button>
      <p className="field-note">Fills in the provider, model and address you set once for new Projects. The key is not copied — it stays in the Keychain, and this form cannot read it.</p>
    </div>}
    <IntelligenceForm
      value={form}
      onChange={change}
      judgmentKey={{ stored: Boolean(settings?.judgment.keyConfigured), hint: settings?.judgment.keyHint, provider: settings?.judgment.provider, baseUrl: settings?.judgment.baseUrl }}
      embeddingKey={{ stored: Boolean(settings?.embeddings.keyConfigured), hint: settings?.embeddings.keyHint, provider: settings?.embeddings.provider, baseUrl: settings?.embeddings.baseUrl }}
      keyLocation={local ? "in this Project’s database on this Mac" : "on this Project’s server"}
      savedEmbeddingModel={settings?.embeddings.keyConfigured ? settings.embeddings.model : undefined}
      saveLabel="Save intelligence settings"
      pending={pending}
      onSave={() => void save()}
      hold={!local && sendsKey && !approved ? "Confirm that these keys may be stored on this Project’s server." : null}
      footer={<>
        {/* Uploading a key to a server other people's sessions spend it from is
            a decision, so it is taken once per key rather than implied by
            having typed one (ADR-077). */}
        {!local && sendsKey && <label className="inline-option"><input type="checkbox" checked={approved} onChange={(event) => setApproved(event.target.checked)} />Store these keys on this Project’s server for shared intelligence</label>}
        {settings && <p className="field-note">{running(settings.effective)} Saving does not test the connection: a provider that rejects the key is reported in the workroom as degraded semantic processing.</p>}
      </>}
      status={<>
        {saved && <p role="status" className="settings-help">Settings saved.</p>}
        {error && <p className="form-error" role="alert">{error}</p>}
      </>}
    />
  </section>;
}

/**
 * What is running right now, which is not always what this form contains: a
 * deployment may hold operator keys of its own (ADR-073), and a member reading
 * an empty form on a working Project deserves to be told why it works.
 */
function running(effective: AISettings["effective"]): string {
  const judgment = effective.judgment === "project" ? "Judgment runs on the key saved here."
    : effective.judgment === "operator" ? "Judgment runs on the deployment’s own key, not one saved here."
    : "No judgment provider is running; findings rest on structural evidence.";
  const embeddings = effective.embeddings === "project" ? "Related work is found with the embedding key saved here."
    : effective.embeddings === "operator" ? "Related work uses the deployment’s own embedding key."
    : "Related work is matched by the built-in comparison.";
  return `${judgment} ${embeddings}`;
}

/**
 * This Mac's defaults, as a change to this Project's form.
 *
 * Only the non-secret half crosses: the keys are in the login Keychain and the
 * page has no route to them, which is the property that makes this button safe
 * to offer on a shared Project at all. A section whose destination the copy
 * changes drops its saved key, on the same rule the picker follows; a section
 * pointed at the same provider and address keeps it, because the key that is
 * already there is a key for that address.
 *
 * Returns null when there is nothing to copy or nothing would change, so the
 * control never appears offering to do nothing.
 */
function applyDefaults(form: IntelligenceWrite, settings: AISettings, defaults: AIDefaults): IntelligenceWrite | null {
  const configured = defaults.judgment.provider !== "none" || defaults.embeddings.provider === "openai";
  if (!configured) return null;
  const next: IntelligenceWrite = {
    judgment: {
      provider: defaults.judgment.provider, model: defaults.judgment.model,
      ...(defaults.judgment.baseUrl ? { baseUrl: defaults.judgment.baseUrl } : {}),
      ...(retains(settings.judgment, defaults.judgment) ? {} : { removeKey: true }),
    },
    embeddings: {
      provider: defaults.embeddings.provider, model: defaults.embeddings.model, dimensions: 1024,
      ...(defaults.embeddings.baseUrl ? { baseUrl: defaults.embeddings.baseUrl } : {}),
      ...(retains(settings.embeddings, defaults.embeddings) ? {} : { removeKey: true }),
    },
  };
  return same(form, next) ? null : next;
}

const origin = (value: string | null | undefined): string => (value ?? "").replace(/\/+$/, "");
const retains = (saved: { provider: string; baseUrl: string | null }, incoming: { provider: string; baseUrl: string }): boolean =>
  saved.provider === incoming.provider && origin(saved.baseUrl) === origin(incoming.baseUrl);
const same = (left: IntelligenceWrite, right: IntelligenceWrite): boolean =>
  left.judgment.provider === right.judgment.provider && left.judgment.model === right.judgment.model && origin(left.judgment.baseUrl) === origin(right.judgment.baseUrl)
  && left.embeddings.provider === right.embeddings.provider && left.embeddings.model === right.embeddings.model && origin(left.embeddings.baseUrl) === origin(right.embeddings.baseUrl);
