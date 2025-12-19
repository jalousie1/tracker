import { proxyGet } from "../../_proxy";
import type { BackendProfileResponse, BackendUserHistoryRow, BackendConnectedAccount } from "@/lib/api-types";
import { NextResponse } from "next/server";

export const dynamic = "force-dynamic";

// transformar formato do backend para formato esperado pelo frontend
type JsonObject = Record<string, unknown>;

function asObject(v: unknown): JsonObject {
  return v && typeof v === "object" ? (v as JsonObject) : {};
}

function asString(v: unknown): string | null {
  return typeof v === "string" ? v : null;
}

function transformBackendResponse(data: unknown): BackendProfileResponse {
  const root = asObject(data);
  const discordId = asString(root.discord_id) ?? "";
  const firstSeen = asString(root.first_seen) ?? new Date().toISOString();
  // o backend retorna: { discord_id, first_seen, last_updated, username_history, avatar_history, bio_history, connections }
  // o frontend espera: { user: { id, status, created_at }, user_history: [...], connected_accounts: [...] }

  const userHistory: BackendUserHistoryRow[] = [];
  let idCounter = 1;
  
  // converter username_history em user_history rows
  const usernameHistory = Array.isArray(root.username_history) ? root.username_history : [];
  if (usernameHistory.length > 0) {
    for (const entry of usernameHistory) {
      const e = asObject(entry);
      userHistory.push({
        id: idCounter++,
        user_id: discordId,
        username: asString(e.username),
        discriminator: asString(e.discriminator),
        global_name: asString(e.global_name),
        nickname: null,
        avatar_hash: null,
        avatar_url: null,
        bio_content: null,
        observed_at: asString(e.changed_at) ?? firstSeen,
      });
    }
  }

  // adicionar avatares ao user_history
  const avatarHistory = Array.isArray(root.avatar_history) ? root.avatar_history : [];
  if (avatarHistory.length > 0) {
    for (const entry of avatarHistory) {
      const e = asObject(entry);
      // construir url do avatar se nao existir
      let avatarUrl = asString(e.avatar_url);
      const avatarHash = asString(e.avatar_hash);
      if (!avatarUrl && avatarHash) {
        const hash = avatarHash;
        const ext = hash.startsWith("a_") ? "gif" : "png";
        avatarUrl = `https://cdn.discordapp.com/avatars/${discordId}/${hash}.${ext}`;
      }

      userHistory.push({
        id: idCounter++,
        user_id: discordId,
        username: null,
        discriminator: null,
        global_name: null,
        nickname: null,
        avatar_hash: avatarHash,
        avatar_url: avatarUrl,
        bio_content: null,
        observed_at: asString(e.changed_at) ?? firstSeen,
      });
    }
  }

  // adicionar bios ao user_history
  const bioHistory = Array.isArray(root.bio_history) ? root.bio_history : [];
  if (bioHistory.length > 0) {
    for (const entry of bioHistory) {
      const e = asObject(entry);
      userHistory.push({
        id: idCounter++,
        user_id: discordId,
        username: null,
        discriminator: null,
        global_name: null,
        nickname: null,
        avatar_hash: null,
        avatar_url: null,
        bio_content: asString(e.bio_content),
        observed_at: asString(e.changed_at) ?? firstSeen,
      });
    }
  }

  // ordenar por observed_at desc
  userHistory.sort((a, b) => {
    const dateA = new Date(a.observed_at).getTime();
    const dateB = new Date(b.observed_at).getTime();
    return dateB - dateA;
  });

  // converter connections
  const connectedAccounts: BackendConnectedAccount[] = [];
  const connections = Array.isArray(root.connections) ? root.connections : [];
  if (connections.length > 0) {
    for (let i = 0; i < connections.length; i++) {
      const conn = asObject(connections[i]);
      connectedAccounts.push({
        id: i + 1,
        user_id: discordId,
        type: asString(conn.type) ?? "unknown",
        external_id: asString(conn.external_id),
        name: asString(conn.name),
        observed_at: asString(conn.first_seen) ?? firstSeen,
      });
    }
  }

  return {
    user: {
      id: discordId,
      status: "active", // assumir active por padrao
      created_at: firstSeen,
    },
    user_history: userHistory,
    connected_accounts: connectedAccounts,
  };
}

export async function GET(req: Request, ctx: { params: Promise<{ discordId: string }> }) {
  const { discordId } = await ctx.params;
  
  try {
    const response = await proxyGet(`/profile/${encodeURIComponent(discordId)}`, req.signal);
    
    if (!response.ok) {
      return response; // retornar erro como esta
    }

    const raw: unknown = await response.json();

    const obj = raw && typeof raw === "object" ? (raw as JsonObject) : null;
    if (!obj) {
      return NextResponse.json(
        { error: { code: "invalid_response", message: "Invalid response from backend" } },
        { status: 500 }
      );
    }

    const normalized: JsonObject = {
      ...obj,
      discord_id: asString(obj.discord_id) ?? discordId,
    };

    const transformed = transformBackendResponse(normalized);
    
    // validar que a transformacao retornou um objeto valido
    if (!transformed || !transformed.user || !transformed.user.id) {
      console.error("Transform failed:", { raw, transformed });
      return NextResponse.json(
        { error: { code: "transform_error", message: "Failed to transform profile data" } },
        { status: 500 }
      );
    }
    
    return NextResponse.json(transformed, {
      status: 200,
      headers: {
        "cache-control": "no-store",
        "x-content-type-options": "nosniff",
      },
    });
  } catch (error) {
    console.error("Profile fetch error:", error);
    return NextResponse.json(
      { error: { code: "internal_error", message: error instanceof Error ? error.message : "Failed to fetch profile" } },
      { status: 500 }
    );
  }
}


