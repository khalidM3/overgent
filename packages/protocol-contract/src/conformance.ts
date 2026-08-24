import type { components } from "../../../protocol/generated/typescript/schema.js";

export type EventEnvelope = components["schemas"]["EventEnvelope"];

export function isKnownEventSource(source: string): source is EventEnvelope["source"] {
  return ["git", "manual", "mcp", "hook", "adapter/v1"].includes(source);
}
