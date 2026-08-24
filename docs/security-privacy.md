# Stickguy — Security and Privacy Requirements

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
- Agent hooks are event-allowlisted. Ignore transcript paths and prohibit default hooks whose payload exposes prompts, patches, command/test output, or assistant messages; broader processing requires explicit adapter ADR/consent.
- Opaque public IDs; separate dev/preview/prod data and credentials.
- Audit security events without secrets; implement retention and deletion jobs.

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
| Prohibited V1 | source/diff content, Git objects, raw transcript, system prompt, env values, raw command/test output | Never collect/transmit. |

Secret scanning is defense in depth and never justifies prohibited collection.

## 4. Privacy UX

Enrollment explains metadata leaving the device, including summaries being embedded by the configured provider. UI always shows sharing/fidelity and semantic-processing degradation. Pause is immediate from CLI/dashboard. Members can inspect their recent events, semantic objects/findings, revoke devices, and clear local state. Owners can remove members/revoke invites. Retention/deletion is visible.

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
