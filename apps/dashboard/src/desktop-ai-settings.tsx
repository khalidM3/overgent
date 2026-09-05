import { useEffect, useState } from "react";
import type { AISettings, AISettingsWrite } from "./native";

type AISettingsAPI = { aiSettings: (projectId: string) => Promise<AISettings>; putAISettings: (projectId: string, write: AISettingsWrite) => Promise<AISettings> };

export function DesktopAISettings({ api, projectId }: { api: AISettingsAPI; projectId: string }) {
  const [settings, setSettings] = useState<AISettings | null>(null);
  const [form, setForm] = useState<AISettingsWrite | null>(null);
  const [judgmentKey, setJudgmentKey] = useState("");
  const [embeddingKey, setEmbeddingKey] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    void api.aiSettings(projectId).then((value) => {
      setSettings(value);
      setForm({
        judgment: { provider: value.judgment.provider, model: value.judgment.model, baseUrl: value.judgment.baseUrl ?? undefined },
        embeddings: { provider: value.embeddings.provider, model: value.embeddings.model, dimensions: 1024, baseUrl: value.embeddings.baseUrl ?? undefined },
      });
    }).catch((cause: Error) => setError(cause.message));
  }, [api, projectId]);

  const save = async () => {
    if (!form) return;
    setPending(true); setError("");
    try {
      const write: AISettingsWrite = {
        judgment: { ...form.judgment, ...(judgmentKey ? { apiKey: judgmentKey } : {}) },
        embeddings: { ...form.embeddings, dimensions: 1024, ...(embeddingKey ? { apiKey: embeddingKey } : {}) },
      };
      const updated = await api.putAISettings(projectId, write);
      setSettings(updated); setJudgmentKey(""); setEmbeddingKey("");
    } catch (cause) { setError((cause as Error).message); }
    finally { setPending(false); }
  };

  if (!form) return <section className="intelligence-settings" aria-label="Intelligence"><h2>Intelligence</h2>{error ? <p className="form-error" role="alert">{error}</p> : <p className="field-note">Loading provider settings…</p>}</section>;
  return <section className="intelligence-settings" aria-label="Intelligence">
    <h2>Intelligence</h2>
    <div className="provider-settings">
      <h3>Judgment</h3>
      <label className="field"><span>Provider</span><select value={form.judgment.provider} onChange={(event) => setForm({ ...form, judgment: { ...form.judgment, provider: event.target.value as AISettingsWrite["judgment"]["provider"] } })}><option value="anthropic">Anthropic</option><option value="openai-compatible">OpenAI-compatible</option><option value="none">None</option></select></label>
      <label className="field"><span>Model</span><input value={form.judgment.model} maxLength={120} onChange={(event) => setForm({ ...form, judgment: { ...form.judgment, model: event.target.value } })} /></label>
      <label className="field"><span>Base URL</span><input value={form.judgment.baseUrl ?? ""} placeholder="Provider default" onChange={(event) => setForm({ ...form, judgment: { ...form.judgment, baseUrl: event.target.value || undefined } })} /></label>
      <label className="field"><span>API key</span><input type="password" value={judgmentKey} autoComplete="new-password" placeholder={settings?.judgment.keyHint ?? "Write only"} onChange={(event) => setJudgmentKey(event.target.value)} /></label>
    </div>
    <div className="provider-settings">
      <h3>Embeddings</h3>
      <label className="field"><span>Provider</span><select value={form.embeddings.provider} onChange={(event) => setForm({ ...form, embeddings: { ...form.embeddings, provider: event.target.value as AISettingsWrite["embeddings"]["provider"] } })}><option value="openai">OpenAI</option><option value="deterministic">Deterministic</option></select></label>
      <label className="field"><span>Model</span><input value={form.embeddings.model} maxLength={120} onChange={(event) => setForm({ ...form, embeddings: { ...form.embeddings, model: event.target.value } })} /></label>
      <label className="field"><span>Base URL</span><input value={form.embeddings.baseUrl ?? ""} placeholder="Provider default" onChange={(event) => setForm({ ...form, embeddings: { ...form.embeddings, baseUrl: event.target.value || undefined } })} /></label>
      <label className="field"><span>API key</span><input type="password" value={embeddingKey} autoComplete="new-password" placeholder={settings?.embeddings.keyHint ?? "Write only"} onChange={(event) => setEmbeddingKey(event.target.value)} /></label>
    </div>
    {settings && <p className="intelligence-effective">judgment={settings.effective.judgment} embeddings={settings.effective.embeddings}</p>}
    {error && <p className="form-error" role="alert">{error}</p>}
    <button className="pill" disabled={pending} onClick={() => void save()}>{pending ? "Saving…" : "Save intelligence settings"}</button>
  </section>;
}
