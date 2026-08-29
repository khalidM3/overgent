import { useEffect, useId, useRef } from "react";
import { ChevronLeft } from "lucide-react";
import type { ReactNode } from "react";

/**
 * The shell every full screen shares.
 *
 * Settings, People and New Project used to be modal dialogs. A modal is for a
 * decision that must be finished before returning; none of these are. They are
 * places a member goes, so they take over the main and inspector columns and
 * keep the same shape as the workroom: own toolbar, own scroll, one 680px
 * column. The sidebar stays put so leaving is never a hunt for a close button.
 *
 * Escape still goes back, because the dialogs these replaced closed that way and
 * removing the habit would be a regression even though nothing is modal now.
 */
export function Screen({ backLabel, onBack, title, sub, lede, actions, children }: {
  backLabel: string;
  onBack: () => void;
  title: string;
  sub?: ReactNode;
  lede?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
}) {
  useEscape(onBack);
  const headingId = useId();
  // The screen is the labelled region now that no dialog carries the name, so
  // assistive technology and tests can still ask for "Settings" by name.
  return <main className="screen" aria-labelledby={headingId}>
    <div className="screen-bar">
      <button className="screen-back" onClick={onBack} aria-label={`Back to ${backLabel}`}><ChevronLeft size={15} />{backLabel}</button>
      <span className="spacer" />
      {actions}
    </div>
    <div className="screen-scroll">
      <div className="screen-column">
        <header className="screen-head">
          <h1 id={headingId}>{title}</h1>
          {sub && <div className="screen-sub">{sub}</div>}
          {lede && <p className="screen-lede">{lede}</p>}
        </header>
        {children}
      </div>
    </div>
  </main>;
}

/** A titled group inside a screen. The heading is a real `h2` at the same
 *  weight as the workroom's block heads, so the two read as one system. */
export function ScreenSection({ title, count, help, danger, children }: {
  title: string;
  count?: number;
  help?: ReactNode;
  danger?: boolean;
  children: ReactNode;
}) {
  return <section className={danger ? "screen-section danger" : "screen-section"}>
    <div className="block-head"><h2>{title}</h2>{typeof count === "number" && <span className="count">{count.toLocaleString()}</span>}</div>
    {help && <p className="settings-help">{help}</p>}
    {children}
  </section>;
}

export function useEscape(onEscape: () => void): void {
  // Held in a ref so the listener is bound once rather than re-bound on every
  // render of the screen it belongs to.
  const latest = useRef(onEscape);
  latest.current = onEscape;
  useEffect(() => {
    const handleKey = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || event.defaultPrevented) return;
      // The command palette is a real modal and dismisses itself. Leaving the
      // screen underneath it alone means one Escape closes one thing.
      if (document.querySelector("dialog[open]")) return;
      latest.current();
    };
    window.addEventListener("keydown", handleKey);
    return () => window.removeEventListener("keydown", handleKey);
  }, []);
}
