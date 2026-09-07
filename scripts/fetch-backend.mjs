import { createHash } from "node:crypto";
import { chmod, mkdir, mkdtemp, readFile, rename, rm, stat, writeFile } from "node:fs/promises";
import { createWriteStream } from "node:fs";
import { Readable } from "node:stream";
import { pipeline } from "node:stream/promises";
import { createInflateRaw } from "node:zlib";
import { fileURLToPath } from "node:url";
import path from "node:path";

// The bundled backend, one published archive per target triple.
//
// These are the binary assets the pinned release actually carries. Windows on
// ARM is missing on purpose rather than by oversight: the release publishes no
// aarch64-pc-windows-msvc asset, which is the same reason GoReleaser excludes
// windows/arm64 from the CLI matrix. The keys are Node's
// `${process.platform}-${process.arch}`, which is how scripts/backend-version.json
// keys its hashes; internal/localbackend/pin.go holds the Go half of that map,
// and a test fails if the two ever disagree.
export const targets = {
  "darwin-arm64": { triple: "aarch64-apple-darwin", binary: "convex-local-backend" },
  "darwin-x64": { triple: "x86_64-apple-darwin", binary: "convex-local-backend" },
  "linux-x64": { triple: "x86_64-unknown-linux-gnu", binary: "convex-local-backend" },
  "linux-arm64": { triple: "aarch64-unknown-linux-gnu", binary: "convex-local-backend" },
  "win32-x64": { triple: "x86_64-pc-windows-msvc", binary: "convex-local-backend.exe" },
};

// A zip reader, because the extractor has to run on all three platforms.
//
// /usr/bin/ditto only exists on macOS. `unzip` is not guaranteed on a minimal
// Linux image and tar's zip support varies by platform, so the archive is
// unpacked here with node:zlib instead of by shelling out. The safety
// properties ditto used to provide are now this code's job: the hash is
// checked before any of this runs, entry names are refused if they can escape
// the destination, and only regular files and directories are ever created.

const LOCAL_HEADER = 0x04034b50;
const CENTRAL_HEADER = 0x02014b50;
const END_OF_CENTRAL = 0x06054b50;
const ZIP64_LOCATOR = 0x07064b50;
const ZIP64_END_OF_CENTRAL = 0x06064b50;
const U16_MAX = 0xffff;
const U32_MAX = 0xffffffff;
const STORED = 0;
const DEFLATED = 8;
const UNIX = 3;
const FILE_TYPE_MASK = 0o170000;
const REGULAR_FILE = 0o100000;
const DIRECTORY = 0o040000;
const CONTROL_CHARACTER = /[\u0000-\u001f\u007f]/;

export async function extract(archive, destination) {
  const entries = readEntries(archive);
  const names = [];
  for (const entry of entries) {
    const file = resolveEntry(destination, entry.name);
    if (entry.name.endsWith("/")) {
      await mkdir(file, { recursive: true });
      continue;
    }
    await mkdir(path.dirname(file), { recursive: true });
    const body = entryBody(archive, entry);
    const sink = createWriteStream(file, { mode: entry.unixMode & 0o111 ? 0o755 : 0o644 });
    if (entry.method === STORED) {
      await pipeline(Readable.from([body]), sink);
    } else if (entry.method === DEFLATED) {
      await pipeline(Readable.from([body]), createInflateRaw(), sink);
    } else {
      throw new Error(`backend archive entry ${JSON.stringify(entry.name)} uses compression method ${entry.method}`);
    }
    // The archive-level SHA-256 already proves the bytes; this catches a
    // truncated or mis-framed entry, which would otherwise land as a short
    // binary that fails much later and much less legibly.
    const written = await stat(file);
    if (written.size !== entry.uncompressedSize) {
      throw new Error(
        `backend archive entry ${JSON.stringify(entry.name)} unpacked to ${written.size} bytes, expected ${entry.uncompressedSize}`,
      );
    }
    names.push(entry.name);
  }
  return names;
}

// resolveEntry is the path-traversal gate. ditto refused a "../.." entry for
// us; a hand-rolled extractor has to refuse it itself, or a hostile archive
// writes wherever the process can write. Backslashes are rejected outright
// because zip names are defined to use "/", so a backslash is only ever a
// Windows separator smuggled past a "/"-only check.
function resolveEntry(destination, name) {
  const shown = JSON.stringify(name);
  if (name === "") throw new Error("backend archive entry has an empty name");
  if (CONTROL_CHARACTER.test(name)) throw new Error(`backend archive entry ${shown} has a control character in its name`);
  if (name.includes("\\")) throw new Error(`backend archive entry ${shown} uses a backslash in its name`);
  if (name.startsWith("/") || /^[a-zA-Z]:/.test(name)) throw new Error(`backend archive entry ${shown} is an absolute path`);
  if (name.split("/").includes("..")) throw new Error(`backend archive entry ${shown} escapes the destination directory`);
  const root = path.resolve(destination);
  const resolved = path.resolve(root, name);
  if (resolved !== root && !resolved.startsWith(root + path.sep)) {
    throw new Error(`backend archive entry ${shown} escapes the destination directory`);
  }
  return resolved;
}

function readEntries(archive) {
  const { count, offset } = centralDirectory(archive);
  const entries = [];
  let cursor = offset;
  for (let index = 0; index < count; index++) {
    if (cursor + 46 > archive.length || archive.readUInt32LE(cursor) !== CENTRAL_HEADER) {
      throw new Error("backend archive central directory is malformed");
    }
    const madeBy = archive.readUInt16LE(cursor + 4);
    const nameLength = archive.readUInt16LE(cursor + 28);
    const extraLength = archive.readUInt16LE(cursor + 30);
    const commentLength = archive.readUInt16LE(cursor + 32);
    const entry = {
      name: archive.subarray(cursor + 46, cursor + 46 + nameLength).toString("utf8"),
      method: archive.readUInt16LE(cursor + 10),
      compressedSize: archive.readUInt32LE(cursor + 20),
      uncompressedSize: archive.readUInt32LE(cursor + 24),
      localOffset: archive.readUInt32LE(cursor + 42),
      // Only a Unix-made archive carries a mode; an MS-DOS one (the Windows
      // asset) carries none, and Windows has no executable bit to set.
      unixMode: madeBy >> 8 === UNIX ? archive.readUInt32LE(cursor + 38) >>> 16 : 0,
    };
    applyZip64(archive.subarray(cursor + 46 + nameLength, cursor + 46 + nameLength + extraLength), entry);
    const fileType = entry.unixMode & FILE_TYPE_MASK;
    if (fileType !== 0 && fileType !== REGULAR_FILE && fileType !== DIRECTORY) {
      // Symlinks and devices are never created, so an archive cannot lay down
      // a link and then write a file through it.
      throw new Error(`backend archive entry ${JSON.stringify(entry.name)} is not a regular file or directory`);
    }
    entries.push(entry);
    cursor += 46 + nameLength + extraLength + commentLength;
  }
  return entries;
}

// centralDirectory finds where the entry table starts, following the zip64
// records when the 32-bit fields are saturated. The pinned archives are single
// entries well under 4 GB, but a future release that crosses that line should
// fail its hash check, not the parser.
function centralDirectory(archive) {
  const end = findEndOfCentralDirectory(archive);
  let count = archive.readUInt16LE(end + 10);
  let offset = archive.readUInt32LE(end + 16);
  if (count !== U16_MAX && offset !== U32_MAX) return { count, offset };
  const locator = end - 20;
  if (locator < 0 || archive.readUInt32LE(locator) !== ZIP64_LOCATOR) {
    throw new Error("backend archive needs a zip64 record and does not have one");
  }
  const record = Number(archive.readBigUInt64LE(locator + 8));
  if (record + 56 > archive.length || archive.readUInt32LE(record) !== ZIP64_END_OF_CENTRAL) {
    throw new Error("backend archive zip64 end-of-central-directory record is malformed");
  }
  count = Number(archive.readBigUInt64LE(record + 32));
  offset = Number(archive.readBigUInt64LE(record + 48));
  return { count, offset };
}

function findEndOfCentralDirectory(archive) {
  // The record is 22 bytes plus a comment of at most 65535.
  const earliest = Math.max(0, archive.length - (22 + U16_MAX));
  for (let offset = archive.length - 22; offset >= earliest; offset--) {
    if (archive.readUInt32LE(offset) === END_OF_CENTRAL) return offset;
  }
  throw new Error("backend archive has no zip end-of-central-directory record");
}

function applyZip64(extra, entry) {
  let cursor = 0;
  while (cursor + 4 <= extra.length) {
    const id = extra.readUInt16LE(cursor);
    const size = extra.readUInt16LE(cursor + 2);
    const body = extra.subarray(cursor + 4, cursor + 4 + size);
    if (id === 0x0001) {
      let at = 0;
      for (const field of ["uncompressedSize", "compressedSize", "localOffset"]) {
        if (entry[field] !== U32_MAX) continue;
        if (at + 8 > body.length) throw new Error("backend archive zip64 extra field is truncated");
        entry[field] = Number(body.readBigUInt64LE(at));
        at += 8;
      }
    }
    cursor += 4 + size;
  }
}

// entryBody returns the compressed bytes of one entry. The sizes come from the
// central directory rather than from the local header, because a local header
// written with a trailing data descriptor carries zeroes there.
function entryBody(archive, entry) {
  const header = entry.localOffset;
  if (header + 30 > archive.length || archive.readUInt32LE(header) !== LOCAL_HEADER) {
    throw new Error(`backend archive entry ${JSON.stringify(entry.name)} has no local header`);
  }
  const start = header + 30 + archive.readUInt16LE(header + 26) + archive.readUInt16LE(header + 28);
  const finish = start + entry.compressedSize;
  if (finish > archive.length) {
    throw new Error(`backend archive entry ${JSON.stringify(entry.name)} runs past the end of the archive`);
  }
  return archive.subarray(start, finish);
}

// --platform and --out exist so a packaging job can fetch a backend for a
// machine it is not running on; without them this script can only ever produce
// the host's binary, and a release that ships the backend inside the CLI
// archive builds every platform on one runner.
function option(name) {
  const argv = process.argv.slice(2);
  const inline = argv.find((argument) => argument.startsWith(`--${name}=`));
  if (inline !== undefined) return inline.slice(name.length + 3);
  const index = argv.indexOf(`--${name}`);
  return index >= 0 ? argv[index + 1] : undefined;
}

// main downloads the pinned archive for this platform, verifies it against the
// manifest, unpacks it beside the license it ships under, and prints the path
// to the binary.
async function main() {
  const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
  const platformKey = option("platform") ?? `${process.platform}-${process.arch}`;
  if (platformKey === "win32-arm64") {
    throw new Error(
      "the pinned Convex backend release publishes no Windows arm64 binary (there is no " +
        "aarch64-pc-windows-msvc asset), so Windows on ARM has no bundled backend and must use a hosted one",
    );
  }
  const target = targets[platformKey];
  if (!target) throw new Error(`bundled backend download is not pinned for ${platformKey}`);

  const manifestPath = path.join(root, "scripts", "backend-version.json");
  const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
  const expectedSha256 = manifest.sha256?.[platformKey];
  if (!/^([a-f0-9]{64})$/.test(expectedSha256 ?? "")) {
    throw new Error(`missing valid SHA-256 for ${platformKey}`);
  }
  if (!/^precompiled-[0-9]{4}-[0-9]{2}-[0-9]{2}-[a-f0-9]+$/.test(manifest.version ?? "")) {
    throw new Error("backend release version is invalid");
  }

  const archiveName = `convex-local-backend-${target.triple}.zip`;
  const releaseUrl = `https://github.com/get-convex/convex-backend/releases/download/${manifest.version}`;
  const downloadUrl = `${releaseUrl}/${archiveName}`;
  const destination = path.resolve(option("out") ?? path.join(root, "apps", "desktop", "build", "backend"));
  // The staging directory is a sibling of the destination, so the final rename
  // stays inside one filesystem whatever --out names.
  await mkdir(path.dirname(destination), { recursive: true });
  const stage = await mkdtemp(path.join(path.dirname(destination), ".backend-stage-"));

  try {
    const response = await fetch(downloadUrl, { redirect: "follow" });
    if (!response.ok) throw new Error(`backend download failed: HTTP ${response.status}`);
    const archive = Buffer.from(await response.arrayBuffer());
    const actualSha256 = createHash("sha256").update(archive).digest("hex");
    if (actualSha256 !== expectedSha256) {
      throw new Error(`backend checksum mismatch: expected ${expectedSha256}, got ${actualSha256}`);
    }

    // Nothing is unpacked until the whole archive has matched the pinned hash:
    // the extractor only ever runs over bytes the manifest vouched for.
    const extracted = path.join(stage, "extracted");
    await mkdir(extracted);
    const names = await extract(archive, extracted);

    const binary = path.join(extracted, target.binary);
    const info = await stat(binary).catch(() => null);
    if (!info?.isFile()) {
      throw new Error(`backend archive did not contain ${target.binary} (it contained ${names.join(", ") || "nothing"})`);
    }
    if (info.size === 0) throw new Error("backend archive contained an empty binary");
    // The executable bit is a property of the host filesystem rather than of the
    // target platform: a Mac packaging the Linux backend still has to mark it
    // runnable. Windows has no such bit.
    if (process.platform !== "win32") await chmod(binary, 0o755);

    // The binary is redistributed under the Convex release's own FSL-1.1 terms,
    // not the npm package's Apache-2.0, and that license is a separate asset of
    // the same release. It ships beside the binary it governs; NOTICE names it.
    const licenseResponse = await fetch(`${releaseUrl}/LICENSE.md`, { redirect: "follow" });
    if (!licenseResponse.ok) throw new Error(`backend license download failed: HTTP ${licenseResponse.status}`);
    const license = Buffer.from(await licenseResponse.arrayBuffer());
    if (!license.includes("Functional Source License")) throw new Error("backend license asset is not the expected FSL text");
    await writeFile(path.join(extracted, "LICENSE.md"), license, { mode: 0o644 });
    await rm(destination, { recursive: true, force: true });
    await rename(extracted, destination);
    process.stdout.write(`${path.join(destination, target.binary)}\n`);
  } finally {
    await rm(stage, { recursive: true, force: true });
  }
}

// The download runs only when this file is the program. Importing it - a test
// exercising the extractor, or a packaging step that wants the target map -
// gets the exports without touching the network.
if (process.argv[1] !== undefined && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  await main();
}
