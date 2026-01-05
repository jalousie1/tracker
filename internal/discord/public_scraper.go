package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"identity-archive/internal/db"
	"identity-archive/internal/redis"
)

type PublicScraper struct {
	tokenManager *TokenManager
	db           *db.DB
	redis        *redis.Client
	logger       *slog.Logger
	httpClient   *http.Client
}

func NewPublicScraper(logger *slog.Logger, dbConn *db.DB, redisClient *redis.Client, tokenManager *TokenManager, botToken string) *PublicScraper {
	return &PublicScraper{
		tokenManager: tokenManager,
		db:           dbConn,
		redis:        redisClient,
		logger:       logger,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// ScrapeAvatar verifica se avatar existe via CDN
func (ps *PublicScraper) ScrapeAvatar(ctx context.Context, userID, avatarHash string) (bool, error) {
	if avatarHash == "" {
		return false, nil
	}

	url := fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.png", userID, avatarHash)
	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return false, err
	}

	resp, err := ps.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

// ScrapeBanner verifica se banner existe via CDN
func (ps *PublicScraper) ScrapeBanner(ctx context.Context, userID, bannerHash string) (bool, string, error) {
	if bannerHash == "" {
		return false, "", nil
	}

	// tentar diferentes formatos
	formats := []string{"png", "gif", "webp"}
	for _, format := range formats {
		url := fmt.Sprintf("https://cdn.discordapp.com/banners/%s/%s.%s", userID, bannerHash, format)
		req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
		if err != nil {
			continue
		}

		resp, err := ps.httpClient.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			return true, url, nil
		}
	}

	return false, "", nil
}

// FindUserInGuilds busca usuário em guilds conhecidas
func (ps *PublicScraper) FindUserInGuilds(ctx context.Context, userID string) (*DiscordUser, error) {
	// obter token disponível
	_, err := ps.tokenManager.GetNextAvailableToken()
	if err != nil {
		return nil, fmt.Errorf("no_token_available: %w", err)
	}

	// buscar guilds do token
	// nota: isso requer que o gateway manager tenha carregado as guilds
	// por enquanto, vamos tentar buscar diretamente via api

	// tentar buscar em algumas guilds grandes conhecidas (se token tiver acesso)
	// isso é limitado, mas pode funcionar para alguns casos

	return nil, fmt.Errorf("not_implemented_yet")
}

// CheckAvatarChange verifica se avatar mudou comparando com banco
func (ps *PublicScraper) CheckAvatarChange(ctx context.Context, userID string) error {
	// buscar ultimo avatar conhecido
	var lastAvatarHash string
	err := ps.db.Pool.QueryRow(ctx,
		`SELECT hash_avatar FROM avatar_history 
		 WHERE user_id = $1 
		 ORDER BY changed_at DESC 
		 LIMIT 1`,
		userID,
	).Scan(&lastAvatarHash)

	if err != nil {
		// sem avatar anterior, nada a fazer
		return nil
	}

	// verificar se ainda existe
	exists, err := ps.ScrapeAvatar(ctx, userID, lastAvatarHash)
	if err != nil {
		return err
	}

	if !exists {
		ps.logger.Info("avatar_removed", "user_id", userID, "avatar_hash", lastAvatarHash)
	}

	return nil
}

// FetchUserViaCDN tenta descobrir informações do usuário via CDN
func (ps *PublicScraper) FetchUserViaCDN(ctx context.Context, userID string) (*DiscordUser, error) {
	// tentar diferentes avatares conhecidos (limitado)
	// isso não é muito eficaz, mas pode funcionar para alguns casos

	// por enquanto, retornar erro pois não há forma confiável de descobrir avatar via CDN sem saber o hash
	return nil, fmt.Errorf("cdn_lookup_not_supported")
}

// RateLimitDelay aguarda respeitando rate limits do discord
func (ps *PublicScraper) RateLimitDelay(ctx context.Context) {
	// discord permite 50 req/s por bot
	// aguardar 20ms entre requisições para ficar seguro
	select {
	case <-ctx.Done():
		return
	case <-time.After(20 * time.Millisecond):
		return
	}
}

// DownloadAvatar baixa avatar do CDN e retorna bytes
func (ps *PublicScraper) DownloadAvatar(ctx context.Context, userID, avatarHash string) ([]byte, error) {
	if avatarHash == "" {
		return nil, fmt.Errorf("empty_avatar_hash")
	}

	url := fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.png", userID, avatarHash)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := ps.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("avatar_not_found: status=%d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// PublicUserData representa dados públicos de um usuário do Discord
type PublicUserData struct {
	ID          string   `json:"id"`
	Username    string   `json:"username"`
	GlobalName  string   `json:"global_name"`
	Avatar      string   `json:"avatar"`
	Banner      string   `json:"banner"`
	AccentColor int      `json:"accent_color"`
	Flags       int      `json:"public_flags"`
	Bio         string   `json:"bio"`
	Connections []string `json:"connections"`
	Source      string   `json:"source"`
	FetchedAt   time.Time
}

// FetchPublicData tenta buscar dados públicos do usuário de múltiplas fontes
func (ps *PublicScraper) FetchPublicData(ctx context.Context, userID string) (*PublicUserData, error) {
	// Tentar discord.id primeiro (API pública)
	data, err := ps.fetchFromDiscordID(ctx, userID)
	if err == nil && data != nil {
		data.Source = "discord.id"
		data.FetchedAt = time.Now()
		return data, nil
	}
	ps.logger.Debug("discord_id_fetch_failed", "user_id", userID, "error", err)

	// Tentar discordlookup.com
	data, err = ps.fetchFromDiscordLookup(ctx, userID)
	if err == nil && data != nil {
		data.Source = "discordlookup.com"
		data.FetchedAt = time.Now()
		return data, nil
	}
	ps.logger.Debug("discordlookup_fetch_failed", "user_id", userID, "error", err)

	return nil, fmt.Errorf("no_public_data_found")
}

// fetchFromDiscordID busca dados de discord.id (user info cache)
func (ps *PublicScraper) fetchFromDiscordID(ctx context.Context, userID string) (*PublicUserData, error) {
	url := fmt.Sprintf("https://discord.id/api/user/%s", userID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := ps.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discord_id_status=%d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Parse response
	var result struct {
		ID         string `json:"id"`
		Username   string `json:"username"`
		GlobalName string `json:"global_name"`
		Avatar     string `json:"avatar"`
		Banner     string `json:"banner"`
		Flags      int    `json:"public_flags"`
	}

	if err := parseJSON(body, &result); err != nil {
		return nil, err
	}

	if result.ID == "" {
		return nil, fmt.Errorf("empty_response")
	}

	return &PublicUserData{
		ID:         result.ID,
		Username:   result.Username,
		GlobalName: result.GlobalName,
		Avatar:     result.Avatar,
		Banner:     result.Banner,
		Flags:      result.Flags,
	}, nil
}

// fetchFromDiscordLookup busca dados de discordlookup.com
func (ps *PublicScraper) fetchFromDiscordLookup(ctx context.Context, userID string) (*PublicUserData, error) {
	url := fmt.Sprintf("https://discordlookup.com/api/user/%s", userID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := ps.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discordlookup_status=%d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Parse response
	var result struct {
		ID          string `json:"id"`
		Username    string `json:"username"`
		GlobalName  string `json:"global_name"`
		Avatar      string `json:"avatar"`
		Banner      string `json:"banner"`
		AccentColor int    `json:"accent_color"`
		Flags       int    `json:"public_flags"`
	}

	if err := parseJSON(body, &result); err != nil {
		return nil, err
	}

	if result.ID == "" {
		return nil, fmt.Errorf("empty_response")
	}

	return &PublicUserData{
		ID:          result.ID,
		Username:    result.Username,
		GlobalName:  result.GlobalName,
		Avatar:      result.Avatar,
		Banner:      result.Banner,
		AccentColor: result.AccentColor,
		Flags:       result.Flags,
	}, nil
}

// SavePublicData salva dados públicos no banco de dados
func (ps *PublicScraper) SavePublicData(ctx context.Context, data *PublicUserData) error {
	if data == nil || data.ID == "" {
		return fmt.Errorf("invalid_data")
	}

	// Garantir que usuário existe
	_, _ = ps.db.Pool.Exec(ctx,
		`INSERT INTO users (id) VALUES ($1) ON CONFLICT (id) DO NOTHING`,
		data.ID,
	)

	// Salvar username se existir
	if data.Username != "" {
		var exists bool
		_ = ps.db.Pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM username_history WHERE user_id = $1 AND username = $2 LIMIT 1)`,
			data.ID, data.Username,
		).Scan(&exists)

		if !exists {
			_, _ = ps.db.Pool.Exec(ctx,
				`INSERT INTO username_history (user_id, username, global_name, changed_at)
				 VALUES ($1, $2, $3, NOW())`,
				data.ID, data.Username, data.GlobalName,
			)
			ps.logger.Info("public_username_saved", "user_id", data.ID, "username", data.Username, "source", data.Source)
		}
	}

	// Salvar avatar se existir
	if data.Avatar != "" {
		var exists bool
		_ = ps.db.Pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM avatar_history WHERE user_id = $1 AND hash_avatar = $2 LIMIT 1)`,
			data.ID, data.Avatar,
		).Scan(&exists)

		if !exists {
			_, _ = ps.db.Pool.Exec(ctx,
				`INSERT INTO avatar_history (user_id, hash_avatar, changed_at)
				 VALUES ($1, $2, NOW())`,
				data.ID, data.Avatar,
			)
			ps.logger.Info("public_avatar_saved", "user_id", data.ID, "avatar", data.Avatar, "source", data.Source)
		}
	}

	// Salvar banner se existir
	if data.Banner != "" {
		var exists bool
		_ = ps.db.Pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM banner_history WHERE user_id = $1 AND banner_hash = $2 LIMIT 1)`,
			data.ID, data.Banner,
		).Scan(&exists)

		if !exists {
			_, _ = ps.db.Pool.Exec(ctx,
				`INSERT INTO banner_history (user_id, banner_hash, changed_at)
				 VALUES ($1, $2, NOW())`,
				data.ID, data.Banner,
			)
			ps.logger.Info("public_banner_saved", "user_id", data.ID, "banner", data.Banner, "source", data.Source)
		}
	}

	return nil
}

// parseJSON helper para parsear JSON
func parseJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
