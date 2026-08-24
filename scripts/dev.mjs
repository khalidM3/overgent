import { existsSync, mkdirSync } from "node:fs";
import { spawn, spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import os from "node:os";
import path from "node:path";

if (process.platform !== "darwin") throw new Error("the full local development stack is currently validated only on macOS");
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const children = new Set();
const start = (name, command, args, options = {}) => {
  const child = spawn(command, args, { cwd: root, stdio: "inherit", ...options });
  child.devName = name;
  children.add(child);
  child.on("exit", () => children.delete(child));
  return child;
};
const stop = () => { for (const child of children) child.kill("SIGTERM"); };
process.on("SIGINT", stop);
process.on("SIGTERM", stop);

mkdirSync(path.join(root, "bin"), { recursive: true });
const cli = path.join(root, "bin", "stickguy");
const build = spawnSync("go", ["build", "-o", cli, "./cmd/stickguy"], { cwd: root, stdio: "inherit" });
if (build.status !== 0) process.exit(build.status ?? 1);

start("backend", "pnpm", ["dev:backend"]);
start("ui", "pnpm", ["dev:ui"]);
const uiURL = "http://127.0.0.1:5173/?desktop=onboarding";
for (let attempt = 0; attempt < 120; attempt++) {
  try { if ((await fetch("http://127.0.0.1:5173/", { signal: AbortSignal.timeout(500) })).ok) break; } catch {}
  await new Promise((resolve) => setTimeout(resolve, 250));
}

const desktopBuild = spawnSync(process.execPath, [path.join(root, "scripts", "build-desktop.mjs"), "--development"], { cwd: root, stdio: "inherit" });
if (desktopBuild.status !== 0) { stop(); process.exit(desktopBuild.status ?? 1); }
const desktopBinary = path.join(root, "apps", "desktop", "build", "bin", "Stickguy Dev.app", "Contents", "MacOS", "stickguy-desktop-dev");
start("desktop", desktopBinary, [], { env: { ...process.env, FRONTEND_DEVSERVER_URL: "http://127.0.0.1:5173", STICKGUY_API_ORIGIN: "http://127.0.0.1:3211", STICKGUY_DASHBOARD_ORIGIN: "http://127.0.0.1:5173/api", STICKGUY_CLI_BINARY: cli } });

const configPath = path.join(os.homedir(), "Library", "Application Support", "Stickguy", "config.json");
let service;
const ensureService = () => {
  if (!service && existsSync(configPath)) {
    const existing = spawnSync(cli, ["service", "status"], { cwd: root, stdio: "ignore" });
    if (existing.status === 0) {
      service = { external: true };
      process.stdout.write("An existing Stickguy service is already managing the enrolled development profile.\n");
      return;
    }
    service = start("service", cli, ["service", "run"]);
    service.on("exit", () => { service = undefined; });
    process.stdout.write("Stickguy local service started for the enrolled development profile.\n");
  }
};
ensureService();
const monitor = setInterval(ensureService, 1_000);
process.stdout.write(`\nStickguy development is running.\nUI hot reload: ${uiURL}\nCLI: ${cli}\nIf this is a fresh profile, create a Project after the backend reports ready; the service will start automatically.\n\n`);
await new Promise((resolve) => {
  const finish = () => { clearInterval(monitor); stop(); resolve(); };
  process.on("SIGINT", finish);
  process.on("SIGTERM", finish);
});
