# L5B macOS desktop preview evidence

Date: 2026-08-24
Outcome: PASS for the labeled macOS preview; distribution remains narrowed.

## Delivered boundary

- `apps/desktop` is a separate Go 1.26 module with exact-pinned Wails
  `v3.0.0-beta.12`; the root local core remains pure-Go.
- The native window embeds the existing React/Vite dashboard assets and opens no
  localhost or TCP listener.
- One persistent system-tray item reports local service and queue status and
  invokes open, pause/resume-all, scan, and quit through the existing mode-0600
  current-user Unix socket. It never starts a second service.
- Window data is deliberately fixture-backed and visibly labeled. Real hosted
  desktop authentication is not implied.

## Native validation

The isolated synthetic spike launched on macOS 27 arm64 with Go 1.26.7. Native
UI inspection proved the embedded window, close-to-hide persistence, tray menu,
Open, Pause/Resume state transition, Scan, and clean Quit. Wails v2.15.0 also
launched as a window-only fallback but lacks the required tray API.

The integrated ad-hoc-signed `Stickguy.app` rendered the shared coordination
dashboard and explicit fixture banner in its native webview. Its stripped
executable was 8,616,336 bytes. At 29 seconds the integrated preview measured
101,040 KiB RSS and 0.0% CPU, and `lsof -nP -a -p <pid> -iTCP` returned no TCP
descriptors. Closing the native window left the application running for its
menu-bar client. The isolated spike measured 88,864 KiB RSS and 0.0% CPU; the
preview is not yet held to the local service's memory budget.

## Privacy and security review

- Embedded static assets avoid a loopback web server and browser ticket handoff.
- Menu actions use the existing socket permission and request validation
  boundary; disconnected service state fails closed.
- The preview reads no repository content, diffs, Git objects, transcripts,
  prompts, environment values, credentials, or hosted account data.
- No production signing identity, notarization credential, secret, or customer
  data was used. The local bundle is ad-hoc signed for smoke testing only.

## Verification commands

```text
pnpm desktop:assets
pnpm desktop:test
node scripts/build-desktop.mjs
codesign --verify --deep --strict apps/desktop/build/bin/Stickguy.app
```

Root Go/TypeScript/protocol checks and the final native launch smoke are recorded
in the milestone commit handoff. Final results were PASS for root `go test
./...`, `go vet ./...`, `go test -race ./...`, frozen pnpm install, recursive
typecheck/test/build, protocol generation/drift check, desktop test/vet/build,
strict code-signature verification, and native visual/close-to-hide inspection.

## Honest limits

Wails v3 is beta, macOS is the only native runtime tested, and release signing,
notarization, update/rollback, notifications, deep links, and hosted desktop
authentication are absent. The hosted browser remains the fallback. L8 must
re-evaluate the framework version and qualify every advertised platform.
