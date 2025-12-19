import { ProfileCard } from "@/components/ProfileCard";
import { NotFoundCard } from "@/components/NotFoundCard";
import { ArrowLeft } from "lucide-react";
import Link from "next/link";
import type { BackendAltCheckResponse, BackendProfileResponse } from "@/lib/api-types";
import { backendFetchJson, type BackendFetchError } from "@/lib/backend";
import { buildProfileViewModel } from "@/lib/profile-mapper";

// Transforma o formato do backend para o formato esperado pelo frontend
function transformBackendResponse(data: Record<string, unknown>, discordId: string): BackendProfileResponse {
  const firstSeen = typeof data.first_seen === "string" ? data.first_seen : new Date().toISOString();
  
  const userHistory: BackendProfileResponse["user_history"] = [];
  let idCounter = 1;
  
  // Converter username_history
  const usernameHistory = Array.isArray(data.username_history) ? data.username_history : [];
  for (const entry of usernameHistory) {
    const e = entry as Record<string, unknown>;
    userHistory.push({
      id: idCounter++,
      user_id: discordId,
      username: typeof e.username === "string" ? e.username : null,
      discriminator: typeof e.discriminator === "string" ? e.discriminator : null,
      global_name: typeof e.global_name === "string" ? e.global_name : null,
      nickname: null,
      avatar_hash: null,
      avatar_url: null,
      bio_content: null,
      observed_at: typeof e.changed_at === "string" ? e.changed_at : firstSeen,
    });
  }

  // Converter avatar_history
  const avatarHistory = Array.isArray(data.avatar_history) ? data.avatar_history : [];
  for (const entry of avatarHistory) {
    const e = entry as Record<string, unknown>;
    let avatarUrl = typeof e.avatar_url === "string" && e.avatar_url ? e.avatar_url : null;
    const avatarHash = typeof e.avatar_hash === "string" ? e.avatar_hash : null;
    if (!avatarUrl && avatarHash) {
      const ext = avatarHash.startsWith("a_") ? "gif" : "png";
      avatarUrl = `https://cdn.discordapp.com/avatars/${discordId}/${avatarHash}.${ext}`;
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
      observed_at: typeof e.changed_at === "string" ? e.changed_at : firstSeen,
    });
  }

  // Converter bio_history
  const bioHistory = Array.isArray(data.bio_history) ? data.bio_history : [];
  for (const entry of bioHistory) {
    const e = entry as Record<string, unknown>;
    userHistory.push({
      id: idCounter++,
      user_id: discordId,
      username: null,
      discriminator: null,
      global_name: null,
      nickname: null,
      avatar_hash: null,
      avatar_url: null,
      bio_content: typeof e.bio_content === "string" ? e.bio_content : null,
      observed_at: typeof e.changed_at === "string" ? e.changed_at : firstSeen,
    });
  }

  // Ordenar por observed_at desc
  userHistory.sort((a, b) => new Date(b.observed_at).getTime() - new Date(a.observed_at).getTime());

  // Converter connections
  const connectedAccounts: BackendProfileResponse["connected_accounts"] = [];
  const connections = Array.isArray(data.connections) ? data.connections : [];
  for (let i = 0; i < connections.length; i++) {
    const conn = connections[i] as Record<string, unknown>;
    connectedAccounts.push({
      id: i + 1,
      user_id: discordId,
      type: typeof conn.type === "string" ? conn.type : "unknown",
      external_id: typeof conn.external_id === "string" ? conn.external_id : null,
      name: typeof conn.name === "string" ? conn.name : null,
      observed_at: typeof conn.first_seen === "string" ? conn.first_seen : firstSeen,
    });
  }

  // Pass through new history arrays
  const nicknameHistory = Array.isArray(data.nickname_history) ? (data.nickname_history as any[]) : [];
  const guilds = Array.isArray(data.guilds) ? (data.guilds as any[]) : [];
  const voiceHistory = Array.isArray(data.voice_history) ? (data.voice_history as any[]) : [];

  return {
    user: {
      id: discordId,
      status: "active",
      created_at: firstSeen,
    },
    user_history: userHistory,
    connected_accounts: connectedAccounts,
    nickname_history: nicknameHistory.map(n => ({
      guild_id: n.guild_id,
      guild_name: n.guild_name,
      nickname: n.nickname,
      changed_at: n.changed_at
    })),
    guilds: guilds.map(g => ({
      guild_id: g.guild_id,
      guild_name: g.guild_name,
      joined_at: g.joined_at,
      last_seen_at: g.last_seen_at
    })),
    voice_history: voiceHistory.map(v => ({
      guild_id: v.guild_id,
      guild_name: v.guild_name,
      channel_id: v.channel_id,
      channel_name: v.channel_name,
      joined_at: v.joined_at,
      left_at: v.left_at,
      duration_seconds: v.duration_seconds,
      was_video: v.was_video,
      was_streaming: v.was_streaming
    })),
  };
}

async function fetchProfile(id: string): Promise<BackendProfileResponse> {
  const raw = await backendFetchJson<Record<string, unknown>>(`/profile/${encodeURIComponent(id)}`);
  return transformBackendResponse(raw, id);
}

async function fetchAltCheck(id: string) {
  return backendFetchJson<BackendAltCheckResponse>(`/alt-check/${encodeURIComponent(id)}`);
}

export default async function ProfilePage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;

  let profile: BackendProfileResponse;
  try {
    profile = await fetchProfile(id);
  } catch (e) {
    const err = e as BackendFetchError;
    if (err?.status === 404 || err?.status === 400) return <NotFoundCard />;
    throw e;
  }

  // validar que profile tem a estrutura esperada
  if (!profile || !profile.user || !profile.user.id) {
    return <NotFoundCard />;
  }

  let alt: BackendAltCheckResponse | null = null;
  try {
    alt = await fetchAltCheck(id);
  } catch {
    // se falhar, a pagina continua sem o badge de alt
  }

  let vm;
  try {
    vm = buildProfileViewModel(profile, alt);
  } catch (e) {
    console.error("Failed to build profile view model:", e);
    return <NotFoundCard />;
  }

  return (
    <div className="w-full min-h-screen py-12 px-4 relative">
       <Link href="/" className="fixed top-8 left-8 p-3 glass rounded-full hover:bg-white/10 transition-colors z-50 text-neutral-400 hover:text-white">
          <ArrowLeft className="w-5 h-5" />
       </Link>

      <ProfileCard 
        user={vm.user}
        timeline={vm.timeline}
        connections={vm.connections}
        possibleAltIds={vm.altCheck.relatedIds}
        guilds={vm.guilds}
        voiceHistory={vm.voiceHistory}
        nicknames={vm.nicknames}
      />
    </div>
  );
}

