import { NextRequest } from "next/server";
import { proxyGet } from "../_proxy";

export const dynamic = "force-dynamic";

export async function GET(req: NextRequest) {
  const q = (req.nextUrl.searchParams.get("q") || "").trim();
  if (q.length < 2) {
    return Response.json(
      { error: { code: "invalid_query", message: "q deve ter pelo menos 2 caracteres" } },
      { status: 400, headers: { "cache-control": "no-store" } },
    );
  }

  return proxyGet(`/search?q=${encodeURIComponent(q)}`, req.signal);
}


