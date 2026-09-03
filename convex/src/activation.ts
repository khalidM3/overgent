type ActivationFailureCopy = { title: string; detail: string };

const RECOVERABLE_TICKET_FAILURE: ActivationFailureCopy = {
  title: "This dashboard link is no longer valid.",
  detail: "Return to the Overgent app and choose Open live Project again. Your Project, repository, and local observations are unchanged.",
};

function activationFailureCopy(code: string): ActivationFailureCopy {
  if (["ticket_invalid", "ticket_consumed", "ticket_expired"].includes(code)) return RECOVERABLE_TICKET_FAILURE;
  if (["unauthorized", "credential_revoked", "forbidden"].includes(code)) return {
    title: "This Mac cannot open that Project.",
    detail: "Return to the Overgent app to check this Mac’s connection. Your repository and local work were not changed.",
  };
  if (code === "rate_limited") return {
    title: "Overgent needs a moment before opening the Project.",
    detail: "Return to the app, wait briefly, and choose Open live Project again. Local observation continues while you wait.",
  };
  return {
    title: "Overgent could not open the live Project.",
    detail: "Return to the app and try again. Local observation continues and your repository was not changed.",
  };
}

export function activationFailureResponse(code: string, status: number, development: boolean): Response {
  const copy = activationFailureCopy(code);
  const target = development ? "overgent-dev://open" : "overgent://open";
  const html = `<!doctype html>
<html lang="en"><meta charset="utf-8"><meta name="referrer" content="no-referrer">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>Overgent could not open the Project</title>
<style>
  :root{color-scheme:light dark;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:#fff;color:#0e0e0e}
  body{margin:0;min-height:100vh;display:grid;place-items:center;background:inherit;color:inherit}
  main{width:min(560px,calc(100vw - 48px));padding:48px 0;border-top:1px solid #e8e8e5;border-bottom:1px solid #e8e8e5}
  h1{font-size:25px;line-height:1.15;letter-spacing:-.03em;margin:0 0 14px}p{color:#5e5e5a;line-height:1.55;margin:0 0 24px}
  a{display:inline-block;border-radius:999px;padding:9px 15px;background:#0e0e0e;color:#fff;text-decoration:none;font-size:13.5px;font-weight:650}
  @media(prefers-color-scheme:dark){:root{background:#0a0a0a;color:#f5f5f5}main{border-color:#232323}p{color:#a3a39f}a{background:#fff;color:#0a0a0a}}
</style><main><h1>${copy.title}</h1><p>${copy.detail}</p><a href="${target}">Return to Overgent</a></main></html>`;
  return new Response(html, {
    status,
    headers: {
      "content-type": "text/html; charset=utf-8",
      "cache-control": "no-store",
      "content-security-policy": "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'",
      "referrer-policy": "no-referrer",
      "x-content-type-options": "nosniff",
    },
  });
}
