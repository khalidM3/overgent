// Every recognized top-level export form appears exactly once.
import type { Something } from "./elsewhere";

const notExported = 1;

export function rotate<T extends { id: string }>(input: T, at: number): { ok: boolean } {
  return { ok: input.id.length > at };
}

export async function refresh(token: string): Promise<void> {
  await Promise.resolve(token);
}

export abstract class Store extends Base<{ key: string }> implements Lifecycle {
  /* a brace { inside a comment must not open a body */
  open(): void {}
}

export interface Session extends Something {
  id: string;
}

export type Mode =
  | "structural"
  | "semantic";

export type Predicate = (value: string) => boolean;

export const LIMIT: number = 512;

export const enum Layer {
  Baseline,
  Worktree,
}

export enum Fidelity {
  Structural,
  Semantic,
}

function alsoNotExported(): string {
  return `a } brace { inside a template literal is not structure`;
}

export default rotate;
