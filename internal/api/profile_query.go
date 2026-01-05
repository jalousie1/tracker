package api

// profileQueryFull is the complete SQL query for fetching a user profile with all history.
// This is extracted to avoid duplication across multiple handlers.
const profileQueryFull = `SELECT 
	u.id,
	u.created_at::text as first_seen,
	COALESCE(u.last_updated_at, u.created_at)::text as last_updated,
	COALESCE(
		(SELECT json_agg(
			json_build_object(
				'username', uh.username,
				'discriminator', uh.discriminator,
				'global_name', uh.global_name,
				'changed_at', uh.changed_at
			) ORDER BY uh.changed_at DESC
		) FROM username_history uh 
		WHERE uh.user_id = u.id 
		AND (uh.username IS NOT NULL OR uh.global_name IS NOT NULL)
		LIMIT 25
		), '[]'::json
	) as username_history,
	COALESCE(
		(SELECT json_agg(
			json_build_object(
				'avatar_hash', ah.hash_avatar,
				'avatar_url', ah.url_cdn,
				'changed_at', ah.changed_at
			) ORDER BY ah.changed_at DESC
		) FROM avatar_history ah 
		WHERE ah.user_id = u.id 
		LIMIT 25
		), '[]'::json
	) as avatar_history,
	COALESCE(
		(SELECT json_agg(
			json_build_object(
				'bio_content', bh.bio_content,
				'changed_at', bh.changed_at
			) ORDER BY bh.changed_at DESC
		) FROM bio_history bh 
		WHERE bh.user_id = u.id 
		LIMIT 25
		), '[]'::json
	) as bio_history,
	COALESCE(
		(SELECT json_agg(
			json_build_object(
				'type', ca.type,
				'external_id', ca.external_id,
				'name', ca.name,
				'first_seen', ca.observed_at,
				'last_seen', ca.last_seen_at
			) ORDER BY ca.observed_at DESC
		) FROM connected_accounts ca 
		WHERE ca.user_id = u.id 
		LIMIT 25
		), '[]'::json
	) as connections,
	COALESCE(
		(SELECT json_agg(
			json_build_object(
				'guild_id', nh.guild_id,
				'guild_name', COALESCE(g.name, nh.guild_id),
				'guild_icon', g.icon,
				'nickname', nh.nickname,
				'changed_at', nh.changed_at
			) ORDER BY nh.changed_at DESC
		) FROM nickname_history nh 
		LEFT JOIN guilds g ON g.guild_id = nh.guild_id
		WHERE nh.user_id = u.id 
		LIMIT 25
		), '[]'::json
	) as nickname_history,
	COALESCE(
		(SELECT json_agg(
			json_build_object(
				'guild_id', gm.guild_id,
				'guild_name', COALESCE(g.name, gm.guild_id),
				'guild_icon', g.icon,
				'joined_at', gm.joined_at,
				'last_seen_at', gm.last_seen_at
			) ORDER BY gm.last_seen_at DESC
		) FROM (
			SELECT DISTINCT ON (guild_id) guild_id, joined_at, last_seen_at 
			FROM guild_members 
			WHERE user_id = u.id
		) gm
		LEFT JOIN guilds g ON g.guild_id = gm.guild_id
		LIMIT 20
		), '[]'::json
	) as guilds,
	COALESCE(
		(SELECT json_agg(
			json_build_object(
				'guild_id', vs.guild_id,
				'guild_name', COALESCE(g.name, vs.guild_id),
				'guild_icon', g.icon,
				'channel_id', vs.channel_id,
				'channel_name', vs.channel_name,
				'joined_at', vs.joined_at,
				'left_at', vs.left_at,
				'duration_seconds', vs.duration_seconds,
				'was_video', vs.was_video,
				'was_streaming', vs.was_streaming,
				'was_muted', vs.was_muted,
				'was_deafened', vs.was_deafened
			) ORDER BY vs.joined_at DESC
		) FROM voice_sessions vs 
		LEFT JOIN guilds g ON g.guild_id = vs.guild_id
		WHERE vs.user_id = u.id 
		LIMIT 20
		), '[]'::json
	) as voice_history,
	COALESCE(
		(SELECT json_agg(
			json_build_object(
				'status', ph.status,
				'guild_id', ph.guild_id,
				'changed_at', ph.changed_at
			) ORDER BY ph.changed_at DESC
		) FROM presence_history ph 
		WHERE ph.user_id = u.id 
		LIMIT 25
		), '[]'::json
	) as presence_history,
	COALESCE(
		(SELECT json_agg(
			json_build_object(
				'name', ah.name,
				'details', ah.details,
				'state', ah.state,
				'type', ah.activity_type,
				'started_at', ah.started_at,
				'ended_at', ah.ended_at,
				'url', ah.url,
				'application_id', ah.application_id,
				'spotify_track_id', ah.spotify_track_id,
				'spotify_artist', ah.spotify_artist,
				'spotify_album', ah.spotify_album
			) ORDER BY ah.started_at DESC
		) FROM activity_history ah 
		WHERE ah.user_id = u.id 
		LIMIT 20
		), '[]'::json
	) as activity_history,
	COALESCE(
		(SELECT json_agg(
			json_build_object(
				'message_id', m.message_id,
				'guild_id', m.guild_id,
				'guild_name', COALESCE(g.name, m.guild_id),
				'guild_icon', g.icon,
				'channel_id', m.channel_id,
				'channel_name', COALESCE(ch.name, m.channel_name),
				'content', m.content,
				'created_at', m.created_at,
				'has_attachments', m.has_attachments,
				'has_embeds', m.has_embeds,
				'reply_to_user_id', m.reply_to_user_id
			) ORDER BY m.created_at DESC
		) FROM messages m
		LEFT JOIN guilds g ON g.guild_id = m.guild_id
		LEFT JOIN channels ch ON ch.channel_id = m.channel_id
		WHERE m.user_id = u.id 
		LIMIT 20
		), '[]'::json
	) as messages,
	COALESCE(
		(SELECT json_agg(
			json_build_object(
				'partner_id', vps.partner_id,
				'partner_name', COALESCE(
					(SELECT uh.global_name FROM username_history uh WHERE uh.user_id = vps.partner_id ORDER BY uh.changed_at DESC LIMIT 1),
					(SELECT uh.username FROM username_history uh WHERE uh.user_id = vps.partner_id ORDER BY uh.changed_at DESC LIMIT 1),
					vps.partner_id
				),
				'partner_avatar_hash', (SELECT ah.hash_avatar FROM avatar_history ah WHERE ah.user_id = vps.partner_id ORDER BY ah.changed_at DESC LIMIT 1),
				'guild_id', vps.guild_id,
				'guild_name', COALESCE(g.name, vps.guild_id),
				'guild_icon', g.icon,
				'session_count', vps.total_sessions,
				'total_duration_seconds', vps.total_duration_seconds,
				'last_session_at', vps.last_call_at
			) ORDER BY vps.total_sessions DESC
		) FROM voice_partner_stats vps
		LEFT JOIN guilds g ON g.guild_id = vps.guild_id
		WHERE vps.user_id = u.id 
		LIMIT 15
		), '[]'::json
	) as voice_partners,
	COALESCE(
		(SELECT json_agg(
			json_build_object(
				'banner_hash', bh.banner_hash,
				'banner_color', bh.banner_color,
				'url_cdn', bh.url_cdn,
				'changed_at', bh.changed_at
			) ORDER BY bh.changed_at DESC
		) FROM banner_history bh 
		WHERE bh.user_id = u.id 
		LIMIT 20
		), '[]'::json
	) as banner_history,
	COALESCE(
		(SELECT json_agg(
			json_build_object(
				'clan_tag', ch.clan_tag,
				'badge', ch.badge,
				'changed_at', ch.changed_at
			) ORDER BY ch.changed_at DESC
		) FROM clan_history ch 
		WHERE ch.user_id = u.id 
		LIMIT 20
		), '[]'::json
	) as clan_history,
	COALESCE(
		(SELECT json_agg(
			json_build_object(
				'decoration_asset', adh.decoration_asset,
				'decoration_sku_id', adh.decoration_sku_id,
				'changed_at', adh.changed_at
			) ORDER BY adh.changed_at DESC
		) FROM avatar_decoration_history adh 
		WHERE adh.user_id = u.id 
		LIMIT 20
		), '[]'::json
	) as avatar_decoration_history
FROM users u
WHERE u.id = $1`

// profileQueryBasic is a simplified query for newly fetched users (no guild/voice/activity data yet).
const profileQueryBasic = `SELECT 
	u.id,
	u.created_at::text as first_seen,
	COALESCE(u.last_updated_at, u.created_at)::text as last_updated,
	COALESCE(
		(SELECT json_agg(
			json_build_object(
				'username', uh.username,
				'discriminator', uh.discriminator,
				'global_name', uh.global_name,
				'changed_at', uh.changed_at
			) ORDER BY uh.changed_at DESC
		) FROM username_history uh 
		WHERE uh.user_id = u.id 
		AND (uh.username IS NOT NULL OR uh.global_name IS NOT NULL)
		LIMIT 500
		), '[]'::json
	) as username_history,
	COALESCE(
		(SELECT json_agg(
			json_build_object(
				'avatar_hash', ah.hash_avatar,
				'avatar_url', ah.url_cdn,
				'changed_at', ah.changed_at
			) ORDER BY ah.changed_at DESC
		) FROM avatar_history ah 
		WHERE ah.user_id = u.id 
		LIMIT 500
		), '[]'::json
	) as avatar_history,
	COALESCE(
		(SELECT json_agg(
			json_build_object(
				'bio_content', bh.bio_content,
				'changed_at', bh.changed_at
			) ORDER BY bh.changed_at DESC
		) FROM bio_history bh 
		WHERE bh.user_id = u.id 
		LIMIT 500
		), '[]'::json
	) as bio_history,
	COALESCE(
		(SELECT json_agg(
			json_build_object(
				'type', ca.type,
				'external_id', ca.external_id,
				'name', ca.name,
				'first_seen', ca.observed_at,
				'last_seen', ca.last_seen_at
			) ORDER BY ca.observed_at DESC
		) FROM connected_accounts ca 
		WHERE ca.user_id = u.id 
		LIMIT 500
		), '[]'::json
	) as connections
FROM users u
WHERE u.id = $1`
