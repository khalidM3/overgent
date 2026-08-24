# L6C automatic agent-session evidence

Date: 2026-08-24
Outcome: PASS for local Codex and Claude Code activity/v1; explicit limits below

## Delivered proof

- Selecting a Codex or Claude Code adapter structurally installs the existing
  Project MCP entry plus exact project-local lifecycle hook groups. Unrelated
  configuration is preserved, setup is idempotent, drift fails closed, and
  removal deletes only Stickguy-managed groups.
- Hook input is bounded to 256 KiB and transformed in the short-lived hook
  process. Only vendor, hashed session workstream/alias, allowlisted lifecycle
  status, a Stickguy-generated action label, allowlisted tool/subagent metadata,
  and safe repository-relative paths cross the loopback IPC boundary.
- The one per-user service resolves a nested session cwd to exactly one
  registered repository and durably queues a schema-v1
  `agent.activity_reported` envelope. Protected/escaping paths reject the whole
  candidate without blocking the coding agent.
- Hosted projection creates a distinct workstream per session in one checkout.
  Two active session path sets that overlap create a deterministic
  `direct_collision` finding with hook/path provenance.
- Dashboard snapshots render vendor/session identity, active/waiting/idle/error
  status, current action, current tool, active subagent count, and safe paths.
  The onboarding UI no longer requires branches or linked worktrees.

## Reproducible verification

All data in the live suite was synthetic and the deployment was loopback-only.

```text
go test ./...                                      PASS
go vet ./...                                       PASS
go test -race ./...                                PASS
pnpm typecheck                                     PASS
pnpm test                                          PASS
pnpm build                                         PASS
pnpm protocol:check                                PASS
pnpm --dir convex test:live                        PASS
```

The live suite created two authenticated devices in one synthetic Project,
published an active Codex session and active Claude session with the same safe
path, loaded the authorized dashboard snapshot, and asserted:

```text
automaticAgentSessionsVisible: true
sameCheckoutAgentPathCollision: true
secondManifestAndFindingMs: 21
atomicManifest1000PathsMs: 72
```

The enrolled local test repository was updated through the reviewed setup
boundary. Status reported Codex hooks `active` and Claude configuration
`required_by_claude`; no credential, prompt, transcript, source, diff, or
environment value was printed or retained as evidence.

## Security/privacy review

- No source/diff, Git object, raw prompt/message, transcript path/content,
  system/developer prompt, reasoning, environment value, raw command/output,
  or tool response is a protocol field or durable local/hosted field.
- `.env`, `.env.*`, credential/SSH/cloud/package-auth paths, key material, and
  repository escapes reject the complete candidate before enqueue.
- Vendor session and subagent IDs are domain-separated SHA-256 aliases; raw IDs
  never leave the hook process.
- Hooks are passive. They run asynchronously where supported, return no agent
  decision, and a missing service or rejected candidate exits without changing
  tool execution.
- Hosted validation independently rejects prohibited keys, invalid enums,
  non-relative/protected paths, oversized arrays, mixed workspace batches,
  unauthorized Projects/devices, and unknown event types.

## Honest limits

- Existing sessions must restart once after setup so the vendor loads project
  hooks. Codex also applies its normal project trust/review flow; Claude applies
  its normal project settings/MCP approval flow.
- Codex documentation notes that hosted and specialized tool paths may not use
  local tool hooks. Those sessions still show lifecycle, and Git remains the
  combined-checkout fallback; Stickguy does not invent per-agent attribution.
- A tool event without an explicit safe path (for example a raw shell command)
  shows tool-category activity but does not claim a path.
- Conversation text, visible assistant messages, arbitrary Claude chats, system
  prompts, and hidden reasoning are not collected in activity/v1. Adding any
  content-bearing profile remains a separate consent/security gate.
