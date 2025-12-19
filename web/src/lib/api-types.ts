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
  nickname: string;
  changed_at: string;
};

export type BackendGuildMember = {
  guild_id: string;
  guild_name?: string | null;
  joined_at?: string | null;
  last_seen_at: string;
};

export type BackendVoiceSession = {
  guild_id: string;
  guild_name?: string | null;
  channel_id: string;
  channel_name?: string | null;
  joined_at: string;
  left_at?: string | null;
  duration_seconds?: number | null;
  was_video?: boolean;
  was_streaming?: boolean;
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


