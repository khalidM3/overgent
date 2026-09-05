# Per-Project AI providers

Overgent's deterministic coordination evidence works without AI. A Project
owner may optionally configure judgment and embedding providers for that
Project. The Project's key is used only for its own semantic work: your key,
your bill.

## Configure a Project

Use the desktop Intelligence section or the CLI:

```bash
printf '%s\n' "$ANTHROPIC_API_KEY" | overgent ai set \
  --judgment-provider anthropic \
  --judgment-model claude-sonnet-5 \
  --judgment-key-stdin \
  --embedding-provider deterministic \
  --embedding-model deterministic-v1

overgent ai status --json
```

Keys are accepted only from standard input or a named environment variable,
never as command-line values. The dedicated settings operation encrypts them at
rest, returns only `keyConfigured` and a four-character hint, and never sends
them through the coordination event path or secret classifier.

Judgment supports Anthropic and OpenAI-compatible chat completion endpoints,
including OpenAI, OpenRouter, Ollama, and LM Studio. Embeddings support OpenAI-
compatible embedding endpoints. A custom base URL must be an HTTPS origin, or
an HTTP loopback origin for a provider running on the same machine as a local
backend.

## Resolution and degradation

Both judgment and embedding actions resolve their provider independently:

1. A configured Project provider with a Project key.
2. An operator provider only when `OVERGENT_OPERATOR_KEYS_ENABLED=true` and the
   matching `ANTHROPIC_API_KEY` or `OPENAI_API_KEY` is present.
3. No judgment provider and deterministic embeddings.

With no judgment provider, a candidate that needs semantic adjudication is
recorded with `provider_unconfigured` degradation. It remains visible as
deterministic fidelity and is never presented as full intelligence. Provider
errors and timeouts degrade in the same honest way and never block structural
findings, workstreams, or agent sessions.

## Embedding dimensions and convergence

The deployment vector index is fixed at 1024 dimensions. Any other value is
rejected with `400 unsupported_dimensions`. Changing a Project between OpenAI
and deterministic embeddings schedules the existing convergence path so one
repository scope does not retain mixed-model vectors.

Only bounded intents or checkpoint summaries that passed semantic policy reach
a provider. Source, diffs, Git objects, prompts, transcript files, raw tool or
command output, environment values, and credentials do not. Provider vectors
are candidate-retrieval evidence, never collision proof.

## Deployment secret

Project keys are encrypted with `OVERGENT_SECRETS_KEY`, a base64-encoded
32-byte deployment secret. A key-bearing settings write fails with
`503 secrets_key_unconfigured` when it is absent; settings that contain no keys
still save. Back up and rotate this secret as deployment credential material,
not as repository configuration.
