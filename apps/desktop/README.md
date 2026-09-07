# Overgent desktop

The desktop shell packages the shared React dashboard in an embedded Wails
webview and adds a persistent tray client for local service health,
pause/resume-all, scan, open, and quit. It never starts a second Overgent
service and opens no localhost listener.

Production starts with native create/join onboarding, installs the bundled Go
CLI at a stable per-user path, installs that replaceable copy as the current
user's background service, and opens the authenticated live Project through a
one-time dashboard handoff. The development build retains its loopback profile.
The tray reads and mutates only the existing current-user service through its
local IPC endpoint.

Run from the repository root:

```bash
pnpm desktop:test
pnpm desktop:build
pnpm desktop:run
pnpm desktop:dev
pnpm dev:install
```

`desktop:dev` builds the separately identified development application once and
loads the loopback Vite server, so React and CSS hot reload in the native
window. The development menu can perform a one-time local Project activation
inside the webview after enrollment. Production builds ignore development URLs.
See `docs/development.md` for the complete local stack and two-agent exercise.

Wails `v3.0.0-beta.12` is exact-pinned in this separate Go module because Wails
v3 remains prerelease and requires CGO on macOS and Linux. The root Go core
remains pure-Go, does not import Wails, and never will: only this module may.

## Platforms

| | webview | build needs | status |
| --- | --- | --- | --- |
| macOS 12+ (arm64) | WKWebView | CGO, Xcode command line tools | shipped; signed, notarized, stapled |
| Linux (x64, arm64) | WebKitGTK | CGO, `libgtk-3-dev`, `libwebkit2gtk-4.1-dev` | compiles and tests in CI; not yet exercised on a desktop |
| Windows 10 1809+ (x64, arm64) | WebView2 | no CGO — Wails' Windows backend is pure Go | compiles and tests in CI; not yet exercised on a desktop |

Wails links `webkit2gtk-4.1`. The older `4.0` package is a different ABI and
does not satisfy it.

Windows is the one target that can be type-checked from any machine, which is
worth knowing when changing this package:

```bash
cd apps/desktop && CGO_ENABLED=0 GOOS=windows go vet ./...
```

Linux cannot be checked that way. With CGO off, Wails' own GTK bindings do not
compile; with CGO on, there is no Linux toolchain to cross to from macOS. Only a
Linux runner proves the Linux build, which is what the `desktop` job in
`.github/workflows/ci.yml` is for.

## What each platform decides

Everything else is shared, in untagged files. The per-OS surface is small and
each piece is reached through a named seam:

- `main_darwin.go` / `main_linux.go` / `main_windows.go` — the Wails
  application, window and tray options. `shell.go` is the application itself.
- `platform_*.go` — where each coding agent's binary is installed, what counts
  as a runnable file, how a URL is handed to the system, and how the per-user
  background service names the account it runs as.
- `resources_*.go` — where an installed application keeps its bundled CLI and
  backend, and where the CLI is installed to so it outlives the application
  directory.
- `deeplink_register_*.go` — how `overgent://` is claimed. The document each one
  writes is built in `deeplink_register.go`, which is untagged so it is testable
  from any machine.

`deeplink.go` is the single validator for what this application will open,
whichever route a link arrives by.

## Installed layout

macOS uses the bundle format. Linux and Windows use a `resources` directory
beside the executable, which is the one arrangement that survives an AppImage
(paths only resolve inside a mount that exists while the app runs), a tarball
extracted anywhere, and a package under `/opt` reached through a `/usr/bin`
symlink.

```
<application directory>/
  overgent-desktop[.exe]
  resources/
    overgent[.exe]                       the CLI this build carries
    backend/convex-local-backend[.exe]   the bundled Convex backend
    backend/backend-push.json            its release-time deploy payload
    icons/overgent.{png,ico}
  overgent.desktop                       Linux only, for a packager to install
```

A production build refuses to package without **both** the backend binary and
the deploy payload. Shipping half the pair produces an application whose
local-Project button fails when pressed, which is worse than a build that says
what is missing.

The CLI is installed out of that directory on first run — `~/.local/bin` on both
Unixes (`XDG_BIN_HOME` honoured), `%LOCALAPPDATA%\Overgent\bin` on Windows — so
that a managed hook never names a path inside an application directory the next
update replaces.

## Packaging

`pnpm desktop:build` produces the staged tree above. Turning that into an
installable artifact is a separate step, and the format is not decided here.
These are the trade-offs as they stand for this application.

### Linux: AppImage or deb

The decisive fact is that **WebKitGTK cannot realistically be bundled**. It is
large, it pulls in most of the GTK stack, and the accepted practice for both
formats is to link the system copy. So the first-run failure this product has to
design against is not a missing icon; it is a member on a machine with no
`libwebkit2gtk-4.1-0`, or with the older `4.0` ABI, getting a dynamic linker
error instead of a window.

**deb** is the only one of the two that can state that requirement:
`Depends: libwebkit2gtk-4.1-0, libgtk-3-0` makes apt install the webview as part
of installing the application. It also gets system integration for free — the
`.desktop` file into `/usr/share/applications`, the icon into the hicolor theme,
`/opt/overgent` with a `/usr/bin` symlink — and an upgrade path if the release
is served from a repository. The costs are that it needs root to install, and
that it serves Debian and Ubuntu only; Fedora and Arch members are unserved
until an rpm and an AUR recipe exist.

**AppImage** costs almost nothing to add: it is the same staged tree with a
runtime prepended, needs no root, no package manager, and no repository
infrastructure, and it runs on any distribution. It suits the per-user shape of
this application well, and the two things AppImages are usually criticised for —
no desktop entry, no icon, no scheme registration — are already handled, because
the application writes all three itself at launch and resolves `$APPIMAGE`
rather than the FUSE mount when it does. Its real costs are that it cannot
declare the WebKitGTK dependency, that newer Ubuntu images no longer ship
`libfuse2` so the file needs a fuse3 runtime or
`--appimage-extract-and-run`, and that updates have to come from the
application's own updater rather than from the system.

**Recommendation: ship the deb as the primary Linux artifact and the AppImage
as a secondary "any distribution" download.** The deb is what makes the one
failure that actually stops a first-time member — a missing or wrong-ABI
webview — impossible rather than merely diagnosable, and the AppImage covers
everyone the deb does not for very little extra work. If only one can be built
at first, build the deb.

### Windows: NSIS or MSI

The decisive fact here is that **this application is per-user by construction**.
The URL scheme is registered in `HKCU`, the background service is a per-user
task, the CLI is installed under `%LOCALAPPDATA%`, and the profile and
credentials are per user. Nothing it does wants machine-wide state.

**NSIS** matches that exactly: it installs to `%LOCALAPPDATA%\Programs` with no
elevation prompt, which means a member can install it on a managed laptop
without asking anyone. It is also scriptable enough to do the two things that
actually matter on first install — chain the WebView2 Evergreen bootstrapper
when the runtime is absent, and stop a running background service before
replacing the binaries it is executing. Its costs are that the uninstall entry
and upgrade logic are hand-written, and that it is not what an IT department
deploys with.

**MSI** is what an IT department deploys with: Group Policy, Intune and SCCM all
take one, the install is transactional and rolls back, and uninstall is
standard. But a per-user MSI (`ALLUSERS=""`) is awkward, and a machine-wide one
undercuts its own value here — the application would be installed for everyone
and still register its scheme and service per user at first launch, so the
managed deployment would not actually have finished the install. The WiX
toolchain is also heavier and makes the conditional WebView2 step harder.

**Recommendation: NSIS for the beta and for as long as the product is
per-user.** Add an MSI only when an organization asks for managed deployment,
and then as a second artifact wrapping the same staged tree rather than as a
replacement — the two are not exclusive, and the staged layout is already the
input to both.

Both Windows artifacts need Authenticode signing before they are useful to
anyone outside the test group: SmartScreen blocks an unsigned installer with a
dialog most people will not click through. That is the same class of problem as
macOS notarization, which `docs/release.md` already covers, and the same answer
— a certificate the release workflow holds, not this script.

## Release boundary

The release workflow signs, notarizes, staples, and packages the macOS
application; unsigned local builds remain development-only. Linux and Windows
artifacts are not yet part of that workflow. See `docs/release.md` for the exact
supported boundary.
