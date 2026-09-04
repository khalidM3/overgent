# Lane 04 — Bring-your-own AI provider per Project

Status: brief
Last updated: 2026-09-04
Executor: Sonnet 5; hand the Convex encryption/resolution section to Opus 5
only if it stalls.
Depends on: ADR-073 accepted. This lane is the **only** lane allowed to touch
`protocol/`. Works against any backend (local, Cloud, self-hosted) because
the change is above the wire.

## Goal

A Project owner sets which judgment model and embedding model the Project
uses, with their own key, from the CLI or the desktop. The backend uses those
for that Project's `adjudicateFinding` and `embedSemanticObject` work, falls
back to operator keys only when the deployment explicitly enables them, and
otherwise degrades to deterministic evidence with visible fidelity exactly as
today. Keys are never returned, logged, or synced.

## Read first

- `convex/functions/intelligence.ts`: `embedSemanticObject` (reads
  `process.env.OPENAI_API_KEY`), `adjudicateFinding` (reads
  `process.env.ANTHROPIC_API_KEY`), `configureFallbackEmbeddingModel`,
  `convergeForeignEmbeddings`, `recordOpenAIEmbeddingFailure`,
  `claimJudgmentBudget`, `applyJudgmentVerdict`, `recordJudgmentDegraded`
- `packages/coordination/src/anthropic.ts` (`ANTHROPIC_JUDGMENT_MODEL =
  "claude-sonnet-5"`, `judgmentRequestText`), `openai.ts`
  (`OPENAI_EMBEDDING_MODEL = "text-embedding-3-large"`, 1024 dimensions),
  `judgment.ts`, `index.ts` (`EmbeddingProvider` interface),
  and their tests under `packages/coordination/test/`
- `convex/functions/http.ts`: the `/v1/projects/` `POST`/`GET`/`PATCH`
  prefix routes, helpers `bearer`, `withErrors`, `consumeAuthenticatedEdge`,
  `readJson`, `json`, `classify`
- `convex/functions/service.ts`: how a route resolves the device token to a
  member and checks `members.role` (search for `role` and the project GET
  handler used by `/v1/projects/{id}`); the `rateLimits` usage (ADR-070)
- `convex/functions/schema.ts` (`projects`, `members`, `semanticEmbeddings`,
  `findings`)
- `protocol/openapi.yaml`, `protocol/schemas/`, `scripts/protocol-generate.mjs`,
  `docs/protocol.md` (compatibility rules)
- `internal/hosted/client.go` (request helpers, `APIError`), `cmd/overgent/main.go`
- `docs/openai-embeddings.md`, `docs/coordination-intelligence.md`
- `apps/desktop/onboarding_service_darwin.go` (how the desktop calls hosted
  operations) and `apps/dashboard/src/native.ts` if a settings pane is added
  to the embedded UI

## Wire contract (fixed)

Add to `protocol/openapi.yaml` under the existing `/v1/projects/{projectId}`
family:

```
GET /v1/projects/{projectId}/ai-settings      any member
PUT /v1/projects/{projectId}/ai-settings      role owner only; full replace
```

`AISettingsWrite` (request body for PUT):

```json
{
  "judgment":   { "provider": "anthropic" | "openai-compatible" | "none",
                  "model": "string (1..120)", "baseUrl": "https origin, optional",
                  "apiKey": "string (8..512), optional; omitted = keep existing, \"\" = clear" },
  "embeddings": { "provider": "openai" | "deterministic",
                  "model": "string (1..120)", "dimensions": 1024,
                  "baseUrl": "https origin, optional",
                  "apiKey": "string (8..512), optional; same semantics" }
}
```

`AISettings` (response for GET and PUT):

```json
{
  "judgment":   { "provider": "...", "model": "...", "baseUrl": "...|null",
                  "keyConfigured": true, "keyHint": "…7f3a" | null },
  "embeddings": { "provider": "...", "model": "...", "dimensions": 1024,
                  "baseUrl": "...|null", "keyConfigured": false, "keyHint": null },
  "effective":  { "judgment": "project" | "operator" | "none",
                  "embeddings": "project" | "operator" | "deterministic" },
  "revision": 3,
  "updatedAt": "RFC3339"
}
```

Rules:
- `baseUrl` must be an `https` origin, or an `http` loopback origin (so an
  Ollama or LM Studio server on the member's own Mac works in local mode).
  Anything else is `400`.
- `provider: "none"` / `"deterministic"` ignores model and key fields.
- Keys are validated for shape only (length, no whitespace); never tested
  against the provider inside the request. A separate
  `POST /v1/projects/{projectId}/ai-settings/check` is **not** in scope.
- Rate limit the PUT with `consumeAuthenticatedEdge` at the same tier as
  project PATCH.
- The PUT body is the one payload in the system that legitimately contains a
  credential. It must **never** pass through the event batch path or the
  secret classifier; it goes through its own route. Add a test that the
  classifier is not invoked and that `apiKey` never appears in any log line.

Run `pnpm protocol:generate` and commit generated Go and TypeScript. Bump the
protocol minor version per `docs/protocol.md`; older clients ignore the new
routes.

## Backend (Convex)

### Schema

New table `projectAISettings`:

```
projectId, revision,
judgmentProvider, judgmentModel, judgmentBaseUrl?, judgmentKeyCiphertext?, judgmentKeyHint?,
embeddingProvider, embeddingModel, embeddingDimensions, embeddingBaseUrl?, embeddingKeyCiphertext?, embeddingKeyHint?,
updatedAt, updatedByMemberId
```
Index by `projectId` (one row per Project). Ciphertext is a base64 string of
`nonce || AES-256-GCM(ciphertext)`; the AEAD additional data is
`projectId + ":" + field name` so a ciphertext cannot be moved between rows.

### Encryption

Key material: deployment env `OVERGENT_SECRETS_KEY` (base64, 32 bytes). Lane 03
sets it on local backends; Lane 05 documents it for Cloud and self-host. If it
is missing, PUT with an `apiKey` returns `503 secrets_key_unconfigured` with an
actionable message; settings without keys still save. Use WebCrypto
(`crypto.subtle`) inside an action; mutations only store the ciphertext.
Decrypt only inside `embedSemanticObject` and `adjudicateFinding`, into a
local variable, never into a returned value or a stored document.

### Resolution

Add `resolveProviders(projectId)` used by both actions:

1. Project row exists and `judgmentProvider != "none"` with a key → project.
2. Else if deployment env `OVERGENT_OPERATOR_KEYS_ENABLED === "true"` and the
   matching env key exists → operator (keeps today's behavior for a deployment
   that opts in).
3. Else `none`: judgment records `recordJudgmentDegraded` with reason
   `provider_unconfigured`; embeddings use the deterministic fallback already
   implemented via `configureFallbackEmbeddingModel`.

The `effective` block in the GET response is computed by the same function so
the CLI shows the truth.

### Providers

In `packages/coordination`:
- `anthropic.ts`: constructor takes `{ apiKey, model, baseUrl? }`;
  `ANTHROPIC_JUDGMENT_MODEL` becomes the default, not a constant used inline.
- New `openai-compatible.ts` judgment provider: `POST {baseUrl}/v1/chat/completions`
  with `response_format: { type: "json_object" }` and the same prompt text from
  `judgment.ts`; parse with the same verdict schema. Covers OpenAI, OpenRouter,
  Ollama, LM Studio.
- `openai.ts`: constructor takes `{ apiKey, model, dimensions, baseUrl? }`.
- Changing embedding model or dimensions for a Project must trigger the
  existing re-embedding path (`convergeForeignEmbeddings` /
  `configureFallbackEmbeddingModel`); read those before wiring, and add a test
  that switching from `openai` 1024 to `deterministic` and back does not leave
  mixed-model vectors in one `scopeKey`.
- Vector index dimensions are fixed per deployment (ADR-040 moved to 1024).
  `dimensions` other than the deployment's index size is `400
  unsupported_dimensions`. Document that in `docs/openai-embeddings.md`.

### Tests

- `convex/test/`: unit tests for ciphertext round trip with AAD binding,
  resolution order, PUT authorization (member vs owner), GET never containing
  a key, and the classifier-not-invoked assertion. Extend the live suite
  (`pnpm test:live`) with one case: set `judgment.provider = "none"` and
  assert a finding that previously awaited judgment records
  `provider_unconfigured` degradation rather than silence.

## Go client and CLI

- `internal/hosted/client.go`: `AISettings(ctx, projectID)` and
  `PutAISettings(ctx, projectID, write AISettingsWrite)` using the generated
  types.
- `cmd/overgent/main.go`:
  ```
  overgent ai status [--project <id>] [--json]
  overgent ai set   [--project <id>] --judgment-provider anthropic --judgment-model claude-sonnet-5 [--judgment-base-url URL] [--judgment-key-stdin | --judgment-key-env NAME]
                                     --embedding-provider openai --embedding-model text-embedding-3-large [--embedding-base-url URL] [--embedding-key-stdin | --embedding-key-env NAME]
  overgent ai clear [--project <id>] [--judgment] [--embeddings]
  ```
  Keys are never accepted as argv values (they would land in shell history and
  `ps`). `--project` defaults to the Project of the current directory's
  workspace (`workspaceForCWD` in `internal/app`). `set` reads current
  settings first and sends a full replacement so omitted fields keep their
  values.
- `overgent doctor` gains one line: `AI: judgment=<effective> embeddings=<effective>`.

## Desktop

A per-Project "Intelligence" section in the existing Project view: provider
and model selectors, base URL field, a key field that is write-only (shows
`keyHint` when configured), and the `effective` line rendered in monospace.
Errors from `503 secrets_key_unconfigured` and `400 unsupported_dimensions`
are shown verbatim from the server's message. Follow `docs/design-system.md`.
No dashboard (browser) change in this lane.

## Docs

- `docs/openai-embeddings.md` → rename to `docs/ai-providers.md` (update
  `docs/README.md`): per-Project settings, resolution order, the operator
  fallback env vars, dimensions constraint, degradation behavior, and a short
  cost note ("your key, your bill").
- `docs/security-privacy.md` "Hosted" section: key storage control.

## Acceptance

- Against `pnpm dev` (local Convex): `overgent ai set ... --judgment-key-stdin`
  then `overgent ai status --json` shows `keyConfigured: true`, a hint, and
  `effective.judgment: "project"`; the log and the database row contain no
  plaintext key (grep the SQLite file of the local backend for the key).
- With no key and `OVERGENT_OPERATOR_KEYS_ENABLED` unset, a candidate finding
  that needs judgment is recorded as degraded with `provider_unconfigured` and
  the dashboard shows deterministic fidelity, not intelligence.
- With a real Anthropic key in a throwaway Project on the local backend, one
  adjudication succeeds end to end (record the finding id in the handoff, not
  the key).
- `pnpm protocol:check` passes; all verification commands pass.

## Out of scope

Provider connectivity tests, per-member keys, key rotation UI, spend caps
(the ADR-070 ceilings already bound call volume), accounts or billing.
