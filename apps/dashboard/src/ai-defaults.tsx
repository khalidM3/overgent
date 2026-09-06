import { useEffect, useState } from "react";
import { ScreenSection } from "./screen";
import type { AIDefaults, AIDefaultsWrite } from "./native";

type AIDefaultsAPI = { aiDefaults: () => Promise<AIDefaults>; putAIDefaults: (write: AIDefaultsWrite) => Promise<AIDefaults> };

const editable = (value: AIDefaults): AIDefaultsWrite => ({
  judgment: { provider: value.judgment.provider, model: value.judgment.model, ...(value.judgment.baseUrl ? { baseUrl: value.judgment.baseUrl } : {}) },
  embeddings: { provider: value.embeddings.provider, model: value.embeddings.model, dimensions: 1024, ...(value.embeddings.baseUrl ? { baseUrl: value.embeddings.baseUrl } : {}) },
});

/**
 * What new Projects on this Mac start from.
 *
 * The per-Project panel is the one that decides what actually runs. This is the
 * tier above it, and the reason it exists is that configuring intelligence was
 * data entry repeated per Project rather than a preference set once.
 *
 * The split in where these are applied is the whole point, and it is stated on
 * screen rather than left to be discovered: a Project on this Mac takes them
 * automatically, because its backend is this machine and applying them sends
 * the key nowhere. A shared Project does not, because saving a key there
 * uploads it to a server other members' sessions spend it from — a decision
 * that belongs on that Project's own settings screen, next to the sentence
 * that says so.
 */
export function AIDefaultsSettings({ api }: { api: AIDefaultsAPI }) {
  const [defaults, setDefaults] = useState<AIDefaults | null>(null);
  const [form, setForm] = useState<AIDefaultsWrite | null>(null);
  const [pending, setPending] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState("");
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let active = true;
    setForm(null); setDefaults(null); setError(""); setSaved(false);
    void api.aiDefaults()
      .then((value) => { if (active) { setDefaults(value); setForm(editable(value)); } })
      .catch((cause: Error) => { if (active) setError(cause.message); });
    return () => { active = false; };
  }, [api.aiDefaults, attempt]);

  const change = (value: AIDefaultsWrite) => { setForm(value); setSaved(false); };
  const save = async () => {
    if (!form) return;
    setPending(true); setError(""); setSaved(false);
    try {
      const updated = await api.putAIDefaults(form);
      setDefaults(updated); setForm(editable(updated)); setSaved(true);
    } catch (cause) { setError((cause as Error).message); }
    finally { setPending(false); }
  };

  if (!form) {
    return <ScreenSection title="Defaults for new Projects">
      {error
        ? <><p className="form-error" role="alert">These defaults could not be loaded.</p><button className="pill" onClick={() => setAttempt((value) => value + 1)}>Try again</button></>
        : <p role="status" className="field-note">Loading…</p>}
    </ScreenSection>;
  }

  const judgmentOn = form.judgment.provider !== "none";
  const embeddingsOn = form.embeddings.provider === "openai";
  return <ScreenSection
    title="Defaults for new Projects"
    help="Set intelligence once and every new Project on this Mac starts here. Keys are stored in your login Keychain, never in a file Overgent writes."
  >
    <p className="settings-help">A Project on this Mac takes these automatically. A shared Project does not: saving a key there uploads it to that Project’s server, so it is offered on the Project’s own settings screen instead of applied for you.</p>
    <fieldset disabled={pending} className="provider-fields">
      <div className="provider-settings">
        <h3>Assess coordination</h3>
        <label className="field"><span>Judgment provider</span>
          <select value={form.judgment.provider} onChange={(event) => change({ ...form, judgment: { provider: event.target.value as AIDefaultsWrite["judgment"]["provider"], model: "", removeKey: true } })}>
            <option value="none">Off · structural evidence only</option>
            <option value="anthropic">Anthropic</option>
            <option value="openai-compatible">OpenAI or compatible provider</option>
          </select>
        </label>
        {judgmentOn && <>
          <label className="field"><span>Judgment model</span><input required value={form.judgment.model} maxLength={120} placeholder="Model ID from your provider" onChange={(event) => change({ ...form, judgment: { ...form.judgment, model: event.target.value } })} /></label>
          <label className="field"><span>Judgment API key</span><input type="password" autoComplete="new-password" spellCheck={false} value={form.judgment.apiKey ?? ""} placeholder={defaults?.judgment.keyStored && !form.judgment.removeKey ? "Key saved · leave blank to keep" : "Enter an API key"} onChange={(event) => change({ ...form, judgment: { ...form.judgment, apiKey: event.target.value || undefined, removeKey: false } })} /></label>
          {defaults?.judgment.keyStored && <button className="text-button" onClick={() => change({ ...form, judgment: { ...form.judgment, apiKey: undefined, removeKey: true } })}>{form.judgment.removeKey ? "Key will be removed on save" : "Remove saved judgment key"}</button>}
          <details className="field-advanced"><summary>Custom endpoint</summary>
            <label className="field"><span>Judgment server address</span><input value={form.judgment.baseUrl ?? ""} placeholder="Provider default" onChange={(event) => change({ ...form, judgment: { ...form.judgment, baseUrl: event.target.value || undefined, apiKey: undefined, removeKey: true } })} /></label>
            <p className="field-note">Changing the endpoint removes the saved key. Enter a key for the new destination.</p>
          </details>
        </>}
      </div>
      <details className="field-advanced provider-settings">
        <summary>Find related work · {embeddingsOn ? "AI embeddings" : "built-in matching"}</summary>
        <label className="field"><span>Embedding provider</span>
          <select value={form.embeddings.provider} onChange={(event) => change({ ...form, embeddings: { provider: event.target.value as AIDefaultsWrite["embeddings"]["provider"], model: event.target.value === "openai" ? "text-embedding-3-large" : "overgent-concepts/v1", dimensions: 1024, removeKey: true } })}>
            <option value="deterministic">Built-in · no API key</option>
            <option value="openai">OpenAI or compatible provider</option>
          </select>
        </label>
        {embeddingsOn && <>
          <label className="field"><span>Embedding model</span><input value={form.embeddings.model} maxLength={120} onChange={(event) => change({ ...form, embeddings: { ...form.embeddings, model: event.target.value } })} /></label>
          <label className="field"><span>Embedding API key</span><input type="password" autoComplete="new-password" spellCheck={false} value={form.embeddings.apiKey ?? ""} placeholder={defaults?.embeddings.keyStored && !form.embeddings.removeKey ? "Key saved · leave blank to keep" : "Enter an API key"} onChange={(event) => change({ ...form, embeddings: { ...form.embeddings, apiKey: event.target.value || undefined, removeKey: false } })} /></label>
          {defaults?.embeddings.keyStored && <button className="text-button" onClick={() => change({ ...form, embeddings: { ...form.embeddings, apiKey: undefined, removeKey: true } })}>{form.embeddings.removeKey ? "Key will be removed on save" : "Remove saved embedding key"}</button>}
          <label className="field"><span>Embedding server address</span><input value={form.embeddings.baseUrl ?? ""} placeholder="Provider default" onChange={(event) => change({ ...form, embeddings: { ...form.embeddings, baseUrl: event.target.value || undefined, apiKey: undefined, removeKey: true } })} /></label>
          <p className="field-note">Models must support 1024-dimensional embeddings. Changing the endpoint removes the saved key.</p>
        </>}
      </details>
      <p className="field-note">Projects you already have keep the settings they have. These apply to the next one you open.</p>
      <button className="pill solid" disabled={judgmentOn && !form.judgment.model.trim()} onClick={() => void save()}>{pending ? "Saving…" : "Save defaults"}</button>
    </fieldset>
    {saved && <p role="status" className="settings-help">Defaults saved.</p>}
    {error && <p className="form-error" role="alert">{error}</p>}
  </ScreenSection>;
}
