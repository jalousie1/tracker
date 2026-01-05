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
	tokenManager   *TokenManager
	db             *db.DB
	redis          *redis.Client
	logger         *slog.Logger
	httpClient     *http.Client
	botToken       string // bot token do .env para fallback
	circuitBreaker *CircuitBreaker
	retryConfig    RetryConfig
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
		tokenManager:   tokenManager,
		db:             dbConn,
		redis:          redisClient,
		logger:         logger,
		botToken:       botToken,
		httpClient:     DiscordHTTPClient, // Use shared optimized HTTP client
		circuitBreaker: NewCircuitBreaker(),
		retryConfig:    DefaultRetryConfig(),
	}
}

// FetchUserByID busca um usuário via Discord API usando USER TOKEN prioritariamente
// A ordem de prioridade é:
// 1. User token que tem acesso direto ao usuário (mesmo servidor)
// 2. Qualquer user token disponível
// 3. Bot token como último recurso (limitado - não retorna bio, banner completo)
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

	// PRIORIDADE 1: tentar encontrar um USER TOKEN que tem acesso a este usuario
	tokenEntry, err := uf.findTokenWithAccess(ctx, userID)
	if err != nil {
		// PRIORIDADE 2: tenta qualquer USER TOKEN disponivel
		uf.logger.Debug("no_token_with_access_found", "user_id", userID, "error", err)
		tokenEntry, err = uf.tokenManager.GetNextAvailableToken()
	}

	// Se temos um user token, usar ele
	if err == nil && tokenEntry != nil {
		uf.logger.Info("fetching_user_with_user_token", "user_id", userID, "token_id", tokenEntry.ID)
		user, fetchErr := uf.fetchWithUserToken(ctx, userID, tokenEntry)
		if fetchErr == nil {
			return user, nil
		}
		uf.logger.Debug("user_token_fetch_failed", "user_id", userID, "error", fetchErr)
	}

	// PRIORIDADE 3: tentar bot token como fallback (dados limitados)
	if uf.botToken != "" {
		uf.logger.Info("trying_bot_token_fallback", "user_id", userID)
		return uf.fetchWithBotToken(ctx, userID)
	}

	return nil, fmt.Errorf("no_token_available: nenhum user token ou bot token disponivel")
}

// fetchWithUserToken busca usuário usando um user token específico
func (uf *UserFetcher) fetchWithUserToken(ctx context.Context, userID string, tokenEntry *TokenEntry) (*DiscordUser, error) {
	// Check circuit breaker first
	if !uf.circuitBreaker.Allow() {
		uf.logger.Warn("circuit_breaker_open", "user_id", userID, "state", uf.circuitBreaker.StateString())
		return nil, fmt.Errorf("circuit_breaker_open: Discord API temporarily unavailable")
	}

	var resp *http.Response
	var lastErr error

	for attempt := 0; attempt < uf.retryConfig.MaxRetries; attempt++ {
		// Verificar se contexto foi cancelado antes de cada tentativa
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return nil, fmt.Errorf("context_cancelled: %w", lastErr)
			}
			return nil, ctx.Err()
		default:
		}

		url := fmt.Sprintf("https://discord.com/api/v10/users/%s", userID)
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed_to_create_request: %w", err)
		}

		// Headers para simular cliente Discord real (user token)
		req.Header.Set("Authorization", tokenEntry.DecryptedValue)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) discord/1.0.9032 Chrome/120.0.6099.291 Electron/28.2.10 Safari/537.36")
		req.Header.Set("X-Super-Properties", "eyJvcyI6IldpbmRvd3MiLCJicm93c2VyIjoiRGlzY29yZCBDbGllbnQiLCJyZWxlYXNlX2NoYW5uZWwiOiJzdGFibGUiLCJjbGllbnRfdmVyc2lvbiI6IjEuMC45MDMyIiwib3NfdmVyc2lvbiI6IjEwLjAuMTkwNDUiLCJvc19hcmNoIjoieDY0IiwiYXBwX2FyY2giOiJ4NjQiLCJzeXN0ZW1fbG9jYWxlIjoicHQtQlIiLCJicm93c2VyX3VzZXJfYWdlbnQiOiJNb3ppbGxhLzUuMCAoV2luZG93cyBOVCAxMC4wOyBXaW42NDsgeDY0KSBBcHBsZVdlYktpdC81MzcuMzYgKEtIVE1MLCBsaWtlIEdlY2tvKSBkaXNjb3JkLzEuMC45MDMyIENocm9tZS8xMjAuMC42MDk5LjI5MSBFbGVjdHJvbi8yOC4yLjEwIFNhZmFyaS81MzcuMzYiLCJicm93c2VyX3ZlcnNpb24iOiIyOC4yLjEwIiwiY2xpZW50X2J1aWxkX251bWJlciI6MjkwODg4LCJuYXRpdmVfYnVpbGRfbnVtYmVyIjo0NjU2MCwiY2xpZW50X2V2ZW50X3NvdXJjZSI6bnVsbH0=")

		resp, err = uf.httpClient.Do(req)
		if err != nil {
			uf.circuitBreaker.RecordFailure()
			uf.logger.Warn("api_request_failed", "user_id", userID, "error", err, "attempt", attempt+1)
			lastErr = fmt.Errorf("request_failed: %w", err)

			// Exponential backoff before retry
			backoff := CalculateBackoff(uf.retryConfig, attempt, 0)
			time.Sleep(backoff)
			continue
		}

		// Se rate limited (429), usar exponential backoff com Retry-After
		if resp.StatusCode == http.StatusTooManyRequests {
			uf.circuitBreaker.RecordFailure()
			var retryAfter time.Duration
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if parsed, parseErr := time.ParseDuration(ra + "s"); parseErr == nil {
					retryAfter = parsed
				}
			}
			backoff := CalculateBackoff(uf.retryConfig, attempt, retryAfter)
			uf.logger.Warn("rate_limited", "user_id", userID, "backoff", backoff.String(), "attempt", attempt+1)
			resp.Body.Close()
			time.Sleep(backoff)
			continue
		}

		// Success - record it
		uf.circuitBreaker.RecordSuccess()
		break
	}

	if resp == nil {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("failed_to_fetch_user_after_retries")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("user_not_found")
	}

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("token_unauthorized")
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("rate_limited_after_retries")
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("discord_api_error: status=%d body=%s", resp.StatusCode, string(bodyBytes))
	}

	var user DiscordUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("failed_to_decode_response: %w", err)
	}

	// cachear resultado por 5 minutos
	cacheKey := fmt.Sprintf("discord_user:%s", userID)
	if userJSON, err := json.Marshal(user); err == nil {
		uf.redis.Set(ctx, cacheKey, string(userJSON), 5*time.Minute)
	}

	uf.logger.Info("user_fetched_with_user_token", "user_id", userID, "username", user.Username, "has_bio", user.Bio != "", "has_banner", user.Banner != "")

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
		// buscar os valores atuais para evitar 'vazio' em updates parciais
		var currUsername, currGlobalName, currDisc *string
		_ = uf.db.Pool.QueryRow(ctx,
			`SELECT username, global_name, discriminator 
			 FROM username_history 
			 WHERE user_id = $1 
			 ORDER BY changed_at DESC LIMIT 1`,
			user.ID,
		).Scan(&currUsername, &currGlobalName, &currDisc)

		// preparar novos valores
		newUsername := currUsername
		if user.Username != "" {
			newUsername = &user.Username
		}
		newGlobalName := currGlobalName
		if user.GlobalName != "" {
			newGlobalName = &user.GlobalName
		}
		newDisc := currDisc
		if user.Discriminator != "" && user.Discriminator != "0" {
			newDisc = &user.Discriminator
		}

		// so inserir se algo mudou E nao estamos limpando valores (vazio)
		// se o novo valor for vazio mas o antigo nao, ignoramos a mudanca no historico
		if (newUsername != nil || newGlobalName != nil) && ((newUsername != nil && currUsername == nil) ||
			(newUsername != nil && currUsername != nil && *newUsername != *currUsername) ||
			(newGlobalName != nil && currGlobalName == nil) ||
			(newGlobalName != nil && currGlobalName != nil && *newGlobalName != *currGlobalName) ||
			(newDisc != nil && currDisc == nil) ||
			(newDisc != nil && currDisc != nil && *newDisc != *currDisc)) {

			_, err = uf.db.Pool.Exec(ctx,
				`INSERT INTO username_history (user_id, username, discriminator, global_name, changed_at)
				 VALUES ($1, $2, $3, $4, NOW())`,
				user.ID, newUsername, newDisc, newGlobalName,
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

	// bot token precisa do prefixo "Bot "
	authHeader := uf.botToken
	if !strings.HasPrefix(strings.ToLower(authHeader), "bot ") {
		authHeader = "Bot " + authHeader
	}

	var resp *http.Response
	var lastErr error

	for attempt := 0; attempt < uf.retryConfig.MaxRetries; attempt++ {
		// Verificar se contexto foi cancelado antes de cada tentativa
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return nil, fmt.Errorf("context_cancelled: %w", lastErr)
			}
			return nil, ctx.Err()
		default:
		}

		url := fmt.Sprintf("https://discord.com/api/v10/users/%s", userID)
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed_to_create_request: %w", err)
		}

		req.Header.Set("Authorization", authHeader)
		req.Header.Set("User-Agent", "DiscordBot (https://github.com/discord/discord-api-docs, 1.0)")

		resp, err = uf.httpClient.Do(req)
		if err != nil {
			uf.circuitBreaker.RecordFailure()
			uf.logger.Warn("bot_token_request_failed", "user_id", userID, "error", err, "attempt", attempt+1)
			lastErr = fmt.Errorf("bot_token_request_failed: %w", err)

			// Exponential backoff before retry
			backoff := CalculateBackoff(uf.retryConfig, attempt, 0)
			time.Sleep(backoff)
			continue
		}

		// Se rate limited (429), usar exponential backoff com Retry-After
		if resp.StatusCode == http.StatusTooManyRequests {
			uf.circuitBreaker.RecordFailure()
			var retryAfter time.Duration
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if parsed, parseErr := time.ParseDuration(ra + "s"); parseErr == nil {
					retryAfter = parsed
				}
			}
			backoff := CalculateBackoff(uf.retryConfig, attempt, retryAfter)
			uf.logger.Warn("bot_token_rate_limited", "user_id", userID, "backoff", backoff.String(), "attempt", attempt+1)
			resp.Body.Close()
			time.Sleep(backoff)
			continue
		}

		// Success - record it
		uf.circuitBreaker.RecordSuccess()
		break
	}

	if resp == nil {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("failed_to_fetch_user_with_bot_token_after_retries")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		uf.logger.Info("user_not_found_via_bot_token", "user_id", userID)
		return nil, fmt.Errorf("user_not_found")
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		bodyBytes, _ := io.ReadAll(resp.Body)
		uf.logger.Warn("bot_token_rate_limited_exhausted", "user_id", userID, "body", string(bodyBytes))
		return nil, fmt.Errorf("bot_token_rate_limited_after_retries")
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
