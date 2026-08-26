# Stickguy — Agent Activity Sharing

Status: activity/v1 enabled; session-share/v1 enabled only by explicit per-session consent under ADR-034
Last updated: 2026-08-24

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
| `activity` | no; explicitly selected per member/repository | session/turn/subagent lifecycle, Stickguy-generated current-action label, allowlisted tool name/category/status, permission-needed state, safe affected paths | prompt/message text, tool payload content, raw argv/stdout/stderr, source/diffs |
| `conversation` | no; explicit per-session opt-in with preview | bounded user-authored prompts, visible assistant messages, vendor-exposed reasoning summaries, and explicitly surfaced system instructions, each as an independently classified event | transcript files, hidden reasoning, source/diffs/tool-result content |

An adapter may capture a vendor event transiently in memory only long enough to
classify and transform it into the selected profile. It must discard fields that
are not allowed by that profile before durable local storage or network enqueue.
It must not tail or upload vendor transcript files as a shortcut.

Under ADR-042, the short vendor-visible session title already projected by
`activity/v1` may also seed an honestly labeled intent. It is independently
classified and bounded before upload, then passes hosted semantic policy before
storage or embedding. No transcript message or other prompt content is derived
or embedded by this path.

## 3. Always-prohibited data

No profile may collect, persist, transmit, index, display to teammates, or send
to a model:

- environment names paired with values, `.env` files or variants, credentials,
  API/session tokens, cookies, private keys, keychain/credential-store data;
- source or diff content, Git objects, binary/file contents, raw tool results,
  raw command lines, stdout/stderr, test logs, stack traces, or screenshots;
- hidden chain of thought or unsupported internal reasoning; or
- data from protected paths such as credential, SSH, cloud, package-auth, and
  secret-management locations.

A protected-path or secret classification rejects the entire candidate event.
Redaction is defense in depth and never converts prohibited data into allowed
data. A conversation event containing source/diff-like blocks, credentials, or
other prohibited content is rejected rather than partially shared. System
instructions and reasoning summaries are allowed only when the supported vendor
event exposes them directly, the local preview labels their kind, and the member
explicitly enables `session-share/v1` for that exact session. Stickguy does not
infer, reconstruct, scrape, or claim access to hidden reasoning.

## 4. Consent and controls

- The Project owner enables which optional profiles are available; each member
  independently opts in for their workspace/session. The narrower choice wins.
- Consent is versioned by adapter, profile, destination, fields, and retention.
  Adapter/schema expansion requires renewed consent.
- Before enabling `conversation`, show representative fields and a local preview;
  consent records the exact audience, allowed message kinds, expiry, adapter,
  and `session-share/v1` contract version.
- Pause or downgrade takes effect synchronously before success returns and stops
  new enqueue. Already shared items remain visible only for their disclosed
  retention window and can be deleted by the member or Project owner.
- The member can inspect exactly what their device shared, with source/fidelity,
  audience, capture time, expiry, and rejection reason for locally blocked data.
- Unsupported or disconnected adapters degrade to Git/manual fidelity and never
  imply that an agent is being observed.

## 5. Adapter contract

Use supported, documented surfaces only. Project-local Codex and Claude Code
hooks cover independently started supported sessions after the configuration is
loaded. Arbitrary process scanning, memory
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

ADR-033 concludes this gate for the bounded `activity/v1` projection. ADR-034
adds the reviewed `session-share/v1` projection without enabling transcript-file
ingestion or hidden reasoning. If a vendor surface requires Stickguy to own the coding loop, it narrows to
hooks/MCP/Git/manual observation or proposes a separate architecture ADR. It does
not silently supersede the coordination-harness boundary.

## Session content under ADR-036

Supported hooks do not carry assistant text, reasoning, or system instructions.
Only `UserPromptSubmit` carries content, so hook-only session detail is
structurally empty. Stickguy therefore reads the vendor transcript named by the
hook's own `transcript_path`.

- The read is local, bounded from the tail, and never copied to a second store.
  Only the path, the session title, and the branch are recorded.
- The session owner always sees their own session. Sharing is a separate,
  explicit, per-session choice with a preview, an audience, an expiry, and
  deletion on revoke.
- Parsed kinds are `user`, `assistant`, `thinking`, `system`, and `tool`. `tool`
  carries a name only and is never shareable content. Raw tool results,
  attachments, command output, and vendor-encrypted reasoning are dropped during
  parsing.
- Each vendor has its own adapter (ADR-039). Claude Code names its transcript in
  the hook payload; Codex does not, so its rollout is located by session id.
  Codex conversation comes from its `event_msg` stream, not the raw model I/O,
  so injected context is never shown as something a person wrote.
- Quoted code and diffs are allowed inside a consented conversation. Naming a
  configuration file is allowed; its contents are not (ADR-038). Environment
  assignments, credentials, tokens, private keys, raw tool results, and command
  output reject the whole message at both boundaries; nothing is redacted.
- Where a vendor records no reasoning, Stickguy shows none and claims none.
