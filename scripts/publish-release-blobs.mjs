import { put } from "@vercel/blob";

const [mode, version] = process.argv.slice(2);
const token = process.env.BLOB_READ_WRITE_TOKEN;
const configuredOrigin = process.env.OVERGENT_RELEASE_BLOB_ORIGIN;
const manifestURL = process.env.RELEASE_MANIFEST_URL;

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

if (mode === "promote") {
  const source = releaseManifestURL(version, manifestURL);
  const response = await fetch(source, { redirect: "error" });
  if (!response.ok) throw new Error(`release manifest returned HTTP ${response.status}`);
  const manifest = Buffer.from(await response.arrayBuffer());
  if (manifest.length === 0 || manifest.length > 1024 * 1024) throw new Error("release manifest size is invalid");
  const promoted = await put("current/update-manifest.json", manifest, {
    access: "public",
    addRandomSuffix: false,
    allowOverwrite: true,
    cacheControlMaxAge: 60,
    contentType: "application/json; charset=utf-8",
    token,
  });
  const expected = new URL("current/update-manifest.json", origin).href;
  if (promoted.url !== expected) throw new Error("unexpected promoted URL for update-manifest.json");
  process.stdout.write(`promoted ${version}\n`);
} else {
  throw new Error("usage: node scripts/publish-release-blobs.mjs promote <version>");
}

function releaseManifestURL(releaseVersion, configuredURL) {
  const expected = `https://github.com/khalidM3/overgent/releases/download/${releaseVersion}/update-manifest.json`;
  if (configuredURL !== expected) throw new Error("RELEASE_MANIFEST_URL must be this release's GitHub manifest asset");
  return expected;
}
