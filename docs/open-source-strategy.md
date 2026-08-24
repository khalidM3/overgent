# Stickguy — Open-Source and Trust Strategy

Status: proposed for owner approval before public launch  
Last updated: 2026-08-23

## 1. Recommendation

Use an **open-source application plus managed cloud** model, not a mostly closed client marketed as open core.

Publish all code that users install or that determines what project metadata is collected:

- Go CLI, background service, updater, Git watcher, local persistence, and MCP server;
- agent adapters and their configuration writers;
- HTTP/event/MCP schemas and generated SDKs;
- installer scripts and release workflows;
- dashboard application;
- core Convex schema, functions, authorization rules, retention jobs, structural/semantic retrieval interfaces, evidence-fusion/finding engine, and public evaluation harness;
- telemetry event definitions;
- security/privacy documentation, threat model, and tests;
- optional self-hosting support when it becomes maintainable.

Keep private only hosted-business and operational material that does not change the installed client's collection behavior:

- production credentials, environment values, and account identifiers;
- private infrastructure state and incident runbooks;
- billing-provider integration and commercial entitlements;
- internal support/admin tooling;
- anti-abuse detection details whose disclosure would materially enable bypass;
- private customer-derived evaluation data;
- unreleased experiments and proprietary hosted AI prompts/models, provided they cannot expand client collection beyond the public protocol.

The hosted service remains the product customers pay for: reliable operation, updates, storage, collaboration, support, and future commercial features. Source availability is not the business moat.

## 2. Why this boundary

The local executable can observe repositories, paths, agents, credentials, and local processes. Users should be able to inspect exactly what it reads, stores, sends, and updates. Publishing only a small SDK while closing the installed binary would not create meaningful trust.

The backend also handles sensitive project metadata and authorization. Publishing the core backend rules lets users inspect what the hosted service is intended to retain and who may access it. Production deployment secrets and operations do not need to be public for that review.

Open source alone does not prove a downloaded binary matches the source. Release provenance, signatures, checksums, SBOMs, transparent workflows, and verifiable version metadata are equally required.

## 3. Licensing recommendation

Recommended initial license for the public application repository: **Apache License 2.0**.

Reasons:

- permissive for individual and company adoption;
- explicit patent grant;
- familiar to infrastructure and developer-tool contributors;
- permits commercial hosted Stickguy without a dual-license program;
- avoids discouraging adapter and platform contributions.

Tradeoff: another company may host a fork without publishing modifications. If preventing competing hosted forks becomes more important than adoption, evaluate AGPL-3.0 for the hosted server in a separate legal/market decision. Mixing licenses at launch adds contribution and dependency complexity and is not recommended.

Do not describe code as open source if its license merely allows viewing while restricting use, modification, or redistribution. Use the accurate term `source-available` for such code.

License selection is a business/legal decision. Obtain counsel before accepting substantial external contributions or introducing a CLA/dual-license strategy.

## 4. Repository model

Use two repositories rather than public/private folders in one repository. A private folder in a public Git history is easy to leak.

### Public: `stickguy/stickguy`

```text
stickguy/
├── cmd/stickguy/                 # Go executable entry point
├── internal/                     # all installed local-core code
│   ├── app/
│   ├── auth/
│   ├── config/
│   ├── daemon/
│   ├── events/
│   ├── git/
│   ├── manifest/
│   ├── mcp/
│   ├── platform/
│   ├── store/
│   ├── sync/
│   └── watcher/
├── adapters/                     # public agent/platform adapters
│   ├── codex/
│   ├── claude-code/
│   └── cursor/                   # only after verified support
├── apps/
│   └── dashboard/                # public React dashboard
├── convex/                       # public core hosted backend
│   ├── auth/
│   ├── domain/
│   ├── http/
│   ├── intelligence/            # retrieval, fusion, findings, public eval harness
│   ├── context/                 # relevance routing and deterministic brief rendering
│   ├── providers/               # public provider/index interfaces and adapters
│   ├── retention/
│   └── schema.ts
├── protocol/
│   ├── openapi.yaml
│   ├── schemas/
│   └── generated/
├── install/                      # readable install/uninstall scripts
├── security/                     # threat model, audit artifacts, test fixtures
├── docs/
├── scripts/
├── .github/
│   ├── ISSUE_TEMPLATE/
│   ├── dependabot.yml
│   └── workflows/
│       ├── ci.yml
│       ├── codeql.yml
│       └── release.yml           # public, provenance-producing build
├── AGENTS.md
├── CODE_OF_CONDUCT.md
├── CONTRIBUTING.md
├── GOVERNANCE.md                 # add when maintainers expand
├── SECURITY.md
├── LICENSE                       # recommended Apache-2.0
├── NOTICE
├── go.mod
├── go.sum
├── package.json
├── pnpm-lock.yaml
└── README.md
```

### Private: `stickguy/cloud-ops`

```text
cloud-ops/
├── environments/                # production/staging references, no secrets
├── infrastructure/              # private deployment/IaC state configuration
├── billing/                     # commercial plans and provider integration
├── admin/                       # internal support/admin applications
├── abuse/                       # private signals and response automation
├── evals/private/               # customer-derived/redacted private eval data
├── runbooks/                    # incident, recovery, escalation procedures
└── .github/workflows/           # deployment workflows
```

Secrets never belong in either repository. Use the hosting platform's secret store and short-lived workload identity.

## 5. Public extension boundary

Adapters should live behind a documented interface so contributors can add agent support without editing the daemon core. Each adapter declares:

- stable identifier and version;
- detected application/config locations;
- capabilities supplied;
- files/settings it proposes to change;
- install, status, and uninstall behavior;
- privacy classification of every input/output;
- supported OS/client versions;
- tests using fixtures rather than a contributor's real agent data.

The public protocol must be sufficient for third-party clients and self-hosted experiments. Do not require reverse engineering of the hosted service.

## 6. Release trust chain

For every tagged release:

1. Public CI checks out the public commit/tag.
2. CI runs tests, vulnerability scans, static analysis, and protocol-drift checks.
3. CI builds every supported binary from the tag on hosted isolated runners.
4. Release publishes SHA-256 checksums, SBOM, build provenance, and signatures.
5. Installer downloads a versioned artifact and verifies checksum/signature before placement.
6. `stickguy version --json` reports version, commit, build time, protocol range, and artifact identity.
7. Update client verifies signed metadata and artifact before replacement and supports safe rollback.
8. Release page links source tag, workflow run, provenance, SBOM, checksums, and verification instructions.

Target SLSA Build Level 2 provenance first. Improve reproducibility over time and publish documented reproduction commands. Sigstore/Cosign keyless signing is appropriate for CI-built blobs, but verification must be embedded in the installer/update flow rather than requiring every user to install Cosign.

## 7. Security transparency

Public repository must include:

- `SECURITY.md` with private vulnerability-reporting channel, supported versions, response targets, and disclosure policy;
- threat model and prohibited-data policy;
- exact telemetry schema and default state;
- architecture/data-flow diagrams;
- dependency lockfiles and automated updates;
- CodeQL/static analysis and vulnerability scanning;
- security regression tests;
- published advisories and patch releases;
- third-party audit reports when commissioned.

Do not accept security reports through a public issue. Enable GitHub private vulnerability reporting or publish a dedicated security email before launch.

## 8. Contribution and governance

Start with Developer Certificate of Origin sign-off rather than a CLA unless future dual licensing requires a CLA. Require focused pull requests, tests, security/privacy review for data-flow changes, and maintainer approval for protocol changes.

Public contribution labels should include `good first issue`, `adapter`, `platform`, `security`, `protocol`, and `privacy`. The contributor guide must make a local fake-backend/fake-agent path available so contributors never need production credentials or private data.

## 9. Open-core boundary if needed later

If commercial differentiation is needed, keep these public permanently:

- installed client and updater;
- all collection/transmission code;
- protocol and SDKs;
- baseline server authorization/retention;
- baseline dashboard, structural/semantic candidate retrieval, evidence fusion/findings, context router/brief renderer, and public evaluation harness;
- embedding, semantic-index, and adjudication interfaces plus initial managed adapters;
- adapter interface and first-party adapters.

Reasonable private hosted additions include organization policy, enterprise administration, advanced audit/search, commercial billing, managed compliance, premium retention, support tooling, private customer-derived evaluation data, and premium coordination models. The V1 intelligence contract and baseline implementation remain public. A private feature must never silently widen local collection beyond the public protocol and consent UI.

## 10. Launch checklist

- Owner confirms license with counsel.
- Public/private repository split exists before any production secret or private data.
- LICENSE, NOTICE, CONTRIBUTING, CODE_OF_CONDUCT, SECURITY are complete.
- Security contact/private reporting works.
- Public release workflow signs and attests artifacts.
- Installer verifies artifacts.
- Binary exposes source commit/version.
- Telemetry and privacy defaults are documented/tested.
- No secret exists in Git history.
- Backend export/deletion works.
- Public demo can run against fixtures without production access.
