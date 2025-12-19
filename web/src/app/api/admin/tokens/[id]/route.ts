import { NextRequest, NextResponse } from "next/server";

function backendBaseUrl() {
  const raw = process.env.BACKEND_URL || "http://localhost:8080";
  return raw.replace(/\/+$/, "");
}

export async function DELETE(req: NextRequest, { params }: { params: Promise<{ id: string }> }) {
  const adminKey = req.headers.get("X-Admin-Key");
  if (!adminKey) {
    return NextResponse.json({ error: "missing_key" }, { status: 401 });
  }

  const { id } = await params;
  const url = `${backendBaseUrl()}/api/v1/admin/tokens/${id}`;
  
  const res = await fetch(url, {
    method: "DELETE",
    headers: { "X-Admin-Key": adminKey },
  });

  const data = await res.json();
  return NextResponse.json(data, { status: res.status });
}

