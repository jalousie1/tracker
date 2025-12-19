import { NextResponse } from "next/server";

function backendBaseUrl() {
  const raw = process.env.BACKEND_URL || "http://localhost:8080";
  return raw.replace(/\/+$/, "");
}

export async function proxyGet(pathWithQuery: string, signal?: AbortSignal) {
  // Adicionar prefixo /api/v1 se não estiver presente
  let apiPath = pathWithQuery;
  if (!apiPath.startsWith("/api/v1")) {
    apiPath = `/api/v1${apiPath.startsWith("/") ? "" : "/"}${apiPath}`;
  }
  const url = `${backendBaseUrl()}${apiPath}`;

  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(new Error("timeout")), 15_000);
  try {
    if (signal) {
      if (signal.aborted) controller.abort(signal.reason);
      else signal.addEventListener("abort", () => controller.abort(signal.reason), { once: true });
    }

    const upstream = await fetch(url, {
      method: "GET",
      headers: { accept: "application/json" },
      cache: "no-store",
      signal: controller.signal,
    });

    const text = await upstream.text();
    const contentType = upstream.headers.get("content-type") || "application/json; charset=utf-8";

    return new NextResponse(text, {
      status: upstream.status,
      headers: {
        "content-type": contentType,
        "cache-control": "no-store",
        // hardening basico pra evitar interpretacao incorreta
        "x-content-type-options": "nosniff",
      },
    });
  } finally {
    clearTimeout(timeout);
  }
}


