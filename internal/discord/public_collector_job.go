package discord

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"identity-archive/internal/db"
	"identity-archive/internal/external"
	"identity-archive/internal/redis"
)

type PublicCollectorJob struct {
	db            *db.DB
	redis         *redis.Client
	sources       *external.SourceManager
	userFetcher   *UserFetcher
	tokenManager  *TokenManager
	publicScraper *PublicScraper
	logger        *slog.Logger
	stopChan      chan bool
}

func NewPublicCollectorJob(logger *slog.Logger, dbConn *db.DB, redisClient *redis.Client, sources *external.SourceManager, userFetcher *UserFetcher, tokenManager *TokenManager, publicScraper *PublicScraper) *PublicCollectorJob {
	return &PublicCollectorJob{
		db:            dbConn,
		redis:         redisClient,
		sources:       sources,
		userFetcher:   userFetcher,
		tokenManager:  tokenManager,
		publicScraper: publicScraper,
		logger:        logger,
		stopChan:      make(chan bool, 1),
	}
}

func (pcj *PublicCollectorJob) Start() {
	pcj.logger.Info("public_collector_job_started", "interval", "1_hour", "initial_delay", "5_minutes")

	ticker := time.NewTicker(1 * time.Hour) // roda a cada 1 hora
	defer ticker.Stop()

	// executar primeira vez apos 5 minutos
	go func() {
		pcj.logger.Info("waiting_for_initial_collection", "delay", "5_minutes")
		time.Sleep(5 * time.Minute)
		pcj.logger.Info("starting_initial_collection")
		pcj.runCollection(context.Background())
	}()

	for {
		select {
		case <-ticker.C:
			pcj.logger.Info("scheduled_collection_starting")
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			pcj.runCollection(ctx)
			cancel()
		case <-pcj.stopChan:
			pcj.logger.Info("public_collector_job_stopped")
			return
		}
	}
}

func (pcj *PublicCollectorJob) Stop() {
	select {
	case pcj.stopChan <- true:
	default:
	}
}

func (pcj *PublicCollectorJob) runCollection(ctx context.Context) {
	pcj.logger.Info("starting_public_collection")

	// Contar total de usuarios que precisam atualizacao
	var totalUsers int
	err := pcj.db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM users 
		 WHERE last_public_fetch IS NULL 
			OR last_public_fetch < NOW() - INTERVAL '24 hours'`,
	).Scan(&totalUsers)
	if err != nil {
		pcj.logger.Warn("failed_to_count_users", "error", err)
	}

	pcj.logger.Info("users_pending_update", "total", totalUsers)

	// Processar em lotes de 500 usuarios
	batchSize := 500
	totalProcessed := 0
	batchNum := 0

	for {
		// verificar se contexto foi cancelado
		select {
		case <-ctx.Done():
			pcj.logger.Warn("collection_cancelled", "processed", totalProcessed)
			return
		default:
		}

		batchNum++
		// buscar proximo lote de usuarios que precisam atualizacao
		rows, err := pcj.db.Pool.Query(ctx,
			`SELECT id FROM users 
			 WHERE last_public_fetch IS NULL 
				OR last_public_fetch < NOW() - INTERVAL '24 hours'
			 ORDER BY COALESCE(last_public_fetch, '1970-01-01'::timestamptz) ASC
			 LIMIT $1`,
			batchSize,
		)
		if err != nil {
			pcj.logger.Warn("failed_to_query_users_for_update", "error", err)
			return
		}

		userIDs := make([]string, 0)
		for rows.Next() {
			var userID string
			if err := rows.Scan(&userID); err != nil {
				continue
			}
			userIDs = append(userIDs, userID)
		}
		rows.Close()

		if len(userIDs) == 0 {
			break // nao ha mais usuarios para processar
		}

		pcj.logger.Info("processing_batch",
			"batch", batchNum,
			"users_in_batch", len(userIDs),
			"total_processed", totalProcessed,
			"remaining", totalUsers-totalProcessed,
		)

		// processar este lote
		pcj.processBatch(ctx, userIDs)
		totalProcessed += len(userIDs)

		// se o lote veio menor que o tamanho maximo, acabou
		if len(userIDs) < batchSize {
			break
		}

		// pequena pausa entre lotes
		time.Sleep(1 * time.Second)
	}

	pcj.logger.Info("public_collection_completed",
		"users_updated", totalProcessed,
		"batches", batchNum,
	)
}

// processBatch processa um lote de usuarios usando WORKERS PARALELOS (um por token)
func (pcj *PublicCollectorJob) processBatch(ctx context.Context, userIDs []string) {
	if pcj.tokenManager == nil || len(userIDs) == 0 {
		return
	}

	// pegar todos os tokens disponiveis
	tokens := pcj.tokenManager.GetAllActiveTokens()
	numWorkers := len(tokens)
	if numWorkers == 0 {
		pcj.logger.Warn("no_tokens_available_for_parallel_processing")
		// fallback: processar sequencialmente
		pcj.processBatchSequential(ctx, userIDs)
		return
	}

	// limitar workers ao numero de usuarios se necessario
	if numWorkers > len(userIDs) {
		numWorkers = len(userIDs)
	}

	// IMPORTANTE: limitar a no maximo 3 workers para evitar rate limit
	// Discord rate limit eh global por IP, nao por token
	if numWorkers > 3 {
		numWorkers = 3
	}

	pcj.logger.Info("starting_parallel_batch",
		"users", len(userIDs),
		"workers", numWorkers,
		"tokens_available", len(tokens),
	)

	// canal para distribuir usuarios entre workers
	userChan := make(chan string, len(userIDs))
	for _, userID := range userIDs {
		userChan <- userID
	}
	close(userChan)

	// contadores atomicos
	var successCount int64
	var failCount int64

	// WaitGroup para esperar todos os workers
	var wg sync.WaitGroup

	// iniciar workers (um por token)
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int, token *TokenEntry) {
			defer wg.Done()

			workerSuccess := 0
			workerFail := 0

			for userID := range userChan {
				// verificar se contexto foi cancelado
				select {
				case <-ctx.Done():
					return
				default:
				}

				// tentar buscar usuario com este token especifico
				updated := pcj.fetchUserWithToken(ctx, userID, token)
				if updated {
					atomic.AddInt64(&successCount, 1)
					workerSuccess++
				} else {
					atomic.AddInt64(&failCount, 1)
					workerFail++
				}

				// rate limiting por worker: 500ms entre requests (2 req/s por token)
				// com 3 workers = 6 req/s total, bem abaixo do limite do Discord
				time.Sleep(500 * time.Millisecond)
			}

			pcj.logger.Debug("worker_finished",
				"worker_id", workerID,
				"token_id", token.ID,
				"success", workerSuccess,
				"failed", workerFail,
			)
		}(i, tokens[i])
	}

	// esperar todos os workers terminarem
	wg.Wait()

	pcj.logger.Info("parallel_batch_completed",
		"success", successCount,
		"failed", failCount,
		"workers_used", numWorkers,
	)
}

// fetchUserWithToken busca um usuario usando um token especifico
func (pcj *PublicCollectorJob) fetchUserWithToken(ctx context.Context, userID string, token *TokenEntry) bool {
	// primeiro tentar fontes externas (nao usa token)
	if pcj.sources != nil {
		data, err := pcj.sources.FetchUser(ctx, userID)
		if err == nil && data != nil {
			discordUser := &DiscordUser{
				ID:            data.UserID,
				Username:      data.Username,
				Discriminator: data.Discriminator,
				GlobalName:    data.GlobalName,
				Avatar:        data.Avatar,
				Banner:        data.Banner,
				Bio:           data.Bio,
			}
			if saveErr := pcj.userFetcher.SaveUserToDatabase(ctx, discordUser, data.Source); saveErr == nil {
				pcj.logger.Info("user_found_in_source", "user_id", userID, "source", data.Source, "username", data.Username)
				return true
			}
		}
	}

	// se nao achou em fontes externas, buscar com o token especifico
	if pcj.userFetcher != nil {
		discordUser, err := pcj.userFetcher.fetchWithUserToken(ctx, userID, token)
		if err == nil && discordUser != nil {
			if saveErr := pcj.userFetcher.SaveUserToDatabase(ctx, discordUser, "discord_user_token"); saveErr == nil {
				return true
			}
		}
	}

	return false
}

// processBatchSequential eh o fallback quando nao ha tokens disponiveis
func (pcj *PublicCollectorJob) processBatchSequential(ctx context.Context, userIDs []string) {
	for i, userID := range userIDs {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if pcj.sources != nil && pcj.userFetcher != nil {
			data, err := pcj.sources.FetchUser(ctx, userID)
			if err == nil && data != nil {
				discordUser := &DiscordUser{
					ID:            data.UserID,
					Username:      data.Username,
					Discriminator: data.Discriminator,
					GlobalName:    data.GlobalName,
					Avatar:        data.Avatar,
					Banner:        data.Banner,
					Bio:           data.Bio,
				}
				pcj.userFetcher.SaveUserToDatabase(ctx, discordUser, data.Source)
			}
		}

		if i < len(userIDs)-1 {
			time.Sleep(50 * time.Millisecond)
		}
	}
}

// CollectNewUsers busca novos usuarios baseado em seeds (mensagens, membros, etc)
func (pcj *PublicCollectorJob) CollectNewUsers(ctx context.Context, userIDs []string) {
	if pcj.userFetcher == nil {
		return
	}

	for _, userID := range userIDs {
		// verificar se ja existe no banco
		var exists bool
		err := pcj.db.Pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`,
			userID,
		).Scan(&exists)

		if err != nil || exists {
			continue
		}

		// tentar fontes primeiro
		if pcj.sources != nil {
			data, err := pcj.sources.FetchUser(ctx, userID)
			if err == nil && data != nil {
				discordUser := &DiscordUser{
					ID:            data.UserID,
					Username:      data.Username,
					Discriminator: data.Discriminator,
					GlobalName:    data.GlobalName,
					Avatar:        data.Avatar,
					Banner:        data.Banner,
					Bio:           data.Bio,
				}
				if saveErr := pcj.userFetcher.SaveUserToDatabase(ctx, discordUser, data.Source); saveErr != nil {
					pcj.logger.Warn("failed_to_save_new_user", "user_id", userID, "source", data.Source, "error", saveErr)
				} else {
					pcj.logger.Info("new_user_collected", "user_id", userID, "source", data.Source)
				}
				// rate limiting
				time.Sleep(20 * time.Millisecond)
				continue
			}
		}

		// fallback: buscar usuario via discord api
		discordUser, err := pcj.userFetcher.FetchUserByID(ctx, userID)
		if err == nil && discordUser != nil {
			if saveErr := pcj.userFetcher.SaveUserToDatabase(ctx, discordUser, "discord_api"); saveErr != nil {
				pcj.logger.Warn("failed_to_save_new_user", "user_id", userID, "error", saveErr)
			} else {
				pcj.logger.Info("new_user_collected", "user_id", userID, "source", "discord_api")
			}
		}

		// rate limiting
		time.Sleep(20 * time.Millisecond)
	}
}

// CheckAvatarChanges verifica mudanças de avatar para usuarios conhecidos
func (pcj *PublicCollectorJob) CheckAvatarChanges(ctx context.Context) {
	if pcj.publicScraper == nil {
		return
	}

	// buscar usuarios com avatar conhecido
	rows, err := pcj.db.Pool.Query(ctx,
		`SELECT DISTINCT user_id, hash_avatar 
		 FROM avatar_history 
		 ORDER BY changed_at DESC 
		 LIMIT 1000`,
	)
	if err != nil {
		return
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var userID, avatarHash string
		if err := rows.Scan(&userID, &avatarHash); err != nil {
			continue
		}

		// verificar se avatar ainda existe
		exists, err := pcj.publicScraper.ScrapeAvatar(ctx, userID, avatarHash)
		if err == nil && !exists {
			pcj.logger.Debug("avatar_removed", "user_id", userID, "avatar_hash", avatarHash)
		}

		count++
		if count%100 == 0 {
			// rate limiting: aguardar um pouco a cada 100 verificacoes
			time.Sleep(1 * time.Second)
		}
	}
}
