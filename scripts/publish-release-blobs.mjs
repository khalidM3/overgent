import { copy, put } from "@vercel/blob";
import { createReadStream } from "node:fs";
import { lstat, readdir } from "node:fs/promises";
import path from "node:path";

const [mode, version] = process.argv.slice(2);
const token = process.env.BLOB_READ_WRITE_TOKEN;
const configuredOrigin = process.env.OVERGENT_RELEASE_BLOB_ORIGIN;

if (!token) throw new Error("BLOB_READ_WRITE_TOKEN is required");
if (!/^v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(version ?? "")) {
  throw new Error("version must be an immutable v-prefixed semantic version");
}

const origin = new URL(configuredOrigin ?? "");
if (
  origin.protocol !== "https:" ||
  origin.username ||
  origin.password ||
  !origin.hostname.endsWith(".public.blob.vercel-storage.com") ||
  origin.pathname !== "/" ||
  origin.search ||
  origin.hash
) {
  throw new Error("OVERGENT_RELEASE_BLOB_ORIGIN must be a public Vercel Blob origin");
}

if (mode === "candidate") {
  const files = await candidateFiles();
  for (const file of files) {
    const filename = path.basename(file);
    const pathname = `releases/${version}/${filename}`;
    const uploaded = await put(pathname, createReadStream(file), {
      access: "public",
      addRandomSuffix: false,
      allowOverwrite: false,
      cacheControlMaxAge: 31_536_000,
      multipart: true,
      token,
    });
    const expected = new URL(pathname, origin).href;
    if (uploaded.url !== expected) throw new Error(`unexpected Blob URL for ${filename}`);
    process.stdout.write(`published ${pathname}\n`);
  }
} else if (mode === "promote") {
  const promotions = [
    ["update-manifest.json", "update-manifest.json"],
    ["install.sh", "install.sh"],
    ["uninstall.sh", "uninstall.sh"],
    [`Overgent_${version}_macOS_arm64.zip`, "Overgent_macOS_arm64.zip"],
  ];
  for (const [sourceName, currentName] of promotions) {
    const source = new URL(`releases/${version}/${sourceName}`, origin).href;
    const pathname = `current/${currentName}`;
    const promoted = await copy(source, pathname, {
      access: "public",
      allowOverwrite: true,
      cacheControlMaxAge: 60,
      token,
    });
    const expected = new URL(pathname, origin).href;
    if (promoted.url !== expected) throw new Error(`unexpected promoted URL for ${currentName}`);
  }
  process.stdout.write(`promoted ${version}\n`);
} else {
  throw new Error("usage: node scripts/publish-release-blobs.mjs <candidate|promote> <version>");
}

async function candidateFiles() {
  const selected = [];
  for (const directory of ["dist", "dist-desktop"]) {
    for (const entry of await readdir(directory)) {
      if (!/^[0-9A-Za-z._+-]+$/.test(entry)) throw new Error(`unsafe release filename: ${entry}`);
      const file = path.join(directory, entry);
      const stat = await lstat(file);
      if (!stat.isFile()) continue;
      if (shouldPublish(entry)) selected.push(file);
    }
  }
  if (!selected.some((file) => path.basename(file) === "update-manifest.json")) {
    throw new Error("signed update-manifest.json is missing");
  }
  if (!selected.some((file) => file.startsWith(`dist-desktop${path.sep}`))) {
    throw new Error("notarized desktop archive is missing");
  }
  return selected.sort();
}

function shouldPublish(name) {
  return (
    name.endsWith(".tar.gz") ||
    name.endsWith(".zip") ||
    name.endsWith(".spdx.json") ||
    name.endsWith(".sigstore.json") ||
    ["checksums.txt", "desktop-checksums.txt", "update-manifest.json", "install.sh", "uninstall.sh"].includes(name)
  );
}
