package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"identity-archive/internal/security"
)

func (s *Server) getProfile(c *gin.Context) {
	discordID := c.Param("discord_id")
	if _, err := security.ParseSnowflake(discordID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_discord_id", "message": "discord_id invalido"}})
		return
	}

	ctx, cancel := s.ctx(c)
	defer cancel()

	// check cache
	cacheKey := fmt.Sprintf("profile:%s", discordID)
	if cached, err := s.redis.Get(ctx, cacheKey); err == nil && cached != "" {
		c.Data(http.StatusOK, "application/json", []byte(cached))
		c.Header("X-Cache", "HIT")
		return
	}

	// buscar perfil com agregação json
	var userID, firstSeen, lastUpdated string
	var usernameHistoryJSON, avatarHistoryJSON, bioHistoryJSON, connectionsJSON []byte
	var nicknameHistoryJSON, guildsJSON, voiceHistoryJSON, presenceHistoryJSON, activityHistoryJSON []byte

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
			COALESCE(
				(SELECT json_agg(
					json_build_object(
						'guild_id', nh.guild_id,
						'guild_name', COALESCE(g.name, nh.guild_id),
						'nickname', nh.nickname,
						'changed_at', nh.changed_at
					) ORDER BY nh.changed_at DESC
				) FROM nickname_history nh 
				LEFT JOIN guilds g ON g.guild_id = nh.guild_id
				WHERE nh.user_id = u.id 
				LIMIT 500
				), '[]'::json
			) as nickname_history,
			COALESCE(
				(SELECT json_agg(
					json_build_object(
						'guild_id', gm.guild_id,
						'guild_name', COALESCE(g.name, gm.guild_id),
						'joined_at', gm.joined_at,
						'last_seen_at', gm.last_seen_at
					) ORDER BY gm.last_seen_at DESC
				) FROM (
					SELECT DISTINCT ON (guild_id) guild_id, joined_at, last_seen_at 
					FROM guild_members 
					WHERE user_id = u.id
				) gm
				LEFT JOIN guilds g ON g.guild_id = gm.guild_id
				LIMIT 100
				), '[]'::json
			) as guilds,
			COALESCE(
				(SELECT json_agg(
					json_build_object(
						'guild_id', vs.guild_id,
						'guild_name', COALESCE(g.name, vs.guild_id),
						'channel_id', vs.channel_id,
						'channel_name', vs.channel_name,
						'joined_at', vs.joined_at,
						'left_at', vs.left_at,
						'duration_seconds', vs.duration_seconds,
						'was_video', vs.was_video,
						'was_streaming', vs.was_streaming
					) ORDER BY vs.joined_at DESC
				) FROM voice_sessions vs 
				LEFT JOIN guilds g ON g.guild_id = vs.guild_id
				WHERE vs.user_id = u.id 
				LIMIT 100
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
				LIMIT 500
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
						'ended_at', ah.ended_at
					) ORDER BY ah.started_at DESC
				) FROM activity_history ah 
				WHERE ah.user_id = u.id 
				LIMIT 100
				), '[]'::json
			) as activity_history
		FROM users u
		WHERE u.id = $1`,
		discordID,
	).Scan(&userID, &firstSeen, &lastUpdated, &usernameHistoryJSON, &avatarHistoryJSON, &bioHistoryJSON, &connectionsJSON, &nicknameHistoryJSON, &guildsJSON, &voiceHistoryJSON, &presenceHistoryJSON, &activityHistoryJSON)

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
						'[]'::json as activity_history
					FROM users u
					WHERE u.id = $1`,
					discordID,
				).Scan(&userID, &firstSeen, &lastUpdated, &usernameHistoryJSON, &avatarHistoryJSON, &bioHistoryJSON, &connectionsJSON, &nicknameHistoryJSON, &guildsJSON, &voiceHistoryJSON, &presenceHistoryJSON, &activityHistoryJSON)

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

	json.Unmarshal(usernameHistoryJSON, &usernameHistory)
	json.Unmarshal(avatarHistoryJSON, &avatarHistory)
	json.Unmarshal(bioHistoryJSON, &bioHistory)
	json.Unmarshal(connectionsJSON, &connections)
	json.Unmarshal(nicknameHistoryJSON, &nicknameHistory)
	json.Unmarshal(guildsJSON, &guilds)
	json.Unmarshal(voiceHistoryJSON, &voiceHistory)
	json.Unmarshal(presenceHistoryJSON, &presenceHistory)
	json.Unmarshal(activityHistoryJSON, &activityHistory)

	response := gin.H{
		"discord_id":       userID,
		"first_seen":       firstSeen,
		"last_updated":     lastUpdated,
		"username_history": usernameHistory,
		"avatar_history":   avatarHistory,
		"bio_history":      bioHistory,
		"connections":      connections,
		"nickname_history": nicknameHistory,
		"guilds":           guilds,
		"voice_history":    voiceHistory,
		"presence_history": presenceHistory,
		"activity_history": activityHistory,
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
		 LIMIT 50`,
		q,
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

	out := make([]result, 0, 50)
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

	c.JSON(http.StatusOK, gin.H{
		"query":   q,
		"total":   totalCount,
		"results": out,
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

	// check database
	var dbStatus string = "connected"
	if err := s.db.Pool.Ping(ctx); err != nil {
		dbStatus = "disconnected"
	}

	// check redis
	var redisStatus string = "connected"
	if err := s.redis.RDB().Ping(ctx).Err(); err != nil {
		redisStatus = "disconnected"
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

	status := "healthy"
	if dbStatus != "connected" || redisStatus != "disconnected" {
		status = "unhealthy"
	}

	response := gin.H{
		"status":                 status,
		"database":               dbStatus,
		"redis":                  redisStatus,
		"active_tokens":          activeTokens,
		"active_connections":     activeConnections,
		"events_processed_today": eventsProcessedToday,
	}

	if status == "unhealthy" {
		c.JSON(http.StatusServiceUnavailable, response)
		return
	}

	c.JSON(http.StatusOK, response)
}

func (s *Server) addToken(c *gin.Context) {
	var req struct {
		Token       string `json:"token" binding:"required"`
		OwnerUserID string `json:"owner_user_id" binding:"required"`
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

	// validar user_id
	if _, err := security.ParseSnowflake(req.OwnerUserID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_user_id", "message": "owner_user_id invalido"}})
		return
	}

	// se o tokenManager estiver disponivel, usa ele
	if s.tokenManager != nil {
		if err := s.tokenManager.AddToken(req.Token, req.OwnerUserID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "token_add_failed", "message": err.Error()}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "token adicionado com sucesso"})
		return
	}

	// fallback: adicionar com validacao de token
	ctx, cancel := s.ctx(c)
	defer cancel()

	// verificar se o token funciona
	if !validateTokenHealth(ctx, req.Token) {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "token_invalid", "message": "token invalido ou nao funciona"}})
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

	_, err = s.db.Pool.Exec(ctx,
		`INSERT INTO tokens (token, token_encrypted, user_id, status, created_at) VALUES ($1, $2, $3, 'ativo', NOW())`,
		encrypted, encrypted, req.OwnerUserID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "db_error", "message": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "token adicionado com sucesso"})
}

// validateTokenHealth verifica se o token funciona fazendo uma requisicao para a api do discord
func validateTokenHealth(ctx context.Context, token string) bool {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://discord.com/api/v10/users/@me", nil)
	if err != nil {
		return false
	}

	req.Header.Set("Authorization", token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

func (s *Server) listTokens(c *gin.Context) {
	ctx, cancel := s.ctx(c)
	defer cancel()

	// listar tokens diretamente do banco (funciona mesmo sem token manager)
	rows, err := s.db.Pool.Query(ctx,
		`SELECT id, user_id, status, failure_count, COALESCE(last_used, created_at) as last_used, suspended_until
		 FROM tokens
		 ORDER BY id DESC`,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "db_error", "message": err.Error()}})
		return
	}
	defer rows.Close()

	type tokenResp struct {
		ID             int64      `json:"id"`
		Token          string     `json:"token_masked"`
		UserID         string     `json:"user_id"`
		Status         string     `json:"status"`
		FailureCount   int        `json:"failure_count"`
		LastUsed       time.Time  `json:"last_used"`
		SuspendedUntil *time.Time `json:"suspended_until,omitempty"`
	}

	resp := make([]tokenResp, 0)
	for rows.Next() {
		var t tokenResp
		if err := rows.Scan(&t.ID, &t.UserID, &t.Status, &t.FailureCount, &t.LastUsed, &t.SuspendedUntil); err != nil {
			continue
		}
		t.Token = fmt.Sprintf("token...ID%d", t.ID)
		resp = append(resp, t)
	}

	c.JSON(http.StatusOK, gin.H{"tokens": resp})
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
