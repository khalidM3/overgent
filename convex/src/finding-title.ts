/**
 * The one sentence a finding leads with.
 *
 * The dashboard used to title findings with the finding kind itself, so a card
 * read "stale assumption" and the member had to understand Stickguy's own
 * vocabulary before they could understand their repository. A title should say
 * what happened to whom; the kind, the evidence, and the confidence band are
 * the drill-down.
 *
 * Deterministic on purpose. Core behavior has to work with AI disabled, and a
 * finding that cannot be named offline is a finding that cannot be shown
 * offline. The managed judgment layer already writes the explanation underneath
 * this line, which is where a model's wording earns its cost.
 */

export type FindingTitleKind =
  | "direct_collision"
  | "likely_collision"
  | "redundant_work"
  | "shared_dependency"
  | "assumption_conflict"
  | "downstream_impact"
  | "stale_assumption"
  | "dependency_ready";

export interface FindingTitleInput {
  kind: FindingTitleKind;
  /** Display names of the sessions the finding is routed to, in finding order. */
  actors: string[];
  /**
   * The other party, when the finding names only one side. A stale assumption
   * is routed to the session that read a contract, but the sentence is only
   * useful if it also says who moved it.
   */
  counterpart?: string;
  /** The symbol, path, dependency, or component the finding is about. */
  subject?: string;
}

/** Matches the dashboard contract's title bound. */
const MAX_TITLE = 160;

function bounded(text: string): string {
  const collapsed = text.replace(/\s+/g, " ").trim();
  return collapsed.length <= MAX_TITLE ? collapsed : `${collapsed.slice(0, MAX_TITLE - 1).trimEnd()}…`;
}

function clean(value: string | undefined): string | undefined {
  const trimmed = value?.trim();
  return trimmed ? trimmed : undefined;
}

/**
 * How to name the parties in a sentence about two or more of them.
 *
 * One member running two agents in one repository is the product's first case,
 * not an edge case (ADR-054), and there both sides carry the same name. Listing
 * that name twice reads as a typo and listing it once reads as one session
 * colliding with itself, so it becomes "Two of Khalid's sessions".
 */
function party(names: string[]): { text: string; plural: boolean } {
  const cleaned = names.map((name) => name.trim()).filter(Boolean);
  const unique = [...new Set(cleaned)];
  if (unique.length === 0) return { text: "Two sessions", plural: true };
  if (unique.length === 1) {
    return cleaned.length > 1
      ? { text: `Two of ${unique[0]}'s sessions`, plural: true }
      : { text: unique[0]!, plural: false };
  }
  if (unique.length === 2) return { text: `${unique[0]} and ${unique[1]}`, plural: true };
  return { text: `${unique.slice(0, -1).join(", ")} and ${unique[unique.length - 1]}`, plural: true };
}

/**
 * Two sides named separately, for the sentences where the roles differ. Falls
 * back to a role noun rather than inventing a name, because a wrong name is
 * worse than an unnamed party.
 */
function sides(actors: string[], counterpart: string | undefined): { first: string; second: string } {
  const named = [...new Set(actors.map((name) => name.trim()).filter(Boolean))];
  const other = clean(counterpart);
  const first = named[0] ?? "One session";
  const second = other && other !== first ? other : named[1] && named[1] !== first ? named[1]! : "another session";
  return { first, second };
}

export function findingTitle(input: FindingTitleInput): string {
  const subject = clean(input.subject);
  const group = party(input.actors);
  // "Both" is only true of exactly two parties; three sessions sharing a
  // dependency all share it.
  const uniqueActors = new Set(input.actors.map((name) => name.trim()).filter(Boolean)).size;
  const both = uniqueActors > 2 ? "all" : "both";
  const { first, second } = sides(input.actors, input.counterpart);
  // Several findings are routed to only the session that has to act, so the
  // sentence still has two sides even though the finding names one. Without
  // this a single-session finding read "Khalid both depend on the schema".
  const everyone = group.plural ? group.text : `${first} and ${second}`;

  switch (input.kind) {
    case "stale_assumption":
      // The reader is the one who needs this, so the reader is the subject of
      // the sentence and the change is what happened to them.
      return bounded(subject
        ? `${first} is building on a version of ${subject} that ${second} already changed`
        : `${first} is building on something ${second} already changed`);

    case "direct_collision":
      return bounded(subject
        ? `${everyone} are ${both} changing ${subject}`
        : `${everyone} are changing the same file`);

    case "likely_collision":
      return bounded(subject
        ? `${everyone} are ${both} working inside ${subject}`
        : `${everyone} are working in the same area`);

    case "redundant_work":
      return bounded(subject
        ? `${everyone} may ${both} be building ${subject}`
        : `${everyone} may be building the same thing`);

    case "shared_dependency":
      return bounded(subject
        ? `${everyone} ${both} depend on ${subject}`
        : `${everyone} depend on the same work`);

    case "assumption_conflict":
      return bounded(subject
        ? `${everyone} disagree about how ${subject} behaves`
        : `${everyone} are working from incompatible assumptions`);

    case "downstream_impact":
      return bounded(subject
        ? `${first}'s change to ${subject} affects ${second}`
        : `${first}'s change affects ${second}`);

    case "dependency_ready":
      // The only finding that is good news, and it reads like it.
      return bounded(subject
        ? `${subject}, which ${first} was waiting on, is ready`
        : `Work ${first} was waiting on is ready`);
  }
}
