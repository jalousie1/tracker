package processor

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"

	"identity-archive/internal/db"
)

type AltDetector struct {
	db     *db.DB
	logger *slog.Logger
}

func NewAltDetector(logger *slog.Logger, dbConn *db.DB) *AltDetector {
	return &AltDetector{
		db:     dbConn,
		logger: logger,
	}
}

func (ad *AltDetector) DetectAlts(ctx context.Context, userID string) ([]AltRelationship, error) {
	// Find users sharing external_ids
	rows, err := ad.db.Pool.Query(ctx,
		`SELECT 
			c1.user_id AS user_a, 
			c2.user_id AS user_b,
			c1.type AS connection_type,
			c1.external_id AS shared_id
		FROM connected_accounts c1
		JOIN connected_accounts c2 
			ON c1.external_id = c2.external_id 
			AND c1.type = c2.type 
			AND c1.user_id < c2.user_id
		WHERE (c1.user_id = $1 OR c2.user_id = $1)
			AND c1.external_id IS NOT NULL
			AND c1.external_id != ''
		GROUP BY c1.user_id, c2.user_id, c1.type, c1.external_id
		ORDER BY COUNT(*) DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	relationships := make(map[string]*AltRelationship)

	for rows.Next() {
		var userA, userB, connType, sharedID string
		if err := rows.Scan(&userA, &userB, &connType, &sharedID); err != nil {
			continue
		}

		key := fmt.Sprintf("%s:%s", userA, userB)
		if userA > userB {
			key = fmt.Sprintf("%s:%s", userB, userA)
		}

		rel, exists := relationships[key]
		if !exists {
			rel = &AltRelationship{
				UserA: userA,
				UserB: userB,
			}
			relationships[key] = rel
		}

		rel.SharedAccounts = append(rel.SharedAccounts, SharedAccount{
			Type:       connType,
			ExternalID: sharedID,
		})
	}

	result := make([]AltRelationship, 0, len(relationships))
	for _, rel := range relationships {
		rel.ConfidenceScore = ad.CalculateConfidenceScore(rel.SharedAccounts)
		rel.DetectionMethod = ad.buildDetectionMethod(rel.SharedAccounts)

		// Detect behavior patterns
		behaviorBonus := ad.DetectBehaviorPatterns(ctx, rel.UserA, rel.UserB)
		rel.ConfidenceScore = math.Min(1.0, rel.ConfidenceScore+behaviorBonus)

		result = append(result, *rel)
	}

	return result, nil
}

func (ad *AltDetector) CalculateConfidenceScore(sharedAccounts []SharedAccount) float64 {
	if len(sharedAccounts) == 0 {
		return 0.0
	}

	score := 0.0
	hasSpotify := false
	hasSteam := false

	for _, acc := range sharedAccounts {
		switch acc.Type {
		case "spotify":
			hasSpotify = true
			score += 0.70
		case "steam":
			hasSteam = true
			score += 0.85
		case "twitter", "youtube":
			score += 0.60
		case "twitch", "reddit":
			score += 0.50
		case "xbox", "playstation":
			score += 0.55
		default:
			score += 0.40
		}
	}

	// Bonus for multiple high-confidence connections
	if hasSpotify && hasSteam {
		score = 0.95 // Very high confidence
	} else if hasSpotify {
		score = math.Max(score, 0.70)
	} else if hasSteam {
		score = math.Max(score, 0.85)
	}

	// Average if multiple connections
	if len(sharedAccounts) > 1 {
		score = score / float64(len(sharedAccounts))
		score = math.Min(1.0, score*1.2) // Slight bonus for multiple connections
	}

	return math.Min(1.0, score)
}

func (ad *AltDetector) DetectBehaviorPatterns(ctx context.Context, userA, userB string) float64 {
	bonus := 0.0

	// Check for similar username change timestamps
	var count int
	err := ad.db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM (
			SELECT DATE_TRUNC('minute', changed_at) as minute
			FROM username_history
			WHERE user_id = $1
		) u1
		INNER JOIN (
			SELECT DATE_TRUNC('minute', changed_at) as minute
			FROM username_history
			WHERE user_id = $2
		) u2 ON u1.minute = u2.minute`,
		userA, userB,
	).Scan(&count)

	if err == nil && count > 0 {
		bonus += 0.10
	}

	// Check username similarity using Levenshtein distance
	var usernameA, usernameB string
	_ = ad.db.Pool.QueryRow(ctx,
		`SELECT username FROM username_history 
		 WHERE user_id = $1 ORDER BY changed_at DESC LIMIT 1`,
		userA,
	).Scan(&usernameA)

	_ = ad.db.Pool.QueryRow(ctx,
		`SELECT username FROM username_history 
		 WHERE user_id = $2 ORDER BY changed_at DESC LIMIT 1`,
		userB,
	).Scan(&usernameB)

	if usernameA != "" && usernameB != "" {
		similarity := ad.calculateSimilarity(usernameA, usernameB)
		if similarity >= 0.80 {
			bonus += 0.15
		}
	}

	return bonus
}

func (ad *AltDetector) calculateSimilarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}

	s1 = strings.ToLower(s1)
	s2 = strings.ToLower(s2)

	maxLen := math.Max(float64(len(s1)), float64(len(s2)))
	if maxLen == 0 {
		return 0.0
	}

	distance := ad.levenshteinDistance(s1, s2)
	return 1.0 - (float64(distance) / maxLen)
}

func (ad *AltDetector) levenshteinDistance(s1, s2 string) int {
	r1, r2 := []rune(s1), []rune(s2)
	column := make([]int, len(r1)+1)

	for y := 1; y <= len(r1); y++ {
		column[y] = y
	}

	for x := 1; x <= len(r2); x++ {
		column[0] = x
		lastDiag := x - 1
		for y := 1; y <= len(r1); y++ {
			oldDiag := column[y]
			cost := 0
			if r1[y-1] != r2[x-1] {
				cost = 1
			}
			column[y] = min(column[y]+1, column[y-1]+1, lastDiag+cost)
			lastDiag = oldDiag
		}
	}

	return column[len(r1)]
}

func min(a, b, c int) int {
	if a < b && a < c {
		return a
	}
	if b < c {
		return b
	}
	return c
}

func (ad *AltDetector) buildDetectionMethod(accounts []SharedAccount) string {
	types := make([]string, 0, len(accounts))
	for _, acc := range accounts {
		types = append(types, acc.Type)
	}
	return fmt.Sprintf("shared_%s", strings.Join(types, "_and_"))
}

func (ad *AltDetector) SaveAltRelationship(ctx context.Context, rel AltRelationship) error {
	// Ensure user_a < user_b for consistency
	userA, userB := rel.UserA, rel.UserB
	if userA > userB {
		userA, userB = userB, userA
	}

	_, err := ad.db.Pool.Exec(ctx,
		`INSERT INTO alt_relationships (user_a, user_b, confidence_score, detection_method, detected_at)
		 VALUES ($1, $2, $3, $4, NOW())
		 ON CONFLICT (user_a, user_b) 
		 DO UPDATE SET 
			confidence_score = EXCLUDED.confidence_score,
			detection_method = EXCLUDED.detection_method,
			detected_at = EXCLUDED.detected_at`,
		userA, userB, rel.ConfidenceScore, rel.DetectionMethod,
	)

	return err
}

type AltRelationship struct {
	UserA          string
	UserB          string
	ConfidenceScore float64
	DetectionMethod string
	SharedAccounts  []SharedAccount
}

type SharedAccount struct {
	Type       string
	ExternalID string
}

