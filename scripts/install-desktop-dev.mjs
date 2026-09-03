import { cp, mkdir, rename, rm } from "node:fs/promises";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import os from "node:os";
import path from "node:path";

if (process.platform !== "darwin") throw new Error("Overgent Dev.app is currently macOS-only");
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const build = spawnSync(process.execPath, [path.join(root, "scripts", "build-desktop.mjs"), "--development"], { cwd: root, stdio: "inherit" });
if (build.status !== 0) process.exit(build.status ?? 1);
const source = path.join(root, "apps", "desktop", "build", "bin", "Overgent Dev.app");
const applications = path.join(os.homedir(), "Applications");
const target = path.join(applications, "Overgent Dev.app");
const stage = path.join(applications, ".Overgent Dev.app.installing");
const backup = path.join(applications, ".Overgent Dev.app.previous");
await mkdir(applications, { recursive: true });
await rm(stage, { recursive: true, force: true });
await rm(backup, { recursive: true, force: true });
await cp(source, stage, { recursive: true, preserveTimestamps: true });
try { await rename(target, backup); } catch (error) { if (error.code !== "ENOENT") throw error; }
try {
  await rename(stage, target);
  await rm(backup, { recursive: true, force: true });
} catch (error) {
  try { await rename(backup, target); } catch {}
  throw error;
}
process.stdout.write(`${target}\nRun pnpm dev:ui before opening the installed development app.\n`);
