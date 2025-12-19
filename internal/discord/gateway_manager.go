package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"identity-archive/internal/db"
	"identity-archive/internal/processor"

	"github.com/gorilla/websocket"
)

type GatewayManager struct {
	tokenManager   *TokenManager
	connections    map[int64]*GatewayConnection
	mutex          sync.RWMutex
	eventProcessor *processor.EventProcessor
	scraper        *Scraper
	logger         *slog.Logger
	db             *db.DB
	// tracking de chunks por sessao de scraping (guild_id:nonce)
	// usar nonce permite agrupar todos os chunks de uma sessao alfabetica
	guildChunks      map[string]*GuildChunkTracker
	guildChunksMutex sync.RWMutex

	// controls to reduce rate-limit pressure:
	// - avoid multiple tokens scraping the same guild
	// - limit the number of concurrent guild scrapes overall
	guildScrapeStateMutex sync.Mutex
	guildScrapeInProgress map[string]int64     // guild_id -> token_id
	guildLastScrapedAt    map[string]time.Time // guild_id -> last scrape end
	guildScrapeSemaphore  chan struct{}

	// per-token cooldown when Discord closes the gateway with rate limit
	tokenCooldownMutex    sync.Mutex
	tokenRateLimitedUntil map[int64]time.Time // token_id -> time until we should avoid scraping
}

type GuildChunkTracker struct {
	GuildID        string
	GuildName      string
	Nonce          string
	ChunksReceived int // quantos chunks ja recebemos
	TotalMembers   int // total de membros coletados
	StartedAt      time.Time
	LastChunkAt    time.Time // quando recebeu o ultimo chunk
}

func NewGatewayManager(tokenManager *TokenManager, eventProcessor *processor.EventProcessor, scraper *Scraper, logger *slog.Logger, dbConn *db.DB) *GatewayManager {
	gm := &GatewayManager{
		tokenManager:          tokenManager,
		connections:           make(map[int64]*GatewayConnection),
		eventProcessor:        eventProcessor,
		scraper:               scraper,
		logger:                logger,
		db:                    dbConn,
		guildChunks:           make(map[string]*GuildChunkTracker),
		guildScrapeInProgress: make(map[string]int64),
		guildLastScrapedAt:    make(map[string]time.Time),
		// Default to 1 concurrent guild scrape to keep Discord gateway load low.
		// Increase cautiously if you know your tokens can handle it.
		guildScrapeSemaphore:  make(chan struct{}, 1),
		tokenRateLimitedUntil: make(map[int64]time.Time),
	}

	// iniciar goroutine de cleanup de trackers expirados
	go gm.cleanupExpiredTrackers()

	return gm
}

// cleanupExpiredTrackers limpa trackers de scraping que nao receberam chunks ha mais de 30 segundos
// Isso evita vazamento de memoria e loga o resumo final de cada sessao de scraping
func (gm *GatewayManager) cleanupExpiredTrackers() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		gm.guildChunksMutex.Lock()
		now := time.Now()
		for key, tracker := range gm.guildChunks {
			// se nao recebeu chunk ha mais de 30 segundos, considerar sessao finalizada
			if now.Sub(tracker.LastChunkAt) > 30*time.Second {
				elapsed := tracker.LastChunkAt.Sub(tracker.StartedAt)
				gm.logger.Info("chunk_collection_completed",
					"guild_id", tracker.GuildID,
					"guild_name", tracker.GuildName,
					"nonce", tracker.Nonce,
					"total_chunks_received", tracker.ChunksReceived,
					"total_members", tracker.TotalMembers,
					"duration", elapsed.Round(time.Second).String(),
				)
				delete(gm.guildChunks, key)
			}
		}
		gm.guildChunksMutex.Unlock()
	}
}

func (gm *GatewayManager) setTokenRateLimited(tokenID int64, until time.Time) {
	gm.tokenCooldownMutex.Lock()
	defer gm.tokenCooldownMutex.Unlock()
	gm.tokenRateLimitedUntil[tokenID] = until
}

func (gm *GatewayManager) getTokenRateLimitedUntil(tokenID int64) time.Time {
	gm.tokenCooldownMutex.Lock()
	defer gm.tokenCooldownMutex.Unlock()
	return gm.tokenRateLimitedUntil[tokenID]
}

func (gm *GatewayManager) tryStartGuildScrape(guildID string, tokenID int64, cooldown time.Duration) (bool, func(success bool)) {
	gm.guildScrapeStateMutex.Lock()
	defer gm.guildScrapeStateMutex.Unlock()

	if _, inProgress := gm.guildScrapeInProgress[guildID]; inProgress {
		return false, nil
	}

	if last, ok := gm.guildLastScrapedAt[guildID]; ok && cooldown > 0 {
		if time.Since(last) < cooldown {
			return false, nil
		}
	}

	gm.guildScrapeInProgress[guildID] = tokenID
	return true, func(success bool) {
		gm.guildScrapeStateMutex.Lock()
		defer gm.guildScrapeStateMutex.Unlock()
		delete(gm.guildScrapeInProgress, guildID)
		// Even on failure, mark a timestamp to avoid immediate hot-loops.
		gm.guildLastScrapedAt[guildID] = time.Now()
	}
}

func (gm *GatewayManager) acquireScrapeSlot() {
	gm.guildScrapeSemaphore <- struct{}{}
}

func (gm *GatewayManager) releaseScrapeSlot() {
	select {
	case <-gm.guildScrapeSemaphore:
	default:
	}
}

func (gm *GatewayManager) ConnectAllTokens(ctx context.Context) error {
	activeCount := gm.tokenManager.GetActiveTokenCount()
	if activeCount == 0 {
		return fmt.Errorf("no active tokens available")
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var errors []error

	for i := 0; i < activeCount; i++ {
		token, err := gm.tokenManager.GetNextAvailableToken()
		if err != nil {
			gm.logger.Warn("no_token_available_for_connection", "attempt", i+1)
			continue
		}

		wg.Add(1)
		go func(tok *TokenEntry) {
			defer wg.Done()

			if err := gm.ConnectToken(ctx, tok.ID, tok.DecryptedValue); err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("token %d: %w", tok.ID, err))
				mu.Unlock()
			}
		}(token)
	}

	wg.Wait()

	if len(errors) > 0 {
		return fmt.Errorf("some connections failed: %v", errors)
	}

	return nil
}

func (gm *GatewayManager) ConnectToken(ctx context.Context, tokenID int64, token string) error {
	conn := NewGatewayConnection(tokenID, token, gm.logger)

	if err := conn.Connect(ctx); err != nil {
		gm.logger.Warn("gateway_connect_failed",
			"token_id", tokenID,
			"error", err,
		)
		return err
	}

	gm.mutex.Lock()
	gm.connections[tokenID] = conn
	gm.mutex.Unlock()

	// salvar guilds que este token tem acesso
	go func() {
		// Nao usar o ctx do fluxo de conexao (pode ser cancelado assim que ConnectAll terminar).
		// Usa um timeout dedicado para o sync inicial do token.
		syncCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		gm.saveTokenGuilds(syncCtx, conn)
	}()

	// Start heartbeat
	go conn.StartHeartbeat()

	// Start connection handler
	go gm.HandleConnection(conn)

	// fazer scraping inicial dos guilds para coletar dados existentes
	if gm.scraper != nil {
		go gm.scrapeInitialGuilds(conn)
	}

	return nil
}

// scrapeInitialGuilds faz scraping de todos os guilds que o token tem acesso
func (gm *GatewayManager) scrapeInitialGuilds(conn *GatewayConnection) {
	// aguardar um pouco para garantir que a conexao esta estavel
	// jitter deterministico por token para evitar todos iniciarem ao mesmo tempo
	jitter := time.Duration(conn.TokenID%10) * 600 * time.Millisecond
	time.Sleep(5*time.Second + jitter)

	guilds := conn.GetGuilds()
	if len(guilds) == 0 {
		gm.logger.Info("no_guilds_to_scrape", "token_id", conn.TokenID)
		return
	}

	gm.logger.Info("starting_initial_scrape",
		"token_id", conn.TokenID,
		"guilds_count", len(guilds),
	)

	scraped := 0
	skipped := 0
	for i, guildID := range guilds {
		if !conn.Connected {
			gm.logger.Warn("scrape_interrupted_connection_lost", "token_id", conn.TokenID)
			break
		}

		// If this token was recently rate-limited by Discord, don't keep scraping.
		if until := gm.getTokenRateLimitedUntil(conn.TokenID); !until.IsZero() {
			if now := time.Now(); now.Before(until) {
				sleepFor := until.Sub(now)
				gm.logger.Warn("token_scrape_rate_limited_cooldown",
					"token_id", conn.TokenID,
					"sleep", sleepFor.Round(time.Second).String(),
				)
				time.Sleep(sleepFor)
			}
		}

		// Prevent duplicate scrapes across tokens during startup.
		canStart, finish := gm.tryStartGuildScrape(guildID, conn.TokenID, 30*time.Minute)
		if !canStart {
			skipped++
			if i < len(guilds)-1 {
				time.Sleep(250 * time.Millisecond)
			}
			continue
		}

		gm.acquireScrapeSlot()
		func() {
			defer gm.releaseScrapeSlot()

			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			err := gm.scraper.ScrapeGuildMembers(ctx, guildID, conn)
			cancel()
			finish(err == nil)
			if err != nil {
				gm.logger.Warn("guild_scrape_failed",
					"token_id", conn.TokenID,
					"guild_id", guildID,
					"error", err,
				)
				return
			}
			scraped++
		}()

		// aguardar entre guilds para evitar rate limit (3 segundos)
		if i < len(guilds)-1 {
			time.Sleep(3 * time.Second)
		}
	}

	gm.logger.Info("initial_scrape_completed",
		"token_id", conn.TokenID,
		"guilds_scraped", scraped,
		"guilds_skipped", skipped,
	)
}

func (gm *GatewayManager) HandleConnection(conn *GatewayConnection) {
	defer func() {
		if r := recover(); r != nil {
			gm.logger.Error("panic_in_handle_connection",
				"token_id", conn.TokenID,
				"panic", r,
			)
		}
		gm.mutex.Lock()
		delete(gm.connections, conn.TokenID)
		gm.mutex.Unlock()
		conn.Close()
	}()

	maxReconnectAttempts := 5
	reconnectAttempts := 0
	baseBackoff := 5 * time.Second

	for {
		if !conn.Connected {
			if reconnectAttempts >= maxReconnectAttempts {
				gm.logger.Error("max_reconnect_attempts_reached",
					"token_id", conn.TokenID,
				)
				_ = gm.tokenManager.MarkTokenAsSuspended(conn.TokenID, "max_reconnect_attempts", 10)
				return
			}

			reconnectAttempts++
			gm.logger.Info("attempting_reconnect",
				"token_id", conn.TokenID,
				"attempt", reconnectAttempts,
			)

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			err := conn.Resume(ctx)
			cancel()

			if err != nil {
				gm.logger.Warn("resume_failed",
					"token_id", conn.TokenID,
					"error", err,
				)

				// Try full reconnect
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				err = conn.Connect(ctx)
				cancel()

				if err != nil {
					gm.logger.Warn("reconnect_failed",
						"token_id", conn.TokenID,
						"error", err,
					)
					time.Sleep(baseBackoff)
					continue
				}

				// Restart heartbeat
				go conn.StartHeartbeat()
			} else {
				// Resume succeeded: restart heartbeat for the new connection
				go conn.StartHeartbeat()
			}

			reconnectAttempts = 0
		}

		// Read messages
		var msg GatewayMessage
		conn.mutex.RLock()
		wsConn := conn.Conn
		conn.mutex.RUnlock()

		if wsConn == nil {
			break
		}

		if err := wsConn.ReadJSON(&msg); err != nil {
			gm.logger.Warn("read_message_failed", "token_id", conn.TokenID, "error", err)

			// Sempre fechar a conexao atual para parar heartbeat e limpar o websocket
			_ = conn.Close()

			// Se for close frame, podemos aplicar backoff melhor.
			if ce, ok := err.(*websocket.CloseError); ok {
				// 4008 = Rate limited (Discord gateway close code)
				if ce.Code == 4008 {
					gm.logger.Warn("gateway_rate_limited",
						"token_id", conn.TokenID,
						"close_text", ce.Text,
					)
					// backoff maior para nao entrar em loop de rate-limit
					cooldown := time.Now().Add(2 * time.Minute)
					gm.setTokenRateLimited(conn.TokenID, cooldown)
					time.Sleep(2 * time.Minute)
					continue
				}
			}

			// Close/EOF/network error normal: tenta reconectar com backoff curto
			time.Sleep(baseBackoff)
			continue
		}

		// Update sequence
		if msg.S > 0 {
			conn.mutex.Lock()
			conn.LastSequence = msg.S
			conn.mutex.Unlock()
		}

		// Handle opcode
		switch msg.Op {
		case 0: // DISPATCH
			// tentar converter D para map
			dataMap, ok := msg.D.(map[string]interface{})
			if !ok {
				// se D nao for map, pode ser array ou outro tipo - logar e continuar
				gm.logger.Debug("event_data_not_map", "token_id", conn.TokenID, "event_type", msg.T, "data_type", fmt.Sprintf("%T", msg.D))
				continue
			}

			// Handle GUILD_MEMBERS_CHUNK specially for scraper
			if msg.T == "GUILD_MEMBERS_CHUNK" && gm.scraper != nil {
				guildID, _ := dataMap["guild_id"].(string)
				members, _ := dataMap["members"].([]interface{})
				chunkIndex, _ := dataMap["chunk_index"].(float64)
				chunkCount, _ := dataMap["chunk_count"].(float64)
				nonce, _ := dataMap["nonce"].(string)

				// usar (guild_id:nonce) como chave para agrupar chunks da mesma sessao de scraping
				// se nonce estiver vazio, usar apenas guild_id (compatibilidade)
				trackerKey := guildID
				if nonce != "" {
					trackerKey = guildID + ":" + nonce
				}

				// inicializar tracker se for o primeiro chunk desta sessao
				gm.guildChunksMutex.Lock()
				tracker, exists := gm.guildChunks[trackerKey]
				if !exists {
					tracker = &GuildChunkTracker{
						GuildID:        guildID,
						GuildName:      gm.getGuildName(guildID, conn),
						Nonce:          nonce,
						ChunksReceived: 0,
						TotalMembers:   0,
						StartedAt:      time.Now(),
						LastChunkAt:    time.Now(),
					}
					gm.guildChunks[trackerKey] = tracker

					gm.logger.Info("chunk_collection_started",
						"guild_id", guildID,
						"guild_name", tracker.GuildName,
						"nonce", nonce,
						"token_id", conn.TokenID,
					)
				}
				tracker.ChunksReceived++
				tracker.TotalMembers += len(members)
				tracker.LastChunkAt = time.Now()
				gm.guildChunksMutex.Unlock()

				// Convert []interface{} to []map[string]interface{}
				memberMaps := make([]map[string]interface{}, 0, len(members))
				for _, m := range members {
					if mMap, ok := m.(map[string]interface{}); ok {
						memberMaps = append(memberMaps, mMap)
					}
				}

				if len(memberMaps) > 0 {
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					// usa ProcessGuildMembersChunkWithToken para salvar a relacao com o token
					if err := gm.scraper.ProcessGuildMembersChunkWithToken(ctx, guildID, memberMaps, conn.TokenID); err != nil {
						gm.logger.Warn("chunk_processing_failed",
							"guild_id", guildID,
							"guild_name", tracker.GuildName,
							"chunk", fmt.Sprintf("%d/%d", int(chunkIndex)+1, int(chunkCount)),
							"error", err,
						)
					} else {
						// log apenas a cada 10 chunks ou no ultimo para reduzir spam
						shouldLog := tracker.ChunksReceived%10 == 0 || int(chunkIndex)+1 == int(chunkCount)
						if shouldLog {
							elapsed := time.Since(tracker.StartedAt)
							gm.logger.Debug("chunk_processed",
								"guild_id", guildID,
								"guild_name", tracker.GuildName,
								"chunk", fmt.Sprintf("%d/%d", int(chunkIndex)+1, int(chunkCount)),
								"members_in_chunk", len(memberMaps),
								"total_members_collected", tracker.TotalMembers,
								"chunks_received", tracker.ChunksReceived,
								"elapsed", elapsed.Round(time.Second).String(),
								"token_id", conn.TokenID,
							)
						}

						// se for o ultimo chunk deste request especifico, logar
						// Nota: com scraping alfabetico, podem haver multiplos "ultimos chunks"
						// entao nao deletamos o tracker aqui - ele sera limpo por timeout
						if int(chunkIndex)+1 == int(chunkCount) {
							gm.logger.Debug("chunk_batch_done",
								"guild_id", guildID,
								"chunk_index", int(chunkIndex),
								"chunk_count", int(chunkCount),
							)
						}
					}
					cancel()
				}
			}

			// processar evento normalmente
			gm.HandleEvent(conn.TokenID, msg.T, dataMap)
		case 1: // HEARTBEAT
			conn.sendHeartbeat()
		case 7: // RECONNECT
			gm.logger.Info("reconnect_requested", "token_id", conn.TokenID)
			conn.mutex.Lock()
			conn.Connected = false
			conn.mutex.Unlock()
		case 9: // INVALID_SESSION
			gm.logger.Warn("invalid_session", "token_id", conn.TokenID)
			conn.mutex.Lock()
			conn.Connected = false
			conn.SessionID = "" // Force full reconnect
			conn.mutex.Unlock()
		case 10: // HELLO
			// Already handled in Connect
		case 11: // HEARTBEAT_ACK
			gm.logger.Debug("heartbeat_ack_received", "token_id", conn.TokenID)
		default:
			gm.logger.Debug("unknown_opcode",
				"token_id", conn.TokenID,
				"opcode", msg.Op,
			)
		}
	}
}

func (gm *GatewayManager) HandleEvent(tokenID int64, eventType string, data map[string]interface{}) {
	if gm.eventProcessor == nil {
		return
	}

	eventDataBytes, err := json.Marshal(data)
	if err != nil {
		gm.logger.Warn("failed_to_marshal_event",
			"token_id", tokenID,
			"event_type", eventType,
			"error", err,
		)
		return
	}

	var eventData map[string]interface{}
	if err := json.Unmarshal(eventDataBytes, &eventData); err != nil {
		return
	}

	event := processor.Event{
		Type:      eventType,
		Data:      eventData,
		Timestamp: time.Now(),
		TokenID:   tokenID,
	}

	// Send to event processor queue (non-blocking)
	select {
	case gm.eventProcessor.GetEventQueue() <- event:
	default:
		gm.logger.Warn("event_queue_full",
			"token_id", tokenID,
			"event_type", eventType,
		)
	}
}

func (gm *GatewayManager) GetConnection(tokenID int64) *GatewayConnection {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()
	return gm.connections[tokenID]
}

func (gm *GatewayManager) GetActiveConnectionsCount() int {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()
	return len(gm.connections)
}

func (gm *GatewayManager) CloseAll() {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()

	for _, conn := range gm.connections {
		conn.Close()
	}

	gm.connections = make(map[int64]*GatewayConnection)
}

// getGuildName tenta pegar o nome do guild dos dados da conexao
func (gm *GatewayManager) getGuildName(guildID string, conn *GatewayConnection) string {
	// tentar pegar do cache de guilds da conexao
	conn.mutex.RLock()
	defer conn.mutex.RUnlock()

	// por enquanto retorna apenas o ID formatado
	// podemos melhorar isso depois pegando do banco ou cache
	if len(guildID) > 8 {
		return fmt.Sprintf("Guild_%s...%s", guildID[:4], guildID[len(guildID)-4:])
	}
	return fmt.Sprintf("Guild_%s", guildID)
}

func (gm *GatewayManager) GetAllConnections() []*GatewayConnection {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()

	result := make([]*GatewayConnection, 0, len(gm.connections))
	for _, conn := range gm.connections {
		result = append(result, conn)
	}
	return result
}

// saveTokenGuilds salva os guilds que este token tem acesso
func (gm *GatewayManager) saveTokenGuilds(ctx context.Context, conn *GatewayConnection) {
	if gm.db == nil {
		return
	}

	guilds := conn.GetGuilds()
	if len(guilds) == 0 {
		return
	}

	gm.logger.Info("saving_token_guilds",
		"token_id", conn.TokenID,
		"guilds_count", len(guilds),
	)

	for _, guildID := range guilds {
		_, err := gm.db.Pool.Exec(ctx,
			`INSERT INTO token_guilds (token_id, guild_id, discovered_at, last_synced_at)
			 VALUES ($1, $2, NOW(), NOW())
			 ON CONFLICT (token_id, guild_id) DO UPDATE SET last_synced_at = NOW()`,
			conn.TokenID, guildID,
		)
		if err != nil {
			gm.logger.Warn("failed_to_save_token_guild",
				"token_id", conn.TokenID,
				"guild_id", guildID,
				"error", err,
			)
		}
	}

	gm.logger.Info("token_guilds_saved",
		"token_id", conn.TokenID,
		"guilds_count", len(guilds),
	)
}
