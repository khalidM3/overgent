# Overgent desktop beta

This Apple Silicon macOS beta packages the shared React dashboard in an embedded Wails
webview and adds a persistent menu-bar client for local service health,
pause/resume-all, scan, open, and quit. It never starts a second Overgent
service and opens no localhost listener.

Production starts with native create/join onboarding, installs the bundled Go
CLI into `~/.local/bin` when needed, installs that replaceable copy as a
current-user LaunchAgent, and opens the authenticated live Project
through a one-time dashboard handoff. The development build retains its
loopback profile. The menu bar reads and mutates only the existing current-user
service through its mode-0600 Unix socket.

Run from the repository root:

```bash
pnpm desktop:test
pnpm desktop:build
pnpm desktop:run
pnpm desktop:dev
pnpm dev:install
```

`desktop:dev` builds the separately identified `Overgent Dev.app` once and
loads the loopback Vite server, so React and CSS hot reload in the native
window. The development menu can perform a one-time local Project activation
inside the webview after enrollment. Production builds ignore development URLs.
See `docs/development.md` for the complete local stack and two-agent exercise.

Wails `v3.0.0-beta.12` is exact-pinned in this separate Go module because Wails
v3 remains prerelease and requires CGO. The root Go core remains pure-Go and
does not import Wails. The release workflow signs, notarizes, staples, and
packages the production app; unsigned local builds remain development-only.
See `docs/release.md` for the exact supported boundary.
