export type ApiError = {
  error: {
    code: string;
    message: string;
  };
};

export type BackendUser = {
  id: string;
  status: string;
  created_at: string;
};

export type BackendUserHistoryRow = {
  id: number;
  user_id: string;
  username?: string | null;
  discriminator?: string | null;
  global_name?: string | null;
  nickname?: string | null;
  avatar_hash?: string | null;
  avatar_url?: string | null;
  bio_content?: string | null;
  observed_at: string;
};

export type BackendConnectedAccount = {
  id: number;
  user_id: string;
  type: string;
  external_id?: string | null;
  name?: string | null;
  observed_at: string;
};

export type BackendNicknameHistory = {
  guild_id: string;
  guild_name?: string | null;
  guild_icon?: string | null;
  nickname: string;
  changed_at: string;
};

export type BackendGuildMember = {
  guild_id: string;
  guild_name?: string | null;
  guild_icon?: string | null;
  joined_at?: string | null;
  last_seen_at: string;
};

export type BackendVoiceSession = {
  guild_id: string;
  guild_name?: string | null;
  guild_icon?: string | null;
  channel_id: string;
  channel_name?: string | null;
  joined_at: string;
  left_at?: string | null;
  duration_seconds?: number | null;
  was_video?: boolean;
  was_streaming?: boolean;
  was_muted?: boolean;
  was_deafened?: boolean;
};

export type BackendPresenceHistory = {
  status: string;
  guild_id?: string | null;
  changed_at: string;
};

export type BackendActivityHistory = {
  name: string;
  details?: string | null;
  state?: string | null;
  type: number;
  started_at: string;
  ended_at?: string | null;
  url?: string | null;
  application_id?: string | null;
  spotify_track_id?: string | null;
  spotify_artist?: string | null;
  spotify_album?: string | null;
};

export type BackendBannerHistory = {
  banner_hash?: string | null;
  banner_color?: string | null;
  url_cdn?: string | null;
  changed_at: string;
};

export type BackendClanHistory = {
  clan_tag?: string | null;
  badge?: string | null;
  changed_at: string;
};

export type BackendAvatarDecorationHistory = {
  decoration_asset?: string | null;
  decoration_sku_id?: string | null;
  changed_at: string;
};

export type BackendProfileResponse = {
  user: BackendUser;
  user_history: BackendUserHistoryRow[];
  connected_accounts: BackendConnectedAccount[];
  nickname_history?: BackendNicknameHistory[];
  guilds?: BackendGuildMember[];
  voice_history?: BackendVoiceSession[];
  presence_history?: BackendPresenceHistory[];
  activity_history?: BackendActivityHistory[];
  messages?: BackendMessage[];
  voice_partners?: BackendVoicePartner[];
  banner_history?: BackendBannerHistory[];
  clan_history?: BackendClanHistory[];
  avatar_decoration_history?: BackendAvatarDecorationHistory[];
};

export type BackendSearchResponse = {
  query: string;
  results: Array<{
    user_id: string;
    username?: string | null;
    global_name?: string | null;
    nickname?: string | null;
    observed_at: string;
  }>;
};

export type BackendAltCheckResponse = {
  discord_id: string;
  external_ids: string[];
  related: string[];
};

export type BackendMessage = {
  message_id: string;
  guild_id: string;
  guild_name?: string | null;
  guild_icon?: string | null;
  channel_id: string;
  channel_name?: string | null;
  content?: string | null;
  has_attachments?: boolean;
  has_embeds?: boolean;
  reply_to_user_id?: string | null;
  created_at: string;
};

export type BackendVoicePartner = {
  partner_id: string;
  partner_name?: string | null;
  partner_avatar_hash?: string | null;
  guild_id?: string | null;
  guild_name?: string | null;
  guild_icon?: string | null;
  session_count: number;
  total_duration_seconds: number;
  last_session_at: string;
};


