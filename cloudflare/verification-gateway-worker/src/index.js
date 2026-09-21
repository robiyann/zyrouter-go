const ROUTES = [
  /^\/api\/user\/verification\//,
  /^\/api\/user\/verification$/,
  /^\/api\/telegram\/webhook$/,
];

export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    if (!ROUTES.some((route) => route.test(url.pathname))) return fetch(request);
    if (request.method === "OPTIONS") return fetch(request);

    const ip = request.headers.get("CF-Connecting-IP") || "unknown";
    const kind = url.pathname === "/api/user/verification/start"
      ? "start"
      : url.pathname === "/api/telegram/webhook"
        ? "webhook"
        : url.pathname.endsWith("/complete") ? "complete" : "poll";

    const limits = { start: [30, 600], poll: [120, 60], complete: [15, 300], webhook: [120, 60] };
    const [limit, windowSeconds] = limits[kind];
    const key = `${kind}:${ip}`;
    const gate = env.RATE_LIMITER.get(env.RATE_LIMITER.idFromName(key));

    // Support instant rate-limit flush via header or query
    if (request.headers.get("x-reset-rate-limit") === "1" || url.searchParams.get("reset") === "1") {
      await gate.fetch("https://rate-limit/reset", { method: "POST" });
    }

    const allowed = await gate.fetch("https://rate-limit/check", {
      method: "POST",
      body: JSON.stringify({ limit, windowSeconds }),
    });
    if (!allowed.ok) {
      return new Response(JSON.stringify({ error: { code: "rate_limited", message: "Too many verification requests" } }), {
        status: 429,
        headers: { "content-type": "application/json", "retry-after": String(windowSeconds) },
      });
    }

    const target = new URL(env.ORIGIN_BASE);
    target.pathname = url.pathname;
    target.search = url.search;
    const headers = new Headers(request.headers);
    headers.set("X-Zyrouter-Edge-Secret", env.EDGE_SHARED_SECRET);
    headers.set("X-Forwarded-Proto", "https");
    headers.delete("host");
    return fetch(target, {
      method: request.method,
      headers,
      body: request.method === "GET" || request.method === "HEAD" ? undefined : request.body,
      redirect: "manual",
    });
  },
};

export class RateLimiter {
  constructor(state) { this.state = state; }

  async fetch(request) {
    const url = new URL(request.url);
    if (url.pathname === "/reset" || request.method === "DELETE") {
      await this.state.storage.deleteAll();
      return new Response("reset_ok");
    }

    const body = await request.json();
    const now = Date.now();
    const current = (await this.state.storage.get("hits")) || [];
    const cutoff = now - body.windowSeconds * 1000;
    const hits = current.filter((value) => value > cutoff);
    if (hits.length >= body.limit) {
      await this.state.storage.put("hits", hits);
      return new Response("limited", { status: 429 });
    }
    hits.push(now);
    await this.state.storage.put("hits", hits);
    return new Response("ok");
  }
}
