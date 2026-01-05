import type {
  BackendAltCheckResponse,
  BackendConnectedAccount,
  BackendProfileResponse,
  BackendUserHistoryRow,
  BackendPresenceHistory,
  BackendActivityHistory,
} from "@/lib/api-types";

export type ProfileViewModel = {
  user: {
    id: string;
    status: string;
    createdAt: string;
    displayName: string;
    username: string | null;
    globalName: string | null;
    avatarUrl: string | null;
    bioText: string | null;
  };
  currentVoice: {
    channelName: string;
    guildName: string;
    joinedAt: string;
  } | null;
  lastPresence: {
    status: string;
    changedAt: string;
    guildId: string | null;
  } | null;
  lastActivity: {
    name: string;
    startedAt: string;
    endedAt: string | null;
    details: string | null;
    state: string | null;
    type: number;
  } | null;
  timeline: {
    events: Array<{
      id: string;
      kind: "username" | "global_name" | "nickname" | "bio";
      at: string;
      from: string | null;
      to: string | null;
    }>;
    avatars: Array<{ id: string; url: string; at: string }>;
  };
  connections: Array<{
    id: string;
    platform: string;
    label: string;
    url: string | null;
  }>;
  altCheck: {
    relatedIds: string[];
  };
  guilds: Array<{
    id: string;
    name: string;
    joinedAt: string;
    leftAt: string | null;
    iconUrl: string | null;
  }>;
  voiceHistory: Array<{
    id: string;
    channelName: string;
    guildName: string;
    joinedAt: string;
    leftAt: string | null;
    duration: string;
    wasVideo: boolean;
    wasStreaming: boolean;
    wasMuted: boolean;
    wasDeafened: boolean;
  }>;
  nicknames: Array<{
    id: string;
    nickname: string;
    guildName: string;
    observedAt: string;
  }>;
  messages: Array<{
    id: string;
    content: string;
    guildName: string;
    channelName: string;
    createdAt: string;
    hasAttachments: boolean;
    hasEmbeds: boolean;
    replyToUserId: string | null;
  }>;
  voicePartners: Array<{
    id: string;
    partnerId: string;
    partnerName: string;
    partnerAvatarUrl: string | null;
    sessionCount: number;
    totalDuration: string;
    lastSessionAt: string;
  }>;
  presenceHistory: Array<{
    id: string;
    status: string;
    guildId: string | null;
    changedAt: string;
  }>;
  activityHistory: Array<{
    id: string;
    name: string;
    details: string | null;
    state: string | null;
    type: number;
    startedAt: string;
    endedAt: string | null;
    url: string | null;
    applicationId: string | null;
    spotifyTrackId: string | null;
    spotifyArtist: string | null;
    spotifyAlbum: string | null;
  }>;
  bannerHistory: Array<{
    id: string;
    bannerHash: string | null;
    bannerColor: string | null;
    urlCdn: string | null;
    changedAt: string;
  }>;
  clanHistory: Array<{
    id: string;
    clanTag: string | null;
    badge: string | null;
    changedAt: string;
  }>;
  avatarDecorationHistory: Array<{
    id: string;
    decorationAsset: string | null;
    decorationSkuId: string | null;
    changedAt: string;
  }>;
};

function formatDuration(seconds: number | null | undefined): string | null {
  if (!seconds) return null;
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
}

function bestName(r: BackendUserHistoryRow | undefined | null, fallback: string) {
  // Requirement: if user has no display name (global_name), show username.
  // Do not prefer guild nickname over username here.
  return r?.global_name || r?.username || fallback;
}

function normalizeText(v: string | null | undefined) {
  const t = (v || "").trim();
  return t === "" ? null : t;
}

function uniqBy<T>(arr: T[], key: (t: T) => string) {
  const seen = new Set<string>();
  const out: T[] = [];
  for (const item of arr) {
    const k = key(item);
    if (seen.has(k)) continue;
    seen.add(k);
    out.push(item);
  }
  return out;
}

function accountUrl(a: BackendConnectedAccount): string | null {
  const ext = normalizeText(a.external_id || null);
  if (!ext) return null;
  switch (a.type) {
    case "steam":
      return `https://steamcommunity.com/profiles/${encodeURIComponent(ext)}`;
    case "spotify":
      return `https://open.spotify.com/user/${encodeURIComponent(ext)}`;
    case "github":
      return `https://github.com/${encodeURIComponent(ext)}`;
    default:
      return null;
  }
}

function accountLabel(a: BackendConnectedAccount) {
  return normalizeText(a.name || null) || normalizeText(a.external_id || null) || "unknown";
}

function guildIconUrl(guildId: string, iconHash: string | null | undefined): string | null {
  if (!iconHash) return null;
  return `https://cdn.discordapp.com/icons/${guildId}/${iconHash}.png`;
}

function firstNonEmpty<T>(arr: T[], pick: (t: T) => string | null | undefined): string | null {
  for (const item of arr) {
    const v = normalizeText(pick(item) || null);
    if (v) return v;
  }
  return null;
}

export function buildProfileViewModel(
  profile: BackendProfileResponse,
  alt: BackendAltCheckResponse | null,
): ProfileViewModel {
  // validar que profile e profile.user existem
  if (!profile || !profile.user || !profile.user.id) {
    throw new Error("Invalid profile data: missing user information");
  }

  const history = profile.user_history || [];
  const head = history[0];

  // Valores "atuais" devem ser o último valor conhecido por campo,
  // não necessariamente o mesmo registro do topo (que pode ser só username/presence).
  const currentUsername = firstNonEmpty(history, (h) => h.username);
  const currentGlobalName = firstNonEmpty(history, (h) => h.global_name);
  const currentBio = firstNonEmpty(history, (h) => h.bio_content);

  // avatars (dedupe por url)
  const avatars = uniqBy(
    history
      .filter((h) => normalizeText(h.avatar_url || null))
      .map((h) => ({
        id: String(h.id),
        url: h.avatar_url as string,
        at: h.observed_at,
      })),
    (a) => a.url,
  ).slice(0, 30);

  const currentAvatarUrl = normalizeText(avatars[0]?.url || null);

  // eventos de mudança (comparando linha atual vs próxima)
  const events: ProfileViewModel["timeline"]["events"] = [];
  for (let i = 0; i < Math.min(history.length - 1, 200); i++) {
    const cur = history[i];
    const prev = history[i + 1];
    const at = cur.observed_at;

    const fields: Array<{
      kind: ProfileViewModel["timeline"]["events"][number]["kind"];
      cur: string | null;
      prev: string | null;
    }> = [
        { kind: "username", cur: normalizeText(cur.username), prev: normalizeText(prev.username) },
        { kind: "global_name", cur: normalizeText(cur.global_name), prev: normalizeText(prev.global_name) },
        { kind: "nickname", cur: normalizeText(cur.nickname), prev: normalizeText(prev.nickname) },
        { kind: "bio", cur: normalizeText(cur.bio_content), prev: normalizeText(prev.bio_content) },
      ];

    for (const f of fields) {
      // Ignorar se ambos são null - significa que são registros de tipos diferentes
      // (um registro de avatar comparado com um de username, por exemplo)
      if (f.cur === null && f.prev === null) continue;

      // Só mostrar mudança se realmente houve alteração no valor
      if (f.cur !== f.prev) {
        events.push({
          id: `${cur.id}-${f.kind}`,
          kind: f.kind,
          at,
          from: f.prev,
          to: f.cur,
        });
      }
    }
  }

  const connections = uniqBy(
    (profile.connected_accounts || []).map((a) => ({
      id: String(a.id),
      platform: a.type,
      label: accountLabel(a),
      url: accountUrl(a),
    })),
    (c) => `${c.platform}:${c.label}:${c.url || ""}`,
  ).slice(0, 50);

  // Map guilds
  const guilds = (profile.guilds || []).map((g) => ({
    id: g.guild_id,
    name: g.guild_name || g.guild_id,
    joinedAt: g.joined_at || new Date().toISOString(),
    leftAt: null,
    iconUrl: guildIconUrl(g.guild_id, g.guild_icon),
  }));

  // Map voice history - filter out sessions with 0 duration unless they are still active (no left_at)
  const voiceHistory = (profile.voice_history || [])
    .filter((v) => v.left_at === null || v.left_at === undefined || (v.duration_seconds && v.duration_seconds > 0))
    .map((v, index) => ({
      id: `voice-${index}`,
      channelName: v.channel_name || v.channel_id || "Unknown Channel",
      guildName: v.guild_name || v.guild_id || "Unknown Guild",
      joinedAt: v.joined_at,
      leftAt: v.left_at || null,
      duration: formatDuration(v.duration_seconds) || "0s",
      wasVideo: v.was_video || false,
      wasStreaming: v.was_streaming || false,
      wasMuted: v.was_muted || false,
      wasDeafened: v.was_deafened || false,
    }));

  // Map nickname history
  const nicknames = (profile.nickname_history || []).map((n, index) => ({
    id: `nick-${index}`,
    nickname: n.nickname,
    guildName: n.guild_name || n.guild_id || "Unknown Guild",
    observedAt: n.changed_at,
  }));

  // Map presence history
  const presenceHistory = (profile.presence_history || []).map((p, index) => ({
    id: `presence-${index}`,
    status: p.status,
    guildId: p.guild_id || null,
    changedAt: p.changed_at,
  }));

  // Map activity history
  const activityHistory = (profile.activity_history || []).map((a, index) => ({
    id: `activity-${index}`,
    name: a.name,
    details: a.details || null,
    state: a.state || null,
    type: a.type,
    startedAt: a.started_at,
    endedAt: a.ended_at || null,
    url: a.url || null,
    applicationId: a.application_id || null,
    spotifyTrackId: a.spotify_track_id || null,
    spotifyArtist: a.spotify_artist || null,
    spotifyAlbum: a.spotify_album || null,
  }));

  // Map banner history
  const bannerHistory = (profile.banner_history || []).map((b, index) => ({
    id: `banner-${index}`,
    bannerHash: b.banner_hash || null,
    bannerColor: b.banner_color || null,
    urlCdn: b.url_cdn || null,
    changedAt: b.changed_at,
  }));

  // Map clan history
  const clanHistory = (profile.clan_history || []).map((c, index) => ({
    id: `clan-${index}`,
    clanTag: c.clan_tag || null,
    badge: c.badge || null,
    changedAt: c.changed_at,
  }));

  // Map avatar decoration history
  const avatarDecorationHistory = (profile.avatar_decoration_history || []).map((ad, index) => ({
    id: `decoration-${index}`,
    decorationAsset: ad.decoration_asset || null,
    decorationSkuId: ad.decoration_sku_id || null,
    changedAt: ad.changed_at,
  }));

  // Map messages
  const messages = (profile.messages || []).map((m) => ({
    id: m.message_id,
    content: m.content || "",
    guildName: m.guild_name || m.guild_id || "DM",
    channelName: m.channel_name || m.channel_id || "Unknown Channel",
    createdAt: m.created_at,
    hasAttachments: m.has_attachments || false,
    hasEmbeds: m.has_embeds || false,
    replyToUserId: m.reply_to_user_id || null,
  }));

  // Map voice partners (people they call with most)
  const voicePartners = (profile.voice_partners || []).map((p, index) => {
    // Build partner avatar URL from hash
    let partnerAvatarUrl: string | null = null;
    if (p.partner_avatar_hash) {
      const ext = p.partner_avatar_hash.startsWith("a_") ? "gif" : "png";
      partnerAvatarUrl = `https://cdn.discordapp.com/avatars/${p.partner_id}/${p.partner_avatar_hash}.${ext}`;
    }
    return {
      id: `partner-${index}`,
      partnerId: p.partner_id,
      partnerName: p.partner_name || p.partner_id,
      partnerAvatarUrl,
      sessionCount: p.session_count,
      totalDuration: formatDuration(p.total_duration_seconds) || "0s",
      lastSessionAt: p.last_session_at,
    };
  });

  // Detect current voice session
  const currentVoice = voiceHistory.length > 0 && !voiceHistory[0].leftAt
    ? {
      channelName: voiceHistory[0].channelName,
      guildName: voiceHistory[0].guildName,
      joinedAt: voiceHistory[0].joinedAt,
    }
    : null;

  const presenceHead: BackendPresenceHistory | undefined = (profile.presence_history || [])[0];
  const lastPresence = presenceHead
    ? {
      status: presenceHead.status,
      changedAt: presenceHead.changed_at,
      guildId: presenceHead.guild_id || null,
    }
    : null;

  const activityHead: BackendActivityHistory | undefined = (profile.activity_history || [])[0];
  const lastActivity = activityHead
    ? {
      name: activityHead.name,
      startedAt: activityHead.started_at,
      endedAt: activityHead.ended_at || null,
      details: activityHead.details || null,
      state: activityHead.state || null,
      type: activityHead.type,
    }
    : null;

  return {
    user: {
      id: profile.user.id,
      status: profile.user.status || "active",
      createdAt: profile.user.created_at || new Date().toISOString(),
      displayName: currentGlobalName || currentUsername || bestName(head, profile.user.id),
      username: currentUsername,
      globalName: currentGlobalName,
      avatarUrl: currentAvatarUrl,
      bioText: currentBio,
    },
    currentVoice,
    lastPresence,
    lastActivity,
    timeline: { events, avatars },
    connections,
    altCheck: { relatedIds: alt?.related || [] },
    guilds,
    voiceHistory,
    nicknames,
    messages,
    voicePartners,
    presenceHistory,
    activityHistory,
    bannerHistory,
    clanHistory,
    avatarDecorationHistory,
  };
}


