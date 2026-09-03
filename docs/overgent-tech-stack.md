# Overgent — Technology and Repository Plan

Status: canonical  
Last updated: 2026-08-24

## 1. Stack decision

| Layer | Technology | Reason |
|---|---|---|
| Installed local core | Go 1.26 baseline | Standalone binaries, simple concurrency, Tier 1 MCP, accessible OSS contributions. |
| Local persistence | SQLite via pure-Go driver | Durable queue/offsets without CGO or user native libraries. |
| Agent protocol | Official MCP Go SDK | Stdio and optional loopback Streamable HTTP. Pin stable. |
| Hosted backend | Convex + TypeScript | Realtime subscriptions, transactions, schedules, HTTP actions, low ops. |
| Dashboard | Vite + React + TypeScript | Hosted SPA; no SSR requirement. |
| macOS desktop preview | Exact-pinned Wails v3 beta + existing React UI | Provides a native window/menu bar without replacing or duplicating the Go service. |
| Contract | OpenAPI 3.1 + JSON Schema | Language-neutral source with generated Go/TS types. |
| V1 embeddings | Backend `EmbeddingProvider` interface | Embed only approved coordination summaries; model/version recorded and replaceable. |
| V1 semantic index | `SemanticIndex` domain interface; Convex vector adapter first | Shared cross-device retrieval without a second database or Rust runtime. |
| Optional adjudication | Backend provider interface | Strict bounded input/output for ambiguous candidates; deterministic engine remains independent. |
| Context routing | Testable TypeScript domain module + generated contracts | Shared state is hosted; deterministic workstream briefs remain portable and provider-free. |

Go 1.27 is current but newly released. Use Go 1.26 as module baseline and test 1.26/1.27 in CI. Raise the minimum only through an ADR.

## 2. Repository shape

```text
overgent/
├── cmd/overgent/               # one executable; CLI/service/MCP modes
├── internal/
│   ├── app/                    # composition/lifecycle
│   ├── auth/                   # enrollment/credentials
│   ├── config/                 # versioned local config
│   ├── daemon/                 # single instance and IPC
│   ├── events/                 # event construction/validation
│   ├── git/                    # Git CLI adapter/repo identity
│   ├── manifest/               # baseline/current change manifests
│   ├── mcp/                    # MCP adapter only
│   ├── platform/               # keychain, OS paths/services/browser
│   ├── store/                  # SQLite state/queue
│   ├── sync/                   # Overgent HTTP client
│   └── watcher/                # change aggregation
├── protocol/
│   ├── openapi.yaml
│   ├── schemas/
│   └── generated/              # never hand-edit
├── adapters/                   # public agent/platform adapters
├── apps/dashboard/
├── apps/desktop/               # separate Wails preview module; embedded dashboard
├── convex/
│   ├── intelligence/           # retrieval, evidence fusion, findings, evals
│   ├── context/                # relevance router, brief rendering/versioning
│   └── providers/              # embedding/adjudication adapters
├── install/                    # public installer/uninstaller scripts
├── security/                   # public threat model/audit artifacts
├── docs/
├── scripts/
├── .github/workflows/          # public CI/release/provenance
├── AGENTS.md
├── SECURITY.md
├── CONTRIBUTING.md
├── CODE_OF_CONDUCT.md
├── LICENSE
├── NOTICE
├── go.mod
├── pnpm-workspace.yaml
└── package.json
```

Use one root Go module for the service. ADR-029 permits one narrow separate
`apps/desktop` module so Wails beta/CGO dependencies do not enter the pure-Go
service graph. Do not create a generic `pkg/` until a package is intentionally
public.

## 3. Initial Go preferences

| Concern | Preference |
|---|---|
| CLI | `github.com/spf13/cobra` |
| Logging | Standard `log/slog` |
| HTTP | Standard `net/http`; add router only if needed |
| MCP | `github.com/modelcontextprotocol/go-sdk` stable |
| Filesystem | `github.com/fsnotify/fsnotify` |
| SQLite | `modernc.org/sqlite` via `database/sql` |
| IDs | `github.com/google/uuid` UUIDv7; serialized as prefixed opaque strings |
| Keychain | `github.com/zalando/go-keyring`; no plaintext fallback without explicit consent |
| HTTP generation | `github.com/oapi-codegen/oapi-codegen` for Go; `openapi-typescript` for TypeScript |
| Testing | Standard testing/httptest plus temporary Git repositories |

Rules: execute Git with `exec.CommandContext` argument arrays; prefer standard library; no CGO release builds without ADR; pin dependencies; check protocol-generation drift in CI. Use `golangci-lint`, `govulncheck`, and frontend lockfile auditing in CI.

## 4. Executable modes

```text
overgent create
overgent join <code>
overgent projects
overgent status
overgent pause|resume [--project|--workspace]
overgent service run|start|stop|status
overgent mcp
overgent doctor
overgent update
```

`overgent mcp` is a thin stdio bridge to the running service. It never creates another watcher, queue, or hosted connection.

## 5. Backend/dashboard boundary

Convex stores coordination state, a separate vector table/index, and live dashboard projections. Go never imports a Convex SDK; it calls versioned Overgent HTTP endpoints implemented by Convex HTTP actions. Semantic indexing is behind an Overgent domain interface even though the first adapter uses Convex vector search. Domain/evidence-fusion rules live in testable TypeScript modules behind thin Convex wrappers. Device enrollment and browser tickets use Overgent's own hashed-token flow; Convex Auth is not part of alpha.

Frontend: React/TypeScript/Vite; minimal accessible components; no global state library until necessary; Playwright for critical flows. The hosted and embedded desktop views reuse the same build; the preview window is fixture-backed while its menu bar talks only to the local service.

## 6. Distribution

Initial targets: macOS arm64/amd64, Windows amd64, Linux amd64/arm64. Add Windows arm64 when tested.

Use GoReleaser for checksums, archives/installers, SBOMs, and provenance, with Sigstore/Cosign-compatible signing. Provide direct signed downloads, macOS/Linux script, Windows PowerShell installer, then Homebrew/WinGet. Update metadata must be signed.

End users need no runtime. Contributors need Go 1.26+ and pnpm via Corepack.
Building the optional macOS desktop preview additionally needs Xcode command-line
tools and the exact-pinned Wails module dependencies.

The application repository is intended to be public. Production operations, billing, internal admin/abuse systems, private runbooks, and private evaluation data live in a separate private `cloud-ops` repository. See `docs/open-source-strategy.md`.

## 7. CI gates

- Go test (race where supported), vet, formatting, static analysis;
- dashboard/backend typecheck, lint, unit tests, build;
- protocol generation drift check;
- temporary-Git integration tests;
- secret/dependency vulnerability scanning;
- Playwright smoke flow once UI exists;
- release builds and clean-runner install/update/uninstall verification.

## 8. Deferred choices

- Wails v3 remains beta and is accepted only for the ADR-029 macOS preview; signed/notarized and cross-platform support waits for L8 re-evaluation.
- Embedding and adjudication provider activation waits for privacy fixtures and labeled coordination evals; their interfaces/schemas are V1 contracts.
- TurboVec remains an optional local/self-hosted semantic-index adapter pending corpus benchmarks and a packaging ADR.
- Checkpoint namespace waits for Git-host spikes.
- Native agent-log adapters wait for Git/MCP dogfood.
- Self-hosted Convex is an escape hatch, not initial ops.
