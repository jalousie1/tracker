package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"identity-archive/internal/config"
	"identity-archive/internal/db"
	"identity-archive/internal/discord"
	"identity-archive/internal/processor"
	"identity-archive/internal/redis"
)

type Server struct {
	log            *slog.Logger
	db             *db.DB
	redis          *redis.Client
	ep             *processor.EventProcessor
	cfg            config.Config
	router         *gin.Engine
	tokenManager   *discord.TokenManager
	gatewayManager *discord.GatewayManager
	userFetcher    *discord.UserFetcher
	publicScraper  *discord.PublicScraper
	sourceManager  interface{} // será *external.SourceManager quando implementado
}

func NewServer(log *slog.Logger, dbConn *db.DB, redisClient *redis.Client, ep *processor.EventProcessor, cfg config.Config) *Server {
	return NewServerWithManagers(log, dbConn, redisClient, ep, cfg, nil, nil)
}

func NewServerWithManagers(log *slog.Logger, dbConn *db.DB, redisClient *redis.Client, ep *processor.EventProcessor, cfg config.Config, tokenManager *discord.TokenManager, gatewayManager *discord.GatewayManager) *Server {
	// If running API without a TokenManager, still dedupe tokens in DB so the admin panel stays clean.
	if tokenManager == nil && len(cfg.EncryptionKey) == 32 {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if removed, err := discord.DeduplicateTokensInDB(ctx, log, dbConn, cfg.EncryptionKey); err != nil {
			log.Warn("token_dedup_failed", "error", err)
		} else if removed > 0 {
			log.Info("token_dedup_removed", "removed", removed)
		}
		cancel()
	}

	var userFetcher *discord.UserFetcher
	var publicScraper *discord.PublicScraper
	if tokenManager != nil {
		userFetcher = discord.NewUserFetcher(log, dbConn, redisClient, tokenManager, cfg.BotToken)
		publicScraper = discord.NewPublicScraper(log, dbConn, redisClient, tokenManager, cfg.BotToken)
		if cfg.BotToken != "" {
			log.Info("bot_token_configured", "can_fetch_any_user", true)
		} else {
			log.Info("bot_token_not_configured", "can_fetch_only_shared_guilds", true)
		}
	}

	s := &Server{
		log:            log,
		db:             dbConn,
		redis:          redisClient,
		ep:             ep,
		cfg:            cfg,
		router:         gin.New(),
		tokenManager:   tokenManager,
		gatewayManager: gatewayManager,
		userFetcher:    userFetcher,
		publicScraper:  publicScraper,
	}

	gin.SetMode(gin.ReleaseMode)
	r := s.router
	r.Use(gin.Recovery())
	r.Use(s.corsMiddleware())
	r.Use(s.loggingMiddleware())
	r.Use(s.inputValidationMiddleware())
	r.Use(s.rateLimitMiddleware())

	// API v1 routes
	v1 := r.Group("/api/v1")
	{
		v1.GET("/profile/:discord_id", s.getProfile)
		v1.GET("/public-lookup/:discord_id", s.publicLookup)
		v1.GET("/search", s.search)
		v1.GET("/alt-check/:discord_id", s.altCheck)
		v1.GET("/health", s.health)

		// Admin routes
		admin := v1.Group("/admin")
		admin.Use(s.adminAuthMiddleware())
		{
			admin.GET("/tokens", s.listTokens)
			admin.POST("/tokens", s.addToken)
			admin.DELETE("/tokens/:id", s.removeToken)
			admin.POST("/fetch-user/:discord_id", s.fetchUser)
		}
	}

	// Legacy routes for backward compatibility
	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })
	r.GET("/profile/:discord_id", s.getProfile)
	r.GET("/search", s.search)
	r.GET("/alt-check/:discord_id", s.altCheck)

	return s
}

func (s *Server) Handler() http.Handler {
	return s.router
}

func (s *Server) ctx(c *gin.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(c.Request.Context(), 10*time.Second)
}
