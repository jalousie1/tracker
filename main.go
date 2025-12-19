package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"identity-archive/internal/api"
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
	logger.Info("starting", "service", "identity-archive", "http_addr", cfg.HTTPAddr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbConn, err := db.New(ctx, cfg.DBDSN)
	if err != nil {
		logger.Error("db_connect_failed", "err", err.Error())
		os.Exit(1)
	}
	defer dbConn.Close()

	// connect to redis
	redisClient, err := redis.New(cfg.RedisDSN)
	if err != nil {
		logger.Error("redis_connect_failed", "error", err)
		os.Exit(1)
	}
	defer redisClient.Close()

	// initialize storage client
	storageClient := storage.NewR2Simulator(cfg.R2Bucket, cfg.R2Endpoint)
	eventProcessor := processor.NewEventProcessor(logger, dbConn, redisClient, storageClient)

	// iniciar workers para processar eventos
	eventProcessor.StartWorkers(5)

	// inicializar token manager (gerencia tokens criptografados)
	var tokenManager *discord.TokenManager
	var gatewayManager *discord.GatewayManager
	var userFetcher *discord.UserFetcher
	var publicScraper *discord.PublicScraper
	var publicCollectorJob *discord.PublicCollectorJob

	if len(cfg.EncryptionKey) == 32 {
		tokenManager, err = discord.NewTokenManager(logger, dbConn, redisClient, cfg.EncryptionKey)
		if err != nil {
			logger.Warn("token_manager_init_failed", "error", err)
		} else {
			// inicializar user fetcher para buscar usuarios via api
			userFetcher = discord.NewUserFetcher(logger, dbConn, redisClient, tokenManager, cfg.BotToken)
			logger.Info("user_fetcher_initialized", "can_fetch_users", tokenManager.GetActiveTokenCount() > 0, "has_bot_token", cfg.BotToken != "")

			// inicializar public scraper para coletar dados publicos
			publicScraper = discord.NewPublicScraper(logger, dbConn, redisClient, tokenManager, cfg.BotToken)
			logger.Info("public_scraper_initialized")

			// inicializar scraper para coletar dados de guilds
			scraper := discord.NewScraper(logger, dbConn, redisClient)

			// inicializar gateway manager para conectar ao discord
			gatewayManager = discord.NewGatewayManager(tokenManager, eventProcessor, scraper, logger, dbConn)

			// inicializar job de coleta publica
			publicCollectorJob = discord.NewPublicCollectorJob(logger, dbConn, redisClient, userFetcher, publicScraper)
			go publicCollectorJob.Start()
			logger.Info("public_collector_job_started")

			// conectar todos os tokens ativos ao gateway
			if tokenManager.GetActiveTokenCount() > 0 {
				go func() {
					connectCtx, connectCancel := context.WithTimeout(context.Background(), 60*time.Second)
					defer connectCancel()

					if err := gatewayManager.ConnectAllTokens(connectCtx); err != nil {
						logger.Warn("gateway_connect_failed", "error", err)
					} else {
						logger.Info("gateway_connections_established", "count", gatewayManager.GetActiveConnectionsCount())
					}
				}()
			}
		}
	} else {
		logger.Warn("encryption_key_not_configured", "msg", "token manager nao sera iniciado - busca on-demand de usuarios nao disponivel")
		logger.Warn("add_tokens_to_enable_features", "msg", "adicione tokens via /api/v1/admin/tokens para habilitar busca on-demand")
	}

	// initialize API server with managers
	srv := api.NewServerWithManagers(logger, dbConn, redisClient, eventProcessor, cfg, tokenManager, gatewayManager)

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http_listen_failed", "error", err)
			os.Exit(1)
		}
	}()

	logger.Info("api_started", "addr", cfg.HTTPAddr)

	// graceful shutdown
	stop := make(chan os.Signal, 2)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	logger.Info("shutting_down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// parar gateway connections
	if gatewayManager != nil {
		gatewayManager.CloseAll()
		logger.Info("gateway_connections_closed")
	}

	// parar public collector job
	if publicCollectorJob != nil {
		publicCollectorJob.Stop()
		logger.Info("public_collector_job_stopped")
	}

	// parar event workers
	eventProcessor.StopWorkers()
	logger.Info("event_workers_stopped")

	// parar aceitar novas requisicoes http
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("http_shutdown_failed", "error", err)
	} else {
		logger.Info("http_server_stopped")
	}

	// fechar conexoes redis
	if err := redisClient.Close(); err != nil {
		logger.Warn("redis_close_error", "error", err)
	} else {
		logger.Info("redis_closed")
	}

	// fechar conexao db
	dbConn.Close()
	logger.Info("db_closed")

	logger.Info("api_stopped")
}
