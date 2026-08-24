# L6B native onboarding evidence

Date: 2026-08-24
Outcome: PASS under ADR-032

## Delivered behavior

- `Stickguy Dev.app` retains its Wails runtime while Vite supplies React/CSS hot
  reload, using the framework's loopback dev-server proxy instead of navigating
  the webview directly away from the native origin.
- The dashboard explicitly imports `/wails/runtime.js` only on Wails origins;
  a regression test verifies the fully qualified native service call. Fresh
  `pnpm dev` runs force Convex's documented anonymous agent mode so a TTY cannot
  block backend readiness with an account prompt.
- The first-run screen chooses a directory with the native macOS picker,
  canonicalizes it, and reuses the reviewed Git baseline/fingerprint preflight.
- A user can create a Project or consume an invite, name the device, explicitly
  opt detected Codex/Claude Code adapters in, and receive the creator's one-use
  invite without a terminal command.
- The connected screen shows deterministic Git observation and adapter fidelity,
  opens the Project through a short-lived nonce handoff, and can configure an
  adapter later without overwriting unrelated Project configuration.
- Existing linked worktrees can be assigned separately to Codex or Claude. The
  native boundary checks a shared Git common directory, rejects the enrolled
  checkout and an already-assigned opposing adapter root, calls the development
  workspace-add CLI with an argument array, and installs only the selected MCP
  entry. It never runs a Git-mutating command.

## Verification

Focused checks passed while implementing:

```text
GOCACHE=/private/tmp/stickguy-onboarding-go-cache go test ./internal/activation
GOCACHE=/private/tmp/stickguy-onboarding-desktop-cache go test ./...  # apps/desktop
CI=true pnpm --dir apps/dashboard typecheck
CI=true pnpm --dir apps/dashboard test  # 16 tests
CI=true pnpm desktop:assets
GOCACHE=/private/tmp/stickguy-onboarding-desktop-cache node scripts/build-desktop.mjs --development
```

Final frozen verification also passed:

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

The live suite reported PASS for all L2/L6 assertions. Representative local
measurements were 125 ms creator enrollment, 34 ms for the second
manifest/finding transaction, and 86 ms for atomic 1,000-path activation. Its
anonymous loopback backend was stopped and disposable `.convex`, `.env.local`,
and generated `.gitignore` state was removed afterward.

The development bundle compiled and was ad-hoc signed. A direct GUI launch was
not performed during the focused pass because launching a newly built local app
requires separate interactive confirmation in the execution environment; the
previous L6A real-bundle visual smoke remains applicable to the unchanged
window/menu shell. This is not represented as a new visual pass.

## Security and privacy

- The native UI receives IDs, labels, canonical local roots, adapter flags, and
  bounded errors only. Device credentials remain in Keychain; dashboard tickets
  remain inside escaped form POSTs; neither is returned to React.
- Development API and dashboard origins accept only credential-free loopback
  HTTP URLs. Production values are compile-time fixed.
- Agent detection uses executable lookup only. Setup writes no credential and
  refuses managed-entry drift. CLI and Git calls use argument arrays with
  canonical roots and bounded output.
- No source, diff, Git object, prompt, transcript, system prompt, hidden
  reasoning, raw command/output, environment value, or credential is uploaded
  or retained as evidence.

## Honest limits

- One checkout is one combined workstream; process authorship cannot be inferred
  from filesystem events. Separate existing linked worktrees are required for
  honest local Codex-versus-Claude collision attribution.
- Codex remains `mcp_with_git_fallback` under ADR-026. Claude Code MCP is
  available; optional hook/activity sharing remains disabled.
- Generic Claude Desktop is not a supported repository lifecycle adapter.
- The current config has one device identity. Adding a second Project/device to
  an already-running local profile is not exposed by this milestone.
- Production onboarding, hosted multiplayer reliability, notarization,
  installers, updates, and service installation remain L8 gates.
