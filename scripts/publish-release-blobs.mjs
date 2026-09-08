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
  const response = await fetchFromGitHub(source);
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

// A release asset download never answers directly: github.com issues a 302 to
// a githubusercontent.com object URL. `redirect: "error"` therefore failed
// every time it was asked to promote anything, which is why the release channel
// had never moved off the version it was first seeded with.
//
// Following redirects blindly is what that setting was avoiding, so this
// follows them one at a time and refuses any hop that leaves GitHub. The
// manifest's Ed25519 signature is what actually authenticates the contents -
// every installer verifies it before using it - and this keeps the transport
// from being pointed somewhere else entirely.
async function fetchFromGitHub(source) {
  let target = source;
  for (let hop = 0; hop < 5; hop += 1) {
    const response = await fetch(target, { redirect: "manual" });
    if (response.status < 300 || response.status > 399) return response;
    const location = response.headers.get("location");
    if (!location) throw new Error(`release manifest redirected with no location from ${target}`);
    const next = new URL(location, target);
    if (next.protocol !== "https:" || next.username || next.password) {
      throw new Error(`release manifest redirected to a non-HTTPS or credentialed URL`);
    }
    if (next.hostname !== "github.com" && !next.hostname.endsWith(".githubusercontent.com")) {
      throw new Error(`release manifest redirected off GitHub to ${next.hostname}`);
    }
    target = next.href;
  }
  throw new Error("release manifest redirected too many times");
}
