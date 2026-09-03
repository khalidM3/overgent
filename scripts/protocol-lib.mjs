import { mkdirSync, mkdtempSync, rmSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

export const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");

function run(command, args) {
  const result = spawnSync(command, args, { cwd: root, stdio: "inherit" });
  if (result.error) throw result.error;
  if (result.status !== 0) process.exit(result.status ?? 1);
}

export function generate(goOutput, typeScriptOutput) {
  mkdirSync(dirname(goOutput), { recursive: true });
  mkdirSync(dirname(typeScriptOutput), { recursive: true });
  const scratch = mkdtempSync(join(tmpdir(), "overgent-openapi-bundle-"));
  const bundled = join(scratch, "openapi.yaml");
  try {
    run("pnpm", ["exec", "redocly", "bundle", "protocol/openapi.yaml", "--dereferenced", "--output", bundled]);
    run("go", [
      "run",
      "github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen",
      "-generate", "types",
      "-package", "protocoltypes",
      "-o", goOutput,
      bundled,
    ]);
    run("pnpm", ["exec", "openapi-typescript", bundled, "--output", typeScriptOutput]);
  } finally {
    rmSync(scratch, { recursive: true, force: true });
  }
}
