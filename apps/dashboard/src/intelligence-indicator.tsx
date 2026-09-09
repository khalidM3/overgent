import { useEffect, useRef, useState } from "react";
import { AlertTriangle } from "lucide-react";
import type { FixtureProjectSource } from "./fixture-source";
import type { ProjectIntelligence } from "./model";

/**
 * A Project-scoped readout of the detection pipeline.
 *
 * The bar reports capabilities, not a score: core detection is always present,
 * embeddings add the second segment, and model judgment adds the third. The
 * active segments share one colour so the bar reads as a single level rather
 * than three unrelated statuses. Provider degradation is called out separately
 * because configured depth and current health are different facts.
 */
export function IntelligenceIndicator({ projectId, source, onConfigure }: {
  projectId: string;
  source: FixtureProjectSource;
  onConfigure: () => void;
}) {
  const [intelligence, setIntelligence] = useState<ProjectIntelligence | null>(() => source.get(projectId).project.intelligence ?? null);
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let active = true;
    setIntelligence(source.get(projectId).project.intelligence ?? null);
    void source.getProjectIntelligence(projectId).then((value) => { if (active) setIntelligence(value); }).catch(() => {
      if (!active) return;
      // A settings read failing must not erase the capabilities that are
      // guaranteed locally. Keep the honest baseline visible and mark provider
      // health separately instead of leaving the toolbar stuck in loading.
      const project = source.get(projectId).project;
      setIntelligence({ structural: "active", embeddings: "built_in", judgment: "off", degraded: project.semanticStatus === "degraded" });
    });
    return () => { active = false; };
  }, [projectId, source]);

  useEffect(() => {
    if (!open) return;
    const closeOutside = (event: PointerEvent) => { if (!root.current?.contains(event.target as Node)) setOpen(false); };
    const closeEscape = (event: KeyboardEvent) => { if (event.key === "Escape") setOpen(false); };
    document.addEventListener("pointerdown", closeOutside);
    window.addEventListener("keydown", closeEscape);
    return () => {
      document.removeEventListener("pointerdown", closeOutside);
      window.removeEventListener("keydown", closeEscape);
    };
  }, [open]);

  const level = intelligence ? 1 + Number(intelligence.embeddings !== "off") + Number(intelligence.judgment === "active") : null;
  const accessibleLabel = level === null ? "Loading intelligence layers" : `Intelligence layer ${level} of 3`;

  return <div className="intelligence-control" ref={root}>
    <button className={`intelligence-trigger ${level === null ? "loading" : `level-${level}`}`} aria-label={accessibleLabel} aria-expanded={open} aria-controls="intelligence-level-detail" onClick={() => setOpen((value) => !value)}>
      <span className="intelligence-trigger-head"><strong>Intelligence</strong>{level !== null && <span>{level} of 3</span>}</span>
      <Meter level={level} />
    </button>

    {open && <section className="intelligence-popover" id="intelligence-level-detail" aria-label="Intelligence layer details">
      <header><strong>Intelligence layers</strong>{level !== null && <span>{level} OF 3 ACTIVE</span>}</header>
      {intelligence?.degraded && <p className="intelligence-degraded"><AlertTriangle size={13} />Provider-backed processing is unavailable. Core detection remains active.</p>}
      <Layer number={1} title="Core detection" state="On" active description="Git paths, symbols, changed contracts, and exact matching. No key needed." />
      <Layer number={2} title="Embedding depth" state={intelligence?.embeddings === "provider" ? "Enhanced" : intelligence?.embeddings === "built_in" ? "Built in" : intelligence ? "Not set" : "Loading"} active={intelligence?.embeddings !== "off" && Boolean(intelligence)} description="Finds conceptual overlap across different files and wording." />
      <Layer number={3} title="Model judgment" state={intelligence?.judgment === "active" ? "On" : intelligence ? "Not set" : "Loading"} active={intelligence?.judgment === "active"} description="Classifies meaning, certainty, severity, and when to interrupt." />
      <button className="intelligence-configure" onClick={onConfigure}>{level === 3 ? "Configure intelligence" : "Improve intelligence"}<span aria-hidden="true"> →</span></button>
    </section>}
  </div>;
}

function Meter({ level }: { level: number | null }) {
  if (level === null) return <span className="intelligence-meter-loading" role="status" aria-label="Loading intelligence layers"><i /></span>;
  return <span className={`intelligence-meter level-${level}`} role="img" aria-label={`${level} of 3 intelligence layers active`}>
    {[1, 2, 3].map((segment) => <i className={segment <= level ? "on" : ""} key={segment} />)}
  </span>;
}

function Layer({ number, title, state, active, description }: { number: number; title: string; state: string; active?: boolean; description: string }) {
  return <div className="intelligence-layer">
    <span className="intelligence-layer-number" aria-hidden="true">L{number}</span>
    <span><strong>{title}</strong><small>{description}</small></span>
    <span className={active ? "intelligence-layer-state active" : "intelligence-layer-state"}>{state}</span>
  </div>;
}
