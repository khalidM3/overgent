import type { components } from "../../protocol/generated/typescript/schema.js";

export type EventEnvelope = components["schemas"]["EventEnvelope"];
export type EventType = EventEnvelope["type"];
export type ChangeStatus = "added" | "modified" | "deleted" | "renamed" | "copied" | "untracked";
export type ManifestChange = { status: ChangeStatus; oldPath?: string };
export type ManifestEntry = {
  path: string;
  states: { baseline?: ManifestChange; index?: ManifestChange; worktree?: ManifestChange };
  symbols?: string[];
  dependencies?: string[];
};
const MANIFEST_LAYERS = ["baseline", "index", "worktree"] as const;

export const LIMITS = Object.freeze({
  requestBytes: 256 * 1024,
  eventBatchCount: 100,
  eventPayloadProperties: 24,
  manifestChunks: 1_000,
  manifestEntriesPerChunk: 100,
  pathLength: 512,
  summaryLength: 2_000,
  changesPage: 100,
  briefItems: 64,
});

// Child/vector rows precede their owning rows so bounded sweeps cannot leave a
// newly orphaned record when a batch stops at its write limit.
export const RETENTION_TABLES = [
  "invites",
  "dashboardTickets",
  "browserSessions",
  "changeManifestChunks",
  "changeManifests",
  "activityEvents",
  "findingFeedback",
  "findings",
  "semanticEmbeddings",
  "semanticObjects",
  "contextDeliveries",
  "rateLimits",
] as const;

const ID = /^[a-z][a-z0-9_]{2,127}$/;
const HASH = /^[0-9a-f]{64}$/;
const EVENT_TYPES = new Set<EventType>([
  "workspace.registered",
  "workspace.manifest_started",
  "workspace.manifest_chunk",
  "workspace.manifest_completed",
  "workspace.paused",
  "workspace.resumed",
  "workstream.intent_reported",
  "workstream.checkpoint_reported",
  "workstream.status_changed",
  "context.acknowledged",
  "activity.reported",
  "agent.activity_reported",
  "claim.created",
  "claim.released",
]);
const SOURCES = new Set(["git", "manual", "mcp", "hook", "adapter/v1"]);
const PROHIBITED_KEYS = /^(source(Content)?|diff|patch|blob|gitObject|transcript|systemPrompt|prompt|environment|env|raw(Command|Output|Log|TestOutput))$/i;

export class ValidationError extends Error {
  constructor(readonly code: string, message = code) {
    super(message);
    this.name = "ValidationError";
  }
}

export function expectObject(value: unknown, code = "validation_failed"): Record<string, unknown> {
  if (typeof value !== "object" || value === null || Array.isArray(value)) throw new ValidationError(code);
  return value as Record<string, unknown>;
}

export function expectExactKeys(
  value: Record<string, unknown>,
  required: readonly string[],
  optional: readonly string[] = [],
): void {
  const allowed = new Set([...required, ...optional]);
  if (required.some((key) => !(key in value)) || Object.keys(value).some((key) => !allowed.has(key))) {
    throw new ValidationError("validation_failed");
  }
}

export function expectString(value: unknown, minimum: number, maximum: number, code = "validation_failed"): string {
  if (typeof value !== "string" || value.length < minimum || value.length > maximum) {
    throw new ValidationError(code);
  }
  return value;
}

export function expectInteger(value: unknown, minimum: number, maximum: number, code = "validation_failed"): number {
  if (!Number.isInteger(value) || (value as number) < minimum || (value as number) > maximum) {
    throw new ValidationError(code);
  }
  return value as number;
}

export function expectId(value: unknown): string {
  const id = expectString(value, 3, 128);
  if (!ID.test(id)) throw new ValidationError("validation_failed");
  return id;
}

export function expectTimestamp(value: unknown): string {
  const timestamp = expectString(value, 20, 64);
  if (!Number.isFinite(Date.parse(timestamp))) throw new ValidationError("validation_failed");
  return timestamp;
}

export function validateEventBatch(value: unknown): EventEnvelope[] {
  const body = expectObject(value);
  expectExactKeys(body, ["events"]);
  if (!Array.isArray(body.events) || body.events.length < 1 || body.events.length > LIMITS.eventBatchCount) {
    throw new ValidationError("batch_count_out_of_range");
  }
  const events = body.events.map(validateEventEnvelope);
  const first = events[0];
  if (events.some((event) => event.projectId !== first.projectId || event.deviceId !== first.deviceId || event.workspaceId !== first.workspaceId)) {
    throw new ValidationError("mixed_event_batch");
  }
  return events;
}

export function canActivateManifestRevision(currentRevision: number | undefined, nextRevision: number): boolean {
  return currentRevision === undefined || nextRevision > currentRevision;
}

export function validateEventEnvelope(value: unknown): EventEnvelope {
  const event = expectObject(value);
  expectExactKeys(event, [
    "schemaVersion", "eventId", "projectId", "memberId", "deviceId", "workspaceId", "sessionId",
    "sequence", "observedAt", "sentAt", "source", "type", "payload",
  ]);
  if (event.schemaVersion !== 1) throw new ValidationError("schema_version_unsupported");
  const type = event.type;
  if (typeof type !== "string" || !EVENT_TYPES.has(type as EventType)) throw new ValidationError("event_type_unsupported");
  if (typeof event.source !== "string" || !SOURCES.has(event.source)) throw new ValidationError("validation_failed");
  const payload = expectObject(event.payload);
  if (Object.keys(payload).length > LIMITS.eventPayloadProperties) throw new ValidationError("event_payload_too_large");
  rejectProhibitedData(payload);
  validatePayload(type as EventType, payload);
  return {
    schemaVersion: 1,
    eventId: expectId(event.eventId),
    projectId: expectId(event.projectId),
    memberId: expectId(event.memberId),
    deviceId: expectId(event.deviceId),
    workspaceId: expectId(event.workspaceId),
    sessionId: expectId(event.sessionId),
    sequence: expectInteger(event.sequence, 1, Number.MAX_SAFE_INTEGER),
    observedAt: expectTimestamp(event.observedAt),
    sentAt: expectTimestamp(event.sentAt),
    source: event.source as EventEnvelope["source"],
    type: type as EventType,
    payload: payload as EventEnvelope["payload"],
  } as unknown as EventEnvelope;
}

function validatePayload(type: EventType, payload: Record<string, unknown>): void {
  switch (type) {
    case "workspace.registered": {
      expectExactKeys(payload, ["repoFingerprint", "label", "capabilities"]);
      expectString(payload.repoFingerprint, 1, 256);
      expectString(payload.label, 1, 120);
      const capabilities = expectObject(payload.capabilities);
      if (Object.keys(capabilities).length > 16 || Object.values(capabilities).some((entry) =>
        typeof entry !== "boolean" && typeof entry !== "string" && !Array.isArray(entry))) {
        throw new ValidationError("validation_failed");
      }
      return;
    }
    case "workspace.manifest_started": {
      expectExactKeys(payload, ["manifestId", "revision", "workstreamId", "baselineRef", "headRef", "chunkCount"]);
      expectManifestId(payload.manifestId);
      expectWorkstreamId(payload.workstreamId);
      expectInteger(payload.revision, 1, Number.MAX_SAFE_INTEGER);
      expectGitRef(payload.baselineRef);
      expectGitRef(payload.headRef);
      expectInteger(payload.chunkCount, 0, LIMITS.manifestChunks, "chunk_count_out_of_range");
      return;
    }
    case "workspace.manifest_chunk": {
      expectExactKeys(payload, ["manifestId", "chunkIndex", "entries"]);
      expectManifestId(payload.manifestId);
      expectInteger(payload.chunkIndex, 0, LIMITS.manifestChunks - 1, "chunk_index_out_of_range");
      if (!Array.isArray(payload.entries) || payload.entries.length < 1 || payload.entries.length > LIMITS.manifestEntriesPerChunk) {
        throw new ValidationError("path_count_out_of_range");
      }
      payload.entries.forEach(validateManifestEntry);
      return;
    }
    case "workspace.manifest_completed": {
      expectExactKeys(payload, ["manifestId", "revision", "contentHash"]);
      expectManifestId(payload.manifestId);
      expectInteger(payload.revision, 1, Number.MAX_SAFE_INTEGER);
      const hash = expectString(payload.contentHash, 64, 64);
      if (!HASH.test(hash)) throw new ValidationError("validation_failed");
      return;
    }
    case "workspace.paused":
      expectExactKeys(payload, [], ["reason"]);
      if (payload.reason !== undefined) expectString(payload.reason, 1, 300);
      return;
    case "workspace.resumed":
      expectExactKeys(payload, []);
      return;
    case "workstream.intent_reported":
      expectExactKeys(payload, ["workstreamId", "title", "intendedOutcome"], ["approachSummary", "components", "contracts", "anticipatedPaths", "planItemIds"]);
      expectWorkstreamId(payload.workstreamId);
      expectString(payload.title, 1, 160);
      expectString(payload.intendedOutcome, 1, LIMITS.summaryLength);
      if (payload.approachSummary !== undefined) expectString(payload.approachSummary, 1, LIMITS.summaryLength);
      validateBoundedStrings(payload.components, 32, 160);
      validateBoundedStrings(payload.contracts, 32, 160);
      validateBoundedStrings(payload.anticipatedPaths, 100, LIMITS.pathLength);
      validateBoundedStrings(payload.planItemIds, 32, 128);
      return;
    case "workstream.status_changed":
      expectExactKeys(payload, ["workstreamId", "status"]);
      expectWorkstreamId(payload.workstreamId);
      if (!["active", "idle", "done", "blocked"].includes(String(payload.status))) throw new ValidationError("validation_failed");
      return;
    case "workstream.checkpoint_reported":
      expectExactKeys(payload, ["checkpointId", "workstreamId", "summary"], ["discoveries", "verification", "relatedManifestRevision", "basedOnBriefId"]);
      expectId(payload.checkpointId);
      expectWorkstreamId(payload.workstreamId);
      expectString(payload.summary, 1, LIMITS.summaryLength);
      validateBoundedStrings(payload.discoveries, 32, 500);
      return;
    case "context.acknowledged":
      expectExactKeys(payload, ["briefId", "consideredItemIds"]);
      expectId(payload.briefId);
      validateBoundedStrings(payload.consideredItemIds, 64, 128, true);
      return;
    case "activity.reported":
      expectExactKeys(payload, ["kind", "summary"]);
      if (!["decision", "completion", "blocker"].includes(String(payload.kind))) throw new ValidationError("validation_failed");
      expectString(payload.summary, 1, LIMITS.summaryLength);
      return;
    case "agent.activity_reported": {
      expectExactKeys(payload, ["workstreamId", "vendor", "sessionAlias", "kind", "status", "action"], ["tool", "agentType", "subagentAlias", "paths"]);
      expectWorkstreamId(payload.workstreamId);
      if (payload.vendor !== "codex" && payload.vendor !== "claude") throw new ValidationError("validation_failed");
      const alias = expectString(payload.sessionAlias, 12, 13);
      if (!/^(codex|claude)-[0-9a-f]{6}$/.test(alias)) throw new ValidationError("validation_failed");
      if (!["SessionStart", "UserPromptSubmit", "PreToolUse", "PermissionRequest", "PostToolUse", "PostToolUseFailure", "SubagentStart", "SubagentStop", "Stop", "SessionEnd"].includes(expectString(payload.kind, 1, 32))) throw new ValidationError("validation_failed");
      if (!["active", "waiting", "idle", "done", "error"].includes(expectString(payload.status, 1, 16))) throw new ValidationError("validation_failed");
      expectString(payload.action, 1, 300);
      for (const key of ["tool", "agentType"] as const) {
        if (payload[key] !== undefined && !/^[A-Za-z][A-Za-z0-9._:-]{0,63}$/.test(expectString(payload[key], 1, 64))) throw new ValidationError("validation_failed");
      }
      if (payload.subagentAlias !== undefined && !/^sub-[0-9a-f]{6}$/.test(expectString(payload.subagentAlias, 10, 10))) throw new ValidationError("validation_failed");
      validateBoundedStrings(payload.paths, 100, LIMITS.pathLength);
      for (const path of Array.isArray(payload.paths) ? payload.paths : []) validateAgentPath(String(path));
      return;
    }
    case "claim.created":
      expectExactKeys(payload, ["workstreamId", "patterns"]);
      expectWorkstreamId(payload.workstreamId);
      validateBoundedStrings(payload.patterns, 32, LIMITS.pathLength, true);
      return;
    case "claim.released":
      expectExactKeys(payload, ["workstreamId"], ["claimIds", "patterns"]);
      expectWorkstreamId(payload.workstreamId);
      validateBoundedStrings(payload.claimIds, 32, 128);
      validateBoundedStrings(payload.patterns, 32, LIMITS.pathLength);
  }
}

function validateAgentPath(value: string): void {
  if (!value || value.startsWith("/") || value.includes("\\") || value.includes("\0") || value.split("/").includes("..")) throw new ValidationError("protected_path");
  const segments = value.toLowerCase().split("/");
  const protectedNames = new Set([".ssh", ".aws", ".azure", ".kube", ".npmrc", ".pypirc", ".git-credentials", "credentials", "secrets", "id_rsa", "id_ed25519"]);
  if (segments.some((segment) => segment === ".env" || segment.startsWith(".env.") || protectedNames.has(segment) || segment.endsWith(".pem") || segment.endsWith(".key"))) throw new ValidationError("protected_path");
  if (value.toLowerCase().includes(".config/gcloud")) throw new ValidationError("protected_path");
}

function validateManifestEntry(value: unknown): void {
  const entry = expectObject(value);
  expectExactKeys(entry, ["path", "states"], ["symbols", "dependencies"]);
  expectString(entry.path, 1, LIMITS.pathLength);
  const states = expectObject(entry.states);
  expectExactKeys(states, [], MANIFEST_LAYERS);
  if (Object.keys(states).length === 0) throw new ValidationError("manifest_states_empty");
  for (const layer of MANIFEST_LAYERS) {
    if (states[layer] === undefined) continue;
    const change = expectObject(states[layer]);
    expectExactKeys(change, ["status"], ["oldPath"]);
    if (!["added", "modified", "deleted", "renamed", "copied", "untracked"].includes(String(change.status))) {
      throw new ValidationError("validation_failed");
    }
    if (change.oldPath !== undefined) {
      expectString(change.oldPath, 1, LIMITS.pathLength);
      if (change.status !== "renamed" && change.status !== "copied") throw new ValidationError("old_path_status_invalid");
    }
  }
  validateBoundedStrings(entry.symbols, 64, 160);
  validateBoundedStrings(entry.dependencies, 64, 160);
}

function validateBoundedStrings(value: unknown, maximumCount: number, maximumLength: number, required = false): void {
  if (value === undefined && !required) return;
  if (!Array.isArray(value) || (required && value.length < 1) || value.length > maximumCount ||
    value.some((entry) => typeof entry !== "string" || entry.length < 1 || entry.length > maximumLength)) {
    throw new ValidationError("validation_failed");
  }
}

function expectManifestId(value: unknown): string {
  const id = expectString(value, 5, 84);
  if (!/^mft_[A-Za-z0-9_-]{1,80}$/.test(id)) throw new ValidationError("validation_failed");
  return id;
}

function expectWorkstreamId(value: unknown): string {
  const id = expectString(value, 5, 84);
  if (!/^wrk_[A-Za-z0-9_-]{1,80}$/.test(id)) throw new ValidationError("validation_failed");
  return id;
}

function expectGitRef(value: unknown): string {
  const ref = expectString(value, 40, 64);
  if (!/^[0-9a-f]+$/.test(ref)) throw new ValidationError("validation_failed");
  return ref;
}

function rejectProhibitedData(value: unknown): void {
  if (Array.isArray(value)) {
    value.forEach(rejectProhibitedData);
    return;
  }
  if (typeof value !== "object" || value === null) return;
  for (const [key, nested] of Object.entries(value as Record<string, unknown>)) {
    if (PROHIBITED_KEYS.test(key)) throw new ValidationError("prohibited_data");
    rejectProhibitedData(nested);
  }
}

export function manifestContentHash(entries: readonly ManifestEntry[]): string {
  return sha256Hex(entries.map((entry) => {
    const fields = [entry.path];
    for (const layer of MANIFEST_LAYERS) {
      const change = entry.states[layer];
      fields.push(layer, change?.status ?? "", change?.oldPath ?? "");
    }
    return `${fields.join("\0")}\0`;
  }).join(""));
}

export function assertCanonicalManifestOrder(entries: readonly ManifestEntry[]): void {
  for (let index = 1; index < entries.length; index++) {
    if (compareUtf8(entries[index - 1].path, entries[index].path) >= 0) {
      throw new ValidationError("manifest_path_order_invalid");
    }
  }
}

function compareUtf8(left: string, right: string): number {
  const leftBytes = new TextEncoder().encode(left);
  const rightBytes = new TextEncoder().encode(right);
  for (let index = 0; index < Math.min(leftBytes.length, rightBytes.length); index++) {
    if (leftBytes[index] !== rightBytes[index]) return leftBytes[index] - rightBytes[index];
  }
  return leftBytes.length - rightBytes.length;
}

export function scopeKey(projectId: string, repositoryId: string): string {
  return `scp_${sha256Hex(`${projectId}\0${repositoryId}`).slice(0, 48)}`;
}

export function sha256Hex(input: string): string {
  const bytes = new TextEncoder().encode(input);
  const words: number[] = [];
  for (let index = 0; index < bytes.length; index++) words[index >> 2] = (words[index >> 2] ?? 0) | bytes[index] << (24 - (index % 4) * 8);
  const bitLength = bytes.length * 8;
  words[bitLength >> 5] = (words[bitLength >> 5] ?? 0) | 0x80 << (24 - bitLength % 32);
  words[((bitLength + 64 >> 9) << 4) + 15] = bitLength;
  const constants = SHA256_CONSTANTS;
  let [h0, h1, h2, h3, h4, h5, h6, h7] = [0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19];
  for (let offset = 0; offset < words.length; offset += 16) {
    const schedule = new Array<number>(64);
    for (let index = 0; index < 16; index++) schedule[index] = words[offset + index] ?? 0;
    for (let index = 16; index < 64; index++) {
      const a = schedule[index - 15];
      const b = schedule[index - 2];
      const s0 = rotate(a, 7) ^ rotate(a, 18) ^ a >>> 3;
      const s1 = rotate(b, 17) ^ rotate(b, 19) ^ b >>> 10;
      schedule[index] = (schedule[index - 16] + s0 + schedule[index - 7] + s1) | 0;
    }
    let [a, b, c, d, e, f, g, h] = [h0, h1, h2, h3, h4, h5, h6, h7];
    for (let index = 0; index < 64; index++) {
      const s1 = rotate(e, 6) ^ rotate(e, 11) ^ rotate(e, 25);
      const choice = e & f ^ ~e & g;
      const temp1 = (h + s1 + choice + constants[index] + schedule[index]) | 0;
      const s0 = rotate(a, 2) ^ rotate(a, 13) ^ rotate(a, 22);
      const majority = a & b ^ a & c ^ b & c;
      const temp2 = (s0 + majority) | 0;
      [h, g, f, e, d, c, b, a] = [g, f, e, (d + temp1) | 0, c, b, a, (temp1 + temp2) | 0];
    }
    h0 = (h0 + a) | 0; h1 = (h1 + b) | 0; h2 = (h2 + c) | 0; h3 = (h3 + d) | 0;
    h4 = (h4 + e) | 0; h5 = (h5 + f) | 0; h6 = (h6 + g) | 0; h7 = (h7 + h) | 0;
  }
  return [h0, h1, h2, h3, h4, h5, h6, h7].map((word) => (word >>> 0).toString(16).padStart(8, "0")).join("");
}

function rotate(value: number, count: number): number {
  return value >>> count | value << 32 - count;
}

const SHA256_CONSTANTS = [
  0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
  0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
  0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
  0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
  0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
  0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
  0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
  0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
];

export function randomHex(byteCount: number): string {
  const bytes = new Uint8Array(byteCount);
  crypto.getRandomValues(bytes);
  return [...bytes].map((byte) => byte.toString(16).padStart(2, "0")).join("");
}

export function publicId(prefix: string): string {
  return `${prefix}_${randomHex(16)}`;
}

export type RetainedRecord = Readonly<{ id: string; expiresAt?: number; durable?: boolean }>;

export function expiredRecordIds(records: readonly RetainedRecord[], now: number, limit: number): string[] {
  return records
    .filter((record) => !record.durable && record.expiresAt !== undefined && record.expiresAt <= now)
    .sort((left, right) => left.expiresAt! - right.expiresAt! || left.id.localeCompare(right.id))
    .slice(0, Math.max(0, limit))
    .map((record) => record.id);
}
