package discord

import (
	"context"
	"log/slog"
	"time"

	"identity-archive/internal/db"
	"identity-archive/internal/redis"
)

type PublicCollectorJob struct {
	db            *db.DB
	redis         *redis.Client
	userFetcher   *UserFetcher
	publicScraper *PublicScraper
	logger        *slog.Logger
	stopChan      chan bool
}

func NewPublicCollectorJob(logger *slog.Logger, dbConn *db.DB, redisClient *redis.Client, userFetcher *UserFetcher, publicScraper *PublicScraper) *PublicCollectorJob {
	return &PublicCollectorJob{
		db:            dbConn,
		redis:         redisClient,
		userFetcher:   userFetcher,
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

	// buscar usuarios que precisam atualizacao (ultima busca publica > 24h atras ou nunca buscado)
	rows, err := pcj.db.Pool.Query(ctx,
		`SELECT id FROM users 
		 WHERE last_public_fetch IS NULL 
			OR last_public_fetch < NOW() - INTERVAL '24 hours'
		 ORDER BY COALESCE(last_public_fetch, '1970-01-01'::timestamptz) ASC
		 LIMIT 100`,
	)
	if err != nil {
		pcj.logger.Warn("failed_to_query_users_for_update", "error", err)
		return
	}
	defer rows.Close()

	userIDs := make([]string, 0)
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			continue
		}
		userIDs = append(userIDs, userID)
	}

	if len(userIDs) == 0 {
		pcj.logger.Info("no_users_need_update")
		return
	}

	pcj.logger.Info("updating_users", "count", len(userIDs))

	// atualizar cada usuario (com rate limiting)
	for i, userID := range userIDs {
		// verificar se contexto foi cancelado
		select {
		case <-ctx.Done():
			pcj.logger.Warn("collection_cancelled")
			return
		default:
		}

		if pcj.userFetcher != nil {
			// tentar buscar atualizacao via api
			discordUser, err := pcj.userFetcher.FetchUserByID(ctx, userID)
			if err == nil && discordUser != nil {
				// salvar atualizacao
				if saveErr := pcj.userFetcher.SaveUserToDatabase(ctx, discordUser, "discord_api"); saveErr != nil {
					pcj.logger.Warn("failed_to_save_user_update", "user_id", userID, "error", saveErr)
				} else {
					pcj.logger.Debug("user_updated_successfully", "user_id", userID)
				}
			} else {
				pcj.logger.Debug("user_not_found_in_api", "user_id", userID, "error", err)
			}
		} else {
			pcj.logger.Warn("user_fetcher_not_available_for_update", "user_id", userID)
		}

		// rate limiting: aguardar 20ms entre requisições (50 req/s)
		if i < len(userIDs)-1 {
			time.Sleep(20 * time.Millisecond)
		}
	}

	pcj.logger.Info("public_collection_completed", "users_updated", len(userIDs))
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

		// buscar usuario via api
		discordUser, err := pcj.userFetcher.FetchUserByID(ctx, userID)
		if err == nil && discordUser != nil {
			// salvar no banco
			if saveErr := pcj.userFetcher.SaveUserToDatabase(ctx, discordUser, "discord_api"); saveErr != nil {
				pcj.logger.Warn("failed_to_save_new_user", "user_id", userID, "error", saveErr)
			} else {
				pcj.logger.Info("new_user_collected", "user_id", userID)
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

