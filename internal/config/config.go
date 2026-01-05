package config

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	DBDSN      string
	HTTPAddr   string
	LogLevel   string
	R2Endpoint string
	R2Bucket   string
	RedisDSN   string

	// Event processing
	EventWorkerCount int

	// Discord scraping/runtime behavior
	DiscordEnableGuildSubscriptions  bool
	DiscordRequestMemberPresences    bool
	DiscordScrapeInitialGuildMembers bool
	DiscordMaxConcurrentGuildScrapes int
	DiscordScrapeQueryDelayMs        int

	// raw secrets kept in-memory only; never log these
	R2KeysRaw         string
	EncryptionKeysRaw string
	EncryptionKey     []byte // decoded from EncryptionKeysRaw
	AdminSecretKey    string
	CORSOrigins       []string
	BotToken          string // bot token para buscar qualquer usuario
}

func Load() (Config, error) {
	cfg := Config{
		DBDSN:                            os.Getenv("DB_DSN"),
		HTTPAddr:                         getenvDefault("HTTP_ADDR", ":8080"),
		LogLevel:                         getenvDefault("LOG_LEVEL", "info"),
		R2Endpoint:                       getenvDefault("R2_ENDPOINT", ""),
		R2Bucket:                         getenvDefault("R2_BUCKET", ""),
		R2KeysRaw:                        os.Getenv("R2_KEYS"),
		RedisDSN:                         getenvDefault("REDIS_DSN", "redis://localhost:6379/0"),
		EventWorkerCount:                 getenvIntDefault("EVENT_WORKER_COUNT", 20),
		AdminSecretKey:                   getenvDefault("ADMIN_SECRET_KEY", ""),
		BotToken:                         os.Getenv("BOT_TOKEN"),
		DiscordEnableGuildSubscriptions:  getenvBoolDefault("DISCORD_ENABLE_GUILD_SUBSCRIPTIONS", true),
		DiscordRequestMemberPresences:    getenvBoolDefault("DISCORD_REQUEST_MEMBER_PRESENCES", true),
		DiscordScrapeInitialGuildMembers: getenvBoolDefault("DISCORD_SCRAPE_INITIAL_GUILD_MEMBERS", true),
		DiscordMaxConcurrentGuildScrapes: getenvIntDefault("DISCORD_MAX_CONCURRENT_GUILD_SCRAPES", 1),
		DiscordScrapeQueryDelayMs:        getenvIntDefault("DISCORD_SCRAPE_QUERY_DELAY_MS", 250),
	}

	cfg.EncryptionKeysRaw = os.Getenv("ENCRYPTION_KEY")

	if cfg.DBDSN == "" {
		return Config{}, errors.New("missing DB_DSN")
	}

	// light validation: ensure secrets are valid json if set
	if cfg.R2KeysRaw != "" {
		var tmp any
		if err := json.Unmarshal([]byte(cfg.R2KeysRaw), &tmp); err != nil {
			return Config{}, errors.New("R2_KEYS must be valid json")
		}
	}

	// decode encryption key (base64, must be 32 bytes)
	if cfg.EncryptionKeysRaw != "" {
		key, err := base64.StdEncoding.DecodeString(cfg.EncryptionKeysRaw)
		if err != nil {
			return Config{}, errors.New("ENCRYPTION_KEY must be valid base64")
		}
		if len(key) != 32 {
			return Config{}, errors.New("ENCRYPTION_KEY must be 32 bytes (256 bits)")
		}
		cfg.EncryptionKey = key
	}

	// parse CORS origins
	corsOrigins := getenvDefault("CORS_ORIGINS", "")
	if corsOrigins != "" {
		cfg.CORSOrigins = strings.Split(corsOrigins, ",")
		for i := range cfg.CORSOrigins {
			cfg.CORSOrigins[i] = strings.TrimSpace(cfg.CORSOrigins[i])
		}
	} else {
		cfg.CORSOrigins = []string{"http://localhost:3000"} // default
	}

	return cfg, nil
}

func getenvDefault(k, def string) string {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	return v
}

func getenvBoolDefault(k string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return def
	}
}

func getenvIntDefault(k string, def int) int {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	// accept plain ints; on parse failure, fall back
	var n int
	_, err := fmt.Sscanf(v, "%d", &n)
	if err != nil {
		return def
	}
	return n
}
