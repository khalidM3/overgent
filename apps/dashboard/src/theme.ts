import { useEffect, useState } from "react";

/**
 * Which appearance this Mac is set to.
 *
 * The stored value used to be a boolean, which could express "light" and "dark"
 * and had no way to say "whatever the Mac is set to" — so a member on a machine
 * that switches at sunset had to switch Overgent by hand, twice a day. "system"
 * is the default for anyone who has never chosen, because following macOS is
 * what an app that has not been told anything should do.
 *
 * The two explicit values are the ones the old boolean wrote, so a member who
 * had already chosen keeps their choice.
 */
export type ThemeChoice = "light" | "dark" | "system";

const key = "overgent.theme";
const query = "(prefers-color-scheme: dark)";

function stored(): ThemeChoice {
  try {
    const value = localStorage.getItem(key);
    return value === "dark" || value === "light" || value === "system" ? value : "system";
  } catch { return "system"; }
}

/** jsdom and locked-down webviews may have neither API; neither is a reason to
 *  fail to render, so both are treated as "not dark". */
function systemIsDark(): boolean {
  try { return typeof window.matchMedia === "function" && window.matchMedia(query).matches; } catch { return false; }
}

export function resolveTheme(choice: ThemeChoice): boolean {
  return choice === "dark" || (choice === "system" && systemIsDark());
}

/**
 * The appearance the whole app reads from. It stamps `data-theme` with the
 * resolved value rather than removing the attribute for "system", so every
 * other surface — including one rendered inside the desktop shell — can answer
 * "is this dark right now" without repeating the media query.
 */
export function useTheme(): { choice: ThemeChoice; setChoice: (next: ThemeChoice) => void; dark: boolean } {
  const [choice, setChoice] = useState<ThemeChoice>(stored);
  const [systemDark, setSystemDark] = useState(systemIsDark);
  useEffect(() => {
    if (typeof window.matchMedia !== "function") return;
    const media = window.matchMedia(query);
    const listen = () => setSystemDark(media.matches);
    media.addEventListener?.("change", listen);
    return () => media.removeEventListener?.("change", listen);
  }, []);
  const dark = choice === "dark" || (choice === "system" && systemDark);
  useEffect(() => {
    document.documentElement.dataset.theme = dark ? "dark" : "light";
    try { localStorage.setItem(key, choice); } catch { /* Optional local preference. */ }
  }, [choice, dark]);
  return { choice, setChoice, dark };
}
