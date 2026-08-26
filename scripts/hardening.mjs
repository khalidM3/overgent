import { spawnSync } from "node:child_process";
import process from "node:process";

function run(command, args) {
  const result = spawnSync(command, args, { stdio: "inherit", env: process.env });
  if (result.error) throw result.error;
  if (result.status !== 0) process.exit(result.status ?? 1);
}

run("go", ["test", "-race", "./internal/store", "./internal/codexsetup", "./internal/claudesetup", "./internal/update", "./internal/service", "./internal/agentactivity"]);
run("pnpm", ["protocol:check"]);
run("pnpm", ["test"]);

if (process.argv.includes("--live")) {
  const iterations = Number.parseInt(process.env.STICKGUY_HARDENING_ITERATIONS ?? "5", 10);
  if (!Number.isSafeInteger(iterations) || iterations < 1 || iterations > 100) {
    throw new Error("STICKGUY_HARDENING_ITERATIONS must be between 1 and 100");
  }
  for (let iteration = 1; iteration <= iterations; iteration++) {
    console.log(`L8 live hardening iteration ${iteration}/${iterations}`);
    run("pnpm", ["--dir", "convex", "test:live"]);
  }
}
