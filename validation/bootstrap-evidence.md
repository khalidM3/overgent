# Bootstrap evidence

Observed: 2026-08-23 on macOS arm64.

## Specification baseline

The workspace initially contained only `AGENTS.md`, `README.md`, and the reviewed
documents indexed by `docs/README.md`; it was not a Git repository. The ordered
review completed before repository mutation.

Git was initialized on branch `main`. The byte-preserving baseline commit is:

```text
ed34601ec7c006302a5a9bda5acac737ad13cead
docs: preserve reviewed specification baseline
```

Intentional Markdown hard line breaks in document status headers were retained.
The shared synthetic L-1 vocabulary was fixed before parallel gates in commit:

```text
4c9c6ff
validation: freeze L-1 synthetic fixture vocabulary
```

## Contributor tools

| Tool | Observed version/status |
|---|---|
| Go | `go1.26.7 darwin/arm64`; installed as Homebrew `go@1.26` after approval |
| Git | `2.50.1 (Apple Git-155)` |
| Node | `v23.6.0` |
| Corepack | `0.30.0` |
| pnpm | `11.19.0` |
| Codex CLI | `0.148.0-alpha.15` |

Official Go release metadata identified 1.26.7 as the current 1.26 patch for
macOS arm64. No Go 1.27-only code or module baseline was introduced.

## Lane isolation

After both commits and Go verification, Gates A, B, D, and E began only beneath
their own `validation/spikes/gate-*` directories. Gate C began after
`validation/fixtures/l1-scope-v1.json` fixed its shared scope and routing
vocabulary. The root integrator retained ownership of shared fixtures, accepted
ADR outcomes, Git history, and final cleanup.
