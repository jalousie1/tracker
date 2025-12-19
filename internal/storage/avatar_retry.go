package storage

import (
	"context"
	"log/slog"
	"time"

	"identity-archive/internal/db"
	"identity-archive/internal/redis"
)

type AvatarRetryJob struct {
	db          *db.DB
	storage     StorageClient
	logger      *slog.Logger
	redis       *redis.Client
}

func NewAvatarRetryJob(logger *slog.Logger, dbConn *db.DB, storageClient StorageClient, redisClient *redis.Client) *AvatarRetryJob {
	return &AvatarRetryJob{
		db:      dbConn,
		storage: storageClient,
		logger:  logger,
		redis:   redisClient,
	}
}

func (aj *AvatarRetryJob) Start() {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()

	// Run immediately on start
	go aj.runRetryCycle(context.Background())

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Hour)
		aj.runRetryCycle(ctx)
		cancel()
	}
}

func (aj *AvatarRetryJob) runRetryCycle(ctx context.Context) {
	aj.logger.Info("avatar_retry_cycle_started")

	rows, err := aj.db.Pool.Query(ctx,
		`SELECT user_id, hash_avatar 
		 FROM avatar_history 
		 WHERE url_cdn IS NULL 
		 AND hash_avatar IS NOT NULL 
		 AND hash_avatar != ''
		 LIMIT 100`,
	)
	if err != nil {
		aj.logger.Warn("failed_to_fetch_avatars", "error", err)
		return
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var userID, avatarHash string
		if err := rows.Scan(&userID, &avatarHash); err != nil {
			continue
		}

		select {
		case <-ctx.Done():
			return
		default:
		}

		// Try to download and upload
		if s3Client, ok := aj.storage.(*S3Client); ok {
			url, err := s3Client.UploadAvatarFromDiscord(userID, avatarHash)
			if err != nil {
				aj.logger.Warn("avatar_retry_failed",
					"user_id", userID,
					"avatar_hash", avatarHash,
					"error", err,
				)
				continue
			}

			// Update database
			_, err = aj.db.Pool.Exec(ctx,
				`UPDATE avatar_history 
				 SET url_cdn = $1 
				 WHERE user_id = $2 AND hash_avatar = $3`,
				url, userID, avatarHash,
			)
			if err != nil {
				aj.logger.Warn("failed_to_update_avatar_url",
					"user_id", userID,
					"error", err,
				)
				continue
			}

			count++
			aj.logger.Info("avatar_retry_success",
				"user_id", userID,
				"avatar_hash", avatarHash,
			)

			// Rate limiting: wait 1 second between uploads
			time.Sleep(1 * time.Second)
		}
	}

	aj.logger.Info("avatar_retry_cycle_completed", "processed", count)
}

