import { NextRequest, NextResponse } from "next/server";

function backendBaseUrl() {
  const raw = process.env.BACKEND_URL || "http://localhost:8080";
  return raw.replace(/\/+$/, "");
}

export async function GET(req: NextRequest) {
  const adminKey = req.headers.get("X-Admin-Key");
  if (!adminKey) {
    return NextResponse.json({ error: "missing_key" }, { status: 401 });
  }

  const url = `${backendBaseUrl()}/api/v1/admin/tokens`;
  const res = await fetch(url, {
    headers: { "X-Admin-Key": adminKey },
    cache: "no-store",
  });

  const data = await res.json();
  return NextResponse.json(data, { status: res.status });
}

export async function POST(req: NextRequest) {
  const adminKey = req.headers.get("X-Admin-Key");
  if (!adminKey) {
    return NextResponse.json({ error: "missing_key" }, { status: 401 });
  }

  const body = await req.json();
  const url = `${backendBaseUrl()}/api/v1/admin/tokens`;
  
  const res = await fetch(url, {
    method: "POST",
    headers: { 
      "X-Admin-Key": adminKey,
      "Content-Type": "application/json"
    },
    body: JSON.stringify(body),
  });

  const data = await res.json();
  return NextResponse.json(data, { status: res.status });
}

