package processor

import (
	"context"
	"time"
)

func (ad *AltDetector) StartBackgroundJob() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	// Run immediately on start
	go ad.runDetectionCycle(context.Background())

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		ad.runDetectionCycle(ctx)
		cancel()
	}
}

func (ad *AltDetector) runDetectionCycle(ctx context.Context) {
	ad.logger.Info("alt_detection_cycle_started")

	// Get all active users in batches
	batchSize := 1000
	offset := 0

	for {
		rows, err := ad.db.Pool.Query(ctx,
			`SELECT id FROM users 
			 ORDER BY last_updated_at DESC NULLS LAST, id ASC
			 LIMIT $1 OFFSET $2`,
			batchSize, offset,
		)
		if err != nil {
			ad.logger.Warn("failed_to_fetch_users_batch", "error", err)
			break
		}

		userIDs := make([]string, 0, batchSize)
		for rows.Next() {
			var userID string
			if err := rows.Scan(&userID); err != nil {
				continue
			}
			userIDs = append(userIDs, userID)
		}
		rows.Close()

		if len(userIDs) == 0 {
			break
		}

		// Process each user
		for _, userID := range userIDs {
			select {
			case <-ctx.Done():
				ad.logger.Info("alt_detection_cycle_cancelled")
				return
			default:
			}

			relationships, err := ad.DetectAlts(ctx, userID)
			if err != nil {
				ad.logger.Warn("failed_to_detect_alts",
					"user_id", userID,
					"error", err,
				)
				continue
			}

			for _, rel := range relationships {
				if rel.ConfidenceScore >= 0.50 {
					if err := ad.SaveAltRelationship(ctx, rel); err != nil {
						ad.logger.Warn("failed_to_save_alt_relationship",
							"user_a", rel.UserA,
							"user_b", rel.UserB,
							"error", err,
						)
					}
				}
			}
		}

		offset += batchSize

		// Small delay between batches
		time.Sleep(100 * time.Millisecond)
	}

	// Remove relationships with low confidence
	_, _ = ad.db.Pool.Exec(ctx,
		`DELETE FROM alt_relationships 
		 WHERE confidence_score < 0.50 
		 AND detected_at < NOW() - INTERVAL '24 hours'`,
	)

	ad.logger.Info("alt_detection_cycle_completed")
}

