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
  /**
   * This bundle was built by the development harness rather than shipped.
   *
   * The preview banner labels the harness, and being served by the shell's own
   * asset handler used to be evidence of that. It stopped being evidence once
   * opening a Project became a route on that same handler: an installed,
   * production-tagged build then showed "Overgent Dev · Local live Project
   * data" across the top of a member's real Project. The build flag is the only
   * thing that actually answers the question, so it is asked directly.
   */
  development?: boolean;
}

export interface RouteDecision {
  route: DashboardRoute;
  /** Whether to show the "this is the desktop preview / fixture data" banner. */
  banner: "none" | "preview-live" | "preview-fixtures" | "fixtures";
}

export function decideRoute({ pathname, search, desktopShell, desktopWebview, development = false }: RouteInputs): RouteDecision {
  const parameters = new URLSearchParams(search);
  const live = parameters.get("live") === "1";
  const desktopPreview = parameters.get("desktop") === "preview" || (desktopWebview && development);
  const onboarding = parameters.get("desktop") === "onboarding";
  // Fixtures are a design harness, so they are opt-in and always labelled.
  const fixtures = parameters.get("fixtures") === "1" || (parameters.get("desktop") === "preview" && !live);
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
  if (onboarding || (desktopWebview && !live && !fixtures)) return { route: "onboarding", banner: "none" };
  if (fixtures) return { route: "fixtures", banner };
  if (landing) return { route: "landing", banner };
  return { route: "live", banner };
}
