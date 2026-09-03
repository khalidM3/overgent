import { existsSync, mkdirSync } from "node:fs";
import { spawn, spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import os from "node:os";
import path from "node:path";

if (process.platform !== "darwin") throw new Error("the full local development stack is currently validated only on macOS");
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const shared = process.argv.includes("--shared");
const sharedAPIOrigin = String(process.env.OVERGENT_SHARED_API_ORIGIN ?? "").replace(/\/$/, "");
const sharedConfigOverride = String(process.env.OVERGENT_SHARED_CONFIG_ROOT ?? "").trim();
if (shared) {
  let parsed;
  try { parsed = new URL(sharedAPIOrigin); } catch { throw new Error("OVERGENT_SHARED_API_ORIGIN must be a valid HTTPS URL"); }
  if (parsed.protocol !== "https:" || parsed.username || parsed.password || parsed.search || parsed.hash) throw new Error("OVERGENT_SHARED_API_ORIGIN must be a clean HTTPS origin");
  if (sharedConfigOverride && !path.isAbsolute(sharedConfigOverride)) throw new Error("OVERGENT_SHARED_CONFIG_ROOT must be an absolute path");
}
const configRoot = shared
  ? path.normalize(sharedConfigOverride || path.join(os.homedir(), "Library", "Application Support", "Overgent Shared Dev"))
  : path.join(os.homedir(), "Library", "Application Support", "Overgent");
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
const cli = path.join(root, "bin", "overgent");
const build = spawnSync("go", ["build", "-o", cli, "./cmd/overgent"], { cwd: root, stdio: "inherit" });
if (build.status !== 0) process.exit(build.status ?? 1);

if (!shared) start("backend", "pnpm", ["dev:backend"]);
if (shared) start("ui", "pnpm", ["--dir", "apps/dashboard", "dev"], { env: { ...process.env, OVERGENT_DASHBOARD_API_ORIGIN: sharedAPIOrigin } });
else start("ui", "pnpm", ["dev:ui"]);
const uiURL = "http://127.0.0.1:5173/?desktop=onboarding";
for (let attempt = 0; attempt < 120; attempt++) {
  try { if ((await fetch("http://127.0.0.1:5173/", { signal: AbortSignal.timeout(500) })).ok) break; } catch {}
  await new Promise((resolve) => setTimeout(resolve, 250));
}

const desktopBuild = spawnSync(process.execPath, [path.join(root, "scripts", "build-desktop.mjs"), "--development"], { cwd: root, stdio: "inherit" });
if (desktopBuild.status !== 0) { stop(); process.exit(desktopBuild.status ?? 1); }
const desktopApp = path.join(root, "apps", "desktop", "build", "bin", "Overgent Dev.app");
// The development process launches the executable directly so its logs and
// lifetime remain attached to this orchestrator. LaunchServices therefore does
// not discover the bundle automatically as it would after opening an installed
// app. Register the freshly rebuilt bundle before launch so the hosted
// workroom's overgent-dev://new-project handoff reaches this running instance.
const launchServices = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister";
const registration = spawnSync(launchServices, ["-f", desktopApp], { cwd: root, stdio: "inherit" });
if (registration.status !== 0) { stop(); process.exit(registration.status ?? 1); }
const desktopBinary = path.join(desktopApp, "Contents", "MacOS", "overgent-desktop-dev");
start("desktop", desktopBinary, [], { env: { ...process.env, FRONTEND_DEVSERVER_URL: "http://127.0.0.1:5173", OVERGENT_API_ORIGIN: shared ? sharedAPIOrigin : "http://127.0.0.1:3211", OVERGENT_DASHBOARD_ORIGIN: "http://127.0.0.1:5173/api", OVERGENT_CLI_BINARY: cli, OVERGENT_CONFIG_ROOT: configRoot } });

const configPath = path.join(configRoot, "config.json");
let service;
const ensureService = () => {
  if (!service && existsSync(configPath)) {
    // "service status" answers successfully even when nothing is running - an
    // installed LaunchAgent that failed to bootstrap reports
    // {"installed":true,"running":false}. Treating exit 0 as "a service exists"
    // meant a failed install stopped the development service from ever starting.
    const existing = spawnSync(cli, ["--config-root", configRoot, "service", "status"], { cwd: root, encoding: "utf8" });
    let running = false;
    if (existing.status === 0) {
      try {
        const reported = JSON.parse(existing.stdout);
        running = reported.service === "running" || reported.running === true;
      } catch {
        running = false;
      }
    }
    if (running) {
      service = { external: true };
      process.stdout.write("An existing Overgent service is already managing the enrolled development profile.\n");
      return;
    }
    service = start("service", cli, ["--config-root", configRoot, "service", "run"]);
    service.on("exit", () => { service = undefined; });
    process.stdout.write("Overgent local service started for the enrolled development profile.\n");
  }
};
ensureService();
const monitor = setInterval(ensureService, 1_000);
process.stdout.write(`\nOvergent ${shared ? "shared " : ""}development is running.\nUI hot reload: ${uiURL}\nCLI: ${cli}\nProfile: ${configRoot}\n${shared ? `Shared API: ${sharedAPIOrigin}\n` : "If this is a fresh profile, create a Project after the backend reports ready; the service will start automatically.\n"}\n`);
await new Promise((resolve) => {
  const finish = () => { clearInterval(monitor); stop(); resolve(); };
  process.on("SIGINT", finish);
  process.on("SIGTERM", finish);
});
