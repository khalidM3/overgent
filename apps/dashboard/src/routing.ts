/**
 * Which top-level screen a URL asks for.
 *
 * This lived inline in the bootstrap at the bottom of main.tsx, where it could
 * not be tested and where one of its conditions was wrong: `landing` was
 * decided with `isDesktopWebview`, which is only true for pages the desktop
 * shell serves itself. The live Project view is not one of those - the window
 * navigates to whichever origin serves the dashboard - so opening a Project
 * rendered the public marketing page instead of the workroom (pre-existing
 * since b847761; the production `wails://` origin happened to be unaffected,
 * which is why it survived).
 *
 * Two things fix it, and both are stated here rather than implied:
 * `isDesktopShell` is the question actually being asked ("is this page inside
 * the Overgent window", on any origin), and `?live=1` is an explicit request
 * for the live view that no other consideration may override.
 */
export type DashboardRoute = "join" | "onboarding" | "fixtures" | "landing" | "live";

export interface RouteInputs {
  pathname: string;
  search: string;
  /** This page is running inside the Overgent desktop window, on any origin. */
  desktopShell: boolean;
  /** This page is served by the desktop shell's own asset handler. */
  desktopWebview: boolean;
}

export interface RouteDecision {
  route: DashboardRoute;
  /** Whether to show the "this is the desktop preview / fixture data" banner. */
  banner: "none" | "preview-live" | "preview-fixtures" | "fixtures";
}

export function decideRoute({ pathname, search, desktopShell, desktopWebview }: RouteInputs): RouteDecision {
  const parameters = new URLSearchParams(search);
  const live = parameters.get("live") === "1";
  const desktopPreview = parameters.get("desktop") === "preview" || desktopWebview;
  const onboarding = parameters.get("desktop") === "onboarding";
  // Fixtures are a design harness, so they are opt-in and always labelled.
  const fixtures = parameters.get("fixtures") === "1" || (desktopPreview && !live);
  const normalised = pathname.replace(/\/+$/, "") || "/";
  // The public marketing page belongs to the hosted origin and to a browser.
  // Anything inside the Overgent window, anything asking for a harness, and
  // anything asking for the live view belongs in the app.
  const landing = normalised === "/" && !desktopShell && !desktopPreview && !onboarding && !fixtures && !live;

  const banner: RouteDecision["banner"] = onboarding
    ? "none"
    : desktopPreview
      ? (fixtures ? "preview-fixtures" : "preview-live")
      : fixtures
        ? "fixtures"
        : "none";

  if (normalised === "/join") return { route: "join", banner };
  if (onboarding) return { route: "onboarding", banner: "none" };
  if (fixtures) return { route: "fixtures", banner };
  if (landing) return { route: "landing", banner };
  return { route: "live", banner };
}
