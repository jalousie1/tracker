package discord

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"sync"
	"time"

	"identity-archive/internal/db"
	"identity-archive/internal/logging"
	"identity-archive/internal/redis"
	"identity-archive/internal/security"
)

type TokenStatus string

const (
	TokenActive    TokenStatus = "ativo"
	TokenSuspended TokenStatus = "suspenso"
	TokenBanned    TokenStatus = "banido"
)

type TokenEntry struct {
	ID             int64
	EncryptedValue string
	DecryptedValue string // apenas em memória, NUNCA persistir
	UserID         string
	Status         TokenStatus
	FailureCount   int
	LastUsed       time.Time
	SuspendedUntil *time.Time
}

type TokenManager struct {
	db            *db.DB
	redis         *redis.Client
	activeTokens  []TokenEntry
	mutex         sync.RWMutex
	encryptionKey []byte
	currentIndex  int
	logger        *slog.Logger
}

func NewTokenManager(logger *slog.Logger, dbConn *db.DB, redisClient *redis.Client, encryptionKey []byte) (*TokenManager, error) {
	if len(encryptionKey) != 32 {
		return nil, errors.New("encryption key must be 32 bytes")
	}

	tm := &TokenManager{
		db:            dbConn,
		redis:         redisClient,
		activeTokens:  make([]TokenEntry, 0),
		encryptionKey: encryptionKey,
		logger:        logger,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := tm.loadActiveTokens(ctx); err != nil {
		return nil, fmt.Errorf("failed to load tokens: %w", err)
	}

	tm.logger.Info("token_manager_initialized", "active_tokens", len(tm.activeTokens))

	// Start background reactivation job
	go tm.StartReactivationJob()

	return tm, nil
}

func (tm *TokenManager) loadActiveTokens(ctx context.Context) error {
	rows, err := tm.db.Pool.Query(ctx,
		`SELECT id, token_encrypted, user_id, status, failure_count, last_used, suspended_until
		 FROM tokens
		 WHERE status = $1`,
		string(TokenActive),
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	tm.activeTokens = make([]TokenEntry, 0)

	for rows.Next() {
		var entry TokenEntry
		var encryptedValue string
		var lastUsed, suspendedUntil *time.Time

		if err := rows.Scan(
			&entry.ID,
			&encryptedValue,
			&entry.UserID,
			&entry.Status,
			&entry.FailureCount,
			&lastUsed,
			&suspendedUntil,
		); err != nil {
			tm.logger.Warn("failed_to_scan_token", "error", err)
			continue
		}

		// Decrypt token
		decrypted, err := security.DecryptToken(encryptedValue, tm.encryptionKey)
		if err != nil {
			tm.logger.Warn("failed_to_decrypt_token", "token_id", entry.ID, "error", err)
			continue
		}

		// Validate token format (basic check)
		if !tm.validateTokenFormat(decrypted) {
			tm.logger.Warn("invalid_token_format", "token_id", entry.ID)
			continue
		}

		// Health check
		if !tm.validateTokenHealth(ctx, decrypted) {
			tm.logger.Warn("token_failed_health_check", "token_id", entry.ID)
			// Mark as banned but don't add to pool
			_ = tm.markTokenAsBannedDB(ctx, entry.ID, "health_check_failed")
			continue
		}

		entry.EncryptedValue = encryptedValue
		entry.DecryptedValue = decrypted
		if lastUsed != nil {
			entry.LastUsed = *lastUsed
		}
		entry.SuspendedUntil = suspendedUntil

		masked := logging.MaskToken(decrypted)
		tm.logger.Info("token_loaded", "token_id", entry.ID, "token", masked, "user_id", entry.UserID)

		tm.activeTokens = append(tm.activeTokens, entry)
	}

	return nil
}

func (tm *TokenManager) validateTokenFormat(token string) bool {
	// Discord tokens are typically 70+ characters and contain dots
	matched, _ := regexp.MatchString(`^[A-Za-z0-9\.\-_]{70,}$`, token)
	return matched
}

func (tm *TokenManager) validateTokenHealth(ctx context.Context, token string) bool {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://discord.com/api/v10/users/@me", nil)
	if err != nil {
		return false
	}

	req.Header.Set("Authorization", token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return false
	}

	return resp.StatusCode == http.StatusOK
}

func (tm *TokenManager) GetNextAvailableToken() (*TokenEntry, error) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	if len(tm.activeTokens) == 0 {
		return nil, errors.New("no_active_tokens_available")
	}

	// Round-robin with fallback
	attempts := 0
	maxAttempts := len(tm.activeTokens) * 2

	for attempts < maxAttempts {
		if tm.currentIndex >= len(tm.activeTokens) {
			tm.currentIndex = 0
		}

		entry := &tm.activeTokens[tm.currentIndex]
		tm.currentIndex++

		// Check if token is suspended
		if entry.SuspendedUntil != nil && time.Now().Before(*entry.SuspendedUntil) {
			attempts++
			continue
		}

		// Update last used
		entry.LastUsed = time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, _ = tm.db.Pool.Exec(ctx,
			`UPDATE tokens SET last_used = NOW() WHERE id = $1`,
			entry.ID,
		)
		cancel()

		return entry, nil
	}

	// All tokens are suspended, wait and retry
	return nil, errors.New("all_tokens_suspended")
}

func (tm *TokenManager) MarkTokenAsSuspended(tokenID int64, reason string, cooldownMinutes int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	suspendedUntil := time.Now().Add(time.Duration(cooldownMinutes) * time.Minute)

	_, err := tm.db.Pool.Exec(ctx,
		`UPDATE tokens 
		 SET status = $1, suspended_until = $2, failure_count = failure_count + 1 
		 WHERE id = $3`,
		string(TokenSuspended),
		suspendedUntil,
		tokenID,
	)
	if err != nil {
		return err
	}

	// Log failure
	_, _ = tm.db.Pool.Exec(ctx,
		`INSERT INTO token_failures (token_id, reason) VALUES ($1, $2)`,
		tokenID,
		reason,
	)

	// Remove from active pool
	tm.mutex.Lock()
	for i, entry := range tm.activeTokens {
		if entry.ID == tokenID {
			tm.activeTokens = append(tm.activeTokens[:i], tm.activeTokens[i+1:]...)
			break
		}
	}
	tm.mutex.Unlock()

	masked := tm.getMaskedToken(tokenID)
	tm.logger.Warn("token_suspended",
		"token_id", tokenID,
		"token", masked,
		"reason", reason,
		"cooldown_minutes", cooldownMinutes,
	)

	return nil
}

func (tm *TokenManager) MarkTokenAsBanned(tokenID int64, reason string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return tm.markTokenAsBannedDB(ctx, tokenID, reason)
}

func (tm *TokenManager) markTokenAsBannedDB(ctx context.Context, tokenID int64, reason string) error {
	_, err := tm.db.Pool.Exec(ctx,
		`UPDATE tokens SET status = $1, banned_at = NOW() WHERE id = $2`,
		string(TokenBanned),
		tokenID,
	)
	if err != nil {
		return err
	}

	// Log failure
	_, _ = tm.db.Pool.Exec(ctx,
		`INSERT INTO token_failures (token_id, reason) VALUES ($1, $2)`,
		tokenID,
		reason,
	)

	// Remove from active pool permanently
	tm.mutex.Lock()
	for i, entry := range tm.activeTokens {
		if entry.ID == tokenID {
			tm.activeTokens = append(tm.activeTokens[:i], tm.activeTokens[i+1:]...)
			break
		}
	}
	tm.mutex.Unlock()

	masked := tm.getMaskedToken(tokenID)
	tm.logger.Error("token_banned",
		"token_id", tokenID,
		"token", masked,
		"reason", reason,
	)

	return nil
}

func (tm *TokenManager) getMaskedToken(tokenID int64) string {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()

	for _, entry := range tm.activeTokens {
		if entry.ID == tokenID {
			return logging.MaskToken(entry.DecryptedValue)
		}
	}

	return "token...UNKNOWN"
}

func (tm *TokenManager) AddToken(tokenString string, ownerUserID string) error {
	// Validate format
	if !tm.validateTokenFormat(tokenString) {
		return errors.New("invalid_token_format")
	}

	// Test token
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if !tm.validateTokenHealth(ctx, tokenString) {
		return errors.New("token_validation_failed")
	}

	// Encrypt token
	encrypted, err := security.EncryptToken(tokenString, tm.encryptionKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt token: %w", err)
	}

	// Insert into database
	var tokenID int64
	err = tm.db.Pool.QueryRow(ctx,
		`INSERT INTO tokens (token, token_encrypted, user_id, status, created_at)
		 VALUES ($1, $2, $3, $4, NOW())
		 RETURNING id`,
		encrypted,
		encrypted,
		ownerUserID,
		string(TokenActive),
	).Scan(&tokenID)
	if err != nil {
		return fmt.Errorf("failed to insert token: %w", err)
	}

	// Add to active pool
	entry := TokenEntry{
		ID:             tokenID,
		EncryptedValue: encrypted,
		DecryptedValue: tokenString,
		UserID:         ownerUserID,
		Status:         TokenActive,
		LastUsed:       time.Now(),
	}

	tm.mutex.Lock()
	tm.activeTokens = append(tm.activeTokens, entry)
	tm.mutex.Unlock()

	masked := logging.MaskToken(tokenString)
	tm.logger.Info("token_added", "token_id", tokenID, "token", masked, "user_id", ownerUserID)

	return nil
}

func (tm *TokenManager) StartReactivationJob() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

		rows, err := tm.db.Pool.Query(ctx,
			`SELECT id, token_encrypted, user_id, failure_count
			 FROM tokens
			 WHERE status = $1 AND suspended_until <= NOW()`,
			string(TokenSuspended),
		)

		if err != nil {
			tm.logger.Warn("reactivation_job_query_failed", "error", err)
			cancel()
			continue
		}

		for rows.Next() {
			var tokenID int64
			var encryptedValue, userID string
			var failureCount int

			if err := rows.Scan(&tokenID, &encryptedValue, &userID, &failureCount); err != nil {
				continue
			}

			// Decrypt and validate
			decrypted, err := security.DecryptToken(encryptedValue, tm.encryptionKey)
			if err != nil {
				tm.markTokenAsBannedDB(ctx, tokenID, "decryption_failed")
				continue
			}

			if !tm.validateTokenHealth(ctx, decrypted) {
				tm.markTokenAsBannedDB(ctx, tokenID, "reactivation_validation_failed")
				continue
			}

			// Reactivate
			_, err = tm.db.Pool.Exec(ctx,
				`UPDATE tokens SET status = $1, suspended_until = NULL WHERE id = $2`,
				string(TokenActive),
				tokenID,
			)
			if err != nil {
				continue
			}

			// Add back to pool
			entry := TokenEntry{
				ID:             tokenID,
				EncryptedValue: encryptedValue,
				DecryptedValue: decrypted,
				UserID:         userID,
				Status:         TokenActive,
				FailureCount:   failureCount,
				LastUsed:       time.Now(),
			}

			tm.mutex.Lock()
			tm.activeTokens = append(tm.activeTokens, entry)
			tm.mutex.Unlock()

			masked := logging.MaskToken(decrypted)
			tm.logger.Info("token_reactivated", "token_id", tokenID, "token", masked)
		}

		rows.Close()
		cancel()
	}
}

func (tm *TokenManager) GetActiveTokenCount() int {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()
	return len(tm.activeTokens)
}

func (tm *TokenManager) GetAllTokens(ctx context.Context) ([]TokenEntry, error) {
	rows, err := tm.db.Pool.Query(ctx,
		`SELECT id, token_encrypted, user_id, status, failure_count, last_used, suspended_until
		 FROM tokens
		 ORDER BY id DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := make([]TokenEntry, 0)
	for rows.Next() {
		var entry TokenEntry
		var encryptedValue string
		var lastUsed, suspendedUntil *time.Time

		if err := rows.Scan(
			&entry.ID,
			&encryptedValue,
			&entry.UserID,
			&entry.Status,
			&entry.FailureCount,
			&lastUsed,
			&suspendedUntil,
		); err != nil {
			continue
		}

		// Decrypt to mask
		decrypted, err := security.DecryptToken(encryptedValue, tm.encryptionKey)
		if err == nil {
			entry.DecryptedValue = logging.MaskToken(decrypted)
		} else {
			entry.DecryptedValue = "token...ERROR"
		}

		entry.EncryptedValue = "" // Don't return encrypted value
		if lastUsed != nil {
			entry.LastUsed = *lastUsed
		}
		entry.SuspendedUntil = suspendedUntil

		tokens = append(tokens, entry)
	}

	return tokens, nil
}

func (tm *TokenManager) RemoveToken(ctx context.Context, tokenID int64) error {
	// Remove from database
	_, err := tm.db.Pool.Exec(ctx, "DELETE FROM tokens WHERE id = $1", tokenID)
	if err != nil {
		return err
	}

	// Remove from active pool
	tm.mutex.Lock()
	defer tm.mutex.Unlock()
	
	for i, entry := range tm.activeTokens {
		if entry.ID == tokenID {
			tm.activeTokens = append(tm.activeTokens[:i], tm.activeTokens[i+1:]...)
			break
		}
	}

	return nil
}

// GetTokenByID retorna um token especifico pelo ID
func (tm *TokenManager) GetTokenByID(tokenID int64) (*TokenEntry, error) {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()

	for i, entry := range tm.activeTokens {
		if entry.ID == tokenID {
			// atualizar last_used
			tm.activeTokens[i].LastUsed = time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, _ = tm.db.Pool.Exec(ctx,
				`UPDATE tokens SET last_used = NOW() WHERE id = $1`,
				entry.ID,
			)
			cancel()
			return &tm.activeTokens[i], nil
		}
	}

	return nil, errors.New("token_not_found_or_inactive")
}

