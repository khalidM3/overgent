import { spawn, spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const devURL = process.env.STICKGUY_DESKTOP_DEV_URL ?? "http://127.0.0.1:5173/?desktop=preview";
const healthURL = new URL(devURL);
healthURL.pathname = "/";
healthURL.search = "";
let ui;

async function available() {
  try { return (await fetch(healthURL, { signal: AbortSignal.timeout(500) })).ok; }
  catch { return false; }
}

if (!await available()) {
  ui = spawn("pnpm", ["dev:ui"], { cwd: root, stdio: "inherit" });
  for (let attempt = 0; attempt < 80 && !await available(); attempt++) {
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  if (!await available()) {
    ui.kill("SIGTERM");
    throw new Error(`dashboard development server did not start at ${healthURL}`);
  }
}

const build = spawnSync(process.execPath, [path.join(root, "scripts", "build-desktop.mjs"), "--development"], { cwd: root, stdio: "inherit" });
if (build.status !== 0) {
  ui?.kill("SIGTERM");
  process.exit(build.status ?? 1);
}

const executable = path.join(root, "apps", "desktop", "build", "bin", "Stickguy Dev.app", "Contents", "MacOS", "stickguy-desktop-dev");
const desktop = spawn(executable, [], { cwd: root, env: { ...process.env, STICKGUY_DESKTOP_DEV_URL: devURL }, stdio: "inherit" });
const stop = () => {
  desktop.kill("SIGTERM");
  ui?.kill("SIGTERM");
};
process.on("SIGINT", stop);
process.on("SIGTERM", stop);
desktop.on("exit", (code) => {
  ui?.kill("SIGTERM");
  process.exitCode = code ?? 0;
});
