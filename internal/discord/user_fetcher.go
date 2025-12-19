package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"identity-archive/internal/db"
	"identity-archive/internal/redis"
)

type UserFetcher struct {
	tokenManager *TokenManager
	db           *db.DB
	redis        *redis.Client
	logger       *slog.Logger
	httpClient   *http.Client
	botToken     string // bot token do .env para fallback
}

type DiscordUser struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	Discriminator string `json:"discriminator"`
	GlobalName    string `json:"global_name"`
	Avatar        string `json:"avatar"`
	Banner        string `json:"banner"`
	Bio           string `json:"bio"`
	Bot           bool   `json:"bot"`
}

func NewUserFetcher(logger *slog.Logger, dbConn *db.DB, redisClient *redis.Client, tokenManager *TokenManager, botToken string) *UserFetcher {
	return &UserFetcher{
		tokenManager: tokenManager,
		db:           dbConn,
		redis:        redisClient,
		logger:       logger,
		botToken:     botToken,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// FetchUserByID busca um usuário via Discord API usando token que tem acesso
func (uf *UserFetcher) FetchUserByID(ctx context.Context, userID string) (*DiscordUser, error) {
	// verificar cache primeiro
	cacheKey := fmt.Sprintf("discord_user:%s", userID)
	if cached, err := uf.redis.Get(ctx, cacheKey); err == nil && cached != "" {
		var user DiscordUser
		if err := json.Unmarshal([]byte(cached), &user); err == nil {
			uf.logger.Debug("user_fetched_from_cache", "user_id", userID)
			return &user, nil
		}
	}

	// primeiro tentar encontrar um token que tem acesso a este usuario
	token, err := uf.findTokenWithAccess(ctx, userID)
	if err != nil {
		// se nao encontrou token com acesso, tenta qualquer token disponivel
		uf.logger.Debug("no_token_with_access_found", "user_id", userID, "error", err)
		token, err = uf.tokenManager.GetNextAvailableToken()
		if err != nil {
			uf.logger.Warn("no_token_available_for_fetch", "user_id", userID, "error", err)
			return nil, fmt.Errorf("no_token_available: %w", err)
		}
	}
	
	uf.logger.Info("fetching_user_from_api", "user_id", userID, "token_id", token.ID)

	// fazer request para discord api
	url := fmt.Sprintf("https://discord.com/api/v10/users/%s", userID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed_to_create_request: %w", err)
	}

	req.Header.Set("Authorization", token.DecryptedValue)
	req.Header.Set("User-Agent", "DiscordBot (https://github.com/discord/discord-api-docs, 1.0)")

	resp, err := uf.httpClient.Do(req)
	if err != nil {
		uf.logger.Warn("api_request_failed", "user_id", userID, "error", err)
		return nil, fmt.Errorf("request_failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		uf.logger.Info("user_not_found_in_discord_api", "user_id", userID)
		return nil, fmt.Errorf("user_not_found")
	}

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		uf.logger.Warn("user_token_unauthorized", "user_id", userID, "status", resp.StatusCode, "msg", "user token nao pode buscar este usuario - tentando bot token")
		
		// tentar buscar via bot token se disponivel
		if uf.botToken != "" {
			uf.logger.Info("trying_bot_token", "user_id", userID)
			return uf.fetchWithBotToken(ctx, userID)
		}
		
		// sem bot token - usuario nao pode ser buscado
		uf.logger.Warn("no_bot_token_configured", "user_id", userID)
		return nil, fmt.Errorf("user_not_found_in_gateway_data: usuario nao esta em servidores compartilhados e bot token nao configurado")
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		uf.logger.Warn("discord_api_error", "user_id", userID, "status", resp.StatusCode, "body", string(bodyBytes))
		return nil, fmt.Errorf("discord_api_error: status=%d body=%s", resp.StatusCode, string(bodyBytes))
	}

	var user DiscordUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("failed_to_decode_response: %w", err)
	}

	// cachear resultado por 5 minutos
	if userJSON, err := json.Marshal(user); err == nil {
		uf.redis.Set(ctx, cacheKey, string(userJSON), 5*time.Minute)
	}

	uf.logger.Info("user_fetched_from_api", "user_id", userID, "username", user.Username)

	return &user, nil
}

// SaveUserToDatabase salva usuário buscado no banco de dados
func (uf *UserFetcher) SaveUserToDatabase(ctx context.Context, user *DiscordUser, source string) error {
	uf.logger.Info("saving_user_to_database", "user_id", user.ID, "source", source, "username", user.Username)
	
	// garantir que usuario existe
	_, err := uf.db.Pool.Exec(ctx,
		`INSERT INTO users (id, public_data_source, last_public_fetch) 
		 VALUES ($1, $2, NOW())
		 ON CONFLICT (id) DO UPDATE SET 
			public_data_source = COALESCE(EXCLUDED.public_data_source, users.public_data_source),
			last_public_fetch = NOW()`,
		user.ID, source,
	)
	if err != nil {
		uf.logger.Error("failed_to_insert_user", "user_id", user.ID, "error", err)
		return fmt.Errorf("failed_to_insert_user: %w", err)
	}
	
	uf.logger.Debug("user_inserted_or_updated", "user_id", user.ID)

	// salvar username history se houver mudanca
	if user.Username != "" || user.GlobalName != "" {
		var discriminator *string
		if user.Discriminator != "" && user.Discriminator != "0" {
			discriminator = &user.Discriminator
		}

		var username, globalName *string
		if user.Username != "" {
			username = &user.Username
		}
		if user.GlobalName != "" {
			globalName = &user.GlobalName
		}

		// verificar se ja existe
		var exists bool
		_ = uf.db.Pool.QueryRow(ctx,
			`SELECT EXISTS(
				SELECT 1 FROM username_history 
				WHERE user_id = $1 AND username IS NOT DISTINCT FROM $2 
				AND discriminator IS NOT DISTINCT FROM $3 
				AND global_name IS NOT DISTINCT FROM $4
				LIMIT 1
			)`,
			user.ID, username, discriminator, globalName,
		).Scan(&exists)

		if !exists {
			_, err = uf.db.Pool.Exec(ctx,
				`INSERT INTO username_history (user_id, username, discriminator, global_name, changed_at)
				 VALUES ($1, $2, $3, $4, NOW())`,
				user.ID, username, discriminator, globalName,
			)
			if err != nil {
				uf.logger.Warn("failed_to_save_username_history", "user_id", user.ID, "error", err)
			}
		}
	}

	// salvar avatar se houver
	if user.Avatar != "" {
		var exists bool
		_ = uf.db.Pool.QueryRow(ctx,
			`SELECT EXISTS(
				SELECT 1 FROM avatar_history 
				WHERE user_id = $1 AND hash_avatar = $2
				LIMIT 1
			)`,
			user.ID, user.Avatar,
		).Scan(&exists)

		if !exists {
			_, err = uf.db.Pool.Exec(ctx,
				`INSERT INTO avatar_history (user_id, hash_avatar, url_cdn, changed_at)
				 VALUES ($1, $2, NULL, NOW())`,
				user.ID, user.Avatar,
			)
			if err != nil {
				uf.logger.Warn("failed_to_save_avatar_history", "user_id", user.ID, "error", err)
			}
		}
	}

	// salvar bio se houver
	if user.Bio != "" {
		var exists bool
		_ = uf.db.Pool.QueryRow(ctx,
			`SELECT EXISTS(
				SELECT 1 FROM bio_history 
				WHERE user_id = $1 AND bio_content = $2
				LIMIT 1
			)`,
			user.ID, user.Bio,
		).Scan(&exists)

		if !exists {
			_, err = uf.db.Pool.Exec(ctx,
				`INSERT INTO bio_history (user_id, bio_content, changed_at)
				 VALUES ($1, $2, NOW())`,
				user.ID, user.Bio,
			)
			if err != nil {
				uf.logger.Warn("failed_to_save_bio_history", "user_id", user.ID, "error", err)
			}
		}
	}

	// atualizar banner se disponivel
	if user.Banner != "" {
		_, _ = uf.db.Pool.Exec(ctx,
			`UPDATE users SET banner_hash = $1 WHERE id = $2`,
			user.Banner, user.ID,
		)
	}

	uf.logger.Info("user_saved_to_database", "user_id", user.ID, "source", source)
	return nil
}

// TryFetchFromGatewayData tenta buscar usuario dos dados ja coletados via gateway (exportado)
func (uf *UserFetcher) TryFetchFromGatewayData(ctx context.Context, userID string) (*DiscordUser, error) {
	return uf.tryFetchFromGatewayData(ctx, userID)
}

// tryFetchFromGatewayData tenta buscar usuario dos dados ja coletados via gateway (interno)
func (uf *UserFetcher) tryFetchFromGatewayData(ctx context.Context, userID string) (*DiscordUser, error) {
	uf.logger.Info("trying_to_fetch_from_gateway_data", "user_id", userID)
	
	// buscar nos dados ja coletados
	var username, discriminator, globalName, avatar, bio *string
	err := uf.db.Pool.QueryRow(ctx,
		`SELECT 
			uh.username,
			uh.discriminator,
			uh.global_name,
			ah.hash_avatar,
			bh.bio_content
		FROM users u
		LEFT JOIN LATERAL (
			SELECT username, discriminator, global_name
			FROM username_history
			WHERE user_id = u.id
			ORDER BY changed_at DESC
			LIMIT 1
		) uh ON true
		LEFT JOIN LATERAL (
			SELECT hash_avatar
			FROM avatar_history
			WHERE user_id = u.id
			ORDER BY changed_at DESC
			LIMIT 1
		) ah ON true
		LEFT JOIN LATERAL (
			SELECT bio_content
			FROM bio_history
			WHERE user_id = u.id
			ORDER BY changed_at DESC
			LIMIT 1
		) bh ON true
		WHERE u.id = $1`,
		userID,
	).Scan(&username, &discriminator, &globalName, &avatar, &bio)
	
	if err != nil {
		uf.logger.Warn("user_not_found_in_gateway_data", "user_id", userID, "error", err)
		return nil, fmt.Errorf("user_not_found_in_gateway_data: tokens de usuario nao podem buscar usuarios que nao estao em servidores compartilhados")
	}
	
	user := &DiscordUser{
		ID:            userID,
		Username:      stringValue(username),
		Discriminator: stringValue(discriminator),
		GlobalName:    stringValue(globalName),
		Avatar:        stringValue(avatar),
		Bio:           stringValue(bio),
	}
	
	uf.logger.Info("user_found_in_gateway_data", "user_id", userID, "username", user.Username)
	return user, nil
}

func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// findTokenWithAccess busca um token que tem acesso ao usuario via guild_members
func (uf *UserFetcher) findTokenWithAccess(ctx context.Context, userID string) (*TokenEntry, error) {
	// buscar token_id que tem acesso a este usuario
	var tokenID int64
	err := uf.db.Pool.QueryRow(ctx,
		`SELECT gm.token_id 
		 FROM guild_members gm
		 INNER JOIN tokens t ON t.id = gm.token_id AND t.status = 'ativo'
		 WHERE gm.user_id = $1
		 ORDER BY gm.last_seen_at DESC
		 LIMIT 1`,
		userID,
	).Scan(&tokenID)
	
	if err != nil {
		return nil, fmt.Errorf("no_token_with_access: %w", err)
	}
	
	// buscar o token completo do token manager
	uf.logger.Info("found_token_with_access", "user_id", userID, "token_id", tokenID)
	
	// pegar o token descriptografado
	return uf.tokenManager.GetTokenByID(tokenID)
}

// fetchWithBotToken busca usuario usando bot token do .env
func (uf *UserFetcher) fetchWithBotToken(ctx context.Context, userID string) (*DiscordUser, error) {
	uf.logger.Info("fetching_with_bot_token", "user_id", userID)
	
	url := fmt.Sprintf("https://discord.com/api/v10/users/%s", userID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed_to_create_request: %w", err)
	}

	// bot token precisa do prefixo "Bot "
	authHeader := uf.botToken
	if !strings.HasPrefix(strings.ToLower(authHeader), "bot ") {
		authHeader = "Bot " + authHeader
	}
	
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("User-Agent", "DiscordBot (https://github.com/discord/discord-api-docs, 1.0)")

	resp, err := uf.httpClient.Do(req)
	if err != nil {
		uf.logger.Warn("bot_token_request_failed", "user_id", userID, "error", err)
		return nil, fmt.Errorf("bot_token_request_failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		uf.logger.Info("user_not_found_via_bot_token", "user_id", userID)
		return nil, fmt.Errorf("user_not_found")
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		uf.logger.Warn("bot_token_api_error", "user_id", userID, "status", resp.StatusCode, "body", string(bodyBytes))
		return nil, fmt.Errorf("bot_token_api_error: status=%d body=%s", resp.StatusCode, string(bodyBytes))
	}

	var user DiscordUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("failed_to_decode_response: %w", err)
	}

	// cachear resultado
	cacheKey := fmt.Sprintf("discord_user:%s", userID)
	if userJSON, err := json.Marshal(user); err == nil {
		uf.redis.Set(ctx, cacheKey, string(userJSON), 5*time.Minute)
	}

	uf.logger.Info("user_fetched_via_bot_token", "user_id", userID, "username", user.Username)
	return &user, nil
}

