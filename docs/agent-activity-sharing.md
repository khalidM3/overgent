# Stickguy — Agent Activity Sharing

Status: ADR-047 membership sharing model
Last updated: 2026-08-26

## Purpose and boundary

Stickguy observes supported Codex and Claude Code lifecycle/event surfaces so
authorized Project members can understand active workstreams and detect
collisions. Installing Stickguy in a Project and connecting an adapter enables
Project sharing. There is no separate profile, session, audience, expiry, or
message-kind ceremony.

Stickguy remains a coordination harness. It does not own the model loop, start
or steer coding work, execute tools, approve permissions, or mutate a
repository. Project visibility means authorized members of that Project, never
a public feed.

## Sharing gate

An adapter candidate may cross the wire only when all three conditions hold:

1. the device member is enrolled in the Project;
2. the workspace is not paused; and
3. the mandatory secret classifier accepts the complete candidate.

Pause takes effect synchronously before success returns and stops new payload
transmission. Unsupported or disconnected adapters degrade honestly to
Git/manual fidelity. Members and Project owners may delete retained session
messages; hosted authorization, retention, and Project deletion continue to
apply.

## Shared activity and session content

The activity projection includes session/turn/subagent lifecycle, a generated
current-action label, allowlisted tool name/category/status, permission-needed
state, and safe repository-relative paths. It excludes tool inputs/results,
raw commands/output, and file content.

For session detail, Stickguy locally reads the supported vendor record named or
identified by the adapter. The read is bounded and the record is never copied
as a raw transcript. The owner always sees their own local session. Parsed
`user`, `assistant`, `thinking`, and surfaced `system` messages that pass the
classifier are projected to the Project. A `tool` part contributes its name
only to activity and is never projected as message content.

Each vendor has its own adapter (ADR-039). Claude Code supplies its record path;
Codex rollouts are located from the local session ID. Codex conversation comes
from its visible event stream, not raw model I/O, so injected context is not
misrepresented as user-authored text.

## Mandatory classifier boundary

Every content candidate is classified independently before durable local
storage or enqueue. Environment assignments, credentials, tokens, cookies,
private keys, protected credential paths, raw tool results, and command output
reject the whole message. Nothing is redacted into an apparently safe message.
Quoted code and diffs in conversation are allowed; naming a configuration file
is allowed, while pasting its secret-bearing contents is not (ADR-038).

Raw transcript files, attachments, tool results, command output, vendor-
encrypted reasoning, environment values, credentials, and secrets never cross
the wire. Scanner failure, unknown message kinds, and oversize content fail
closed without blocking the coding agent.

## Required validation

Tests must prove zero-step enrollment-to-visibility, synchronous pause with
queued events, whole-message classifier rejection for every prohibited class,
unknown-event failure, Project isolation, member/owner deletion, retention, and
adapter removal/config-drift behavior. Core activity and classifier behavior
must remain useful with AI disabled.
