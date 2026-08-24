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
- Agent adapters are event- and field-allowlisted by sharing profile. The activity/v1 hook process reduces vendor input immediately to a hashed session workstream, lifecycle/status, generated action label, allowlisted tool/subagent metadata, and safe repository-relative paths. Ignore transcript paths; reject protected paths and secret-bearing candidates as whole events; discard disallowed vendor fields before durable local storage or enqueue. Optional conversation processing remains disabled and requires the versioned owner/member consent and controls in `agent-activity-sharing.md`.
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
| Opt-in conversation | bounded user-authored prompt and visible assistant-message events | Owner-enabled plus member opt-in; locally classified; Project-authorized; inspectable/deletable; bounded retention. |
| Prohibited V1 | source/diff/file/tool-result content, Git objects, transcript files, system/developer prompts, hidden reasoning, env values, `.env` variants, credentials/tokens, raw commands/output/test logs | Never persist/transmit; reject the whole candidate event. |

Secret scanning is defense in depth and never justifies prohibited collection.

## 4. Privacy UX

Enrollment explains default coordination metadata leaving the device, including summaries being embedded by the configured provider. Optional agent activity remains off until the Project owner enables a profile and the member separately opts in with a representative local preview. UI always shows the effective profile, audience, sharing/fidelity, and semantic-processing degradation. Pause/downgrade is immediate from CLI/dashboard. Members can inspect and delete exactly what they shared, inspect recent events and semantic objects/findings, revoke devices, and clear local state. Owners can remove members/revoke invites. Retention/deletion is visible.

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
- agent-profile default-off/consent-version tests; Project/member narrower-setting precedence; `.env` variants, protected paths, inline tokens, transcript/system/reasoning/source/diff/raw-output rejection before storage and enqueue; preview/pause/downgrade/deletion/retention; unknown vendor events fail closed; cross-Project activity isolation.
