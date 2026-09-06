// Build the app a member actually installs, from this checkout.
//
// `pnpm dev` cannot answer "does local mode work?": a development build carries
// no backend, so "Use on this Mac" is disabled and every Project it can make
// needs a server. The only build that runs the default, no-account, nothing-
// leaves-the-Mac path is this one. It is the same artifact a release publishes;
// the release adds the Apple identity and notarization, nothing else.
//
// The sequence below is the one docs/development.md spells out by hand, and the
// one .github/workflows/release.yml runs: fetch the pinned backend, record a
// deploy payload from this commit's Convex functions against a scratch backend,
// then build and sign the bundle. Each step is skipped when its output is
// already current, so a rebuild after a code change is the app build alone.
import { createHash } from "node:crypto";
import { spawn, spawnSync } from "node:child_process";
import { mkdtemp, readFile, rm, stat, writeFile, copyFile, readdir } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { createServer } from "node:net";
import os from "node:os";
import path from "node:path";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const buildRoot = path.join(root, "apps", "desktop", "build");
const backendDir = path.join(buildRoot, "backend");
const binary = path.join(backendDir, "convex-local-backend");
const payload = path.join(buildRoot, "backend-push.json");
const force = process.argv.includes("--force");

const say = (message) => process.stdout.write(`${message}\n`);
const die = (message) => { process.stderr.write(`${message}\n`); process.exit(1); };

function need(tool, hint) {
  if (spawnSync("command", ["-v", tool], { shell: true, stdio: "ignore" }).status !== 0) {
    die(`${tool} is required to build the bundled backend payload. ${hint}`);
  }
}

async function isFile(target) {
  return stat(target).then((info) => info.isFile()).catch(() => false);
}

async function freePort() {
  return new Promise((resolve, reject) => {
    const probe = createServer();
    probe.once("error", reject);
    probe.listen(0, "127.0.0.1", () => {
      const { port } = probe.address();
      probe.close(() => resolve(port));
    });
  });
}

// The payload is a recording of this commit's Convex functions, so it is stale
// the moment convex/ changes. Comparing against the newest source file is what
// keeps a rebuild honest without making every build pay the 6-second replay.
async function newestMTime(directory) {
  let newest = 0;
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    if (entry.name === "node_modules" || entry.name.startsWith(".")) continue;
    const full = path.join(directory, entry.name);
    newest = Math.max(newest, entry.isDirectory() ? await newestMTime(full) : (await stat(full)).mtimeMs);
  }
  return newest;
}

async function ensureBackendBinary() {
  const manifest = JSON.parse(await readFile(path.join(root, "scripts", "backend-version.json"), "utf8"));
  const expected = manifest.sha256["darwin-arm64"];
  if (!force && await isFile(binary)) {
    const actual = createHash("sha256");
    // The published checksum covers the zip, not the extracted binary, so a
    // present binary is taken on trust here; fetch-backend.mjs is what verifies
    // the download itself. --force re-fetches.
    actual.update(await readFile(binary));
    say(`backend binary present (${manifest.version})`);
    return;
  }
  say(`downloading the pinned backend (${manifest.version}, ~166 MB, checksum ${expected.slice(0, 12)}…)`);
  const fetched = spawnSync(process.execPath, [path.join(root, "scripts", "fetch-backend.mjs")], { cwd: root, stdio: "inherit" });
  if (fetched.status !== 0) die("backend download failed");
}

async function ensurePayload() {
  if (!force && await isFile(payload)) {
    const [recorded, newest] = [(await stat(payload)).mtimeMs, await newestMTime(path.join(root, "convex", "functions"))];
    if (recorded >= newest) { say("deploy payload is current"); return; }
    say("convex/functions changed since the payload was recorded; re-recording");
  }
  need("jq", "Install it with `brew install jq`.");
  need("openssl", "Install it with `brew install openssl`.");

  const work = await mkdtemp(path.join(os.tmpdir(), "overgent-payload-"));
  const [port, sitePort] = [await freePort(), await freePort()];
  const instance = `overgent-local-${createHash("sha256").update(String(Date.now())).digest("hex").slice(0, 8)}`;
  const secret = createHash("sha256").update(`${instance}:${process.pid}`).digest("hex");
  const keygen = spawnSync(binary, ["keygen", "admin-key", "--instance-name", instance, "--instance-secret", secret], { encoding: "utf8" });
  if (keygen.status !== 0) die(`admin key generation failed: ${keygen.stderr || keygen.stdout}`);
  const adminKey = keygen.stdout.trim().split("\n").at(-1).trim();

  say(`recording the deploy payload against a scratch backend on 127.0.0.1:${port}`);
  const backend = spawn(binary, [
    "--interface", "127.0.0.1", "--port", String(port), "--site-proxy-port", String(sitePort),
    "--convex-origin", `http://127.0.0.1:${port}`, "--convex-site", `http://127.0.0.1:${sitePort}`,
    "--instance-name", instance, "--instance-secret", secret,
    "--local-storage", path.join(work, "storage"), "--disable-beacon", path.join(work, "build.sqlite3"),
  ], { stdio: ["ignore", "ignore", "ignore"] });

  const cleanup = async () => { backend.kill("SIGTERM"); await rm(work, { recursive: true, force: true }); };
  try {
    let healthy = false;
    for (let attempt = 0; attempt < 100 && !healthy; attempt++) {
      try { healthy = (await fetch(`http://127.0.0.1:${port}/version`, { signal: AbortSignal.timeout(500) })).ok; }
      catch { await new Promise((resolve) => setTimeout(resolve, 100)); }
    }
    if (!healthy) die("the scratch backend did not become healthy within 10s");
    const push = spawnSync(path.join(root, "validation", "spikes", "bundled-backend", "push.sh"),
      ["build", `http://127.0.0.1:${port}`, adminKey, work], { cwd: root, stdio: "inherit" });
    if (push.status !== 0) die("recording the deploy payload failed");
    await copyFile(path.join(work, "backend-push.json"), payload);
    say(`deploy payload recorded (${((await stat(payload)).size / 1024).toFixed(0)} KB)`);
  } finally {
    await cleanup();
  }
}

if (process.platform !== "darwin" || process.arch !== "arm64") {
  die("the bundled backend is pinned for Apple Silicon macOS only (ADR-050)");
}
need("brotli", "Install it with `brew install brotli`.");
await ensureBackendBinary();
await ensurePayload();

say("building the dashboard bundle the app embeds");
const assets = spawnSync("pnpm", ["desktop:assets"], { cwd: root, stdio: "inherit" });
if (assets.status !== 0) die("dashboard build failed");

say("building and signing Overgent.app");
const app = spawnSync(process.execPath, [path.join(root, "scripts", "build-desktop.mjs")], { cwd: root, stdio: "inherit" });
if (app.status !== 0) die("app build failed");

const bundle = path.join(buildRoot, "bin", "Overgent.app");
say(`
Built ${bundle}

This is the release artifact, signed ad-hoc rather than with the Apple identity.
It uses the default profile at ~/Library/Application Support/Overgent, which is
what a member's install uses; the development stack now has its own profile.

Open it with:   open -n "${bundle}"
Start over:     ./bin/overgent backend reset --yes && ./bin/overgent reset --all --force`);
