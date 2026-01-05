package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"identity-archive/internal/discord"
	"identity-archive/internal/security"

	"github.com/gin-gonic/gin"
)

func (s *Server) getProfile(c *gin.Context) {
	discordID := c.Param("discord_id")
	if _, err := security.ParseSnowflake(discordID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_discord_id", "message": "discord_id invalido"}})
		return
	}

	ctx, cancel := s.ctx(c)
	defer cancel()

	refreshParam := strings.TrimSpace(c.Query("refresh"))
	refresh := refreshParam == "1" || strings.EqualFold(refreshParam, "true") || strings.EqualFold(refreshParam, "yes")

	// check cache
	cacheKey := fmt.Sprintf("profile:%s", discordID)
	if !refresh {
		if cached, err := s.redis.Get(ctx, cacheKey); err == nil && cached != "" {
			c.Data(http.StatusOK, "application/json", []byte(cached))
			c.Header("X-Cache", "HIT")
			return
		}
	} else {
		// ensure we don't serve stale cached payload
		_ = s.redis.Del(ctx, cacheKey)
		c.Header("X-Cache", "BYPASS")
		// best-effort refresh of user snapshot (username/avatar/bio/connections)
		if s.userFetcher != nil {
			// Always try to fetch fresh data from Discord API first
			if discordUser, fetchErr := s.userFetcher.FetchUserByID(ctx, discordID); fetchErr == nil && discordUser != nil {
				if saveErr := s.userFetcher.SaveUserToDatabase(ctx, discordUser, "refresh_discord_api"); saveErr != nil {
					s.log.Warn("refresh_api_save_failed", "user_id", discordID, "error", saveErr)
				}
			} else {
				// If API fetch fails (e.g. 404 or no access), try to use existing gateway data
				s.log.Debug("refresh_api_fetch_failed", "user_id", discordID, "error", fetchErr)
				if gatewayUser, gatewayErr := s.userFetcher.TryFetchFromGatewayData(ctx, discordID); gatewayErr == nil && gatewayUser != nil {
					if saveErr := s.userFetcher.SaveUserToDatabase(ctx, gatewayUser, "refresh_gateway"); saveErr != nil {
						s.log.Warn("refresh_gateway_save_failed", "user_id", discordID, "error", saveErr)
					}
				}
			}
		}
	}

	// buscar perfil com agregação json
	var userID, firstSeen, lastUpdated string
	var usernameHistoryJSON, avatarHistoryJSON, bioHistoryJSON, connectionsJSON []byte
	var nicknameHistoryJSON, guildsJSON, voiceHistoryJSON, presenceHistoryJSON, activityHistoryJSON []byte
	var messagesJSON, voicePartnersJSON []byte
	var bannerHistoryJSON, clanHistoryJSON, avatarDecorationHistoryJSON []byte

	err := s.db.Pool.QueryRow(ctx,
		`SELECT 
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
						'guild_name', COALESCE(vs.guild_name, g.name, vs.guild_id),
						'guild_icon', g.icon,
						'channel_id', vs.channel_id,
						'channel_name', COALESCE(vs.channel_name, ch.name, vs.channel_id),
						'joined_at', vs.joined_at,
						'left_at', vs.left_at,
						'duration_seconds', vs.duration_seconds,
						'was_video', vs.was_video,
						'was_streaming', vs.was_streaming,
						'was_muted', vs.was_muted,
						'was_deafened', vs.was_deafened,
						'participants', COALESCE(
							(SELECT json_agg(json_build_object(
								'user_id', vp.user_id,
								'username', COALESCE(
									(SELECT uh.global_name FROM username_history uh WHERE uh.user_id = vp.user_id ORDER BY uh.changed_at DESC LIMIT 1),
									(SELECT uh.username FROM username_history uh WHERE uh.user_id = vp.user_id ORDER BY uh.changed_at DESC LIMIT 1),
									vp.user_id
								),
								'avatar_hash', (SELECT ah.hash_avatar FROM avatar_history ah WHERE ah.user_id = vp.user_id ORDER BY ah.changed_at DESC LIMIT 1)
							))
							FROM voice_participants vp
							WHERE vp.session_id = vs.id
							), '[]'::json
						)
					) ORDER BY vs.joined_at DESC
				) FROM (
					SELECT DISTINCT ON (guild_id, channel_id, floor(extract(epoch from joined_at) / 2)) *
					FROM voice_sessions 
					WHERE user_id = u.id
					ORDER BY guild_id, channel_id, floor(extract(epoch from joined_at) / 2), joined_at DESC
				) vs 
				LEFT JOIN guilds g ON g.guild_id = vs.guild_id
				LEFT JOIN channels ch ON ch.channel_id = vs.channel_id
				LIMIT 25
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
		WHERE u.id = $1`,
		discordID,
	).Scan(&userID, &firstSeen, &lastUpdated, &usernameHistoryJSON, &avatarHistoryJSON, &bioHistoryJSON, &connectionsJSON, &nicknameHistoryJSON, &guildsJSON, &voiceHistoryJSON, &presenceHistoryJSON, &activityHistoryJSON, &messagesJSON, &voicePartnersJSON, &bannerHistoryJSON, &clanHistoryJSON, &avatarDecorationHistoryJSON)

	if err != nil {
		// usuario nao encontrado no banco
		s.log.Info("user_not_in_database", "user_id", discordID)

		if s.userFetcher != nil {
			// PRIORIDADE 1: tentar buscar nos dados ja coletados via gateway (servidores compartilhados)
			s.log.Info("checking_gateway_data_first", "user_id", discordID)
			gatewayUser, gatewayErr := s.userFetcher.TryFetchFromGatewayData(ctx, discordID)

			if gatewayErr == nil && gatewayUser != nil {
				s.log.Info("user_found_in_gateway_data", "user_id", discordID, "username", gatewayUser.Username)

				// salvar como "ja coletado via gateway"
				if saveErr := s.userFetcher.SaveUserToDatabase(ctx, gatewayUser, "gateway_data"); saveErr != nil {
					s.log.Warn("failed_to_save_gateway_user", "user_id", discordID, "error", saveErr)
				}

				// buscar novamente do banco agora que foi salvo
				err = s.db.Pool.QueryRow(ctx,
					`SELECT 
						u.id,
						u.created_at::text as first_seen,
						COALESCE(u.last_updated_at, u.created_at)::text as last_updated,
						COALESCE(
							(SELECT json_agg(
								json_build_object(
									'username', uh.username,
									'discriminator', uh.discriminator,
									'global_name', uh.global_name,
									'changed_at', uh.observed_at
								) ORDER BY uh.observed_at DESC
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
						) as connections,
						'[]'::json as nickname_history,
						'[]'::json as guilds,
						'[]'::json as voice_history,
						'[]'::json as presence_history,
						'[]'::json as activity_history,
						'[]'::json as messages,
						'[]'::json as voice_partners
					FROM users u
					WHERE u.id = $1`,
					discordID,
				).Scan(&userID, &firstSeen, &lastUpdated, &usernameHistoryJSON, &avatarHistoryJSON, &bioHistoryJSON, &connectionsJSON, &nicknameHistoryJSON, &guildsJSON, &voiceHistoryJSON, &presenceHistoryJSON, &activityHistoryJSON, &messagesJSON, &voicePartnersJSON)

				if err == nil {
					s.log.Info("gateway_user_loaded_from_db", "user_id", discordID)
					// continua para retornar os dados
				} else {
					s.log.Error("failed_to_load_gateway_user", "user_id", discordID, "error", err)
					c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "internal_error", "message": "erro ao carregar usuario"}})
					return
				}
			} else {
				// PRIORIDADE 2: usuario nao esta em servidores compartilhados - tentar bot token
				s.log.Info("user_not_in_shared_guilds", "user_id", discordID, "trying_bot_token", true)
				discordUser, fetchErr := s.userFetcher.FetchUserByID(ctx, discordID)

				if fetchErr != nil {
					s.log.Warn("bot_fetch_failed", "user_id", discordID, "error", fetchErr)

					// mensagem mais clara para o usuario
					errorMsg := "usuario nao encontrado"
					if strings.Contains(fetchErr.Error(), "user_not_found_in_gateway_data") {
						errorMsg = "usuario nao encontrado - nao esta em servidores compartilhados e bot token nao configurado"
					}

					c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "not_found", "message": errorMsg}})
					return
				}

				if discordUser != nil {
					s.log.Info("user_fetched_from_api", "user_id", discordID, "username", discordUser.Username)

					// salvar no banco
					if saveErr := s.userFetcher.SaveUserToDatabase(ctx, discordUser, "discord_api"); saveErr != nil {
						s.log.Error("failed_to_save_fetched_user", "user_id", discordID, "error", saveErr)
						c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "save_failed", "message": fmt.Sprintf("usuario encontrado mas falha ao salvar: %v", saveErr)}})
						return
					}

					s.log.Info("user_saved_to_database", "user_id", discordID)

					// buscar novamente do banco agora que foi salvo
					err = s.db.Pool.QueryRow(ctx,
						`SELECT 
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
					WHERE u.id = $1`,
						discordID,
					).Scan(&userID, &firstSeen, &lastUpdated, &usernameHistoryJSON, &avatarHistoryJSON, &bioHistoryJSON, &connectionsJSON)

					// set empty arrays for other fields
					nicknameHistoryJSON = []byte("[]")
					guildsJSON = []byte("[]")
					voiceHistoryJSON = []byte("[]")
					presenceHistoryJSON = []byte("[]")
					activityHistoryJSON = []byte("[]")
					messagesJSON = []byte("[]")
					voicePartnersJSON = []byte("[]")
					bannerHistoryJSON = []byte("[]")
					clanHistoryJSON = []byte("[]")
					avatarDecorationHistoryJSON = []byte("[]")

					if err != nil {
						// mesmo apos buscar e salvar, ainda nao conseguiu ler - retornar erro
						s.log.Error("failed_to_read_after_save", "user_id", discordID, "error", err)
						c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "not_found", "message": "usuario nao encontrado"}})
						return
					}

					s.log.Info("user_loaded_after_save", "user_id", discordID)
				} else {
					// nao conseguiu buscar via api - retornar 404
					s.log.Warn("discord_user_is_nil", "user_id", discordID)
					c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "not_found", "message": "usuario nao encontrado"}})
					return
				}
			}
		} else {
			// sem userFetcher disponivel - retornar 404
			s.log.Warn("user_fetcher_not_available", "user_id", discordID, "msg", "token manager nao inicializado ou sem tokens")
			c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "not_found", "message": "usuario nao encontrado - sistema de busca nao disponivel"}})
			return
		}
	}

	var usernameHistory, avatarHistory, bioHistory, connections []interface{}
	var nicknameHistory, guilds, voiceHistory, presenceHistory, activityHistory []interface{}
	var messages, voicePartners []interface{}
	var bannerHistory, clanHistory, avatarDecorationHistory []interface{}

	if err := json.Unmarshal(usernameHistoryJSON, &usernameHistory); err != nil {
		s.log.Warn("failed_to_unmarshal_username_history", "error", err)
		usernameHistory = []interface{}{}
	}
	if err := json.Unmarshal(avatarHistoryJSON, &avatarHistory); err != nil {
		s.log.Warn("failed_to_unmarshal_avatar_history", "error", err)
		avatarHistory = []interface{}{}
	}
	if err := json.Unmarshal(bioHistoryJSON, &bioHistory); err != nil {
		s.log.Warn("failed_to_unmarshal_bio_history", "error", err)
		bioHistory = []interface{}{}
	}
	if err := json.Unmarshal(connectionsJSON, &connections); err != nil {
		s.log.Warn("failed_to_unmarshal_connections", "error", err)
		connections = []interface{}{}
	}
	if err := json.Unmarshal(nicknameHistoryJSON, &nicknameHistory); err != nil {
		s.log.Warn("failed_to_unmarshal_nickname_history", "error", err)
		nicknameHistory = []interface{}{}
	}
	if err := json.Unmarshal(guildsJSON, &guilds); err != nil {
		s.log.Warn("failed_to_unmarshal_guilds", "error", err)
		guilds = []interface{}{}
	}
	if err := json.Unmarshal(voiceHistoryJSON, &voiceHistory); err != nil {
		s.log.Warn("failed_to_unmarshal_voice_history", "error", err)
		voiceHistory = []interface{}{}
	}
	if err := json.Unmarshal(presenceHistoryJSON, &presenceHistory); err != nil {
		s.log.Warn("failed_to_unmarshal_presence_history", "error", err)
		presenceHistory = []interface{}{}
	}
	if err := json.Unmarshal(activityHistoryJSON, &activityHistory); err != nil {
		s.log.Warn("failed_to_unmarshal_activity_history", "error", err)
		activityHistory = []interface{}{}
	}
	if err := json.Unmarshal(messagesJSON, &messages); err != nil {
		s.log.Warn("failed_to_unmarshal_messages", "error", err)
		messages = []interface{}{}
	}
	if err := json.Unmarshal(voicePartnersJSON, &voicePartners); err != nil {
		s.log.Warn("failed_to_unmarshal_voice_partners", "error", err)
		voicePartners = []interface{}{}
	}
	if err := json.Unmarshal(bannerHistoryJSON, &bannerHistory); err != nil {
		s.log.Warn("failed_to_unmarshal_banner_history", "error", err)
		bannerHistory = []interface{}{}
	}
	if err := json.Unmarshal(clanHistoryJSON, &clanHistory); err != nil {
		s.log.Warn("failed_to_unmarshal_clan_history", "error", err)
		clanHistory = []interface{}{}
	}
	if err := json.Unmarshal(avatarDecorationHistoryJSON, &avatarDecorationHistory); err != nil {
		s.log.Warn("failed_to_unmarshal_avatar_decoration_history", "error", err)
		avatarDecorationHistory = []interface{}{}
	}

	response := gin.H{
		"discord_id":                userID,
		"first_seen":                firstSeen,
		"last_updated":              lastUpdated,
		"username_history":          usernameHistory,
		"avatar_history":            avatarHistory,
		"bio_history":               bioHistory,
		"connections":               connections,
		"nickname_history":          nicknameHistory,
		"guilds":                    guilds,
		"voice_history":             voiceHistory,
		"presence_history":          presenceHistory,
		"activity_history":          activityHistory,
		"messages":                  messages,
		"voice_partners":            voicePartners,
		"banner_history":            bannerHistory,
		"clan_history":              clanHistory,
		"avatar_decoration_history": avatarDecorationHistory,
	}

	// cache response
	if jsonData, err := json.Marshal(response); err == nil {
		s.redis.Set(ctx, cacheKey, string(jsonData), 5*time.Minute)
	}

	c.JSON(http.StatusOK, response)
}

func (s *Server) search(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" || len(q) < 2 {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_query", "message": "q deve ter pelo menos 2 caracteres"}})
		return
	}

	// Pagination parameters
	page := 1
	limit := 50
	if p := c.Query("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	offset := (page - 1) * limit

	ctx, cancel := s.ctx(c)
	defer cancel()

	// busca com pg_trgm usando similaridade
	rows, err := s.db.Pool.Query(ctx,
		`SELECT 
			user_id, 
			username, 
			global_name, 
			changed_at,
			GREATEST(
				COALESCE(similarity(username, $1), 0),
				COALESCE(similarity(global_name, $1), 0)
			) as similarity_score
		 FROM username_history
		 WHERE username % $1
		    OR global_name % $1
		 ORDER BY similarity_score DESC, changed_at DESC
		 LIMIT $2 OFFSET $3`,
		q, limit, offset,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "db_error", "message": "falha na busca"}})
		return
	}
	defer rows.Close()

	// contar total de resultados
	var totalCount int64
	err = s.db.Pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT user_id)
		 FROM username_history
		 WHERE username % $1
		    OR global_name % $1`,
		q,
	).Scan(&totalCount)
	if err != nil {
		totalCount = 0
	}

	type result struct {
		UserID     string  `json:"user_id"`
		Username   *string `json:"username,omitempty"`
		GlobalName *string `json:"global_name,omitempty"`
		ChangedAt  string  `json:"changed_at"`
	}

	out := make([]result, 0, limit)
	for rows.Next() {
		var r result
		var changedAt time.Time
		var similarityScore float64
		if err := rows.Scan(&r.UserID, &r.Username, &r.GlobalName, &changedAt, &similarityScore); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "db_error", "message": "falha ao ler resultados"}})
			return
		}
		r.ChangedAt = changedAt.UTC().Format("2006-01-02T15:04:05Z")
		out = append(out, r)
	}

	// Calculate pagination metadata
	totalPages := int64(1)
	if totalCount > 0 {
		totalPages = (totalCount + int64(limit) - 1) / int64(limit)
	}
	hasMore := int64(page) < totalPages

	c.JSON(http.StatusOK, gin.H{
		"query":    q,
		"total":    totalCount,
		"page":     page,
		"limit":    limit,
		"pages":    totalPages,
		"has_more": hasMore,
		"results":  out,
	})
}

func (s *Server) altCheck(c *gin.Context) {
	discordID := c.Param("discord_id")
	if _, err := security.ParseSnowflake(discordID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_discord_id", "message": "discord_id invalido"}})
		return
	}

	ctx, cancel := s.ctx(c)
	defer cancel()

	extRows, err := s.db.Pool.Query(ctx,
		`SELECT DISTINCT external_id
		 FROM connected_accounts
		 WHERE user_id = $1 AND external_id IS NOT NULL`,
		discordID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "db_error", "message": "falha ao buscar external ids"}})
		return
	}
	defer extRows.Close()

	externalIDs := make([]string, 0, 16)
	for extRows.Next() {
		var id string
		if err := extRows.Scan(&id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "db_error", "message": "falha ao ler external ids"}})
			return
		}
		if id != "" {
			externalIDs = append(externalIDs, id)
		}
	}

	if len(externalIDs) == 0 {
		c.JSON(http.StatusOK, gin.H{"discord_id": discordID, "related": []string{}})
		return
	}

	// buscar outros users que compartilham algum external_id
	rows, err := s.db.Pool.Query(ctx,
		`SELECT DISTINCT user_id
		 FROM connected_accounts
		 WHERE external_id = ANY($1) AND user_id <> $2
		 LIMIT 200`,
		externalIDs,
		discordID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "db_error", "message": "falha no alt-check"}})
		return
	}
	defer rows.Close()

	related := make([]string, 0, 32)
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "db_error", "message": "falha ao ler alt-check"}})
			return
		}
		related = append(related, uid)
	}

	// query alt_relationships table com join para username
	altRows, err := s.db.Pool.Query(ctx,
		`SELECT 
			CASE WHEN ar.user_a = $1 THEN ar.user_b ELSE ar.user_a END AS alt_user_id,
			COALESCE(uh.username, '') AS alt_username,
			ar.confidence_score,
			ar.detection_method,
			ar.detected_at
		FROM alt_relationships ar
		LEFT JOIN LATERAL (
			SELECT username 
			FROM username_history 
			WHERE user_id = CASE WHEN ar.user_a = $1 THEN ar.user_b ELSE ar.user_a END
			ORDER BY changed_at DESC 
			LIMIT 1
		) uh ON true
		WHERE ar.user_a = $1 OR ar.user_b = $1
		ORDER BY ar.confidence_score DESC
		LIMIT 10`,
		discordID,
	)
	if err == nil {
		defer altRows.Close()

		alts := make([]gin.H, 0)
		for altRows.Next() {
			var altUserID, altUsername, detectionMethod string
			var confidenceScore float64
			var detectedAt time.Time

			if err := altRows.Scan(&altUserID, &altUsername, &confidenceScore, &detectionMethod, &detectedAt); err != nil {
				continue
			}

			alts = append(alts, gin.H{
				"alt_discord_id": altUserID,
				"alt_username":   altUsername,
				"confidence":     confidenceScore,
				"reason":         detectionMethod,
				"detected_at":    detectedAt.UTC().Format("2006-01-02T15:04:05Z"),
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"discord_id":    discordID,
			"possible_alts": alts,
		})
		return
	}

	// Fallback to old method
	c.JSON(http.StatusOK, gin.H{
		"discord_id":   discordID,
		"external_ids": externalIDs,
		"related":      related,
	})
}

func (s *Server) health(c *gin.Context) {
	ctx, cancel := s.ctx(c)
	defer cancel()

	// Verbose mode for detailed health info
	verbose := strings.EqualFold(c.Query("verbose"), "true") || c.Query("verbose") == "1"

	// check database
	dbStatus := "connected"
	var dbLatencyMs int64
	dbStart := time.Now()
	if err := s.db.Pool.Ping(ctx); err != nil {
		dbStatus = "disconnected"
	} else {
		dbLatencyMs = time.Since(dbStart).Milliseconds()
	}

	// check redis
	redisStatus := "connected"
	var redisLatencyMs int64
	redisStart := time.Now()
	if err := s.redis.RDB().Ping(ctx).Err(); err != nil {
		redisStatus = "disconnected"
	} else {
		redisLatencyMs = time.Since(redisStart).Milliseconds()
	}

	// buscar valores reais
	var activeTokens int64
	if s.tokenManager != nil {
		activeTokens = int64(s.tokenManager.GetActiveTokenCount())
	}

	var activeConnections int64
	if s.gatewayManager != nil {
		activeConnections = int64(s.gatewayManager.GetActiveConnectionsCount())
	}

	// eventos processados hoje (do redis)
	var eventsProcessedToday int64
	eventsKey := fmt.Sprintf("events:processed:%s", time.Now().Format("2006-01-02"))
	if count, err := s.redis.GetInt(ctx, eventsKey); err == nil {
		eventsProcessedToday = count
	}

	// Event queue size (if event processor available)
	var eventQueueSize int64
	if s.ep != nil {
		eventQueueSize = int64(len(s.ep.GetEventQueue()))
	}

	status := "healthy"
	if dbStatus != "connected" || redisStatus != "connected" {
		status = "unhealthy"
	}

	response := gin.H{
		"status":                 status,
		"database":               dbStatus,
		"redis":                  redisStatus,
		"active_tokens":          activeTokens,
		"active_connections":     activeConnections,
		"events_processed_today": eventsProcessedToday,
		"event_queue_size":       eventQueueSize,
		"timestamp":              time.Now().UTC().Format(time.RFC3339),
	}

	// Add verbose details if requested
	if verbose {
		// Database stats
		var totalUsers, totalEvents int64
		_ = s.db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&totalUsers)
		_ = s.db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM user_history`).Scan(&totalEvents)

		// Pool stats
		poolStats := s.db.Pool.Stat()

		response["details"] = gin.H{
			"latency": gin.H{
				"database_ms": dbLatencyMs,
				"redis_ms":    redisLatencyMs,
			},
			"database_stats": gin.H{
				"total_users":   totalUsers,
				"total_history": totalEvents,
				"pool_acquired": poolStats.AcquiredConns(),
				"pool_idle":     poolStats.IdleConns(),
				"pool_total":    poolStats.TotalConns(),
				"pool_max":      poolStats.MaxConns(),
			},
			"queue_capacity": 50000, // Matches event_processor.go
		}
	}

	if status == "unhealthy" {
		c.JSON(http.StatusServiceUnavailable, response)
		return
	}

	c.JSON(http.StatusOK, response)
}

func (s *Server) addToken(c *gin.Context) {
	var req struct {
		Token string `json:"token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request", "message": err.Error()}})
		return
	}

	// validar formato do token
	if len(req.Token) < 50 {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_token", "message": "token invalido"}})
		return
	}

	ctx, cancel := s.ctx(c)
	defer cancel()

	// buscar dados da conta do token automaticamente
	accountInfo, err := fetchTokenAccountInfo(ctx, req.Token)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "token_invalid", "message": "token invalido ou nao funciona: " + err.Error()}})
		return
	}

	// se o tokenManager estiver disponivel, usa ele
	if s.tokenManager != nil {
		if err := s.tokenManager.AddToken(req.Token, accountInfo.UserID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "token_add_failed", "message": err.Error()}})
			return
		}
		// atualizar account info no banco
		_, _ = s.db.Pool.Exec(ctx,
			`UPDATE tokens SET account_user_id = $1, account_username = $2, account_display_name = $3, account_avatar = $4 
			 WHERE user_id = $1 ORDER BY id DESC LIMIT 1`,
			accountInfo.UserID, accountInfo.Username, accountInfo.DisplayName, accountInfo.Avatar,
		)
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "token adicionado com sucesso", "account": accountInfo})
		return
	}

	// criptografar token
	if len(s.cfg.EncryptionKey) != 32 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "config_error", "message": "encryption key nao configurada"}})
		return
	}

	encrypted, err := security.EncryptToken(req.Token, s.cfg.EncryptionKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "encrypt_error", "message": err.Error()}})
		return
	}

	// token_fingerprint (se existir) evita tokens duplicados mesmo com cifragem aleatoria
	hasFingerprint := false
	_ = s.db.Pool.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'tokens' AND column_name = 'token_fingerprint'
		)`,
	).Scan(&hasFingerprint)

	if hasFingerprint {
		fingerprint := discord.TokenFingerprintForAPI(req.Token)
		_, err = s.db.Pool.Exec(ctx,
			`INSERT INTO tokens (token, token_encrypted, token_fingerprint, user_id, status, created_at,
			 account_user_id, account_username, account_display_name, account_avatar)
			 VALUES ($1, $2, $3, $4, 'ativo', NOW(), $5, $6, $7, $8)`,
			encrypted, encrypted, fingerprint, accountInfo.UserID,
			accountInfo.UserID, accountInfo.Username, accountInfo.DisplayName, accountInfo.Avatar,
		)
	} else {
		_, err = s.db.Pool.Exec(ctx,
			`INSERT INTO tokens (token, token_encrypted, user_id, status, created_at,
			 account_user_id, account_username, account_display_name, account_avatar)
			 VALUES ($1, $2, $3, 'ativo', NOW(), $4, $5, $6, $7)`,
			encrypted, encrypted, accountInfo.UserID,
			accountInfo.UserID, accountInfo.Username, accountInfo.DisplayName, accountInfo.Avatar,
		)
	}
	if err != nil {
		// avoid extra dependency: match by constraint/index name
		if strings.Contains(err.Error(), "ux_tokens_token_fingerprint") || strings.Contains(err.Error(), "duplicate key") {
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "token_already_exists", "message": "token ja existe"}})
			return
		}
		// Unique index will throw here if token already exists (fingerprint)
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "db_error", "message": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "token adicionado com sucesso", "account": accountInfo})
}

// TokenAccountInfo holds the account info fetched from Discord API
type TokenAccountInfo struct {
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name,omitempty"`
	Avatar      string `json:"avatar,omitempty"`
}

// fetchTokenAccountInfo fetches the account info from Discord using /users/@me
func fetchTokenAccountInfo(ctx context.Context, token string) (*TokenAccountInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://discord.com/api/v10/users/@me", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", token)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discord api returned %d", resp.StatusCode)
	}

	var data struct {
		ID            string `json:"id"`
		Username      string `json:"username"`
		GlobalName    string `json:"global_name"`
		Discriminator string `json:"discriminator"`
		Avatar        string `json:"avatar"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	return &TokenAccountInfo{
		UserID:      data.ID,
		Username:    data.Username,
		DisplayName: data.GlobalName,
		Avatar:      data.Avatar,
	}, nil
}

func (s *Server) listTokens(c *gin.Context) {
	ctx, cancel := s.ctx(c)
	defer cancel()

	// listar tokens com account info e guilds
	rows, err := s.db.Pool.Query(ctx,
		`SELECT 
			t.id, 
			t.user_id, 
			t.status, 
			t.failure_count, 
			COALESCE(t.last_used, t.created_at) as last_used, 
			t.suspended_until,
			COALESCE(t.account_username, '') as username,
			COALESCE(t.account_display_name, '') as display_name,
			COALESCE(t.account_avatar, '') as avatar,
			COALESCE(
				(SELECT json_agg(json_build_object(
					'guild_id', g.guild_id,
					'name', COALESCE(g.name, tg.guild_id),
					'icon', g.icon,
					'member_count', g.member_count
				))
				FROM token_guilds tg
				LEFT JOIN guilds g ON g.guild_id = tg.guild_id
				WHERE tg.token_id = t.id
				), '[]'::json
			) as guilds
		 FROM tokens t
		 ORDER BY t.id DESC`,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "db_error", "message": err.Error()}})
		return
	}
	defer rows.Close()

	type guildInfo struct {
		GuildID     string `json:"guild_id"`
		Name        string `json:"name"`
		Icon        string `json:"icon,omitempty"`
		MemberCount int    `json:"member_count,omitempty"`
	}

	type tokenResp struct {
		ID             int64       `json:"id"`
		Token          string      `json:"token_masked"`
		UserID         string      `json:"user_id"`
		Username       string      `json:"username,omitempty"`
		DisplayName    string      `json:"display_name,omitempty"`
		Avatar         string      `json:"avatar,omitempty"`
		Status         string      `json:"status"`
		FailureCount   int         `json:"failure_count"`
		LastUsed       time.Time   `json:"last_used"`
		SuspendedUntil *time.Time  `json:"suspended_until,omitempty"`
		Guilds         []guildInfo `json:"guilds"`
	}

	resp := make([]tokenResp, 0)
	for rows.Next() {
		var t tokenResp
		var guildsJSON []byte
		if err := rows.Scan(&t.ID, &t.UserID, &t.Status, &t.FailureCount, &t.LastUsed, &t.SuspendedUntil, &t.Username, &t.DisplayName, &t.Avatar, &guildsJSON); err != nil {
			s.log.Warn("token_scan_error", "error", err)
			continue
		}
		t.Token = fmt.Sprintf("token...ID%d", t.ID)

		// Parse guilds JSON
		t.Guilds = []guildInfo{}
		if len(guildsJSON) > 0 {
			json.Unmarshal(guildsJSON, &t.Guilds)
		}

		resp = append(resp, t)
	}

	c.JSON(http.StatusOK, gin.H{"tokens": resp})
}

func (s *Server) refreshTokens(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()

	// buscar todos os tokens do banco
	rows, err := s.db.Pool.Query(ctx,
		`SELECT id, token_encrypted FROM tokens WHERE status = 'ativo'`,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "db_error", "message": err.Error()}})
		return
	}
	defer rows.Close()

	type tokenToUpdate struct {
		ID             int64
		EncryptedToken string
	}

	var tokens []tokenToUpdate
	for rows.Next() {
		var t tokenToUpdate
		if err := rows.Scan(&t.ID, &t.EncryptedToken); err != nil {
			continue
		}
		tokens = append(tokens, t)
	}

	updated := 0
	failed := 0

	for _, t := range tokens {
		// decrypt token
		decrypted, err := security.DecryptToken(t.EncryptedToken, s.cfg.EncryptionKey)
		if err != nil {
			s.log.Warn("token_decrypt_error", "token_id", t.ID, "error", err)
			failed++
			continue
		}

		// fetch account info
		accountInfo, err := fetchTokenAccountInfo(ctx, decrypted)
		if err != nil {
			s.log.Warn("token_fetch_error", "token_id", t.ID, "error", err)
			failed++
			continue
		}

		// update database
		_, err = s.db.Pool.Exec(ctx,
			`UPDATE tokens SET 
				account_user_id = $1, 
				account_username = $2, 
				account_display_name = $3, 
				account_avatar = $4,
				user_id = $1
			 WHERE id = $5`,
			accountInfo.UserID, accountInfo.Username, accountInfo.DisplayName, accountInfo.Avatar, t.ID,
		)
		if err != nil {
			s.log.Warn("token_update_error", "token_id", t.ID, "error", err)
			failed++
			continue
		}

		updated++
		s.log.Info("token_refreshed", "token_id", t.ID, "username", accountInfo.Username)

		// rate limit: wait 500ms between requests
		time.Sleep(500 * time.Millisecond)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"updated": updated,
		"failed":  failed,
		"total":   len(tokens),
	})
}

func (s *Server) removeToken(c *gin.Context) {
	var req struct {
		ID int64 `uri:"id" binding:"required"`
	}

	if err := c.ShouldBindUri(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_request", "message": "id invalido"}})
		return
	}

	ctx, cancel := s.ctx(c)
	defer cancel()

	// remover diretamente do banco
	_, err := s.db.Pool.Exec(ctx, "DELETE FROM tokens WHERE id = $1", req.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "db_error", "message": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) fetchUser(c *gin.Context) {
	discordID := c.Param("discord_id")
	if _, err := security.ParseSnowflake(discordID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_discord_id", "message": "discord_id invalido"}})
		return
	}

	ctx, cancel := s.ctx(c)
	defer cancel()

	if s.userFetcher == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "service_unavailable", "message": "user fetcher nao disponivel"}})
		return
	}

	// buscar usuario via api
	discordUser, err := s.userFetcher.FetchUserByID(ctx, discordID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "not_found", "message": fmt.Sprintf("usuario nao encontrado: %v", err)}})
		return
	}

	// salvar no banco
	if err := s.userFetcher.SaveUserToDatabase(ctx, discordUser, "discord_api"); err != nil {
		s.log.Warn("failed_to_save_fetched_user", "user_id", discordID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "save_failed", "message": fmt.Sprintf("usuario encontrado mas falha ao salvar: %v", err)}})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "usuario encontrado e salvo",
		"user": gin.H{
			"id":            discordUser.ID,
			"username":      discordUser.Username,
			"discriminator": discordUser.Discriminator,
			"global_name":   discordUser.GlobalName,
			"avatar":        discordUser.Avatar,
			"banner":        discordUser.Banner,
			"bio":           discordUser.Bio,
		},
	})
}

// publicLookup busca dados públicos do Discord de fontes externas como discord.id
func (s *Server) publicLookup(c *gin.Context) {
	discordID := c.Param("discord_id")
	if _, err := security.ParseSnowflake(discordID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_discord_id", "message": "discord_id invalido"}})
		return
	}

	ctx, cancel := s.ctx(c)
	defer cancel()

	if s.publicScraper == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "service_unavailable", "message": "public scraper nao disponivel"}})
		return
	}

	// buscar dados públicos de fontes externas
	data, err := s.publicScraper.FetchPublicData(ctx, discordID)
	if err != nil {
		s.log.Debug("public_lookup_failed", "user_id", discordID, "error", err)
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "not_found", "message": "dados publicos nao encontrados"}})
		return
	}

	// salvar dados no banco
	if saveErr := s.publicScraper.SavePublicData(ctx, data); saveErr != nil {
		s.log.Warn("public_data_save_failed", "user_id", discordID, "error", saveErr)
	}

	// construir URLs de avatar e banner
	var avatarURL, bannerURL string
	if data.Avatar != "" {
		ext := "png"
		if len(data.Avatar) > 2 && data.Avatar[:2] == "a_" {
			ext = "gif"
		}
		avatarURL = fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.%s", discordID, data.Avatar, ext)
	}
	if data.Banner != "" {
		ext := "png"
		if len(data.Banner) > 2 && data.Banner[:2] == "a_" {
			ext = "gif"
		}
		bannerURL = fmt.Sprintf("https://cdn.discordapp.com/banners/%s/%s.%s", discordID, data.Banner, ext)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"source":  data.Source,
		"user": gin.H{
			"id":           data.ID,
			"username":     data.Username,
			"global_name":  data.GlobalName,
			"avatar":       data.Avatar,
			"avatar_url":   avatarURL,
			"banner":       data.Banner,
			"banner_url":   bannerURL,
			"accent_color": data.AccentColor,
			"public_flags": data.Flags,
		},
	})
}
