package discord

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
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

	// behavior flags
	enableGuildSubscriptions  bool
	requestMemberPresences    bool
	scrapeInitialGuildMembers bool

	// periodic scraping
	periodicScrapeInterval time.Duration
	periodicScrapeStop     chan struct{}
}

type GatewayManagerOptions struct {
	EnableGuildSubscriptions  bool
	RequestMemberPresences    bool
	ScrapeInitialGuildMembers bool
	MaxConcurrentGuildScrapes int
	PeriodicScrapeInterval    time.Duration // 0 = disabled, default 1 hour
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
	return NewGatewayManagerWithOptions(tokenManager, eventProcessor, scraper, logger, dbConn, GatewayManagerOptions{
		EnableGuildSubscriptions:  true,
		RequestMemberPresences:    true,
		ScrapeInitialGuildMembers: true,
		MaxConcurrentGuildScrapes: 1,
		PeriodicScrapeInterval:    1 * time.Hour,
	})
}

func NewGatewayManagerWithOptions(tokenManager *TokenManager, eventProcessor *processor.EventProcessor, scraper *Scraper, logger *slog.Logger, dbConn *db.DB, opts GatewayManagerOptions) *GatewayManager {
	maxScrapes := opts.MaxConcurrentGuildScrapes
	if maxScrapes < 1 {
		maxScrapes = 1
	}
	if maxScrapes > 10 {
		maxScrapes = 10
	}

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
		// Limit the number of concurrent guild scrapes overall.
		// Increase cautiously to avoid rate limits.
		guildScrapeSemaphore:      make(chan struct{}, maxScrapes),
		tokenRateLimitedUntil:     make(map[int64]time.Time),
		enableGuildSubscriptions:  opts.EnableGuildSubscriptions,
		requestMemberPresences:    opts.RequestMemberPresences,
		scrapeInitialGuildMembers: opts.ScrapeInitialGuildMembers,
		periodicScrapeInterval:    opts.PeriodicScrapeInterval,
		periodicScrapeStop:        make(chan struct{}),
	}

	// iniciar goroutine de cleanup de trackers expirados
	go gm.cleanupExpiredTrackers()

	// iniciar scraping periodico se configurado
	if opts.PeriodicScrapeInterval > 0 {
		go gm.startPeriodicScraping()
	}

	return gm
}

// cleanupExpiredTrackers limpa trackers de scraping que nao receberam chunks ha mais de 30 segundos
// Isso evita vazamento de memoria e loga o resumo final de cada sessao de scraping
func (gm *GatewayManager) cleanupExpiredTrackers() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	const maxGuildChunkTrackers = 500 // limite máximo para evitar memory leak

	for range ticker.C {
		gm.guildChunksMutex.Lock()
		now := time.Now()

		// Primeiro: remover trackers expirados (30 segundos sem chunks)
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

		// Segundo: se ainda exceder limite, remover os trackers mais antigos
		if len(gm.guildChunks) > maxGuildChunkTrackers {
			// Encontrar e remover os mais antigos (metade do excedente)
			toRemove := len(gm.guildChunks) - maxGuildChunkTrackers/2
			removed := 0
			for key, tracker := range gm.guildChunks {
				if removed >= toRemove {
					break
				}
				gm.logger.Warn("chunk_tracker_force_cleanup",
					"guild_id", tracker.GuildID,
					"reason", "max_trackers_exceeded",
				)
				delete(gm.guildChunks, key)
				removed++
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
	conn.RequestPresences = gm.requestMemberPresences

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

	// Processar e salvar todos os dados do READY (relacionamentos, DMs, presences, etc)
	go func() {
		syncCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		gm.processReadyData(syncCtx, conn)
	}()

	// Start heartbeat
	go conn.StartHeartbeat()

	// Start connection handler
	go gm.HandleConnection(conn)

	// request guild subscriptions (helps Discord send PRESENCE_UPDATE, MESSAGE_CREATE, etc)
	if gm.enableGuildSubscriptions {
		go gm.subscribeAllGuilds(conn)
	}

	// fazer scraping inicial dos guilds para coletar dados existentes
	if gm.scraper != nil && gm.scrapeInitialGuildMembers {
		go gm.scrapeInitialGuilds(conn)
	}

	return nil
}

// subscribeAllGuilds envia subscriptions para todos os guilds do token
// Para user tokens, isso é crucial para receber eventos de presença, mensagens, etc
// Usa rate limiting cuidadoso para evitar detecção
func (gm *GatewayManager) subscribeAllGuilds(conn *GatewayConnection) {
	// wait a bit to ensure connection is stable and READY has fully propagated
	// Jitter para evitar todos os tokens fazerem ao mesmo tempo
	jitter := time.Duration(conn.TokenID%5) * 500 * time.Millisecond
	time.Sleep(3*time.Second + jitter)

	guildInfos := conn.GetGuildInfos()
	if len(guildInfos) == 0 {
		return
	}

	gm.logger.Info("requesting_guild_subscriptions",
		"token_id", conn.TokenID,
		"guilds_count", len(guildInfos),
	)

	// Contador para logging de progresso
	subscribed := 0

	for i, guild := range guildInfos {
		if !conn.Connected {
			return
		}

		// Criar map de canais para subscription
		// Para cada canal de texto (type 0), voice (type 2), etc, definimos ranges de membros
		// [[0, 99]] significa que queremos ver membros nas posicoes 0-99
		channels := make(map[string][][2]int)

		// Limitar numero de canais por subscription para evitar rate limit
		channelCount := 0
		maxChannelsPerSub := 5 // Discord client real nao faz muitos de uma vez

		for _, ch := range guild.Channels {
			if channelCount >= maxChannelsPerSub {
				break
			}
			// Type 0 = TEXT, Type 2 = VOICE, Type 5 = ANNOUNCEMENT, Type 13 = STAGE_VOICE
			if ch.Type == 0 || ch.Type == 2 || ch.Type == 5 || ch.Type == 13 {
				// Solicitar apenas primeiro bloco de membros (mais leve)
				channels[ch.ID] = [][2]int{{0, 99}}
				channelCount++
			}
		}

		// Se nao temos canais, ainda enviamos subscription vazia para eventos basicos
		if err := conn.RequestGuildSubscriptions(guild.ID, channels); err != nil {
			gm.logger.Debug("guild_subscription_failed",
				"token_id", conn.TokenID,
				"guild_id", guild.ID,
				"guild_name", guild.Name,
				"error", err,
			)
		} else {
			subscribed++
		}

		// Tambem enviar subscription para typing e threads
		gm.sendGuildTypingSubscription(conn, guild.ID)

		// Rate limiting mais cuidadoso:
		// - Delay base de 300ms
		// - A cada 10 guilds, pausa maior de 2 segundos
		if (i+1)%10 == 0 {
			time.Sleep(2 * time.Second)
			gm.logger.Debug("guild_subscription_progress",
				"token_id", conn.TokenID,
				"subscribed", subscribed,
				"total", len(guildInfos),
			)
		} else {
			time.Sleep(300 * time.Millisecond)
		}
	}

	gm.logger.Info("guild_subscriptions_complete",
		"token_id", conn.TokenID,
		"subscribed", subscribed,
	)
}

// sendGuildTypingSubscription envia subscription para eventos de typing no guild
func (gm *GatewayManager) sendGuildTypingSubscription(conn *GatewayConnection, guildID string) {
	conn.mutex.RLock()
	wsConn := conn.Conn
	conn.mutex.RUnlock()

	if wsConn == nil {
		return
	}

	// Opcode 37 = GUILD_APPLICATION_COMMANDS_SEARCH (mais moderno)
	// Na verdade para typing events usamos implicitamente via subscription
	// Mas podemos enviar lazy request para manter atualizado

	// Lazy Request (opcode 14 com diferentes parametros)
	lazyPayload := map[string]interface{}{
		"op": 14,
		"d": map[string]interface{}{
			"guild_id":   guildID,
			"typing":     true,
			"threads":    true,
			"activities": true,
		},
	}

	wsConn.WriteJSON(lazyPayload)
}

// scrapeInitialGuilds faz scraping de todos os guilds que o token tem acesso
// Os guilds sao ordenados por tamanho (maiores primeiro) para priorizar servidores grandes
func (gm *GatewayManager) scrapeInitialGuilds(conn *GatewayConnection) {
	// aguardar um pouco para garantir que a conexao esta estavel
	// jitter deterministico por token para evitar todos iniciarem ao mesmo tempo
	jitter := time.Duration(conn.TokenID%10) * 600 * time.Millisecond
	time.Sleep(5*time.Second + jitter)

	guildIDs := conn.GetGuilds()
	if len(guildIDs) == 0 {
		gm.logger.Info("no_guilds_to_scrape", "token_id", conn.TokenID)
		return
	}

	// Buscar member_count e nome do banco para ordenar por tamanho
	ctx := context.Background()
	guildInfo := make(map[string]struct {
		Size int
		Name string
	})
	rows, err := gm.db.Pool.Query(ctx,
		`SELECT guild_id, COALESCE(member_count, 0), COALESCE(name, '') FROM guilds WHERE guild_id = ANY($1)`,
		guildIDs)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var guildID string
			var memberCount int
			var name string
			if err := rows.Scan(&guildID, &memberCount, &name); err == nil {
				guildInfo[guildID] = struct {
					Size int
					Name string
				}{Size: memberCount, Name: name}
			}
		}
	}

	// Ordenar guilds por tamanho (maiores primeiro)
	type guildWithSize struct {
		ID   string
		Name string
		Size int
	}
	guildsToScrape := make([]guildWithSize, 0, len(guildIDs))
	for _, guildID := range guildIDs {
		info := guildInfo[guildID]
		guildsToScrape = append(guildsToScrape, guildWithSize{
			ID:   guildID,
			Name: info.Name,
			Size: info.Size,
		})
	}
	sort.Slice(guildsToScrape, func(i, j int) bool {
		return guildsToScrape[i].Size > guildsToScrape[j].Size
	})

	gm.logger.Info("starting_initial_scrape",
		"token_id", conn.TokenID,
		"guilds_count", len(guildsToScrape),
	)

	if len(guildsToScrape) > 0 && guildsToScrape[0].Size > 0 {
		gm.logger.Info("largest_guild_first",
			"guild_id", guildsToScrape[0].ID,
			"guild_name", guildsToScrape[0].Name,
			"member_count", guildsToScrape[0].Size,
		)
	}

	scraped := 0
	skipped := 0
	for i, guild := range guildsToScrape {
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
		canStart, finish := gm.tryStartGuildScrape(guild.ID, conn.TokenID, 30*time.Minute)
		if !canStart {
			skipped++
			if i < len(guildsToScrape)-1 {
				time.Sleep(250 * time.Millisecond)
			}
			continue
		}

		gm.acquireScrapeSlot()
		func() {
			defer gm.releaseScrapeSlot()

			scrapeCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			err := gm.scraper.ScrapeGuildMembers(scrapeCtx, guild.ID, conn)
			cancel()
			finish(err == nil)
			if err != nil {
				gm.logger.Warn("guild_scrape_failed",
					"token_id", conn.TokenID,
					"guild_id", guild.ID,
					"guild_name", guild.Name,
					"member_count", guild.Size,
					"error", err,
				)
				return
			}
			scraped++
		}()

		// aguardar entre guilds para evitar rate limit (3 segundos)
		if i < len(guildsToScrape)-1 {
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

	maxReconnectAttempts := 10
	reconnectAttempts := 0
	baseBackoff := 3 * time.Second
	maxBackoff := 60 * time.Second

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
				// Resume failed is expected when session expires - use Debug level
				gm.logger.Debug("resume_failed_trying_full_reconnect",
					"token_id", conn.TokenID,
					"attempt", reconnectAttempts,
					"error", err,
				)

				// Exponential backoff before full reconnect
				backoff := baseBackoff * time.Duration(1<<uint(reconnectAttempts-1))
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
				time.Sleep(backoff)

				// Try full reconnect
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				err = conn.Connect(ctx)
				cancel()

				if err != nil {
					gm.logger.Debug("reconnect_failed",
						"token_id", conn.TokenID,
						"attempt", reconnectAttempts,
						"error", err,
					)
					continue
				}

				// Restart heartbeat
				go conn.StartHeartbeat()
				gm.logger.Info("gateway_reconnected", "token_id", conn.TokenID)
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
			// Connection closed is expected - use Debug level to reduce log noise
			gm.logger.Debug("connection_closed", "token_id", conn.TokenID, "error", err)

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

	// Events that are actually processed by the system - only queue these
	processedEvents := map[string]bool{
		"USER_UPDATE":         true,
		"GUILD_MEMBER_UPDATE": true,
		"PRESENCE_UPDATE":     true,
		"GUILD_MEMBERS_CHUNK": true,
		"MESSAGE_CREATE":      true,
		"VOICE_STATE_UPDATE":  true,
		"TYPING_START":        true,
		"GUILD_MEMBER_ADD":    true,
		"GUILD_CREATE":        true,
	}

	// Skip events we don't process - saves queue space and CPU
	if !processedEvents[eventType] {
		return
	}

	// data já é map[string]interface{}, não precisa serializar/deserializar
	event := processor.Event{
		Type:      eventType,
		Data:      data,
		Timestamp: time.Now(),
		TokenID:   tokenID,
	}

	// High-frequency events that can be dropped silently when queue is full
	// These events happen very frequently and losing some is acceptable
	highFreq := map[string]bool{
		"VOICE_STATE_UPDATE": true,
		"PRESENCE_UPDATE":    true,
		"TYPING_START":       true,
	}

	// Try to send to queue
	sent := false
	select {
	case gm.eventProcessor.GetEventQueue() <- event:
		sent = true
	default:
		// Queue full on first try - for important events, wait a bit
		if !highFreq[eventType] {
			select {
			case gm.eventProcessor.GetEventQueue() <- event:
				sent = true
			case <-time.After(1 * time.Second):
				sent = false
			}
		}
	}

	// Log dropped events (debug level to avoid log spam)
	if !sent {
		gm.logger.Debug("event_dropped_queue_full",
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

	guilds := conn.GetGuildInfos()
	if len(guilds) == 0 {
		return
	}

	gm.logger.Info("saving_token_guilds",
		"token_id", conn.TokenID,
		"guilds_count", len(guilds),
	)

	// Log dos nomes das guilds
	guildNames := make([]string, 0, len(guilds))
	for _, g := range guilds {
		if g.Name != "" {
			guildNames = append(guildNames, g.Name)
		}
	}
	if len(guildNames) > 0 {
		// Mostrar até 10 guilds
		displayNames := guildNames
		if len(displayNames) > 10 {
			displayNames = displayNames[:10]
		}
		gm.logger.Info("guilds_discovered",
			"token_id", conn.TokenID,
			"guild_names", displayNames,
			"total", len(guildNames),
		)
	}

	for _, guild := range guilds {
		// Upsert guild info
		_, err := gm.db.Pool.Exec(ctx,
			`INSERT INTO guilds (guild_id, name, icon, discovered_at)
			 VALUES ($1, $2, $3, NOW())
			 ON CONFLICT (guild_id) DO UPDATE SET name = $2, icon = $3`,
			guild.ID, guild.Name, guild.Icon,
		)
		if err != nil {
			gm.logger.Warn("failed_to_save_guild_info",
				"guild_id", guild.ID,
				"guild_name", guild.Name,
				"error", err,
			)
		}

		// Link token to guild
		_, err = gm.db.Pool.Exec(ctx,
			`INSERT INTO token_guilds (token_id, guild_id, discovered_at, last_synced_at)
			 VALUES ($1, $2, NOW(), NOW())
			 ON CONFLICT (token_id, guild_id) DO UPDATE SET last_synced_at = NOW()`,
			conn.TokenID, guild.ID,
		)
		if err != nil {
			gm.logger.Warn("failed_to_save_token_guild",
				"token_id", conn.TokenID,
				"guild_id", guild.ID,
				"guild_name", guild.Name,
				"error", err,
			)
		}
	}

	gm.logger.Info("token_guilds_saved",
		"token_id", conn.TokenID,
		"guilds_count", len(guilds),
	)
}

// GuildWithSize representa um guild com seu tamanho para ordenacao
type GuildWithSize struct {
	GuildID     string
	Name        string
	MemberCount int
}

// getGuildsOrderedBySize retorna todos os guilds do banco ordenados por member_count (maiores primeiro)
func (gm *GatewayManager) getGuildsOrderedBySize(ctx context.Context) ([]GuildWithSize, error) {
	rows, err := gm.db.Pool.Query(ctx,
		`SELECT guild_id, COALESCE(name, ''), COALESCE(member_count, 0) 
		 FROM guilds 
		 ORDER BY member_count DESC NULLS LAST, guild_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var guilds []GuildWithSize
	for rows.Next() {
		var g GuildWithSize
		if err := rows.Scan(&g.GuildID, &g.Name, &g.MemberCount); err != nil {
			continue
		}
		guilds = append(guilds, g)
	}
	return guilds, nil
}

// getConnectionForGuild encontra uma conexao ativa que tenha acesso ao guild especificado
func (gm *GatewayManager) getConnectionForGuild(guildID string) *GatewayConnection {
	gm.mutex.RLock()
	defer gm.mutex.RUnlock()

	for _, conn := range gm.connections {
		if !conn.Connected {
			continue
		}
		for _, g := range conn.GetGuilds() {
			if g == guildID {
				return conn
			}
		}
	}
	return nil
}

// startPeriodicScraping inicia o job de scraping periodico
func (gm *GatewayManager) startPeriodicScraping() {
	if gm.periodicScrapeInterval <= 0 {
		return
	}

	// aguardar um pouco para dar tempo das conexoes iniciais se estabelecerem
	time.Sleep(2 * time.Minute)

	gm.logger.Info("starting_periodic_scraping",
		"interval", gm.periodicScrapeInterval.String(),
	)

	ticker := time.NewTicker(gm.periodicScrapeInterval)
	defer ticker.Stop()

	// executar scraping imediatamente na primeira vez (apos o delay inicial)
	gm.runPeriodicScrape()

	for {
		select {
		case <-ticker.C:
			gm.runPeriodicScrape()
		case <-gm.periodicScrapeStop:
			gm.logger.Info("periodic_scraping_stopped")
			return
		}
	}
}

// runPeriodicScrape executa um ciclo completo de scraping de todos os guilds
func (gm *GatewayManager) runPeriodicScrape() {
	ctx := context.Background()

	gm.logger.Info("periodic_scrape_starting")

	// buscar todos os guilds ordenados por tamanho (maiores primeiro)
	guilds, err := gm.getGuildsOrderedBySize(ctx)
	if err != nil {
		gm.logger.Error("failed_to_get_guilds_for_periodic_scrape", "error", err)
		return
	}

	if len(guilds) == 0 {
		gm.logger.Info("no_guilds_to_scrape_periodically")
		return
	}

	gm.logger.Info("periodic_scrape_guilds_found",
		"total_guilds", len(guilds),
		"largest_guild", guilds[0].Name,
		"largest_guild_members", guilds[0].MemberCount,
	)

	scraped := 0
	skipped := 0
	failed := 0

	for i, guild := range guilds {
		// verificar se devemos parar
		select {
		case <-gm.periodicScrapeStop:
			gm.logger.Info("periodic_scrape_interrupted", "scraped", scraped, "skipped", skipped)
			return
		default:
		}

		// encontrar uma conexao que tenha acesso a este guild
		conn := gm.getConnectionForGuild(guild.GuildID)
		if conn == nil {
			skipped++
			continue
		}

		// verificar rate limit do token
		if until := gm.getTokenRateLimitedUntil(conn.TokenID); !until.IsZero() {
			if time.Now().Before(until) {
				skipped++
				continue
			}
		}

		// tentar iniciar scraping (respeitando cooldown de 30min por guild)
		canStart, finish := gm.tryStartGuildScrape(guild.GuildID, conn.TokenID, 30*time.Minute)
		if !canStart {
			skipped++
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// adquirir slot de scraping
		gm.acquireScrapeSlot()
		func() {
			defer gm.releaseScrapeSlot()

			scrapeCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			err := gm.scraper.ScrapeGuildMembers(scrapeCtx, guild.GuildID, conn)
			cancel()
			finish(err == nil)

			if err != nil {
				gm.logger.Warn("periodic_scrape_guild_failed",
					"guild_id", guild.GuildID,
					"guild_name", guild.Name,
					"error", err,
				)
				failed++
				return
			}
			scraped++

			// log progresso a cada 10 guilds
			if scraped%10 == 0 {
				gm.logger.Info("periodic_scrape_progress",
					"scraped", scraped,
					"total", len(guilds),
					"percent", fmt.Sprintf("%.1f%%", float64(i+1)/float64(len(guilds))*100),
				)
			}
		}()

		// delay entre guilds para evitar rate limit
		time.Sleep(5 * time.Second)
	}

	gm.logger.Info("periodic_scrape_completed",
		"scraped", scraped,
		"skipped", skipped,
		"failed", failed,
		"total", len(guilds),
	)
}

// StopPeriodicScraping para o job de scraping periodico
func (gm *GatewayManager) StopPeriodicScraping() {
	select {
	case gm.periodicScrapeStop <- struct{}{}:
	default:
	}
}

// processReadyData processa e salva todos os dados do evento READY no banco
// User tokens recebem muitos dados no READY que bots não recebem
func (gm *GatewayManager) processReadyData(ctx context.Context, conn *GatewayConnection) {
	conn.mutex.RLock()
	readyData := conn.ReadyData
	tokenID := conn.TokenID
	conn.mutex.RUnlock()

	if readyData == nil {
		return
	}

	gm.logger.Info("processing_ready_data",
		"token_id", tokenID,
		"guilds", len(readyData.Guilds),
		"relationships", len(readyData.Relationships),
		"private_channels", len(readyData.PrivateChannels),
	)

	// 1. Processar dados do próprio usuário do token
	if readyData.User.ID != "" {
		gm.saveUserFromReady(ctx, readyData.User.ID, readyData.User.Username,
			readyData.User.Discriminator, readyData.User.GlobalName, readyData.User.Avatar,
			readyData.User.Banner, readyData.User.Bio, readyData.User.AccentColor,
			readyData.User.PremiumType, readyData.User.PublicFlags)
	}

	// 2. Processar relacionamentos (amigos) - dados exclusivos de user token
	for _, rel := range readyData.Relationships {
		if rel.User.ID == "" {
			continue
		}

		// Salvar usuário amigo
		gm.saveUserFromReady(ctx, rel.User.ID, rel.User.Username,
			rel.User.Discriminator, rel.User.GlobalName, rel.User.Avatar,
			"", "", 0, 0, rel.User.PublicFlags)

		// Salvar relacionamento
		_, _ = gm.db.Pool.Exec(ctx,
			`INSERT INTO relationships (user_id, friend_id, relationship_type, nickname, discovered_at)
			 VALUES ($1, $2, $3, $4, NOW())
			 ON CONFLICT (user_id, friend_id) DO UPDATE SET 
				relationship_type = EXCLUDED.relationship_type,
				nickname = EXCLUDED.nickname,
				last_seen_at = NOW()`,
			readyData.User.ID, rel.User.ID, rel.Type, rel.Nickname,
		)
	}

	// 3. Processar presences dos amigos
	for _, presence := range readyData.Presences {
		if presence.UserID == "" {
			continue
		}

		// Salvar status de presença
		_, _ = gm.db.Pool.Exec(ctx,
			`INSERT INTO presence_history (user_id, guild_id, status, changed_at)
			 VALUES ($1, NULL, $2, NOW())`,
			presence.UserID, presence.Status,
		)

		// Salvar atividades
		for _, act := range presence.Activities {
			if act.Name == "" {
				continue
			}
			_, _ = gm.db.Pool.Exec(ctx,
				`INSERT INTO activity_history 
				 (user_id, activity_type, name, details, state, started_at)
				 VALUES ($1, $2, $3, $4, $5, NOW())`,
				presence.UserID, act.Type, act.Name, act.Details, act.State,
			)
		}
	}

	// 4. Processar guilds completos
	guildNames := make([]string, 0, len(readyData.Guilds))
	for _, g := range readyData.Guilds {
		if g.Name != "" {
			guildNames = append(guildNames, g.Name)
		}
	}
	if len(guildNames) > 0 {
		displayNames := guildNames
		if len(displayNames) > 10 {
			displayNames = displayNames[:10]
		}
		gm.logger.Info("processing_guilds_from_ready",
			"token_id", tokenID,
			"guild_names", displayNames,
			"total", len(guildNames),
		)
	}

	for i, guild := range readyData.Guilds {
		gm.logger.Debug("processing_guild",
			"token_id", tokenID,
			"guild_name", guild.Name,
			"guild_id", guild.ID,
			"member_count", guild.MemberCount,
			"channels", len(guild.Channels),
			"roles", len(guild.Roles),
			"progress", fmt.Sprintf("%d/%d", i+1, len(readyData.Guilds)),
		)
		gm.processReadyGuild(ctx, &guild, tokenID)

		// Pequeno delay entre guilds para evitar sobrecarga
		time.Sleep(100 * time.Millisecond)
	}

	// 5. Processar connected accounts do token owner
	for _, acc := range readyData.ConnectedAccounts {
		_, _ = gm.db.Pool.Exec(ctx,
			`INSERT INTO connected_accounts (user_id, type, external_id, name, verified, visibility, first_seen_at)
			 VALUES ($1, $2, $3, $4, $5, $6, NOW())
			 ON CONFLICT (user_id, type, external_id) DO UPDATE SET
				name = EXCLUDED.name,
				verified = EXCLUDED.verified,
				visibility = EXCLUDED.visibility,
				last_seen_at = NOW()`,
			readyData.User.ID, acc.Type, acc.ID, acc.Name, acc.Verified, acc.Visibility,
		)
	}

	gm.logger.Info("ready_data_processed",
		"token_id", tokenID,
		"guilds_processed", len(readyData.Guilds),
		"relationships_saved", len(readyData.Relationships),
	)
}

// processReadyGuild processa um guild completo do READY
func (gm *GatewayManager) processReadyGuild(ctx context.Context, guild *ReadyGuild, tokenID int64) {
	// Salvar/atualizar guild
	_, _ = gm.db.Pool.Exec(ctx,
		`INSERT INTO guilds (guild_id, name, icon, banner, owner_id, description, 
			member_count, presence_count, premium_tier, premium_subscription_count,
			features, discovered_at, last_updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW(), NOW())
		 ON CONFLICT (guild_id) DO UPDATE SET
			name = EXCLUDED.name,
			icon = EXCLUDED.icon,
			banner = EXCLUDED.banner,
			owner_id = EXCLUDED.owner_id,
			description = EXCLUDED.description,
			member_count = EXCLUDED.member_count,
			presence_count = EXCLUDED.presence_count,
			premium_tier = EXCLUDED.premium_tier,
			premium_subscription_count = EXCLUDED.premium_subscription_count,
			features = EXCLUDED.features,
			last_updated_at = NOW()`,
		guild.ID, guild.Name, guild.Icon, guild.Banner, guild.OwnerID, guild.Description,
		guild.MemberCount, guild.PresenceCount, guild.PremiumTier, guild.PremiumSubscriptionCount,
		guild.Features,
	)

	// Salvar canais
	for _, ch := range guild.Channels {
		_, _ = gm.db.Pool.Exec(ctx,
			`INSERT INTO channels (channel_id, guild_id, name, type, parent_id, position, topic, nsfw, user_limit, discovered_at, last_updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
			 ON CONFLICT (channel_id) DO UPDATE SET
				name = EXCLUDED.name,
				type = EXCLUDED.type,
				parent_id = EXCLUDED.parent_id,
				position = EXCLUDED.position,
				topic = EXCLUDED.topic,
				nsfw = EXCLUDED.nsfw,
				user_limit = EXCLUDED.user_limit,
				last_updated_at = NOW()`,
			ch.ID, guild.ID, ch.Name, ch.Type, ch.ParentID, ch.Position, ch.Topic, ch.NSFW, ch.UserLimit,
		)
	}

	// Salvar roles
	for _, role := range guild.Roles {
		_, _ = gm.db.Pool.Exec(ctx,
			`INSERT INTO roles (role_id, guild_id, name, color, position, permissions, hoist, mentionable, discovered_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
			 ON CONFLICT (role_id) DO UPDATE SET
				name = EXCLUDED.name,
				color = EXCLUDED.color,
				position = EXCLUDED.position,
				permissions = EXCLUDED.permissions,
				hoist = EXCLUDED.hoist,
				mentionable = EXCLUDED.mentionable`,
			role.ID, guild.ID, role.Name, role.Color, role.Position, role.Permissions, role.Hoist, role.Mentionable,
		)
	}

	// Salvar emojis
	for _, emoji := range guild.Emojis {
		_, _ = gm.db.Pool.Exec(ctx,
			`INSERT INTO emojis (emoji_id, guild_id, name, animated, available, discovered_at)
			 VALUES ($1, $2, $3, $4, $5, NOW())
			 ON CONFLICT (emoji_id) DO UPDATE SET
				name = EXCLUDED.name,
				animated = EXCLUDED.animated,
				available = EXCLUDED.available`,
			emoji.ID, guild.ID, emoji.Name, emoji.Animated, emoji.Available,
		)
	}

	// Salvar membros do READY (pode ser parcial)
	for _, member := range guild.Members {
		if member.User.ID == "" {
			continue
		}

		// Salvar usuário
		gm.saveUserFromReady(ctx, member.User.ID, member.User.Username,
			member.User.Discriminator, member.User.GlobalName, member.User.Avatar,
			"", "", 0, 0, member.User.PublicFlags)

		// Salvar como membro do guild
		_, _ = gm.db.Pool.Exec(ctx,
			`INSERT INTO guild_members (guild_id, user_id, token_id, nickname, roles, joined_at, discovered_at, last_seen_at)
			 VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
			 ON CONFLICT (guild_id, user_id, token_id) DO UPDATE SET
				nickname = EXCLUDED.nickname,
				roles = EXCLUDED.roles,
				last_seen_at = NOW()`,
			guild.ID, member.User.ID, tokenID, member.Nick, member.Roles, member.JoinedAt,
		)

		// Se tiver nick diferente, salvar histórico
		if member.Nick != "" {
			_, _ = gm.db.Pool.Exec(ctx,
				`INSERT INTO nickname_history (user_id, guild_id, nickname, changed_at)
				 SELECT $1, $2, $3, NOW()
				 WHERE NOT EXISTS (
					SELECT 1 FROM nickname_history 
					WHERE user_id = $1 AND guild_id = $2 AND nickname = $3
					ORDER BY changed_at DESC LIMIT 1
				 )`,
				member.User.ID, guild.ID, member.Nick,
			)
		}
	}

	// Salvar voice states atuais (quem está em call agora)
	for _, vs := range guild.VoiceStates {
		if vs.UserID == "" || vs.ChannelID == "" {
			continue
		}

		// Garantir usuário existe
		_, _ = gm.db.Pool.Exec(ctx,
			`INSERT INTO users (id) VALUES ($1) ON CONFLICT (id) DO NOTHING`,
			vs.UserID,
		)

		// Criar sessão de voz ativa
		_, _ = gm.db.Pool.Exec(ctx,
			`INSERT INTO voice_sessions (user_id, guild_id, channel_id, joined_at, was_muted, was_deafened, was_streaming, was_video)
			 VALUES ($1, $2, $3, NOW(), $4, $5, $6, $7)
			 ON CONFLICT DO NOTHING`,
			vs.UserID, guild.ID, vs.ChannelID, vs.SelfMute, vs.SelfDeaf, vs.SelfStream, vs.SelfVideo,
		)
	}

	// Salvar presences do guild (status online/offline dos membros)
	for _, presence := range guild.Presences {
		if presence.User.ID == "" {
			continue
		}

		_, _ = gm.db.Pool.Exec(ctx,
			`INSERT INTO presence_history (user_id, guild_id, status, changed_at)
			 VALUES ($1, $2, $3, NOW())`,
			presence.User.ID, guild.ID, presence.Status,
		)

		// Salvar atividades
		for _, act := range presence.Activities {
			if act.Name == "" {
				continue
			}

			var appID *string
			if act.ApplicationID != "" {
				appID = &act.ApplicationID
			}

			_, _ = gm.db.Pool.Exec(ctx,
				`INSERT INTO activity_history 
				 (user_id, activity_type, name, details, state, url, application_id, started_at)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())`,
				presence.User.ID, act.Type, act.Name, act.Details, act.State, act.URL, appID,
			)
		}
	}
}

// saveUserFromReady salva dados de um usuário extraído do READY
func (gm *GatewayManager) saveUserFromReady(ctx context.Context, userID, username, discriminator, globalName, avatar, banner, bio string, accentColor, premiumType, publicFlags int) {
	// Inserir/atualizar usuário
	_, _ = gm.db.Pool.Exec(ctx,
		`INSERT INTO users (id, accent_color, premium_type, public_flags) 
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (id) DO UPDATE SET
			accent_color = COALESCE(EXCLUDED.accent_color, users.accent_color),
			premium_type = COALESCE(EXCLUDED.premium_type, users.premium_type),
			public_flags = COALESCE(EXCLUDED.public_flags, users.public_flags),
			last_updated_at = NOW()`,
		userID, accentColor, premiumType, publicFlags,
	)

	// Salvar username se tiver
	if username != "" {
		var lastUsername, lastDisc, lastGlobal *string
		_ = gm.db.Pool.QueryRow(ctx,
			`SELECT username, discriminator, global_name FROM username_history 
			 WHERE user_id = $1 ORDER BY changed_at DESC LIMIT 1`,
			userID,
		).Scan(&lastUsername, &lastDisc, &lastGlobal)

		needSave := lastUsername == nil || *lastUsername != username
		if !needSave && discriminator != "" && (lastDisc == nil || *lastDisc != discriminator) {
			needSave = true
		}
		if !needSave && globalName != "" && (lastGlobal == nil || *lastGlobal != globalName) {
			needSave = true
		}

		if needSave {
			var discPtr, globalPtr *string
			if discriminator != "" {
				discPtr = &discriminator
			}
			if globalName != "" {
				globalPtr = &globalName
			}
			_, _ = gm.db.Pool.Exec(ctx,
				`INSERT INTO username_history (user_id, username, discriminator, global_name, changed_at)
				 VALUES ($1, $2, $3, $4, NOW())`,
				userID, username, discPtr, globalPtr,
			)
		}
	}

	// Salvar avatar se tiver
	if avatar != "" {
		var lastAvatar string
		_ = gm.db.Pool.QueryRow(ctx,
			`SELECT avatar_hash FROM avatar_history WHERE user_id = $1 ORDER BY changed_at DESC LIMIT 1`,
			userID,
		).Scan(&lastAvatar)

		if lastAvatar != avatar {
			_, _ = gm.db.Pool.Exec(ctx,
				`INSERT INTO avatar_history (user_id, avatar_hash, changed_at) VALUES ($1, $2, NOW())`,
				userID, avatar,
			)
		}
	}

	// Salvar banner se tiver
	if banner != "" {
		var lastBanner *string
		_ = gm.db.Pool.QueryRow(ctx,
			`SELECT banner_hash FROM banner_history WHERE user_id = $1 ORDER BY changed_at DESC LIMIT 1`,
			userID,
		).Scan(&lastBanner)

		if lastBanner == nil || *lastBanner != banner {
			_, _ = gm.db.Pool.Exec(ctx,
				`INSERT INTO banner_history (user_id, banner_hash, changed_at) VALUES ($1, $2, NOW())`,
				userID, banner,
			)
		}
	}

	// Salvar bio se tiver
	if bio != "" {
		var lastBio *string
		_ = gm.db.Pool.QueryRow(ctx,
			`SELECT bio FROM bio_history WHERE user_id = $1 ORDER BY changed_at DESC LIMIT 1`,
			userID,
		).Scan(&lastBio)

		if lastBio == nil || *lastBio != bio {
			_, _ = gm.db.Pool.Exec(ctx,
				`INSERT INTO bio_history (user_id, bio, changed_at) VALUES ($1, $2, NOW())`,
				userID, bio,
			)
		}
	}
}
