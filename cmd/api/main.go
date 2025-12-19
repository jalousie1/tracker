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
	logger.Info("starting_api", "service", "identity-archive-api", "http_addr", cfg.HTTPAddr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Connect to PostgreSQL
	dbConn, err := db.New(ctx, cfg.DBDSN)
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

	// Initialize storage client (simulator for API, no uploads needed)
	storageClient := storage.NewR2Simulator(cfg.R2Bucket, cfg.R2Endpoint)
	eventProcessor := processor.NewEventProcessor(logger, dbConn, redisClient, storageClient)

	// Initialize API server
	srv := api.NewServer(logger, dbConn, redisClient, eventProcessor, cfg)

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

	// parar aceitar novas requisições http
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("http_shutdown_failed", "error", err)
	} else {
		logger.Info("http_server_stopped")
	}

	// aguardar requests em andamento (já feito pelo Shutdown)
	// fechar conexões redis
	if err := redisClient.Close(); err != nil {
		logger.Warn("redis_close_error", "error", err)
	} else {
		logger.Info("redis_closed")
	}

	// fechar conexão db
	dbConn.Close()
	logger.Info("db_closed")

	logger.Info("api_stopped")
}

