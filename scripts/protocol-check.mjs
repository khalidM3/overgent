import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { generate, root } from "./protocol-lib.mjs";

const scratch = mkdtempSync(join(tmpdir(), "stickguy-protocol-check-"));
const outputs = [
  [join(scratch, "types.gen.go"), resolve(root, "protocol/generated/go/types.gen.go")],
  [join(scratch, "schema.d.ts"), resolve(root, "protocol/generated/typescript/schema.d.ts")],
];

try {
  generate(outputs[0][0], outputs[1][0]);
  const drift = outputs.filter(([actual, expected]) =>
    readFileSync(actual).compare(readFileSync(expected)) !== 0,
  );
  if (drift.length > 0) {
    console.error(`generated protocol drift: ${drift.map(([, file]) => file).join(", ")}`);
    process.exitCode = 1;
  }
} finally {
  rmSync(scratch, { recursive: true, force: true });
}
