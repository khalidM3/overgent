import { describe, expect, it } from "vitest";
import { decideRoute } from "../src/routing";

const browser = { desktopShell: false, desktopWebview: false };
const shell = { desktopShell: true, desktopWebview: false };
const webview = { desktopShell: true, desktopWebview: true };

describe("decideRoute", () => {
  it("shows the marketing page for a plain browser hit on the hosted root", () => {
    expect(decideRoute({ pathname: "/", search: "", ...browser }).route).toBe("landing");
  });

  // The regression this file exists for: the desktop window navigates to the
  // origin serving the dashboard to open a Project, so it is no longer a page
  // the shell serves itself. Deciding with isDesktopWebview sent it to the
  // marketing page; deciding with isDesktopShell keeps it in the app.
  it("opens the live view in the desktop window on a hosted origin", () => {
    expect(decideRoute({ pathname: "/", search: "?live=1", ...shell }).route).toBe("live");
  });

  it("never shows the marketing page when the URL asks for the live view", () => {
    expect(decideRoute({ pathname: "/", search: "?live=1", ...browser }).route).toBe("live");
  });

  it("keeps the live view on the dashboard route the hosted backend redirects to", () => {
    expect(decideRoute({ pathname: "/dashboard", search: "?live=1", ...browser }).route).toBe("live");
  });

  it("renders the desktop webview preview with fixtures unless live is asked for", () => {
    expect(decideRoute({ pathname: "/", search: "", ...webview })).toEqual({ route: "fixtures", banner: "preview-fixtures" });
    expect(decideRoute({ pathname: "/", search: "?live=1", ...webview })).toEqual({ route: "live", banner: "preview-live" });
  });

  it("routes onboarding and invite links regardless of shell", () => {
    expect(decideRoute({ pathname: "/", search: "?desktop=onboarding", ...webview })).toEqual({ route: "onboarding", banner: "none" });
    expect(decideRoute({ pathname: "/join", search: "", ...browser }).route).toBe("join");
  });

  it("labels an explicit fixture harness in a plain browser", () => {
    expect(decideRoute({ pathname: "/", search: "?fixtures=1", ...browser })).toEqual({ route: "fixtures", banner: "fixtures" });
  });

  it("ignores a trailing slash", () => {
    expect(decideRoute({ pathname: "//", search: "", ...browser }).route).toBe("landing");
  });
});
