const MAX_MANIFEST_BYTES = 1024 * 1024;

module.exports = async function currentManifest(_request, response) {
  try {
    const configured = process.env.OVERGENT_RELEASE_MANIFEST_URL;
    if (!configured) {
      response.statusCode = 503;
      response.setHeader("retry-after", "300");
      throw new Error("release channel has not been promoted");
    }
    const target = new URL(configured);
    if (target.protocol !== "https:" || target.username || target.password || target.host !== "github.com") {
      throw new Error("release manifest URL must be an HTTPS github.com URL");
    }

    const upstream = await fetch(target, { redirect: "follow" });
    if (!upstream.ok) throw new Error(`release manifest returned HTTP ${upstream.status}`);
    const body = Buffer.from(await upstream.arrayBuffer());
    if (body.length === 0 || body.length > MAX_MANIFEST_BYTES) throw new Error("release manifest size is invalid");

    response.statusCode = 200;
    response.setHeader("cache-control", "public, max-age=60, s-maxage=60, stale-while-revalidate=300");
    response.setHeader("content-type", "application/json; charset=utf-8");
    response.setHeader("x-content-type-options", "nosniff");
    response.end(body);
  } catch (error) {
    if (response.statusCode !== 503) response.statusCode = 502;
    response.setHeader("cache-control", "no-store");
    response.setHeader("content-type", "application/json; charset=utf-8");
    response.setHeader("x-content-type-options", "nosniff");
    response.end(JSON.stringify({ error: { code: "release_channel_unavailable", message: "No verified Overgent release is currently available.", retryable: true } }));
  }
};
