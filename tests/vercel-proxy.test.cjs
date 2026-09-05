const assert = require("node:assert/strict");
const test = require("node:test");

const proxy = require("../api/v1/[...].js");
const currentManifest = require("../api/releases/current-manifest.js");
const vercelConfig = require("../vercel.json");

function responseRecorder() {
  return {
    statusCode: 200,
    headers: {},
    body: Buffer.alloc(0),
    setHeader(name, value) { this.headers[name.toLowerCase()] = value; },
    end(body = "") { this.body = Buffer.from(body); },
  };
}

test("API proxy preserves the versioned route, query, auth, cookie, and response cookie", async () => {
  process.env.CONVEX_SITE_URL = "https://example.convex.site";
  const originalFetch = global.fetch;
  let observed;
  global.fetch = async (url, init) => {
    observed = { url: String(url), init };
    return new Response('{"ok":true}', { status: 201, headers: { "content-type": "application/json", "set-cookie": "session=opaque; Secure; HttpOnly" } });
  };
  try {
    const response = responseRecorder();
    await proxy({ method: "POST", url: "/api/v1/projects/prj_test/invites?fresh=1", query: { path: "projects/prj_test/invites" }, headers: { authorization: "Bearer opaque", cookie: "session=opaque", "content-type": "application/json" }, body: "{}" }, response);
    assert.equal(observed.url, "https://example.convex.site/v1/projects/prj_test/invites?fresh=1");
    assert.equal(observed.init.headers.get("authorization"), "Bearer opaque");
    assert.equal(observed.init.headers.get("cookie"), "session=opaque");
    assert.equal(response.statusCode, 201);
    assert.equal(response.headers["set-cookie"], "session=opaque; Secure; HttpOnly");
    assert.deepEqual(JSON.parse(response.body), { ok: true });
  } finally {
    global.fetch = originalFetch;
    delete process.env.CONVEX_SITE_URL;
  }
});

test("API proxy rejects an oversized parsed body before calling upstream", async () => {
  process.env.CONVEX_SITE_URL = "https://example.convex.site";
  const originalFetch = global.fetch;
  global.fetch = async () => { throw new Error("must not fetch"); };
  try {
    const response = responseRecorder();
    await proxy({ method: "POST", url: "/api/v1/events/batch", query: { path: ["events", "batch"] }, headers: {}, body: "x".repeat(2 * 1024 * 1024 + 1) }, response);
    assert.equal(response.statusCode, 413);
  } finally {
    global.fetch = originalFetch;
    delete process.env.CONVEX_SITE_URL;
  }
});

test("release channel is closed until a signed manifest is promoted", async () => {
  delete process.env.OVERGENT_RELEASE_MANIFEST_URL;
  const response = responseRecorder();
  await currentManifest({}, response);
  assert.equal(response.statusCode, 503);
  assert.equal(JSON.parse(response.body).error.code, "release_channel_unavailable");
});

test("release channel serves only the fixed public Blob manifest", async () => {
  process.env.OVERGENT_RELEASE_MANIFEST_URL = "https://store.public.blob.vercel-storage.com/current/update-manifest.json";
  const originalFetch = global.fetch;
  let observed;
  global.fetch = async (url, init) => {
    observed = { url: String(url), init };
    return new Response('{"schema_version":1}', { status: 200, headers: { "content-type": "application/json" } });
  };
  try {
    const response = responseRecorder();
    await currentManifest({}, response);
    assert.equal(response.statusCode, 200);
    assert.equal(observed.url, process.env.OVERGENT_RELEASE_MANIFEST_URL);
    assert.equal(observed.init.redirect, "error");
    assert.deepEqual(JSON.parse(response.body), { schema_version: 1 });
  } finally {
    global.fetch = originalFetch;
    delete process.env.OVERGENT_RELEASE_MANIFEST_URL;
  }
});

test("release channel rejects non-Blob and non-current targets", async () => {
  const originalFetch = global.fetch;
  global.fetch = async () => { throw new Error("must not fetch"); };
  try {
    for (const target of [
      "https://github.com/khalidM3/overgent/releases/download/v1/update-manifest.json",
      "https://store.public.blob.vercel-storage.com/releases/v1/update-manifest.json",
    ]) {
      process.env.OVERGENT_RELEASE_MANIFEST_URL = target;
      const response = responseRecorder();
      await currentManifest({}, response);
      assert.equal(response.statusCode, 502);
      assert.equal(JSON.parse(response.body).error.code, "release_channel_unavailable");
    }
  } finally {
    global.fetch = originalFetch;
    delete process.env.OVERGENT_RELEASE_MANIFEST_URL;
  }
});

test("public installer and desktop aliases redirect to the latest published GitHub Release assets", () => {
  const redirects = Object.fromEntries(vercelConfig.redirects.map((redirect) => [redirect.source, redirect.destination]));
  assert.equal(redirects["/install.sh"], "https://github.com/khalidM3/overgent/releases/latest/download/install.sh");
  assert.equal(redirects["/uninstall.sh"], "https://github.com/khalidM3/overgent/releases/latest/download/uninstall.sh");
  assert.equal(redirects["/download/macos"], "https://github.com/khalidM3/overgent/releases/latest/download/Overgent_macOS_arm64.zip?download=1");
});
