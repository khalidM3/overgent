# L6A local dogfood evidence

Date: 2026-08-24
Outcome: PASS under ADR-031

## Delivered behavior

- `pnpm dev` composes local Convex, proxied Vite, the default-profile Go
  service, and the separately identified native development bundle.
- `pnpm desktop:dev` launches `Stickguy Dev.app` against a validated loopback
  Vite URL. React/CSS use Vite hot reload without rebuilding Wails.
- `pnpm dev:install` stages and atomically replaces only
  `~/Applications/Stickguy Dev.app`, retaining the previous bundle until the
  new bundle activates.
- The development menu issues a Project-authorized one-time dashboard ticket,
  serves the existing nonce handoff on loopback, and navigates the native
  webview through the form POST. No credential or ticket reaches a URL,
  JavaScript, browser storage, or retained evidence.
- The running one-service core can add a second canonical linked-worktree root,
  watcher, durable workspace registration, and independent workstream without
  restarting or creating a second service.
- `pnpm dev:agents` requires two distinct roots with the same Git common
  directory, registers missing workstreams, and merges explicit
  development-only Codex and Claude project MCP entries. Existing sessions must
  restart. Hooks and optional activity collection remain disabled.

## Verification

PASS:

```text
go test ./...
go vet ./...
go test -race ./...
go mod tidy
go mod verify
CI=true pnpm install --frozen-lockfile
CI=true pnpm typecheck
CI=true pnpm test
CI=true pnpm build
CI=true pnpm protocol:check
CI=true pnpm desktop:test
CI=true pnpm desktop:build
node scripts/build-desktop.mjs --development
codesign --verify --deep --strict apps/desktop/build/bin/Stickguy\ Dev.app
CI=true pnpm --dir convex test:live
```

The native smoke launched the signed development bundle with Vite ready on
`127.0.0.1:5173`, retained its process until explicit termination, and exited
cleanly. The production bundle compiled with the `production` tag, which fixes
its embedded URL and omits local activation. The development bundle identifier
was `dev.stickguy.app.development`.

The anonymous loopback live suite passed creator and invite enrollment,
two-device publication, same-path deterministic findings, 1,000-path atomic
activation, semantic duplicate behavior, pre-edit assumption conflict, scoped
shared-dependency routing, unrelated-work exclusion, stale assumptions,
cross-Project isolation, and radar feedback. Representative measurements were
20 ms for the second manifest/finding transaction and 93 ms for 1,000-path
activation.

## Security and cleanup

- Development URLs accept only loopback HTTP origins. Production builds ignore
  `STICKGUY_DESKTOP_DEV_URL`.
- Hot workspace registration reuses an already enrolled Project/member/device,
  canonicalizes the root, validates IDs, captures the Git baseline, derives the
  existing privacy-safe repository fingerprint, and persists before reporting
  success.
- Agent setup uses an absolute development executable, refuses drift, preserves
  unrelated project configuration, and has an exact removal path.
- No source, diff, Git object, transcript, prompt, raw command/output,
  environment value, or credential was retained as evidence.
- The native/Vite smoke and Convex server were stopped. Disposable `.convex`,
  `.env.local`, and generated local `.gitignore` state created by the live run
  were removed; they were synthetic and are not recoverable or needed.

## Honest limits

- Codex `0.148.0-alpha.15` remains `mcp_with_git_fallback`; this slice does not
  supersede the real-client delivery narrowing in ADR-026.
- Claude may report the MCP lifecycle, but production hook/activity sharing,
  consent UI, retention/deletion contracts, and arbitrary session monitoring
  remain disabled.
- The single-Mac exercise proves local attribution and coordination behavior,
  not production multiplayer reliability, hosted load, signed/notarized
  distribution, or update/service installation.
- Native Go/Wails changes still require a development-app restart; Go service
  changes require a rebuild/restart. React/CSS and Convex functions hot reload.
