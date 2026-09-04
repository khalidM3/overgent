import { createHash } from "node:crypto";
import { chmod, mkdir, mkdtemp, readFile, rename, rm, writeFile } from "node:fs/promises";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const manifestPath = path.join(root, "scripts", "backend-version.json");
const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
const platformKey = `${process.platform}-${process.arch}`;

if (platformKey !== "darwin-arm64") {
  throw new Error(`bundled backend download is not pinned for ${platformKey}`);
}

const expectedSha256 = manifest.sha256?.[platformKey];
if (!/^([a-f0-9]{64})$/.test(expectedSha256 ?? "")) {
  throw new Error(`missing valid SHA-256 for ${platformKey}`);
}
if (!/^precompiled-[0-9]{4}-[0-9]{2}-[0-9]{2}-[a-f0-9]+$/.test(manifest.version ?? "")) {
  throw new Error("backend release version is invalid");
}

const archiveName = "convex-local-backend-aarch64-apple-darwin.zip";
const downloadUrl = `https://github.com/get-convex/convex-backend/releases/download/${manifest.version}/${archiveName}`;
const buildRoot = path.join(root, "apps", "desktop", "build");
const destination = path.join(buildRoot, "backend");
await mkdir(buildRoot, { recursive: true });
const stage = await mkdtemp(path.join(buildRoot, ".backend-stage-"));

try {
  const response = await fetch(downloadUrl, { redirect: "follow" });
  if (!response.ok) throw new Error(`backend download failed: HTTP ${response.status}`);
  const archive = Buffer.from(await response.arrayBuffer());
  const actualSha256 = createHash("sha256").update(archive).digest("hex");
  if (actualSha256 !== expectedSha256) {
    throw new Error(`backend checksum mismatch: expected ${expectedSha256}, got ${actualSha256}`);
  }

  const archivePath = path.join(stage, archiveName);
  const extracted = path.join(stage, "extracted");
  await writeFile(archivePath, archive, { mode: 0o600 });
  await mkdir(extracted);
  const unpack = spawnSync("/usr/bin/ditto", ["-x", "-k", archivePath, extracted], { stdio: "inherit" });
  if (unpack.status !== 0) throw new Error(`backend extraction failed with status ${unpack.status ?? "unknown"}`);

  const binary = path.join(extracted, "convex-local-backend");
  const binaryBytes = await readFile(binary);
  if (binaryBytes.length === 0) throw new Error("backend archive contained an empty binary");
  await chmod(binary, 0o755);
  await rm(destination, { recursive: true, force: true });
  await rename(extracted, destination);
  process.stdout.write(`${path.join(destination, "convex-local-backend")}\n`);
} finally {
  await rm(stage, { recursive: true, force: true });
}
