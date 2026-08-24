# Stickguy desktop preview

This macOS-only preview packages the shared React dashboard in an embedded Wails
webview and adds a persistent menu-bar client for local service health,
pause/resume-all, scan, open, and quit. It never starts a second Stickguy
service and opens no localhost listener.

The window intentionally starts in labeled fixture mode so new UI can be tested
before production desktop authentication is contracted. The menu bar reads and
mutates only the existing current-user service through its mode-0600 Unix socket.

Run from the repository root:

```bash
pnpm desktop:test
pnpm desktop:build
pnpm desktop:run
pnpm desktop:dev
pnpm dev:install
```

`desktop:dev` builds the separately identified `Stickguy Dev.app` once and
loads the loopback Vite server, so React and CSS hot reload in the native
window. The development menu can perform a one-time local Project activation
inside the webview after enrollment. Production builds ignore development URLs.
See `docs/development.md` for the complete local stack and two-agent exercise.

Wails `v3.0.0-beta.12` is exact-pinned in this separate Go module because the
system-tray API remains beta and requires CGO. The root Go core remains pure-Go
and does not import Wails. This preview is not a signed/notarized release.
