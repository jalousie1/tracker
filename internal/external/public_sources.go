package external

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// DiscordIDSource busca dados de discord.id
type DiscordIDSource struct {
	httpClient *http.Client
	logger     *slog.Logger
}

func NewDiscordIDSource(logger *slog.Logger) *DiscordIDSource {
	return &DiscordIDSource{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		logger:     logger,
	}
}

func (d *DiscordIDSource) Name() string {
	return "discord.id"
}

func (d *DiscordIDSource) Priority() int {
	return 2 // prioridade média
}

func (d *DiscordIDSource) FetchUser(ctx context.Context, userID string) (*UserData, error) {
	url := fmt.Sprintf("https://discord.id/api/user/%s", userID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discord.id returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Estrutura esperada de discord.id
	var result struct {
		ID            string `json:"id"`
		Username      string `json:"username"`
		Discriminator string `json:"discriminator"`
		Avatar        string `json:"avatar"`
		Banner        string `json:"banner"`
		GlobalName    string `json:"global_name"`
		Bio           string `json:"bio"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if result.ID == "" {
		return nil, fmt.Errorf("user not found on discord.id")
	}

	d.logger.Debug("fetched_from_discord_id", "user_id", userID, "username", result.Username)

	return &UserData{
		UserID:        result.ID,
		Username:      result.Username,
		Discriminator: result.Discriminator,
		GlobalName:    result.GlobalName,
		Avatar:        result.Avatar,
		Banner:        result.Banner,
		Bio:           result.Bio,
		Source:        "discord.id",
		Confidence:    0.85,
	}, nil
}

// DiscordLookupSource busca dados de discordlookup.com
type DiscordLookupSource struct {
	httpClient *http.Client
	logger     *slog.Logger
}

func NewDiscordLookupSource(logger *slog.Logger) *DiscordLookupSource {
	return &DiscordLookupSource{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		logger:     logger,
	}
}

func (d *DiscordLookupSource) Name() string {
	return "discordlookup.com"
}

func (d *DiscordLookupSource) Priority() int {
	return 3
}

func (d *DiscordLookupSource) FetchUser(ctx context.Context, userID string) (*UserData, error) {
	url := fmt.Sprintf("https://discordlookup.com/api/user/%s", userID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discordlookup.com returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		ID            string `json:"id"`
		Username      string `json:"username"`
		Discriminator string `json:"discriminator"`
		Avatar        struct {
			ID string `json:"id"`
		} `json:"avatar"`
		Banner struct {
			ID string `json:"id"`
		} `json:"banner"`
		GlobalName string `json:"global_name"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if result.ID == "" {
		return nil, fmt.Errorf("user not found on discordlookup.com")
	}

	d.logger.Debug("fetched_from_discordlookup", "user_id", userID, "username", result.Username)

	return &UserData{
		UserID:        result.ID,
		Username:      result.Username,
		Discriminator: result.Discriminator,
		GlobalName:    result.GlobalName,
		Avatar:        result.Avatar.ID,
		Banner:        result.Banner.ID,
		Source:        "discordlookup.com",
		Confidence:    0.80,
	}, nil
}

// LanternSource busca dados de lantern.rest
type LanternSource struct {
	httpClient *http.Client
	logger     *slog.Logger
}

func NewLanternSource(logger *slog.Logger) *LanternSource {
	return &LanternSource{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		logger:     logger,
	}
}

func (l *LanternSource) Name() string {
	return "lantern.rest"
}

func (l *LanternSource) Priority() int {
	return 4
}

func (l *LanternSource) FetchUser(ctx context.Context, userID string) (*UserData, error) {
	url := fmt.Sprintf("https://lantern.rest/api/v1/users/%s", userID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lantern.rest returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		User struct {
			ID            string `json:"id"`
			Username      string `json:"username"`
			Discriminator string `json:"discriminator"`
			Avatar        string `json:"avatar"`
			Banner        string `json:"banner"`
			GlobalName    string `json:"global_name"`
			Bio           string `json:"bio"`
		} `json:"user"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if result.User.ID == "" {
		return nil, fmt.Errorf("user not found on lantern.rest")
	}

	l.logger.Debug("fetched_from_lantern", "user_id", userID, "username", result.User.Username)

	return &UserData{
		UserID:        result.User.ID,
		Username:      result.User.Username,
		Discriminator: result.User.Discriminator,
		GlobalName:    result.User.GlobalName,
		Avatar:        result.User.Avatar,
		Banner:        result.User.Banner,
		Bio:           result.User.Bio,
		Source:        "lantern.rest",
		Confidence:    0.75,
	}, nil
}

// NoneSource busca dados de none.io / discord.rest
type NoneSource struct {
	httpClient *http.Client
	logger     *slog.Logger
}

func NewNoneSource(logger *slog.Logger) *NoneSource {
	return &NoneSource{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		logger:     logger,
	}
}

func (n *NoneSource) Name() string {
	return "none.io"
}

func (n *NoneSource) Priority() int {
	return 5
}

func (n *NoneSource) FetchUser(ctx context.Context, userID string) (*UserData, error) {
	url := fmt.Sprintf("https://japi.rest/discord/v1/user/%s", userID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("japi.rest returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data struct {
			ID            string `json:"id"`
			Username      string `json:"username"`
			Discriminator string `json:"discriminator"`
			Avatar        string `json:"avatar"`
			Banner        string `json:"banner"`
			GlobalName    string `json:"global_name"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if result.Data.ID == "" {
		return nil, fmt.Errorf("user not found on japi.rest")
	}

	n.logger.Debug("fetched_from_japi", "user_id", userID, "username", result.Data.Username)

	return &UserData{
		UserID:        result.Data.ID,
		Username:      result.Data.Username,
		Discriminator: result.Data.Discriminator,
		GlobalName:    result.Data.GlobalName,
		Avatar:        result.Data.Avatar,
		Banner:        result.Data.Banner,
		Source:        "japi.rest",
		Confidence:    0.70,
	}, nil
}

// DiscordCDNSource verifica dados públicos no CDN do Discord (limitado)
type DiscordCDNSource struct {
	httpClient *http.Client
	logger     *slog.Logger
}

func NewDiscordCDNSource(logger *slog.Logger) *DiscordCDNSource {
	return &DiscordCDNSource{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		logger:     logger,
	}
}

func (d *DiscordCDNSource) Name() string {
	return "discord_cdn"
}

func (d *DiscordCDNSource) Priority() int {
	return 10 // baixa prioridade - apenas verifica se recursos existem
}

func (d *DiscordCDNSource) FetchUser(ctx context.Context, userID string) (*UserData, error) {
	// CDN não permite descobrir avatar sem saber o hash
	// Mas podemos tentar verificar avatar padrão
	defaultAvatarURL := fmt.Sprintf("https://cdn.discordapp.com/embed/avatars/%d.png", hashUserID(userID)%5)

	req, err := http.NewRequestWithContext(ctx, "HEAD", defaultAvatarURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()

	// Se conseguiu acessar CDN, usuário existe mas não temos dados
	if resp.StatusCode == http.StatusOK {
		return &UserData{
			UserID:     userID,
			Source:     "discord_cdn",
			Confidence: 0.10, // muito baixa - apenas confirmamos existência
		}, nil
	}

	return nil, fmt.Errorf("could not verify user via CDN")
}

// hashUserID cria hash simples para calcular avatar padrão
func hashUserID(userID string) int {
	var hash int
	for _, c := range userID {
		hash += int(c)
	}
	return hash
}

// MultiSourceFetcher busca de múltiplas fontes em paralelo
type MultiSourceFetcher struct {
	sources []DataSource
	logger  *slog.Logger
}

func NewMultiSourceFetcher(logger *slog.Logger, sources ...DataSource) *MultiSourceFetcher {
	return &MultiSourceFetcher{
		sources: sources,
		logger:  logger,
	}
}

// FetchUserParallel busca de todas as fontes em paralelo e retorna o melhor resultado
func (m *MultiSourceFetcher) FetchUserParallel(ctx context.Context, userID string) (*UserData, error) {
	type result struct {
		data *UserData
		err  error
	}

	results := make(chan result, len(m.sources))

	// buscar de todas as fontes em paralelo
	for _, source := range m.sources {
		go func(s DataSource) {
			data, err := s.FetchUser(ctx, userID)
			results <- result{data: data, err: err}
		}(source)
	}

	var bestResult *UserData
	var lastErr error
	timeout := time.After(20 * time.Second)

	for i := 0; i < len(m.sources); i++ {
		select {
		case r := <-results:
			if r.err == nil && r.data != nil {
				// pegar o resultado com maior confidence
				if bestResult == nil || r.data.Confidence > bestResult.Confidence {
					bestResult = r.data
				}
			} else if r.err != nil {
				lastErr = r.err
			}
		case <-timeout:
			m.logger.Warn("parallel_fetch_timeout", "user_id", userID)
			break
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if bestResult != nil {
		return bestResult, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}

	return nil, fmt.Errorf("user not found in any source")
}

// MergeUserData combina dados de múltiplas fontes, priorizando campos não vazios
func MergeUserData(existing, incoming *UserData) *UserData {
	if existing == nil {
		return incoming
	}
	if incoming == nil {
		return existing
	}

	// manter dados existentes se novos estiverem vazios
	result := &UserData{
		UserID:     existing.UserID,
		Source:     incoming.Source,
		Confidence: incoming.Confidence,
	}

	if incoming.Username != "" {
		result.Username = incoming.Username
	} else {
		result.Username = existing.Username
	}

	if incoming.Discriminator != "" {
		result.Discriminator = incoming.Discriminator
	} else {
		result.Discriminator = existing.Discriminator
	}

	if incoming.GlobalName != "" {
		result.GlobalName = incoming.GlobalName
	} else {
		result.GlobalName = existing.GlobalName
	}

	if incoming.Avatar != "" {
		result.Avatar = incoming.Avatar
	} else {
		result.Avatar = existing.Avatar
	}

	if incoming.Banner != "" {
		result.Banner = incoming.Banner
	} else {
		result.Banner = existing.Banner
	}

	if incoming.Bio != "" {
		result.Bio = incoming.Bio
	} else {
		result.Bio = existing.Bio
	}

	// ajustar confidence baseado em quantos campos temos
	filledFields := 0
	totalFields := 6
	if result.Username != "" {
		filledFields++
	}
	if result.Discriminator != "" {
		filledFields++
	}
	if result.GlobalName != "" {
		filledFields++
	}
	if result.Avatar != "" {
		filledFields++
	}
	if result.Banner != "" {
		filledFields++
	}
	if result.Bio != "" {
		filledFields++
	}

	result.Confidence = float64(filledFields) / float64(totalFields)

	return result
}

// CreateAllPublicSources cria todas as fontes públicas disponíveis
func CreateAllPublicSources(logger *slog.Logger) []DataSource {
	sources := []DataSource{
		NewDiscordIDSource(logger),
		NewDiscordLookupSource(logger),
		NewLanternSource(logger),
		NewNoneSource(logger),
		NewDiscordCDNSource(logger),
	}

	// Filtrar fontes que funcionam (algumas podem estar offline)
	return sources
}

// ValidateUserID verifica se string é um snowflake válido
func ValidateUserID(userID string) bool {
	if len(userID) < 17 || len(userID) > 20 {
		return false
	}
	for _, c := range userID {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// GetAvatarURL retorna URL do avatar baseado nos dados
func GetAvatarURL(userData *UserData, size int) string {
	if userData.Avatar == "" {
		// avatar padrão
		hash := hashUserID(userData.UserID)
		return fmt.Sprintf("https://cdn.discordapp.com/embed/avatars/%d.png", hash%5)
	}

	ext := "png"
	if strings.HasPrefix(userData.Avatar, "a_") {
		ext = "gif"
	}

	return fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.%s?size=%d",
		userData.UserID, userData.Avatar, ext, size)
}

// GetBannerURL retorna URL do banner se existir
func GetBannerURL(userData *UserData, size int) string {
	if userData.Banner == "" {
		return ""
	}

	ext := "png"
	if strings.HasPrefix(userData.Banner, "a_") {
		ext = "gif"
	}

	return fmt.Sprintf("https://cdn.discordapp.com/banners/%s/%s.%s?size=%d",
		userData.UserID, userData.Banner, ext, size)
}
