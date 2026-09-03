# Overgent — Security and Privacy Requirements

Status: mandatory  
Last updated: 2026-08-23

## 1. Threats

Protect against token theft/replay, unauthorized project access, hostile websites targeting localhost, path/symlink escape, shell/argument injection, accidental secret/source/prompt/test-output collection, malicious semantic text/prompt injection, context poisoning/misdirection, abusive clients, cross-project/vector/context retrieval leakage, tampered installers/updates, and log/analytics leakage.

## 2. Mandatory controls

### Local

- Credentials in OS keychain, never config files; state/IPC current-user only.
- Loopback only; validate Host/Origin and require bearer auth.
- Execute Git without shell; validate refs/patterns and canonicalize roots.
- Reject symlink escape; structurally redact tokens before logging.
- Pause stops payload transmission synchronously before success returns.

### Hosted

- Authenticate/authorize every public operation by project and role.
- Hash secrets; support expiry, rotation, revocation.
- Rate-limit enrollment, heartbeat, batch, invite, ticket exchange.
- Validate sizes/counts/strings at first boundary; deduplicate transactionally.
- Authorize before semantic retrieval and before loading matched objects; filter by project/repository and test isolation.
- Treat embedding/adjudication input as untrusted data, require structured outputs, and grant models no tools or authority.
- Authorize every context item before ranking and again on item fetch; never use semantic relevance as authorization.
- Project membership and the synchronous pause switch govern adapter sharing
  (ADR-047). The activity hook reduces vendor input to a hashed session
  workstream, lifecycle/status, generated action label, allowlisted
  tool/subagent metadata, and safe repository-relative paths. Session messages
  are independently classified before enqueue. Reject protected paths and
  secret-bearing candidates whole; discard disallowed vendor fields before
  durable local storage or enqueue.
- A vendor-visible session title may seed automatic intent only after the local
  ADR-042 classifier and hosted semantic policy both accept it. The UI discloses
  that an approved title may reach the configured embedding provider; a rejected
  title never blocks safe lifecycle/path observation.
- Agent profile reconnect recognizes only Overgent's exact managed MCP/hook
  shapes, previews the previous and target local profiles, and requires explicit
  confirmation before detaching another profile. Both files are snapshotted for
  rollback, unrelated provider configuration is preserved, and unknown or
  conflicting managed-looking entries fail closed. Runtime verification stores
  only workspace ID, vendor, and observation time in local SQLite.
- Opaque public IDs; separate dev/preview/prod data and credentials.
- Audit security events without secrets; implement retention and deletion jobs.

Hosted activity, findings, semantic objects, deliveries, and shared session
messages default to 30-day retention and are removed by bounded expiry jobs.
Project owners can export or delete the Project; ordinary members can export
records about their own work and leave with deletion of their retained records.
Authorization is revoked synchronously before batched deletion starts. Deletion
removes rows and orphan devices rather than hiding them behind a Project flag.

### Supply chain

- Pin dependencies; SBOM and signed artifacts/update metadata.
- Minimal release workflow permissions and provenance.
- Go/frontend vulnerability and secret scans in CI.

## 3. Data classification

| Class | Examples | Policy |
|---|---|---|
| Secret | device token, invite secret, API keys | Keychain/secret store only; never logs/analytics. |
| Sensitive metadata | repo identity, paths/symbols/dependencies, intent/change/verification summaries, embeddings, findings, context deliveries/acknowledgements | Project-authorized, disclosed processing, bounded retention/deletion. |
| Durable | plans, decisions, resolved sync cards | Project lifetime or deletion. |
| Ephemeral | heartbeat/local health | Compact and expire quickly. |
| Project conversation | bounded user-authored prompt and visible assistant-message events | Membership-authorized; locally classified; pausable, inspectable/deletable, and bounded by hosted retention. |
| Prohibited V1 | source/diff/file/tool-result content, Git objects, transcript files, system/developer prompts, hidden reasoning, env values, `.env` variants, credentials/tokens, raw commands/output/test logs | Never persist/transmit; reject the whole candidate event. |

Secret scanning is defense in depth and never justifies prohibited collection.

## 4. Privacy UX

Enrollment explains that connecting an adapter shares classifier-passing
activity and session context with authorized Project members, including which
approved summaries may reach the configured semantic provider. The UI shows
sharing/fidelity and semantic-processing degradation. Pause is immediate from
CLI/dashboard. Members can inspect and delete their shared messages, inspect
recent events and semantic objects/findings, revoke devices, and clear local
state. Owners can remove members, revoke invites, and delete shared messages.
Retention/deletion is visible.

## 5. Required security tests

- expired/reused/revoked/brute-forced invites;
- revoked devices and cross-project access on every endpoint;
- duplicate/out-of-order/oversized batches;
- malicious paths/refs, hostile localhost Origin/Host, missing local token;
- symlink escape and token-redaction log snapshots;
- pause with queued events;
- tampered update/checksum;
- retention expiry and project deletion.
- cross-project/repository vector retrieval and deleted/superseded embeddings;
- prompt-injection summaries, code/secret-like summary rejection, malformed model output, and provider outage.
- unauthorized/unrelated context omission, critical-item truncation references, forged/stale brief IDs, and acknowledgement replay.
- zero-step membership sharing; `.env` contents, protected paths, inline tokens,
  raw transcript/tool/command output, environment assignments, credentials, and
  private keys rejected before storage and enqueue; pause, member/owner
  deletion, retention, unknown-vendor failure, and cross-Project isolation.
