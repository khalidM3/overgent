const MAX_REQUEST_BYTES = 2 * 1024 * 1024;

function targetOrigin() {
  const raw = process.env.CONVEX_SITE_URL;
  if (!raw) throw new Error("CONVEX_SITE_URL is not configured");
  const parsed = new URL(raw);
  if (parsed.protocol !== "https:" || parsed.username || parsed.password || parsed.pathname !== "/") {
    throw new Error("CONVEX_SITE_URL must be a clean HTTPS origin");
  }
  return parsed;
}

async function requestBody(request) {
  if (request.method === "GET" || request.method === "HEAD") return undefined;
  if (Buffer.isBuffer(request.body)) return boundedBody(request.body);
  if (typeof request.body === "string") return boundedBody(Buffer.from(request.body));
  if (request.body !== undefined && request.body !== null) return boundedBody(Buffer.from(JSON.stringify(request.body)));

  const chunks = [];
  let size = 0;
  for await (const chunk of request) {
    const bytes = Buffer.from(chunk);
    size += bytes.length;
    if (size > MAX_REQUEST_BYTES) throw Object.assign(new Error("request body exceeds limit"), { statusCode: 413 });
    chunks.push(bytes);
  }
  return chunks.length === 0 ? undefined : Buffer.concat(chunks);
}

function boundedBody(body) {
  if (body.length > MAX_REQUEST_BYTES) throw Object.assign(new Error("request body exceeds limit"), { statusCode: 413 });
  return body;
}

module.exports = async function proxyOvergentAPI(request, response) {
  try {
    const origin = targetOrigin();
    const rawPath = Array.isArray(request.query.path) ? request.query.path.join("/") : request.query.path;
    const segments = typeof rawPath === "string" ? rawPath.split("/") : [];
    if (segments.length === 0 || segments.length > 20 || segments.some((segment) => segment.length === 0 || segment.length > 300 || segment === "." || segment === "..")) {
      throw Object.assign(new Error("invalid API path"), { statusCode: 400 });
    }
    const target = new URL(`/v1/${segments.map(encodeURIComponent).join("/")}`, origin);
    const incomingURL = new URL(request.url, "https://api.overgent.com");
    target.search = incomingURL.search;

    const headers = new Headers();
    for (const name of ["accept", "authorization", "content-type", "cookie", "user-agent"]) {
      const value = request.headers[name];
      if (typeof value === "string") headers.set(name, value);
    }

    const upstream = await fetch(target, {
      method: request.method,
      headers,
      body: await requestBody(request),
      redirect: "manual",
    });

    response.statusCode = upstream.status;
    for (const name of ["cache-control", "content-disposition", "content-type", "location", "set-cookie", "x-content-type-options"]) {
      const value = upstream.headers.get(name);
      if (value) response.setHeader(name, value);
    }
    response.setHeader("x-content-type-options", "nosniff");
    response.end(Buffer.from(await upstream.arrayBuffer()));
  } catch (error) {
    response.statusCode = Number.isInteger(error?.statusCode) ? error.statusCode : 502;
    response.setHeader("cache-control", "no-store");
    response.setHeader("content-type", "application/json; charset=utf-8");
    response.setHeader("x-content-type-options", "nosniff");
    response.end(JSON.stringify({ error: { code: "upstream_unavailable", message: "Overgent is temporarily unavailable.", retryable: true } }));
  }
};

module.exports.config = { api: { bodyParser: false } };
