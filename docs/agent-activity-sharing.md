# Stickguy — Agent Activity Sharing

Status: approved direction; production contracts gated on adapter validation
Last updated: 2026-08-23

## 1. Purpose and boundary

Stickguy may observe supported Codex and Claude Code lifecycle/event surfaces so
authorized Project members can understand what active workstreams are doing and
detect likely collisions earlier. This is observation and disclosure around an
existing coding-agent harness. Stickguy still does not own the model loop, start
or steer coding work, execute tools, approve permissions, or mutate a repository.

Collection capability and Project sharing are separate decisions. Installation,
Project enrollment, or enabling the normal coordination adapter never silently
enables content-bearing activity capture. "Project" visibility means authorized
members of that Project, not a public Internet feed.

## 2. Sharing profiles

Every workspace/session has an effective profile. The dashboard and CLI show it
continuously with adapter/fidelity provenance.

| Profile | Default | May reach the Project | Never included |
|---|---|---|---|
| `coordination` | yes | presence, workstream intent/checkpoints, safe path/status manifests, findings, bounded verification state, adapter health | prompt/message content, raw commands/output, source/diffs |
| `activity` | no; explicit member opt-in after owner enables the capability | session/turn/subagent lifecycle, visible plan/progress, tool name/category/status/duration, permission-needed state, safe affected paths, bounded command/test category and outcome | tool payload content, raw argv/stdout/stderr, source/diffs |
| `conversation` | no; explicit per-workspace or per-session opt-in with preview | bounded user-authored prompt text and visible assistant messages, each as an independently classified event | transcript files, system/developer prompts, hidden reasoning, source/diffs/tool-result content |

An adapter may capture a vendor event transiently in memory only long enough to
classify and transform it into the selected profile. It must discard fields that
are not allowed by that profile before durable local storage or network enqueue.
It must not tail or upload vendor transcript files as a shortcut.

## 3. Always-prohibited data

No profile may collect, persist, transmit, index, display to teammates, or send
to a model:

- environment names paired with values, `.env` files or variants, credentials,
  API/session tokens, cookies, private keys, keychain/credential-store data;
- source or diff content, Git objects, binary/file contents, raw tool results,
  raw command lines, stdout/stderr, test logs, stack traces, or screenshots;
- vendor, organization, developer, or system prompts; hidden chain of thought or
  unsupported internal reasoning; or
- data from protected paths such as credential, SSH, cloud, package-auth, and
  secret-management locations.

A protected-path or secret classification rejects the entire candidate event.
Redaction is defense in depth and never converts prohibited data into allowed
data. A conversation event containing source/diff-like blocks, credentials, or
other prohibited content is rejected rather than partially shared. Reasoning
summaries may be evaluated only in an isolated local spike; they
are not a production sharing type without another owner-approved ADR.

## 4. Consent and controls

- The Project owner enables which optional profiles are available; each member
  independently opts in for their workspace/session. The narrower choice wins.
- Consent is versioned by adapter, profile, destination, fields, and retention.
  Adapter/schema expansion requires renewed consent.
- Before enabling `conversation`, show representative fields and a local preview.
- Pause or downgrade takes effect synchronously before success returns and stops
  new enqueue. Already shared items remain visible only for their disclosed
  retention window and can be deleted by the member or Project owner.
- The member can inspect exactly what their device shared, with source/fidelity,
  audience, capture time, expiry, and rejection reason for locally blocked data.
- Unsupported or disconnected adapters degrade to Git/manual fidelity and never
  imply that an agent is being observed.

## 5. Adapter contract

Use supported, documented surfaces only. Codex App Server/SDK evaluation applies
to sessions connected through that surface; Claude hooks may cover independently
started supported Claude Code sessions. Arbitrary process scanning, memory
inspection, or undocumented session-store parsing is not a production adapter.

All adapters normalize vendor events into a vendor-neutral local candidate before
policy. Required provenance includes adapter/version, session/workstream mapping,
event kind, observed time, fidelity, and whether content was transformed or
omitted. Vendor session IDs remain local aliases; hosted events use Stickguy IDs.

The adapter must fail closed on ambiguous workspace mapping, unknown event types,
oversized content, protected paths, scanner failure, policy-version mismatch, or
missing consent. Observation cannot block or alter the coding agent.

## 6. Validation gate before production

L5A must use isolated synthetic projects and prove the current Codex and Claude
surfaces separately. It records only capability names, normalized fixture values,
counts, hashes, and pass/fail evidence. It must prove:

1. supported session/turn/message/plan/tool/subagent/permission/file-path and
   verification events, with honest absent/unsupported outcomes;
2. independently started versus adapter-connected session coverage;
3. exact profile projection and unknown-field rejection;
4. `.env` variants, inline tokens, credential paths, source/diff/tool results,
   transcript paths, raw command/output, system prompts, and reasoning content
   never reach durable storage or a sender;
5. opt-in, preview, downgrade, pause, deletion, retention, and Project isolation;
6. adapter removal/config drift behavior without weakening agent permissions; and
7. no production collection/upload until shared schemas, generated code, UI copy,
   and security tests are reviewed after the spike.

If a vendor surface requires Stickguy to own the coding loop, the gate narrows to
hooks/MCP/Git/manual observation or proposes a separate architecture ADR. It does
not silently supersede the coordination-harness boundary.
