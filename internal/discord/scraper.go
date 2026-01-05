package discord

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"identity-archive/internal/db"
	"identity-archive/internal/redis"
)

type Scraper struct {
	db         *db.DB
	redis      *redis.Client
	logger     *slog.Logger
	queryDelay time.Duration
	// cache de membros ja processados por guild (para evitar duplicatas no scraping alfabetico)
	processedMembers map[string]map[string]bool // guild_id -> user_id -> true
	membersMutex     sync.RWMutex
}

type ScraperOptions struct {
	QueryDelay time.Duration
}

func NewScraper(logger *slog.Logger, dbConn *db.DB, redisClient *redis.Client) *Scraper {
	return NewScraperWithOptions(logger, dbConn, redisClient, ScraperOptions{QueryDelay: 250 * time.Millisecond})
}

func NewScraperWithOptions(logger *slog.Logger, dbConn *db.DB, redisClient *redis.Client, opts ScraperOptions) *Scraper {
	qd := opts.QueryDelay
	if qd <= 0 {
		qd = 250 * time.Millisecond
	}
	return &Scraper{
		db:               dbConn,
		redis:            redisClient,
		logger:           logger,
		queryDelay:       qd,
		processedMembers: make(map[string]map[string]bool),
	}
}

func (s *Scraper) ScrapeGuildMembers(ctx context.Context, guildID string, conn *GatewayConnection) error {
	// pegar nome do guild se possivel
	guildName := s.getGuildNameFromDB(ctx, guildID)

	s.logger.Info("starting_guild_scrape",
		"guild_id", guildID,
		"guild_name", guildName,
		"token_id", conn.TokenID,
		"method", "alphabetic_scraping",
	)

	// Save guild info
	_, _ = s.db.Pool.Exec(ctx,
		`INSERT INTO guilds (guild_id, name, member_count, discovered_at)
		 VALUES ($1, $2, $3, NOW())
		 ON CONFLICT (guild_id) DO NOTHING`,
		guildID,
		"", // name will be updated from READY event
		0,  // member_count will be updated
	)

	// ESTRATEGIA PARA USER TOKENS: Scraping Alfabetico
	// Fazemos multiplas requests com queries diferentes para coletar todos os membros
	// Isso simula o comportamento de busca na lista de membros do Discord

	// Queries para cobrir todos os membros:
	// 1. A-Z (letras)
	// 2. 0-9 (numeros)
	// 3. Caracteres especiais comuns
	queries := []string{
		"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m",
		"n", "o", "p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z",
		"0", "1", "2", "3", "4", "5", "6", "7", "8", "9",
		"_", "-", ".", // caracteres especiais comuns em usernames
	}

	s.logger.Info("starting_alphabetic_scrape",
		"guild_id", guildID,
		"guild_name", guildName,
		"total_queries", len(queries),
		"token_id", conn.TokenID,
	)

	// Gerar nonce unico para esta sessao de scraping
	// Isso permite rastrear todos os chunks desta sessao como uma unica coleta
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		// fallback para timestamp se nao conseguir gerar random
		nonceBytes = []byte(fmt.Sprintf("%d-%d", conn.TokenID, time.Now().UnixNano()))
	}
	scrapeNonce := hex.EncodeToString(nonceBytes)

	s.logger.Debug("scrape_session_nonce",
		"guild_id", guildID,
		"nonce", scrapeNonce,
		"token_id", conn.TokenID,
	)

	// Fazer requests com delay para evitar rate limit
	for i, query := range queries {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := conn.SendRequestGuildMembersWithQueryAndNonce(guildID, query, 100, scrapeNonce); err != nil {
			s.logger.Warn("failed_to_send_query",
				"guild_id", guildID,
				"query", query,
				"error", err,
			)
			// Se a conexao estiver fechada ou rate-limited, continuar tentando só gera spam e piora o problema.
			return err
		}

		s.logger.Debug("query_sent",
			"guild_id", guildID,
			"query", query,
			"progress", fmt.Sprintf("%d/%d", i+1, len(queries)),
		)

		// Delay entre requests para evitar rate limit
		time.Sleep(s.queryDelay)
	}

	s.logger.Info("alphabetic_scrape_completed",
		"guild_id", guildID,
		"guild_name", guildName,
		"queries_sent", len(queries),
		"token_id", conn.TokenID,
	)

	return nil
}

// ProcessGuildMembersChunk processa membros de um chunk (sem token_id - compatibilidade)
func (s *Scraper) ProcessGuildMembersChunk(ctx context.Context, guildID string, members []map[string]interface{}) error {
	return s.ProcessGuildMembersChunkWithToken(ctx, guildID, members, 0)
}

// ProcessGuildMembersChunkWithToken processa membros salvando a relacao com o token
func (s *Scraper) ProcessGuildMembersChunkWithToken(ctx context.Context, guildID string, members []map[string]interface{}, tokenID int64) error {
	if len(members) == 0 {
		return nil
	}

	// Filtrar membros duplicados (importante para scraping alfabetico)
	s.membersMutex.Lock()
	if s.processedMembers[guildID] == nil {
		s.processedMembers[guildID] = make(map[string]bool)
	}

	uniqueMembers := make([]map[string]interface{}, 0, len(members))
	duplicateCount := 0

	for _, member := range members {
		userData, ok := member["user"].(map[string]interface{})
		if !ok {
			continue
		}

		userID, _ := userData["id"].(string)
		if userID == "" {
			continue
		}

		// verificar se ja processamos este membro
		if s.processedMembers[guildID][userID] {
			duplicateCount++
			continue
		}

		// marcar como processado
		s.processedMembers[guildID][userID] = true
		uniqueMembers = append(uniqueMembers, member)
	}
	s.membersMutex.Unlock()

	if duplicateCount > 0 {
		s.logger.Debug("duplicates_filtered",
			"guild_id", guildID,
			"total_received", len(members),
			"duplicates", duplicateCount,
			"unique", len(uniqueMembers),
		)
	}

	if len(uniqueMembers) == 0 {
		return nil
	}

	// Process members in batch
	batchSize := 100
	for i := 0; i < len(uniqueMembers); i += batchSize {
		end := i + batchSize
		if end > len(uniqueMembers) {
			end = len(uniqueMembers)
		}

		batch := uniqueMembers[i:end]
		if err := s.processMemberBatchWithToken(ctx, guildID, batch, tokenID); err != nil {
			guildName := s.getGuildNameFromDB(ctx, guildID)
			s.logger.Warn("failed_to_process_member_batch",
				"guild_id", guildID,
				"guild_name", guildName,
				"batch_start", i,
				"batch_end", end,
				"error", err,
			)
		}

		// Rate limiting: wait 100ms between batches
		if i+batchSize < len(uniqueMembers) {
			time.Sleep(100 * time.Millisecond)
		}
	}

	return nil
}

// processMemberBatch mantido para compatibilidade
func (s *Scraper) processMemberBatch(ctx context.Context, members []map[string]interface{}) error {
	return s.processMemberBatchWithToken(ctx, "", members, 0)
}

func (s *Scraper) processMemberBatchWithToken(ctx context.Context, guildID string, members []map[string]interface{}, tokenID int64) error {
	// Prepare batch inserts
	userIDs := make([]string, 0, len(members))
	usernameInserts := make([]interface{}, 0)
	avatarInserts := make([]interface{}, 0)
	bioInserts := make([]interface{}, 0)
	accountInserts := make([]interface{}, 0)

	for _, member := range members {
		userData, ok := member["user"].(map[string]interface{})
		if !ok {
			continue
		}

		userID, _ := userData["id"].(string)
		if userID == "" {
			continue
		}

		userIDs = append(userIDs, userID)

		// Extract username fields
		var username, discriminator, globalName *string
		if v, ok := userData["username"].(string); ok && v != "" {
			username = &v
		}
		if v, ok := userData["discriminator"].(string); ok && v != "" {
			discriminator = &v
		}
		if v, ok := userData["global_name"].(string); ok && v != "" {
			globalName = &v
		}

		if username != nil || globalName != nil || discriminator != nil {
			usernameInserts = append(usernameInserts, userID, username, discriminator, globalName)
		}

		// Extract avatar
		if avatarHash, ok := userData["avatar"].(string); ok && avatarHash != "" {
			avatarInserts = append(avatarInserts, userID, avatarHash)
		}

		// Extract bio
		if bio, ok := userData["bio"].(string); ok && bio != "" {
			bioInserts = append(bioInserts, userID, bio)
		}

		// Extract connected accounts
		if accounts, ok := userData["connected_accounts"].([]interface{}); ok {
			for _, acc := range accounts {
				if accMap, ok := acc.(map[string]interface{}); ok {
					accType, _ := accMap["type"].(string)
					externalID, _ := accMap["id"].(string)
					name, _ := accMap["name"].(string)
					if accType != "" {
						accountInserts = append(accountInserts, userID, accType, externalID, name)
					}
				}
			}
		}
	}

	// Insert users
	if len(userIDs) > 0 {
		_, _ = s.db.Pool.Exec(ctx,
			`INSERT INTO users (id) 
			 SELECT unnest($1::text[])
			 ON CONFLICT (id) DO NOTHING`,
			userIDs,
		)

		// salvar relacao guild_members se temos guild e token validos
		if guildID != "" && tokenID > 0 {
			for _, userID := range userIDs {
				_, _ = s.db.Pool.Exec(ctx,
					`INSERT INTO guild_members (guild_id, user_id, token_id, discovered_at, last_seen_at)
					 VALUES ($1, $2, $3, NOW(), NOW())
					 ON CONFLICT (guild_id, user_id, token_id) DO UPDATE SET last_seen_at = NOW()`,
					guildID, userID, tokenID,
				)
			}
		}
	}

	// Insert username history (with deduplication check)
	if len(usernameInserts) > 0 {
		// Process in smaller batches to check for duplicates
		for i := 0; i < len(usernameInserts); i += 4 {
			if i+3 < len(usernameInserts) {
				userID := usernameInserts[i].(string)
				username := usernameInserts[i+1].(*string)
				discriminator := usernameInserts[i+2].(*string)
				globalName := usernameInserts[i+3].(*string)

				var exists bool
				_ = s.db.Pool.QueryRow(ctx,
					`SELECT EXISTS(
						SELECT 1 FROM username_history 
						WHERE user_id = $1 AND username IS NOT DISTINCT FROM $2 
						AND discriminator IS NOT DISTINCT FROM $3 
						AND global_name IS NOT DISTINCT FROM $4
						LIMIT 1
					)`,
					userID, username, discriminator, globalName,
				).Scan(&exists)

				if !exists {
					_, _ = s.db.Pool.Exec(ctx,
						`INSERT INTO username_history (user_id, username, discriminator, global_name, changed_at)
						 VALUES ($1, $2, $3, $4, NOW())`,
						userID, username, discriminator, globalName,
					)
				}
			}
		}
	}

	// Insert avatar history (with deduplication)
	if len(avatarInserts) > 0 {
		for i := 0; i < len(avatarInserts); i += 2 {
			if i+1 < len(avatarInserts) {
				userID := avatarInserts[i].(string)
				avatarHash := avatarInserts[i+1].(string)

				var exists bool
				_ = s.db.Pool.QueryRow(ctx,
					`SELECT EXISTS(
						SELECT 1 FROM avatar_history 
						WHERE user_id = $1 AND hash_avatar = $2
						LIMIT 1
					)`,
					userID, avatarHash,
				).Scan(&exists)

				if !exists {
					_, _ = s.db.Pool.Exec(ctx,
						`INSERT INTO avatar_history (user_id, hash_avatar, url_cdn, changed_at)
						 VALUES ($1, $2, NULL, NOW())`,
						userID, avatarHash,
					)
				}
			}
		}
	}

	// Insert bio history (with deduplication)
	if len(bioInserts) > 0 {
		for i := 0; i < len(bioInserts); i += 2 {
			if i+1 < len(bioInserts) {
				userID := bioInserts[i].(string)
				bio := bioInserts[i+1].(string)

				var exists bool
				_ = s.db.Pool.QueryRow(ctx,
					`SELECT EXISTS(
						SELECT 1 FROM bio_history 
						WHERE user_id = $1 AND bio_content = $2
						LIMIT 1
					)`,
					userID, bio,
				).Scan(&exists)

				if !exists {
					_, _ = s.db.Pool.Exec(ctx,
						`INSERT INTO bio_history (user_id, bio_content, changed_at)
						 VALUES ($1, $2, NOW())`,
						userID, bio,
					)
				}
			}
		}
	}

	// Insert connected accounts
	if len(accountInserts) > 0 {
		for i := 0; i < len(accountInserts); i += 4 {
			if i+3 < len(accountInserts) {
				userID := accountInserts[i].(string)
				accType := accountInserts[i+1].(string)
				externalID := accountInserts[i+2].(string)
				name := accountInserts[i+3].(string)

				_, _ = s.db.Pool.Exec(ctx,
					`INSERT INTO connected_accounts (user_id, type, external_id, name, observed_at, last_seen_at)
					 VALUES ($1, $2, $3, $4, NOW(), NOW())
					 ON CONFLICT DO NOTHING`,
					userID, accType, externalID, name,
				)
			}
		}
	}

	return nil
}

// getGuildNameFromDB tenta buscar o nome do guild no banco
func (s *Scraper) getGuildNameFromDB(ctx context.Context, guildID string) string {
	var name string
	err := s.db.Pool.QueryRow(ctx,
		`SELECT COALESCE(name, '') FROM guilds WHERE guild_id = $1`,
		guildID,
	).Scan(&name)

	if err != nil || name == "" {
		// retornar ID formatado se nao tiver nome
		if len(guildID) > 8 {
			return fmt.Sprintf("Guild_%s...%s", guildID[:4], guildID[len(guildID)-4:])
		}
		return fmt.Sprintf("Guild_%s", guildID)
	}

	return name
}
