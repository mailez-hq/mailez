// Streaming proxy for the AI compose endpoint.
//
// Next.js rewrites buffer the entire response body before replying, which
// would turn the SSE stream into one big blob ("一口气输出"). This route
// handler takes precedence over the /api/v1 rewrite and pipes the upstream
// stream straight through, so the compose editor sees tokens as they are
// generated. Production behind nginx never hits this route (nginx proxies
// /api/v1 directly and honors X-Accel-Buffering: no).

const API_TARGET = process.env.API_TARGET || "http://localhost:8080";

export const dynamic = "force-dynamic";

export async function POST(req: Request) {
  const body = await req.text();
  const cookie = req.headers.get("cookie");
  const upstream = await fetch(`${API_TARGET}/api/v1/ai/compose/stream`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      ...(cookie ? { Cookie: cookie } : {}),
    },
    body,
  });
  if (!upstream.body) {
    return new Response("upstream returned no body", { status: 502 });
  }
  return new Response(upstream.body, {
    status: upstream.status,
    headers: {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-cache",
      Connection: "keep-alive",
      "X-Accel-Buffering": "no",
    },
  });
}
