import type {
  BackendAltCheckResponse,
  BackendConnectedAccount,
  BackendProfileResponse,
  BackendUserHistoryRow,
} from "@/lib/api-types";

export type ProfileViewModel = {
  user: {
    id: string;
    status: string;
    createdAt: string;
    displayName: string;
    avatarUrl: string | null;
    bioText: string | null;
  };
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
  }>;
  nicknames: Array<{
    id: string;
    nickname: string;
    guildName: string;
    observedAt: string;
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
  return r?.global_name || r?.nickname || r?.username || fallback;
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
    iconUrl: null,
  }));

  // Map voice history
  const voiceHistory = (profile.voice_history || []).map((v, index) => ({
    id: `voice-${index}`,
    channelName: v.channel_name || v.channel_id || "Unknown Channel",
    guildName: v.guild_name || v.guild_id || "Unknown Guild",
    joinedAt: v.joined_at,
    leftAt: v.left_at || null,
    duration: formatDuration(v.duration_seconds) || "0s",
  }));

  // Map nickname history
  const nicknames = (profile.nickname_history || []).map((n, index) => ({
    id: `nick-${index}`,
    nickname: n.nickname,
    guildName: n.guild_name || n.guild_id || "Unknown Guild",
    observedAt: n.changed_at,
  }));

  return {
    user: {
      id: profile.user.id,
      status: profile.user.status || "active",
      createdAt: profile.user.created_at || new Date().toISOString(),
      displayName: bestName(head, profile.user.id),
      avatarUrl: normalizeText(head?.avatar_url || null),
      bioText: normalizeText(head?.bio_content || null),
    },
    timeline: { events, avatars },
    connections,
    altCheck: { relatedIds: alt?.related || [] },
    guilds,
    voiceHistory,
    nicknames,
  };
}


