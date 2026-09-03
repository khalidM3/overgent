# Lane G — L8 distribution, updates, and service lifecycle

Goal: Overgent becomes something a person installs, updates, and trusts,
rather than something built from source. Finish and harden the partial work
already committed, then close the gaps around it.

## State of play — read before writing anything

Commit `8baf7f3` carries **partial, in-flight L8 work from an earlier
session**. It builds, but it is not finished and it is not all correct. Do not
discard it and do not rewrite it wholesale; assess it, keep what is sound, and
finish it. It includes:

- `internal/update/` — updater client with an Ed25519-signed metadata check
  (`updatePublicKey` is injected by the release workflow via `-ldflags`;
  development builds refuse remote updates rather than trusting unsigned data)
- `internal/service/` — OS service lifecycle, macOS implementation plus an
  explicit unsupported stub
- `cmd/release-metadata/` — release metadata generation
- `install/install.sh`, `install/uninstall.sh`
- `scripts/sign-darwin-artifact.sh`, and changes to `.goreleaser.yml`,
  `.github/workflows/release.yml`, `scripts/build-desktop.mjs`
- new `overgent update`, `service`, and `diagnostics` commands in
  `cmd/overgent/main.go`

**Two known defects in it, both yours to resolve:**

1. `TestNarrowedAgentSetupFailsClosed` fails. The WIP removed the guard that
   withheld production `setup codex|claude` under ADR-026, but did not
   supersede that ADR or update the test.
2. That removal is a real decision, not a cleanup. ADR-026 withheld production
   adapter setup because a real-client smoke failed. Since then M1–M5 landed
   evidence that the adapters work end to end: the coordination eval suite
   drives the real `agent-hook` executable and the real MCP bridge, and all
   seven scenarios pass (`pnpm eval:coordination`). Write an ADR that
   supersedes ADR-026 on that evidence, state honestly what remains unproven
   (Codex context injection was verified against documentation, not a live
   run), and update the test to assert the new behavior. If you conclude the
   evidence is insufficient, restore the guard instead — but decide, and say
   why.

## Read first

- `docs/implementation-plan.md` (L8), `AGENTS.md`
- ADR-019 (platform narrowing), ADR-026 (withheld setup), ADR-029 (Wails
  preview boundary), ADR-043 (adapter binding states) in `docs/decisions.md`
- `docs/public-repository-boundary.md`, `docs/security-privacy.md`
- the diff of commit `8baf7f3` in full before changing any file it touched

## Decisions already made — do not revisit

- **macOS arm64 is the only supported runtime target.** ADR-019 is explicit
  that Linux and Windows service, keyring, and named-pipe behavior are
  unvalidated, and cross-compilation is not evidence. Build other artifacts if
  the toolchain produces them for free, but never advertise them as supported;
  label them explicitly as unverified.
- **Stop cleanly at the credential boundary.** Code signing and notarization
  need an Apple Developer ID that only the owner can provide. Implement
  signing, notarization, and stapling so they run when credentials are present
  in CI secrets, and degrade to a clearly labeled unsigned local build when
  they are absent. Never commit a credential, never invent placeholder
  identities, and do not attempt to obtain one. List exactly which secrets the
  owner must add, and where.
- **The updater verifies before it replaces.** Signature check, then checksum,
  then atomic swap, then a rollback path that restores the previous binary if
  the new one fails to start. An update that cannot be verified is refused, not
  applied optimistically.
- **Uninstall must actually uninstall.** Service unregistered, agent adapter
  configuration removed through the existing `setup remove` paths (never by
  hand-editing vendor files), local state removed on request, and the user told
  what was left behind and where.

## Deliver

1. Signed, notarized, stapled macOS release artifacts with checksums, an SBOM,
   and build provenance, produced by the release workflow. Verify what you can
   without credentials and document the rest as owner-gated steps.
2. Install, update, rollback, and uninstall paths that work end to end on a
   clean machine, including OS service registration and recovery after a crash
   or reboot.
3. The ADR and test resolution for the ADR-026 question above.
4. Privacy-safe diagnostics: `overgent diagnostics` must never emit source,
   diffs, prompts, transcripts, credentials, tokens, environment values, or
   command output. Test that boundary explicitly, the way the secret classifier
   is tested.
5. A re-evaluation of the Wails beta per ADR-029: either qualify the desktop
   app for release or state plainly that it stays a preview and why.
6. Contributor and adapter guides sufficient for someone outside this
   repository to build, run the eval suite, and add a vendor adapter.

## Acceptance criteria

- Clean install → run → update → rollback → uninstall verified on macOS, with
  evidence recorded in `validation/evidence/`.
- An unverifiable or tampered update is refused, and a failed update rolls back
  to a working binary. Test both.
- `go test ./...` passes with no skipped or failing tests, including the
  ADR-026 test resolution.
- Diagnostics prohibited-data tests pass.
- All standard checks: `go test ./...`, `go vet ./...`, `pnpm typecheck`,
  `pnpm test`, `pnpm build`, `pnpm protocol:check`.
- `pnpm eval:coordination` still passes all seven scenarios (needs Node 22+:
  `export PATH="$HOME/.nvm/versions/node/v22.23.2/bin:$PATH"`).

## Out of scope

Member/device/invite management, export/deletion, and load/soak/security
testing — Lane H owns those. Do not change `protocol/`; if you believe a wire
change is required, STOP and report it.
