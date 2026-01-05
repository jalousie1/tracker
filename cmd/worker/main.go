package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"syscall"
	"time"

	"identity-archive/internal/config"
	"identity-archive/internal/db"
	"identity-archive/internal/discord"
	"identity-archive/internal/logging"
	"identity-archive/internal/processor"
	"identity-archive/internal/redis"
	"identity-archive/internal/storage"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	logger := logging.New(cfg.LogLevel)
	logger.Info("starting_worker", "service", "identity-archive-worker")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Connect to PostgreSQL (with retry)
	var dbConn *db.DB
	for i := 0; i < 5; i++ {
		dbConn, err = db.New(ctx, cfg.DBDSN)
		if err == nil {
			break
		}
		logger.Warn("db_connect_retry", "attempt", i+1, "error", err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		logger.Error("db_connect_failed", "error", err)
		os.Exit(1)
	}
	defer dbConn.Close()

	// Connect to Redis
	redisClient, err := redis.New(cfg.RedisDSN)
	if err != nil {
		logger.Error("redis_connect_failed", "error", err)
		os.Exit(1)
	}
	defer redisClient.Close()

	// Initialize storage client (R2 or simulator)
	var storageClient storage.StorageClient
	if cfg.R2Endpoint != "" && cfg.R2Bucket != "" {
		// Parse R2 keys
		var r2Keys map[string]string
		if err := json.Unmarshal([]byte(cfg.R2KeysRaw), &r2Keys); err == nil {
			s3Client, err := storage.NewS3Client(storage.S3Config{
				Endpoint:        cfg.R2Endpoint,
				AccessKeyID:     r2Keys["access_key_id"],
				SecretAccessKey: r2Keys["secret_access_key"],
				Bucket:          cfg.R2Bucket,
				PublicURL:       r2Keys["public_url"],
				Region:          "auto",
			})
			if err == nil {
				storageClient = s3Client
				logger.Info("using_s3_storage", "endpoint", cfg.R2Endpoint)
			}
		}
	}

	if storageClient == nil {
		storageClient = storage.NewR2Simulator(cfg.R2Bucket, cfg.R2Endpoint)
		logger.Info("using_r2_simulator")
	}

	// Initialize TokenManager
	if len(cfg.EncryptionKey) != 32 {
		logger.Error("invalid_encryption_key", "length", len(cfg.EncryptionKey))
		os.Exit(1)
	}

	tokenManager, err := discord.NewTokenManager(logger, dbConn, redisClient, cfg.EncryptionKey)
	if err != nil {
		logger.Error("token_manager_init_failed", "error", err)
		os.Exit(1)
	}

	// Initialize EventProcessor
	eventProcessor := processor.NewEventProcessor(logger, dbConn, redisClient, storageClient)
	eventProcessor.StartWorkers(cfg.EventWorkerCount)

	// Initialize Scraper
	scraper := discord.NewScraperWithOptions(logger, dbConn, redisClient, discord.ScraperOptions{
		QueryDelay: time.Duration(cfg.DiscordScrapeQueryDelayMs) * time.Millisecond,
	})

	// Initialize GatewayManager
	gatewayManager := discord.NewGatewayManagerWithOptions(tokenManager, eventProcessor, scraper, logger, dbConn, discord.GatewayManagerOptions{
		EnableGuildSubscriptions:  cfg.DiscordEnableGuildSubscriptions,
		RequestMemberPresences:    cfg.DiscordRequestMemberPresences,
		ScrapeInitialGuildMembers: cfg.DiscordScrapeInitialGuildMembers,
		MaxConcurrentGuildScrapes: cfg.DiscordMaxConcurrentGuildScrapes,
	})

	// Initialize AltDetector
	altDetector := processor.NewAltDetector(logger, dbConn)
	go altDetector.StartBackgroundJob()

	// Initialize Avatar Retry Job
	avatarRetryJob := storage.NewAvatarRetryJob(logger, dbConn, storageClient, redisClient)
	go avatarRetryJob.Start()

	// Connect all tokens
	logger.Info("connecting_tokens")
	if err := gatewayManager.ConnectAllTokens(ctx); err != nil {
		logger.Warn("some_tokens_failed_to_connect", "error", err)
	}

	// Scraping inicial (guild members) é disparado dentro do GatewayManager quando habilitado
	// via cfg.DiscordScrapeInitialGuildMembers.

	logger.Info("worker_started", "active_tokens", tokenManager.GetActiveTokenCount())

	// graceful shutdown
	stop := make(chan os.Signal, 2)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	logger.Info("shutting_down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// fechar websockets
	logger.Info("closing_gateway_connections")
	gatewayManager.CloseAll()
	logger.Info("gateway_connections_closed")

	// aguardar workers terminarem
	logger.Info("stopping_event_workers")
	eventProcessor.StopWorkers()
	logger.Info("event_workers_stopped")

	// usar shutdownCtx para evitar erro de variável não usada
	_ = shutdownCtx

	// fechar conexões redis
	if err := redisClient.Close(); err != nil {
		logger.Warn("redis_close_error", "error", err)
	} else {
		logger.Info("redis_closed")
	}

	// fechar conexão db
	dbConn.Close()
	logger.Info("db_closed")

	logger.Info("worker_stopped")
}
