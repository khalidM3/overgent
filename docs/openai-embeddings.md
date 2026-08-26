# OpenAI embeddings for semantic coordination

Stickguy can enrich approved coordination objects with OpenAI
`text-embedding-3-large` embeddings. This is optional: Git/path evidence and
the deterministic offline concept provider remain available when the provider
is not configured or is unavailable.

## What is sent

Only a bounded intent or checkpoint summary that already passed Stickguy's
semantic policy is sent to the embeddings endpoint. Source, diffs, Git objects,
prompts, transcripts, tool input/output, raw command output, environment values,
and credentials are rejected before storage and before this provider runs.

## Configure a hosted deployment

Set `OPENAI_API_KEY` as a Convex deployment environment secret. Do not put it in
the repository, a `.env` file committed to Git, local Stickguy configuration,
or Codex/Claude configuration. The client never receives the key.

The hosted action requests a 1024-dimension vector and records the provider and
model version alongside the embedding. A semantic object is embedded only after
its meaningful content revision changes; an asynchronous revision check drops a
late result rather than overwriting a newer object.

Successful managed vectors trigger a fresh repository-scope finding pass.
Compatible OpenAI vectors are used as semantic candidate evidence; the
deterministic concept vector remains the fallback when either side lacks a
current managed vector. Provider-only similarity stays advisory and is never
presented as proof.

## Failure behavior

If no key is configured, OpenAI rejects a request, or the 10-second bounded
request times out, Stickguy records degraded semantic fidelity. It continues to
publish manifests, structural findings, deterministic concept findings, and
briefs. A provider failure never blocks a workstream, Git observation, or agent
session.

## Evaluation before proactive delivery

OpenAI vectors are candidate retrieval evidence, not collision proof. Keep
semantic findings in the radar and normal relevant brief path until a labeled
team corpus demonstrates sufficient precision for proactive attention requests.
