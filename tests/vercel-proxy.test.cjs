const assert = require("node:assert/strict");
const test = require("node:test");

const proxy = require("../api/v1/[...].js");
const currentManifest = require("../api/releases/current-manifest.js");

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
