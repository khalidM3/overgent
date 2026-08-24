# External implementation references

These are upstream contracts, not substitutes for Stickguy specifications. Re-check current configuration details when implementing an integration, pin stable versions, and record breaking changes.

- Go releases and support policy: <https://go.dev/doc/devel/release>
- Official MCP Go SDK: <https://github.com/modelcontextprotocol/go-sdk>
- MCP transports/security: <https://modelcontextprotocol.io/specification/2025-11-25/basic/transports>
- Codex MCP configuration: <https://learn.chatgpt.com/docs/extend/mcp?surface=cli>
- Codex lifecycle hooks: <https://learn.chatgpt.com/docs/hooks>
- Codex non-interactive JSONL/structured output: <https://learn.chatgpt.com/docs/non-interactive-mode>
- Codex App Server (evaluation reference, not V1 dependency): <https://learn.chatgpt.com/docs/app-server>
- Convex functions/HTTP actions: <https://docs.convex.dev/functions/overview>
- Convex vector search and filter/consistency constraints: <https://docs.convex.dev/search/vector-search>
- Convex limits: <https://docs.convex.dev/production/state/limits>
- Convex import/export and backups: <https://docs.convex.dev/database/backup-restore>
- Convex self-hosting: <https://docs.convex.dev/self-hosting>
- Wails desktop framework: <https://wails.io/>
- OSI-approved licenses: <https://opensource.org/licenses>
- Open Source Definition: <https://opensource.org/osd>
- SLSA build provenance levels: <https://slsa.dev/spec/v1.0-rc2/levels>
- Sigstore/Cosign blob signing: <https://docs.sigstore.dev/cosign/signing/signing_with_blobs/>

Current baseline decisions as of 2026-08-23:

- Go module baseline 1.26; CI also covers 1.27.
- MCP Go SDK uses the latest stable pinned release, never a floating prerelease.
- Local MCP HTTP protections stay enabled; do not use compatibility flags that disable localhost protection.
- Wails is deferred until after deterministic alpha.
- TurboVec is evaluated only as a future local/self-hosted semantic-index adapter: <https://github.com/RyanCodrai/turbovec>
