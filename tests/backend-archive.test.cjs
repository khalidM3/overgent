// The bundled backend's archive extractor.
//
// scripts/fetch-backend.mjs used to unpack with /usr/bin/ditto, which refused a
// hostile entry name on our behalf. Extraction is now hand-rolled so it can run
// on Linux and Windows too, which means the safety ditto provided is this
// code's job and has to be held by a test: a traversal entry in a downloaded
// archive writes wherever the process can write.
//
// The archives are built here rather than committed as fixtures, so a hostile
// case is a line of code instead of a binary blob nobody can review.

const assert = require("node:assert/strict");
const test = require("node:test");
const path = require("node:path");
const { mkdtemp, rm, readFile, stat } = require("node:fs/promises");
const { tmpdir } = require("node:os");
const { pathToFileURL } = require("node:url");

const extractorURL = pathToFileURL(path.join(__dirname, "..", "scripts", "fetch-backend.mjs")).href;
const loadExtractor = async () => (await import(extractorURL)).extract;

const MADE_BY_UNIX = 3;

// zip builds a minimal stored-method archive. Every field the extractor reads
// is settable, including the ones a well-formed writer would never disagree on,
// because disagreeing is exactly what the tests need to do.
function zip(entries) {
  const locals = [];
  const centrals = [];
  let offset = 0;
  for (const entry of entries) {
    const name = Buffer.from(entry.name, "utf8");
    const data = Buffer.from(entry.data ?? "", "utf8");
    const declared = entry.declaredSize ?? data.length;

    const local = Buffer.alloc(30);
    local.writeUInt32LE(0x04034b50, 0);
    local.writeUInt16LE(20, 4);
    local.writeUInt16LE(entry.method ?? 0, 8);
    local.writeUInt32LE(0, 14);
    local.writeUInt32LE(data.length, 18);
    local.writeUInt32LE(declared, 22);
    local.writeUInt16LE(name.length, 26);
    locals.push(local, name, data);

    const central = Buffer.alloc(46);
    central.writeUInt32LE(0x02014b50, 0);
    central.writeUInt16LE(((entry.unixMode ? MADE_BY_UNIX : 0) << 8) | 20, 4);
    central.writeUInt16LE(20, 6);
    central.writeUInt16LE(entry.method ?? 0, 10);
    central.writeUInt32LE(0, 16);
    central.writeUInt32LE(data.length, 20);
    central.writeUInt32LE(declared, 24);
    central.writeUInt16LE(name.length, 28);
    central.writeUInt32LE(((entry.unixMode ?? 0) * 0x10000) >>> 0, 38);
    central.writeUInt32LE(offset, 42);
    centrals.push(central, name);
    offset += 30 + name.length + data.length;
  }
  const body = Buffer.concat(locals);
  const directory = Buffer.concat(centrals);
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0);
  end.writeUInt16LE(entries.length, 8);
  end.writeUInt16LE(entries.length, 10);
  end.writeUInt32LE(directory.length, 12);
  end.writeUInt32LE(body.length, 16);
  return Buffer.concat([body, directory, end]);
}

async function inTemporaryDirectory(run) {
  const root = await mkdtemp(path.join(tmpdir(), "overgent-archive-"));
  try {
    return await run(path.join(root, "into"));
  } finally {
    await rm(root, { recursive: true, force: true });
  }
}

// Each case is a name the extractor must refuse. A refusal is the whole
// assertion: an entry that reaches the filesystem has already escaped.
const refused = [
  ["a parent-directory entry", { name: "../escape", data: "x" }],
  ["a parent segment in the middle of a path", { name: "safe/../../escape", data: "x" }],
  ["an absolute POSIX entry", { name: "/tmp/escape", data: "x" }],
  ["a Windows drive-qualified entry", { name: "C:/escape", data: "x" }],
  // Zip names are defined to use "/", so a backslash is only ever a Windows
  // separator smuggled past a check that splits on "/" alone.
  ["a backslash-separated entry", { name: "..\\..\\escape", data: "x" }],
  ["a control character in the name", { name: "escape\u0000.txt", data: "x" }],
  // A symlink laid down first would let a later entry write through it.
  ["a symlink entry", { name: "link", data: "/etc/passwd", unixMode: 0o120777 }],
  ["an empty entry name", { name: "", data: "x" }],
  // The archive hash proves the bytes; this catches a mis-framed entry that
  // would otherwise land as a short binary and fail much later.
  ["a size that disagrees with the directory", { name: "short", data: "x", declaredSize: 99 }],
  ["an unsupported compression method", { name: "a", data: "x", method: 99 }],
];

for (const [label, entry] of refused) {
  test(`the backend archive extractor refuses ${label}`, async () => {
    const extract = await loadExtractor();
    await inTemporaryDirectory(async (destination) => {
      await assert.rejects(
        () => extract(zip([entry]), destination),
        (error) => error instanceof Error && error.message.length > 0,
        `${label} was extracted instead of being refused`,
      );
    });
  });
}

test("the backend archive extractor unpacks an honest archive and keeps the executable bit", async () => {
  const extract = await loadExtractor();
  await inTemporaryDirectory(async (destination) => {
    const names = await extract(
      zip([
        { name: "nested/", data: "" },
        { name: "nested/convex-local-backend", data: "hello", unixMode: 0o100755 },
        { name: "LICENSE.md", data: "Functional Source License", unixMode: 0o100644 },
      ]),
      destination,
    );
    assert.deepEqual(names, ["nested/convex-local-backend", "LICENSE.md"]);

    const binary = path.join(destination, "nested", "convex-local-backend");
    assert.equal(await readFile(binary, "utf8"), "hello");
    assert.equal(await readFile(path.join(destination, "LICENSE.md"), "utf8"), "Functional Source License");

    // Windows has no executable bit, so only assert the mode where one exists.
    if (process.platform !== "win32") {
      assert.ok((await stat(binary)).mode & 0o111, "the backend binary was not left executable");
      assert.equal((await stat(path.join(destination, "LICENSE.md"))).mode & 0o111, 0);
    }
  });
});

test("the pinned backend targets cover every platform the CLI is released for", async () => {
  const { targets } = await import(extractorURL);
  // GoReleaser builds darwin and linux on both architectures and windows on
  // amd64 only, so these are exactly the machines that can run a local Project.
  assert.deepEqual(Object.keys(targets).sort(), [
    "darwin-arm64",
    "darwin-x64",
    "linux-arm64",
    "linux-x64",
    "win32-x64",
  ]);
  assert.equal(targets["win32-x64"].binary, "convex-local-backend.exe");
  for (const [key, target] of Object.entries(targets)) {
    assert.match(target.triple, /^[a-z0-9_]+-[a-z0-9-]+$/, `${key} has no target triple`);
    if (key !== "win32-x64") assert.equal(target.binary, "convex-local-backend");
  }
});
