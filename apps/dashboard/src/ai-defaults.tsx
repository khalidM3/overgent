import { useEffect, useState } from "react";
import { ScreenSection } from "./screen";
import { IntelligenceForm, type IntelligenceWrite } from "./intelligence-form";
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
 *
 * The fields themselves are `IntelligenceForm`, shared with that screen. Only
 * the sentences about destination differ, because only the destination does.
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

  const change = (value: IntelligenceWrite) => { setForm(value); setSaved(false); };
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

  return <ScreenSection
    title="Defaults for new Projects"
    help="Set intelligence once and every new Project on this Mac starts here. Keys are stored in your login Keychain, never in a file Overgent writes."
  >
    <p className="settings-help sharp">A Project on this Mac takes these automatically. A shared Project does not: saving a key there uploads it to that Project’s server, so it is offered on the Project’s own settings screen instead of applied for you.</p>
    <IntelligenceForm
      value={form}
      onChange={change}
      judgmentKey={{ stored: Boolean(defaults?.judgment.keyStored), provider: defaults?.judgment.provider, baseUrl: defaults?.judgment.baseUrl }}
      embeddingKey={{ stored: Boolean(defaults?.embeddings.keyStored), provider: defaults?.embeddings.provider, baseUrl: defaults?.embeddings.baseUrl }}
      keyLocation="in this Mac’s Keychain"
      saveLabel="Save defaults"
      pending={pending}
      onSave={() => void save()}
      footer={<p className="field-note">Projects you already have keep the settings they have. These apply to the next one you open.</p>}
      status={<>
        {saved && <p role="status" className="settings-help">Defaults saved.</p>}
        {error && <p className="form-error" role="alert">{error}</p>}
      </>}
    />
  </ScreenSection>;
}
