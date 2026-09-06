import { useState } from "react";
import type { ReactNode } from "react";
import { EMBEDDING_PRESETS, JUDGMENT_PRESETS, PRESETS_CHECKED, endpointFor, presetFor, type ProviderPreset } from "./intelligence-catalog";
import type { AIDefaultsWrite, AISettingsWrite } from "./native";

/**
 * The one intelligence form, used by both screens that configure a provider.
 *
 * ## Why it reads as three levels
 *
 * The first version of this screen was a stack of labelled inputs — "Assess
 * coordination", "Judgment provider", "Judgment model" — which named the
 * codebase's internals and left the member to work out what any of it caught.
 * Nobody arrives here wanting to configure a judgment provider. They arrive
 * wanting to know what Overgent notices and what it costs to notice more.
 *
 * So the form is the detection pipeline, in the order it actually runs
 * (`coordination-intelligence.md` §4): structural evidence, then related work,
 * then judgment. Level one is stated and has no controls, because it always
 * runs and configuring it is not a thing anyone can do. Levels two and three
 * carry one picker each and open into fields only once they are switched on,
 * so a member who wants none of it reads three sentences and leaves.
 *
 * Being honest about level one is what makes the other two optional rather
 * than load-bearing: same-file collisions and contract drift are computed on
 * this Mac from Git and do not depend on any of this.
 *
 * ## The safety properties, which must not have two implementations
 *
 *   - **The three-way key contract.** An absent `apiKey` leaves the stored key
 *     alone, `removeKey` deletes it, and an empty string is never sent. A saved
 *     key never comes back from the service, so this form can only ever report
 *     a flag and a four-character hint.
 *   - **A key belongs to one destination.** Changing provider or address drops
 *     it, because a key for Anthropic is not a key for a server on port 11434,
 *     and silently carrying it over would send a live credential to an address
 *     the member did not save it for.
 *
 * What stays with each screen is where the key ends up, and therefore what the
 * member is agreeing to. That sentence is ADR-077's substance and it is not
 * shared, because the two answers are not the same sentence.
 */
export type IntelligenceWrite = AISettingsWrite;

/** The two write shapes are structurally identical and both screens depend on
 *  that. If either grows a field the other lacks, this stops compiling here
 *  rather than at some later call site. */
export type IntelligenceWriteShapesAgree = [
  AIDefaultsWrite extends IntelligenceWrite ? true : never,
  IntelligenceWrite extends AIDefaultsWrite ? true : never,
];

/**
 * What the service will say about a key: that one exists, its last four
 * characters, and the destination it was saved against. Never the key.
 *
 * The destination is here because it decides whether removing the key is
 * something the member chose or something their last choice implied.
 */
export type KeyState = { stored: boolean; hint?: string | null; provider?: string; baseUrl?: string | null };

export function IntelligenceForm({
  value, onChange, judgmentKey, embeddingKey, keyLocation, savedEmbeddingModel, disabled = false,
  footer, hold = null, saveLabel, pending, onSave, status,
}: {
  value: IntelligenceWrite;
  onChange: (next: IntelligenceWrite) => void;
  judgmentKey: KeyState;
  embeddingKey: KeyState;
  /** Where a saved key lives, as a phrase: "in this Mac's Keychain". */
  keyLocation: string;
  /** The embedding model already in force, when there is a corpus embedded with
   *  it. Only a Project has one; defaults have nothing to migrate. */
  savedEmbeddingModel?: string;
  disabled?: boolean;
  footer?: ReactNode;
  /** A reason the caller is holding the save, in words. */
  hold?: string | null;
  saveLabel: string;
  pending: boolean;
  onSave: () => void;
  status?: ReactNode;
}) {
  // A custom address is empty for exactly as long as it takes to type one, and
  // an empty address resolves back to the provider's own default - so the
  // choice is remembered here rather than re-derived, or the picker would snap
  // back to OpenAI under the member's cursor.
  const [judgmentCustom, setJudgmentCustom] = useState(false);
  const [embeddingCustom, setEmbeddingCustom] = useState(false);

  const judgmentPreset = pick(JUDGMENT_PRESETS, value.judgment.provider, value.judgment.baseUrl, judgmentCustom);
  const embeddingPreset = pick(EMBEDDING_PRESETS, value.embeddings.provider, value.embeddings.baseUrl, embeddingCustom);
  const judgmentOn = value.judgment.provider !== "none";
  const embeddingsOn = value.embeddings.provider === "openai";

  const chooseJudgment = (preset: ProviderPreset<IntelligenceWrite["judgment"]["provider"]>) => {
    setJudgmentCustom(Boolean(preset.custom));
    // The model is cleared rather than carried: an ID that means something at
    // one provider is a rejected request at the next.
    onChange({ ...value, judgment: { provider: preset.provider, model: "", ...(preset.baseUrl ? { baseUrl: preset.baseUrl } : {}), removeKey: true } });
  };
  const chooseEmbedding = (preset: ProviderPreset<IntelligenceWrite["embeddings"]["provider"]>) => {
    setEmbeddingCustom(Boolean(preset.custom));
    onChange({ ...value, embeddings: { provider: preset.provider, model: "", dimensions: 1024, ...(preset.baseUrl ? { baseUrl: preset.baseUrl } : {}), removeKey: true } });
  };

  const missing = incomplete(value, judgmentPreset, embeddingPreset);
  const blocked = missing ?? hold;

  return <fieldset disabled={disabled || pending} className="provider-fields">
    <ol className="levels">
      {/* Level one has no controls and is not a lesser version of the two
          below it: it is the part that always runs, and saying so is what
          makes the rest optional rather than load-bearing. */}
      <Level number={1} title="Overlapping code" state="Always on">
        <p className="level-note">
          Two sessions in the same file or the same symbol, and a contract that changed under a session after it read it.
          Computed from Git without a model — no provider and no key, and no source code or diffs are ever part of it: only paths, symbol names and hashes.
        </p>
      </Level>

      <Level number={2} title="Related work" state={embeddingsOn ? embeddingPreset.label : "Built-in"}>
        <p className="level-note">
          Work that overlaps in meaning without overlapping in files — the same capability built twice, a plan that contradicts another.
          Built-in matching compares text on the machine that stores it. An embedding provider finds looser matches, and reads the summaries to do it.
        </p>
        <div className="level-controls">
          <Picker label="Embedding provider" presets={EMBEDDING_PRESETS} preset={embeddingPreset} onPick={chooseEmbedding} />
          {embeddingsOn && <>
            {embeddingPreset.custom && <Address label="Embedding server address" value={value.embeddings.baseUrl ?? ""}
              onChange={(next) => onChange({ ...value, embeddings: { ...value.embeddings, baseUrl: next || undefined, apiKey: undefined, removeKey: true } })} />}
            <ModelField label="Embedding model" preset={embeddingPreset} value={value.embeddings.model}
              onChange={(next) => onChange({ ...value, embeddings: { ...value.embeddings, model: next } })} />
            <KeyField label="Embedding API key" state={embeddingKey} removing={Boolean(value.embeddings.removeKey)} value={value.embeddings.apiKey ?? ""}
              onChange={(next) => onChange({ ...value, embeddings: { ...value.embeddings, apiKey: next || undefined, removeKey: false } })} />
            <KeyLine state={embeddingKey} removing={Boolean(value.embeddings.removeKey)} location={keyLocation}
              retargeted={retargeted("embeddings", embeddingKey, value.embeddings.provider, value.embeddings.baseUrl)}
              onToggle={() => onChange({ ...value, embeddings: { ...value.embeddings, apiKey: undefined, removeKey: !value.embeddings.removeKey } })} />
            {!(embeddingPreset.custom && !value.embeddings.baseUrl?.trim()) && <Endpoint url={endpointFor("embeddings", value.embeddings.provider, value.embeddings.baseUrl)} />}
            {/* Not a preference: the store holds 1024-wide vectors, and a model
                of any other width is rejected by the client, not truncated. */}
            <p className="field-note">Overgent asks for 1024 numbers per item. OpenAI’s v3 models shorten to that on request; a model with a different fixed width cannot be used.</p>
            {savedEmbeddingModel && value.embeddings.model.trim() && value.embeddings.model.trim() !== savedEmbeddingModel && <p className="field-note">
              Changing the model re-embeds everything already indexed. Each item stops matching searches until its turn comes round, and the work happens in the background.
            </p>}
          </>}
        </div>
      </Level>

      <Level number={3} title="Judgment" state={judgmentOn ? judgmentPreset.label : "Off"}>
        <p className="level-note">
          A model reads the two summaries and decides what the candidate means: whether it is a real collision, how certain that is, and whether it should interrupt
          you now or wait in the dashboard. It is sent bounded summaries — never source, diffs, or file contents.
          With it off, findings still appear from level one and the workroom says judgment is not running.
        </p>
        <div className="level-controls">
          <Picker label="Model provider" presets={JUDGMENT_PRESETS} preset={judgmentPreset} onPick={chooseJudgment} />
          {judgmentOn && <>
            {judgmentPreset.custom && <Address label="Server address" value={value.judgment.baseUrl ?? ""}
              onChange={(next) => onChange({ ...value, judgment: { ...value.judgment, baseUrl: next || undefined, apiKey: undefined, removeKey: true } })} />}
            <ModelField label="Model" preset={judgmentPreset} value={value.judgment.model}
              onChange={(next) => onChange({ ...value, judgment: { ...value.judgment, model: next } })} />
            <KeyField label="API key" state={judgmentKey} removing={Boolean(value.judgment.removeKey)} value={value.judgment.apiKey ?? ""}
              onChange={(next) => onChange({ ...value, judgment: { ...value.judgment, apiKey: next || undefined, removeKey: false } })} />
            <KeyLine state={judgmentKey} removing={Boolean(value.judgment.removeKey)} location={keyLocation}
              retargeted={retargeted("judgment", judgmentKey, value.judgment.provider, value.judgment.baseUrl)}
              onToggle={() => onChange({ ...value, judgment: { ...value.judgment, apiKey: undefined, removeKey: !value.judgment.removeKey } })} />
            {!(judgmentPreset.custom && !value.judgment.baseUrl?.trim()) && <Endpoint url={endpointFor("judgment", value.judgment.provider, value.judgment.baseUrl)} />}
          </>}
        </div>
      </Level>
    </ol>

    {footer}
    <div className="provider-save">
      <button className="pill solid" disabled={Boolean(blocked)} onClick={onSave}>{pending ? "Saving…" : saveLabel}</button>
      {/* A control that cannot be pressed says why, beside itself, rather than
          leaving the member to guess which field it is waiting on. */}
      {blocked && <p className="field-note">{blocked}</p>}
    </div>
    {status}
  </fieldset>;
}

/** One rung: a number in the gutter, a title, what it is set to, and its own
 *  controls indented under it. The number is the pipeline's order, not a
 *  ranking of importance. */
function Level({ number, title, state, children }: { number: number; title: string; state: string; children: ReactNode }) {
  return <li className="level">
    <span className="level-mark" aria-hidden="true">{number}</span>
    <div className="level-body">
      <div className="level-head"><h3>{title}</h3><span className="level-state">{state}</span></div>
      {children}
    </div>
  </li>;
}

function Picker<Provider extends string>({ label, presets, preset, onPick }: {
  label: string;
  presets: readonly ProviderPreset<Provider>[];
  preset: ProviderPreset<Provider>;
  onPick: (next: ProviderPreset<Provider>) => void;
}) {
  return <>
    <label className="field"><span>{label}</span>
      <select value={preset.id} onChange={(event) => onPick(presets.find((entry) => entry.id === event.target.value) ?? presets[0]!)}>
        {presets.map((entry) => <option key={entry.id} value={entry.id}>{entry.label}</option>)}
      </select>
    </label>
    {preset.note && <p className="field-note">{preset.note}</p>}
  </>;
}

function Address({ label, value, onChange }: { label: string; value: string; onChange: (next: string) => void }) {
  return <label className="field"><span>{label}</span>
    <input value={value} placeholder="https://llm.example.internal" spellCheck={false} onChange={(event) => onChange(event.target.value)} />
  </label>;
}

/**
 * A model is chosen from a list, and typed only when it is not on one.
 *
 * A bare text input asked the member to remember an ID exactly, and got a
 * rejected request when they did not. The list is what each provider's
 * documentation named on the date in `PRESETS_CHECKED`, and it is never the
 * whole truth for long — so "Other model…" is a first-class choice rather than
 * an escape hatch, and a saved ID the list has never heard of opens the field
 * already typed in rather than being silently dropped.
 */
function ModelField({ label, preset, value, onChange }: { label: string; preset: ProviderPreset<string>; value: string; onChange: (next: string) => void }) {
  const [typing, setTyping] = useState(false);
  const known = preset.models.includes(value.trim());
  const free = preset.models.length === 0 || typing || (!known && value.trim() !== "");
  const name = preset.short ?? preset.label;
  return <>
    {free
      ? <label className="field"><span>{label}</span>
          <input required value={value} maxLength={120} spellCheck={false} autoComplete="off" placeholder={preset.models[0] ?? "Model ID from your provider"}
            onChange={(event) => onChange(event.target.value)} />
        </label>
      : <label className="field"><span>{label}</span>
          <select value={known ? value.trim() : ""} onChange={(event) => {
            if (event.target.value === "__other") { setTyping(true); onChange(""); return; }
            onChange(event.target.value);
          }}>
            <option value="" disabled>Choose a model</option>
            {preset.models.map((model) => <option key={model} value={model}>{model}</option>)}
            <option value="__other">Other model…</option>
          </select>
        </label>}
    <p className="field-note">
      {preset.models.length > 0
        ? <>Checked {PRESETS_CHECKED}; {name}’s own list is the current one. </>
        : <>Any model ID {name} accepts. </>}
      {free && preset.models.length > 0 && <button className="text-button inline" onClick={() => { setTyping(false); onChange(""); }}>Back to the list</button>}
    </p>
  </>;
}

function KeyField({ label, state, removing, value, onChange }: { label: string; state: KeyState; removing: boolean; value: string; onChange: (next: string) => void }) {
  return <label className="field"><span>{label}</span>
    <input type="password" autoComplete="new-password" spellCheck={false} value={value}
      placeholder={state.stored && !removing ? "Key saved · leave blank to keep" : "Enter an API key"}
      onChange={(event) => onChange(event.target.value)} />
  </label>;
}

/** Where the key is spent, spelled out. A URL is a machine fact, so it is
 *  monospace: it earns its place by settling whether the member meant this
 *  provider, not by being loud. */
function Endpoint({ url }: { url: string }) {
  return <p className="provider-endpoint"><span>Sent to </span>{url}</p>;
}

/**
 * What is known about a stored key, and the one action available on it.
 *
 * The remove control used to change its own label to "Key will be removed on
 * save", which named a state rather than an action and left no way back except
 * reloading the screen. The state is a sentence now and the button stays a
 * button, so a mis-click is one click to undo.
 *
 * There is one case with no undo, and it must not be offered one: when the
 * member has pointed this level at a different provider or address, the saved
 * key is being dropped *because* of that, and a control offering to keep it
 * would send a key issued for one destination to another. The removal is then
 * stated as the consequence it is.
 */
function KeyLine({ state, removing, retargeted, location, onToggle }: { state: KeyState; removing: boolean; retargeted: string | null; location: string; onToggle: () => void }) {
  if (!state.stored) return null;
  if (retargeted) return <p className="key-state removing">The key saved for {retargeted} is deleted when you save: a key belongs to the address it was issued for.</p>;
  return <p className={removing ? "key-state removing" : "key-state"}>
    {removing ? "This key is deleted when you save." : <>Key saved {location}{state.hint ? <> · ends <code>{state.hint}</code></> : null}. It is never shown again.</>}
    {" "}<button className="text-button inline" onClick={onToggle}>{removing ? "Keep it" : "Remove it"}</button>
  </p>;
}

/** The host a stored key was saved against, when that is no longer where this
 *  level points. Null means the destination is unchanged. */
function retargeted(kind: "judgment" | "embeddings", state: KeyState, provider: string, baseUrl: string | undefined): string | null {
  if (!state.stored || state.provider === undefined) return null;
  const strip = (value: string | null | undefined) => (value ?? "").replace(/\/+$/, "");
  if (state.provider === provider && strip(state.baseUrl) === strip(baseUrl)) return null;
  const endpoint = endpointFor(kind, state.provider, state.baseUrl);
  try { return new URL(endpoint).host; } catch { return endpoint; }
}

/** Which entry is selected, honouring a custom choice whose address is still
 *  being typed. */
function pick<Provider extends string>(presets: readonly ProviderPreset<Provider>[], provider: Provider, baseUrl: string | undefined, custom: boolean): ProviderPreset<Provider> {
  const resolved = presetFor(presets, provider, baseUrl);
  if (!custom || resolved.custom) return resolved;
  return presets.find((preset) => preset.provider === provider && preset.custom) ?? resolved;
}

/**
 * Why the save is held, in the words of the thing that is missing.
 *
 * Each of these would otherwise be saved as a configuration that runs nothing:
 * a level switched on with no model, or a custom server with no address, which
 * would quietly resolve to OpenAI and spend the key there.
 */
function incomplete(value: IntelligenceWrite, judgment: ProviderPreset<string>, embeddings: ProviderPreset<string>): string | null {
  if (value.judgment.provider !== "none") {
    if (judgment.custom && !value.judgment.baseUrl?.trim()) return "Judgment needs the server’s address before it can be saved.";
    if (!value.judgment.model.trim()) return "Judgment needs a model before it can be saved.";
  }
  if (value.embeddings.provider === "openai") {
    if (embeddings.custom && !value.embeddings.baseUrl?.trim()) return "Related work needs the server’s address before it can be saved.";
    if (!value.embeddings.model.trim()) return "Related work needs an embedding model before it can be saved.";
  }
  return null;
}
