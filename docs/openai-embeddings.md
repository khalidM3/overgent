# OpenAI embeddings for semantic coordination

Overgent can enrich approved coordination objects with OpenAI
`text-embedding-3-large` embeddings. The embedding key is a Project setting
(ADR-073), configured by a Project owner with the Project's own key; a
deployment may also enable an operator key as a fallback for Projects with none
configured, which is an explicit operational choice, not a default. This is
optional either way: Git/path evidence and the deterministic offline concept
provider remain available when no provider is configured or it is unavailable.

## What is sent

Only a bounded intent or checkpoint summary that already passed Overgent's
semantic policy is sent to the embeddings endpoint. Source, diffs, Git objects,
prompts, transcripts, tool input/output, raw command output, environment values,
and credentials are rejected before storage and before this provider runs.

## Configure a Project

Set the embedding key through the Project's AI settings (`/v1` operation), not
the repository, a `.env` file committed to Git, local Overgent configuration,
or Codex/Claude configuration. The key is encrypted at rest, never returned by
a read, and never logged; the client never receives it back. A deployment
operator may instead set `OPENAI_API_KEY` as a Convex deployment environment
secret to offer a fallback for Projects with no key configured.

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
request times out, Overgent records degraded semantic fidelity. It continues to
publish manifests, structural findings, deterministic concept findings, and
briefs. A provider failure never blocks a workstream, Git observation, or agent
session.

## Evaluation before proactive delivery

OpenAI vectors are candidate retrieval evidence, not collision proof. Keep
semantic findings in the radar and normal relevant brief path until a labeled
team corpus demonstrates sufficient precision for proactive attention requests.
