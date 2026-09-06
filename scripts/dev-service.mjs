import { mkdirSync } from "node:fs";
import { spawn, spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { devConfigRoot } from "./dev-profile.mjs";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
mkdirSync(path.join(root, "bin"), { recursive: true });
const build = spawnSync("go", ["build", "-o", path.join(root, "bin", "overgent"), "./cmd/overgent"], { cwd: root, stdio: "inherit" });
if (build.status !== 0) process.exit(build.status ?? 1);
const child = spawn(path.join(root, "bin", "overgent"), ["--config-root", devConfigRoot(), "service", "run"], { cwd: root, stdio: "inherit" });
for (const signal of ["SIGINT", "SIGTERM"]) process.on(signal, () => child.kill(signal));
child.on("exit", (code) => { process.exitCode = code ?? 0; });
