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

  const stringFrom = (obj: Record<string, unknown>, key: string): string | null => {
    const v = obj[key];
    return typeof v === "string" ? v : null;
  };

  const numberFrom = (obj: Record<string, unknown>, key: string): number | null => {
    const v = obj[key];
    return typeof v === "number" && Number.isFinite(v) ? v : null;
  };

  const booleanFrom = (obj: Record<string, unknown>, key: string): boolean | undefined => {
    const v = obj[key];
    return typeof v === "boolean" ? v : undefined;
  };

  const isNotNull = <T,>(v: T | null): v is T => v !== null;

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

  // Pass through new history arrays (sem any)
  const nicknameHistory = Array.isArray(data.nickname_history) ? (data.nickname_history as Record<string, unknown>[]) : [];
  const guilds = Array.isArray(data.guilds) ? (data.guilds as Record<string, unknown>[]) : [];
  const voiceHistory = Array.isArray(data.voice_history) ? (data.voice_history as Record<string, unknown>[]) : [];
  const presenceHistory = Array.isArray(data.presence_history) ? (data.presence_history as Record<string, unknown>[]) : [];
  const activityHistory = Array.isArray(data.activity_history) ? (data.activity_history as Record<string, unknown>[]) : [];
  const messages = Array.isArray(data.messages) ? (data.messages as Record<string, unknown>[]) : [];
  const voicePartners = Array.isArray(data.voice_partners) ? (data.voice_partners as Record<string, unknown>[]) : [];
  const bannerHistory = Array.isArray(data.banner_history) ? (data.banner_history as Record<string, unknown>[]) : [];
  const clanHistory = Array.isArray(data.clan_history) ? (data.clan_history as Record<string, unknown>[]) : [];
  const avatarDecorationHistory = Array.isArray(data.avatar_decoration_history) ? (data.avatar_decoration_history as Record<string, unknown>[]) : [];

  return {
    user: {
      id: discordId,
      status: "active",
      created_at: firstSeen,
    },
    user_history: userHistory,
    connected_accounts: connectedAccounts,
    nickname_history: nicknameHistory
      .map((n) => {
        const guildId = stringFrom(n, "guild_id");
        const nickname = stringFrom(n, "nickname");
        const changedAt = stringFrom(n, "changed_at") ?? stringFrom(n, "observed_at") ?? firstSeen;
        if (!guildId || !nickname) return null;
        return {
          guild_id: guildId,
          guild_name: stringFrom(n, "guild_name"),
          guild_icon: stringFrom(n, "guild_icon"),
          nickname,
          changed_at: changedAt,
        };
      })
      .filter(isNotNull),
    guilds: guilds
      .map((g) => {
        const guildId = stringFrom(g, "guild_id");
        const lastSeenAt = stringFrom(g, "last_seen_at") ?? firstSeen;
        if (!guildId) return null;
        return {
          guild_id: guildId,
          guild_name: stringFrom(g, "guild_name"),
          guild_icon: stringFrom(g, "guild_icon"),
          joined_at: stringFrom(g, "joined_at"),
          last_seen_at: lastSeenAt,
        };
      })
      .filter(isNotNull),
    voice_history: voiceHistory
      .map((v) => {
        const guildId = stringFrom(v, "guild_id");
        const channelId = stringFrom(v, "channel_id");
        const joinedAt = stringFrom(v, "joined_at") ?? firstSeen;
        if (!guildId || !channelId) return null;
        const durationSeconds = numberFrom(v, "duration_seconds");
        return {
          guild_id: guildId,
          guild_name: stringFrom(v, "guild_name"),
          guild_icon: stringFrom(v, "guild_icon"),
          channel_id: channelId,
          channel_name: stringFrom(v, "channel_name"),
          joined_at: joinedAt,
          left_at: stringFrom(v, "left_at"),
          duration_seconds: durationSeconds,
          was_video: booleanFrom(v, "was_video"),
          was_streaming: booleanFrom(v, "was_streaming"),
          was_muted: booleanFrom(v, "was_muted"),
          was_deafened: booleanFrom(v, "was_deafened"),
        };
      })
      .filter(isNotNull),
    presence_history: presenceHistory
      .map((p) => {
        const status = stringFrom(p, "status");
        const changedAt = stringFrom(p, "changed_at") ?? firstSeen;
        if (!status) return null;
        return {
          status,
          guild_id: stringFrom(p, "guild_id"),
          changed_at: changedAt,
        };
      })
      .filter(isNotNull),
    activity_history: activityHistory
      .map((a) => {
        const name = stringFrom(a, "name");
        const type = numberFrom(a, "type");
        const startedAt = stringFrom(a, "started_at") ?? firstSeen;
        if (!name || type === null) return null;
        return {
          name,
          details: stringFrom(a, "details"),
          state: stringFrom(a, "state"),
          type,
          started_at: startedAt,
          ended_at: stringFrom(a, "ended_at"),
          url: stringFrom(a, "url"),
          application_id: stringFrom(a, "application_id"),
          spotify_track_id: stringFrom(a, "spotify_track_id"),
          spotify_artist: stringFrom(a, "spotify_artist"),
          spotify_album: stringFrom(a, "spotify_album"),
        };
      })
      .filter(isNotNull),
    messages: messages
      .map((m) => {
        const messageId = stringFrom(m, "message_id") ?? stringFrom(m, "id");
        const guildId = stringFrom(m, "guild_id");
        const channelId = stringFrom(m, "channel_id");
        const createdAt = stringFrom(m, "created_at") ?? firstSeen;
        if (!messageId || !guildId || !channelId) return null;
        return {
          message_id: messageId,
          guild_id: guildId,
          guild_name: stringFrom(m, "guild_name"),
          guild_icon: stringFrom(m, "guild_icon"),
          channel_id: channelId,
          channel_name: stringFrom(m, "channel_name"),
          content: stringFrom(m, "content"),
          has_attachments: booleanFrom(m, "has_attachments"),
          has_embeds: booleanFrom(m, "has_embeds"),
          reply_to_user_id: stringFrom(m, "reply_to_user_id"),
          created_at: createdAt,
        };
      })
      .filter(isNotNull),
    voice_partners: voicePartners
      .map((p) => {
        const partnerId = stringFrom(p, "partner_id");
        const sessionCount = numberFrom(p, "session_count");
        const totalDurationSeconds = numberFrom(p, "total_duration_seconds");
        const lastSessionAt = stringFrom(p, "last_session_at") ?? firstSeen;
        if (!partnerId || sessionCount === null || totalDurationSeconds === null) return null;
        return {
          partner_id: partnerId,
          partner_name: stringFrom(p, "partner_name"),
          partner_avatar_hash: stringFrom(p, "partner_avatar_hash"),
          guild_id: stringFrom(p, "guild_id"),
          guild_name: stringFrom(p, "guild_name"),
          guild_icon: stringFrom(p, "guild_icon"),
          session_count: sessionCount,
          total_duration_seconds: totalDurationSeconds,
          last_session_at: lastSessionAt,
        };
      })
      .filter(isNotNull),
    banner_history: bannerHistory
      .map((b) => {
        const changedAt = stringFrom(b, "changed_at") ?? firstSeen;
        return {
          banner_hash: stringFrom(b, "banner_hash"),
          banner_color: stringFrom(b, "banner_color"),
          url_cdn: stringFrom(b, "url_cdn"),
          changed_at: changedAt,
        };
      }),
    clan_history: clanHistory
      .map((c) => {
        const changedAt = stringFrom(c, "changed_at") ?? firstSeen;
        return {
          clan_tag: stringFrom(c, "clan_tag"),
          badge: stringFrom(c, "badge"),
          changed_at: changedAt,
        };
      }),
    avatar_decoration_history: avatarDecorationHistory
      .map((ad) => {
        const changedAt = stringFrom(ad, "changed_at") ?? firstSeen;
        return {
          decoration_asset: stringFrom(ad, "decoration_asset"),
          decoration_sku_id: stringFrom(ad, "decoration_sku_id"),
          changed_at: changedAt,
        };
      })
  };
}

async function fetchProfile(id: string): Promise<BackendProfileResponse> {
  const raw = await backendFetchJson<Record<string, unknown>>(`/profile/${encodeURIComponent(id)}?refresh=1`);
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
    <div className="w-full min-h-screen py-12 px-4 relative flex items-center justify-center">
      <Link href="/" className="fixed top-8 left-8 p-3 glass rounded-full hover:bg-white/10 transition-colors z-50 text-neutral-400 hover:text-white">
        <ArrowLeft className="w-5 h-5" />
      </Link>

      <ProfileCard
        user={vm.user}
        currentVoice={vm.currentVoice}
        lastPresence={vm.lastPresence}
        lastActivity={vm.lastActivity}
        timeline={vm.timeline}
        connections={vm.connections}
        possibleAltIds={vm.altCheck.relatedIds}
        guilds={vm.guilds}
        voiceHistory={vm.voiceHistory}
        nicknames={vm.nicknames}
        messages={vm.messages}
        voicePartners={vm.voicePartners}
        presenceHistory={vm.presenceHistory}
        activityHistory={vm.activityHistory}
        bannerHistory={vm.bannerHistory}
        clanHistory={vm.clanHistory}
        avatarDecorationHistory={vm.avatarDecorationHistory}
      />
    </div>
  );
}

