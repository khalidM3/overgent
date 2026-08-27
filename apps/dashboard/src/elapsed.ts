/**
 * One elapsed format across every dashboard surface: "47s", "12m 16s", "1h 04m".
 *
 * Never mm:ss. A colon reads as a wall-clock time, so "12:07" is understood as
 * an hour of the day rather than as time since something happened. Spelling the
 * unit out removes the ambiguity, and because the value ticks, the movement
 * itself tells the reader it is a duration.
 */
export function formatElapsed(totalSeconds: number): string {
  const seconds = Math.max(0, Math.floor(totalSeconds));
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3_600) return `${Math.floor(seconds / 60)}m ${String(seconds % 60).padStart(2, "0")}s`;
  return `${Math.floor(seconds / 3_600)}h ${String(Math.floor((seconds % 3_600) / 60)).padStart(2, "0")}m`;
}

/**
 * The service still reports prose labels ("Now", "8 min", "1 hr") rather than a
 * timestamp. Parse what we can so the clock counts from a truthful starting
 * point; an unparseable label falls back to being rendered verbatim.
 */
export function parseElapsedLabel(label: string): number | null {
  const value = label.trim().toLowerCase();
  if (!value) return null;
  if (value === "now" || value === "just now") return 0;
  const match = /(\d[\d,]*)\s*(seconds?|secs?|s|minutes?|mins?|m|hours?|hrs?|h|days?|d)\b/.exec(value);
  if (!match) return null;
  const amount = Number(match[1].replaceAll(",", ""));
  if (!Number.isFinite(amount)) return null;
  const unit = match[2];
  if (unit.startsWith("s")) return amount;
  if (unit.startsWith("m")) return amount * 60;
  if (unit.startsWith("h")) return amount * 3_600;
  return amount * 86_400;
}

/** Elapsed seconds for anything the service has labelled, or null if unknown. */
export function elapsedFromLabel(label: string | undefined): number | null {
  return label ? parseElapsedLabel(label) : null;
}
