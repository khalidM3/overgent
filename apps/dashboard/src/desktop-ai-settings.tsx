import { useEffect, useState } from "react";
import type { AISettings, AISettingsWrite } from "./native";

type AISettingsAPI = { aiSettings: (projectId: string) => Promise<AISettings>; putAISettings: (projectId: string, write: AISettingsWrite) => Promise<AISettings> };
const editable = (value: AISettings): AISettingsWrite => ({
  judgment: { provider: value.judgment.provider, model: value.judgment.model, ...(value.judgment.baseUrl ? { baseUrl: value.judgment.baseUrl } : {}) },
  embeddings: { provider: value.embeddings.provider, model: value.embeddings.model, dimensions: 1024, ...(value.embeddings.baseUrl ? { baseUrl: value.embeddings.baseUrl } : {}) },
});

export function DesktopAISettings({ api, projectId, local = false }: { api: AISettingsAPI; projectId: string; local?: boolean }) {
  const [settings, setSettings] = useState<AISettings | null>(null);
  const [form, setForm] = useState<AISettingsWrite | null>(null);
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
  const change = (value: AISettingsWrite) => { setForm(value); setSaved(false); setApproved(false); };
  const save = async () => {
    if (!form) return;
    setPending(true); setError(""); setSaved(false);
    try { const updated = await api.putAISettings(projectId, form); setSettings(updated); setForm(editable(updated)); setSaved(true); setApproved(false); }
    catch { setError("The settings could not be saved. Check your connection and Project owner access, then try again."); }
    finally { setPending(false); }
  };
  if (!form) return <section aria-label="Intelligence"><h2>Intelligence</h2>{error ? <><p className="form-error" role="alert">Provider settings could not be loaded.</p><button className="pill" onClick={() => setAttempt((value) => value + 1)}>Try again</button></> : <p role="status" className="field-note">Loading provider settings…</p>}</section>;
  const sendsKey = Boolean(form.judgment.apiKey || form.embeddings.apiKey);
  return <section className="intelligence-settings" aria-label="Intelligence">
    <h2>Intelligence</h2>
    <p className="settings-help">File overlap and contract changes work without AI. Optional providers help assess intent and find related work across different files.</p>
    <p className="settings-help">{local ? "Keys are encrypted in this Project’s database on your Mac. Configuring an external provider sends it approved coordination summaries." : "Intelligence runs on this shared Project’s server for every member. Keys you save here are sent to that server and encrypted there. Other members cannot read them."}</p>
    <fieldset disabled={pending} className="provider-fields">
      <div className="provider-settings"><h3>Assess coordination</h3>
        <label className="field"><span>Judgment provider</span><select value={form.judgment.provider} onChange={(event) => change({ ...form, judgment: { provider: event.target.value as AISettingsWrite["judgment"]["provider"], model: "", removeKey: true } })}><option value="none">Off · structural evidence only</option><option value="anthropic">Anthropic</option><option value="openai-compatible">OpenAI or compatible provider</option></select></label>
        {form.judgment.provider !== "none" && <>
          <label className="field"><span>Judgment model</span><input required value={form.judgment.model} maxLength={120} placeholder="Model ID from your provider" onChange={(event) => change({ ...form, judgment: { ...form.judgment, model: event.target.value } })} /></label>
          <label className="field"><span>Judgment API key</span><input type="password" value={form.judgment.apiKey ?? ""} autoComplete="new-password" spellCheck={false} placeholder={settings?.judgment.keyConfigured && !form.judgment.removeKey ? "Key saved · leave blank to keep" : "Enter an API key"} onChange={(event) => change({ ...form, judgment: { ...form.judgment, apiKey: event.target.value || undefined, removeKey: false } })} /></label>
          {settings?.judgment.keyConfigured && <button className="text-button" onClick={() => change({ ...form, judgment: { ...form.judgment, apiKey: undefined, removeKey: true } })}>{form.judgment.removeKey ? "Key will be removed on save" : "Remove saved judgment key"}</button>}
          <details className="field-advanced"><summary>Custom endpoint</summary><label className="field"><span>Judgment server address</span><input value={form.judgment.baseUrl ?? ""} placeholder="Provider default" onChange={(event) => change({ ...form, judgment: { ...form.judgment, baseUrl: event.target.value || undefined, apiKey: undefined, removeKey: true } })} /></label><p className="field-note">Changing the endpoint removes the saved key. Enter a key for the new destination. Local addresses work only for a Project on this Mac.</p></details>
        </>}
      </div>
      <details className="field-advanced provider-settings"><summary>Find related work · {form.embeddings.provider === "deterministic" ? "built-in matching" : "AI embeddings"}</summary>
        <label className="field"><span>Embedding provider</span><select value={form.embeddings.provider} onChange={(event) => change({ ...form, embeddings: { provider: event.target.value as AISettingsWrite["embeddings"]["provider"], model: event.target.value === "openai" ? "text-embedding-3-large" : "overgent-concepts/v1", dimensions: 1024, removeKey: true } })}><option value="deterministic">Built-in · no API key</option><option value="openai">OpenAI or compatible provider</option></select></label>
        {form.embeddings.provider === "openai" && <><label className="field"><span>Embedding model</span><input value={form.embeddings.model} maxLength={120} onChange={(event) => change({ ...form, embeddings: { ...form.embeddings, model: event.target.value } })} /></label><label className="field"><span>Embedding API key</span><input type="password" autoComplete="new-password" spellCheck={false} value={form.embeddings.apiKey ?? ""} placeholder={settings?.embeddings.keyConfigured && !form.embeddings.removeKey ? "Key saved · leave blank to keep" : "Enter an API key"} onChange={(event) => change({ ...form, embeddings: { ...form.embeddings, apiKey: event.target.value || undefined, removeKey: false } })} /></label>{settings?.embeddings.keyConfigured && <button className="text-button" onClick={() => change({ ...form, embeddings: { ...form.embeddings, apiKey: undefined, removeKey: true } })}>{form.embeddings.removeKey ? "Key will be removed on save" : "Remove saved embedding key"}</button>}<label className="field"><span>Embedding server address</span><input value={form.embeddings.baseUrl ?? ""} placeholder="Provider default" onChange={(event) => change({ ...form, embeddings: { ...form.embeddings, baseUrl: event.target.value || undefined, apiKey: undefined, removeKey: true } })} /></label><p className="field-note">Models must support 1024-dimensional embeddings. Changing the endpoint removes the saved key.</p></>}
      </details>
      {!local && sendsKey && <label className="inline-option"><input type="checkbox" checked={approved} onChange={(event) => setApproved(event.target.checked)} />Store these keys on this Project’s server for shared intelligence</label>}
      {settings && <p className="field-note">{settings.effective.judgment === "none" ? "Structural evidence is active." : "Judgment provider configured."} Saving configuration does not verify a provider connection.</p>}
      <button className="pill solid" disabled={(!local && sendsKey && !approved) || (form.judgment.provider !== "none" && !form.judgment.model.trim())} onClick={() => void save()}>{pending ? "Saving…" : "Save intelligence settings"}</button>
    </fieldset>
    {saved && <p role="status" className="settings-help">Settings saved.</p>}{error && <p className="form-error" role="alert">{error}</p>}
  </section>;
}
