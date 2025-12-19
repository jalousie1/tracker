package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"identity-archive/internal/logging"
)

const (
	gatewayURL = "wss://gateway.discord.gg/?v=10&encoding=json"
)

type GatewayConnection struct {
	TokenID           int64
	Token             string
	Conn              *websocket.Conn
	SessionID         string
	ResumeGatewayURL  string
	LastSequence      int64
	HeartbeatInterval time.Duration
	Connected         bool
	Guilds            []string
	heartbeatTicker   *time.Ticker
	stopChan          chan bool
	mutex             sync.RWMutex
	logger            *slog.Logger
}

type GatewayMessage struct {
	Op int         `json:"op"`
	D  interface{} `json:"d,omitempty"` // pode ser map ou array
	T  string      `json:"t,omitempty"`
	S  int64       `json:"s,omitempty"`
}

type HelloData struct {
	HeartbeatInterval int64 `json:"heartbeat_interval"`
}

type ReadyData struct {
	SessionID        string `json:"session_id"`
	ResumeGatewayURL string `json:"resume_gateway_url"`
	User             struct {
		ID string `json:"id"`
	} `json:"user"`
	Guilds []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"guilds"`
}

func NewGatewayConnection(tokenID int64, token string, logger *slog.Logger) *GatewayConnection {
	return &GatewayConnection{
		TokenID:  tokenID,
		Token:    token,
		logger:   logger,
		stopChan: make(chan bool, 1),
		Guilds:   make([]string, 0),
	}
}

func (gc *GatewayConnection) Connect(ctx context.Context) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 30 * time.Second,
	}

	headers := http.Header{}
	headers.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	headers.Set("Origin", "https://discord.com")
	headers.Set("Accept-Language", "en-US,en;q=0.9")

	conn, _, err := dialer.DialContext(ctx, gatewayURL, headers)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	gc.mutex.Lock()
	gc.Conn = conn
	gc.mutex.Unlock()

	// Read HELLO
	var helloMsg GatewayMessage
	if err := conn.ReadJSON(&helloMsg); err != nil {
		return fmt.Errorf("failed to read HELLO: %w", err)
	}

	if helloMsg.Op != 10 { // HELLO
		return fmt.Errorf("expected HELLO opcode, got %d", helloMsg.Op)
	}

	helloDataBytes, _ := json.Marshal(helloMsg.D)
	var helloData HelloData
	if err := json.Unmarshal(helloDataBytes, &helloData); err != nil {
		return fmt.Errorf("failed to parse HELLO data: %w", err)
	}

	gc.HeartbeatInterval = time.Duration(helloData.HeartbeatInterval) * time.Millisecond

	// Send IDENTIFY
	identifyPayload := map[string]interface{}{
		"op": 2,
		"d": map[string]interface{}{
			"token": gc.Token,
			"properties": map[string]string{
				"$os":      "Windows",
				"$browser": "Chrome",
				"$device":  "PC",
			},
			"compress":        false,
			"large_threshold": 250,
			"presence": map[string]interface{}{
				"status":     "online",
				"since":      0,
				"activities": []interface{}{},
				"afk":        false,
			},
		},
	}

	if err := conn.WriteJSON(identifyPayload); err != nil {
		return fmt.Errorf("failed to send IDENTIFY: %w", err)
	}

	// Read READY
	var readyMsg GatewayMessage
	if err := conn.ReadJSON(&readyMsg); err != nil {
		return fmt.Errorf("failed to read READY: %w", err)
	}

	if readyMsg.Op != 0 || readyMsg.T != "READY" {
		return fmt.Errorf("expected READY event, got op=%d t=%s", readyMsg.Op, readyMsg.T)
	}

	readyDataBytes, _ := json.Marshal(readyMsg.D)
	var readyData ReadyData
	if err := json.Unmarshal(readyDataBytes, &readyData); err != nil {
		return fmt.Errorf("failed to parse READY data: %w", err)
	}

	gc.mutex.Lock()
	gc.SessionID = readyData.SessionID
	gc.ResumeGatewayURL = readyData.ResumeGatewayURL
	gc.Connected = true
	gc.Guilds = make([]string, 0, len(readyData.Guilds))
	for _, guild := range readyData.Guilds {
		gc.Guilds = append(gc.Guilds, guild.ID)
	}
	gc.mutex.Unlock()

	masked := logging.MaskToken(gc.Token)
	gc.logger.Info("gateway_connected",
		"token_id", gc.TokenID,
		"token", masked,
		"session_id", gc.SessionID,
		"guilds_count", len(gc.Guilds),
	)

	return nil
}

func (gc *GatewayConnection) StartHeartbeat() {
	if gc.HeartbeatInterval == 0 {
		return
	}

	gc.heartbeatTicker = time.NewTicker(gc.HeartbeatInterval)
	defer gc.heartbeatTicker.Stop()

	for {
		select {
		case <-gc.heartbeatTicker.C:
			gc.sendHeartbeat()
		case <-gc.stopChan:
			return
		}
	}
}

func (gc *GatewayConnection) sendHeartbeat() {
	gc.mutex.RLock()
	conn := gc.Conn
	seq := gc.LastSequence
	gc.mutex.RUnlock()

	if conn == nil {
		return
	}

	var seqValue interface{} = nil
	if seq > 0 {
		seqValue = seq
	}

	heartbeat := map[string]interface{}{
		"op": 1,
		"d":  seqValue,
	}

	if err := conn.WriteJSON(heartbeat); err != nil {
		gc.logger.Warn("heartbeat_send_failed", "token_id", gc.TokenID, "error", err)
		return
	}

	gc.logger.Debug("heartbeat_sent", "token_id", gc.TokenID, "seq", seq)
}

func (gc *GatewayConnection) Resume(ctx context.Context) error {
	if gc.SessionID == "" || gc.ResumeGatewayURL == "" {
		return fmt.Errorf("cannot resume: missing session_id or resume_gateway_url")
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 30 * time.Second,
	}

	headers := http.Header{}
	headers.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	headers.Set("Origin", "https://discord.com")
	headers.Set("Accept-Language", "en-US,en;q=0.9")

	resumeURL := gc.ResumeGatewayURL + "?v=10&encoding=json"
	conn, _, err := dialer.DialContext(ctx, resumeURL, headers)
	if err != nil {
		return fmt.Errorf("failed to reconnect: %w", err)
	}

	// Read HELLO (Discord sends HELLO first on every new websocket connection)
	var helloMsg GatewayMessage
	if err := conn.ReadJSON(&helloMsg); err != nil {
		_ = conn.Close()
		return fmt.Errorf("failed to read HELLO during resume: %w", err)
	}
	if helloMsg.Op != 10 {
		_ = conn.Close()
		return fmt.Errorf("expected HELLO opcode during resume, got %d", helloMsg.Op)
	}

	helloDataBytes, _ := json.Marshal(helloMsg.D)
	var helloData HelloData
	if err := json.Unmarshal(helloDataBytes, &helloData); err != nil {
		_ = conn.Close()
		return fmt.Errorf("failed to parse HELLO data during resume: %w", err)
	}

	gc.mutex.Lock()
	gc.HeartbeatInterval = time.Duration(helloData.HeartbeatInterval) * time.Millisecond
	gc.mutex.Unlock()

	gc.mutex.Lock()
	gc.Conn = conn
	gc.mutex.Unlock()

	// Send RESUME
	resumePayload := map[string]interface{}{
		"op": 6,
		"d": map[string]interface{}{
			"token":      gc.Token,
			"session_id": gc.SessionID,
			"seq":        gc.LastSequence,
		},
	}

	if err := conn.WriteJSON(resumePayload); err != nil {
		_ = conn.Close()
		return fmt.Errorf("failed to send RESUME: %w", err)
	}

	// Read response (can be DISPATCH/RESUMED or INVALID_SESSION)
	// We'll read a few messages to be tolerant of gateway timing.
	for i := 0; i < 5; i++ {
		var respMsg GatewayMessage
		if err := conn.ReadJSON(&respMsg); err != nil {
			_ = conn.Close()
			return fmt.Errorf("failed to read RESUME response: %w", err)
		}

		if respMsg.Op == 9 { // INVALID_SESSION
			_ = conn.Close()
			return fmt.Errorf("invalid session, need full reconnect")
		}

		if respMsg.Op == 0 && respMsg.T == "RESUMED" {
			gc.mutex.Lock()
			gc.Connected = true
			gc.mutex.Unlock()

			gc.logger.Info("gateway_resumed", "token_id", gc.TokenID, "seq", gc.LastSequence)
			return nil
		}

		// If we see HELLO again here, session isn't resumable / protocol out of sync.
		if respMsg.Op == 10 {
			_ = conn.Close()
			return fmt.Errorf("unexpected HELLO after RESUME, need full reconnect")
		}
	}

	_ = conn.Close()
	return fmt.Errorf("resume did not complete after multiple messages")
}

func (gc *GatewayConnection) Close() error {
	gc.mutex.Lock()
	defer gc.mutex.Unlock()

	gc.Connected = false
	if gc.heartbeatTicker != nil {
		gc.heartbeatTicker.Stop()
	}

	select {
	case gc.stopChan <- true:
	default:
	}

	if gc.Conn != nil {
		return gc.Conn.Close()
	}

	return nil
}

func (gc *GatewayConnection) SendRequestGuildMembers(guildID string) error {
	gc.mutex.RLock()
	conn := gc.Conn
	gc.mutex.RUnlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	// ESTRATEGIA PARA USER TOKENS:
	// User tokens nao podem fazer query vazia, entao fazemos scraping alfabetico
	// Vamos fazer multiplas requests com queries de A-Z, 0-9, etc
	// Isso simula o comportamento de busca na lista de membros do Discord

	// Por enquanto, fazer request com query vazia ainda (pode funcionar em alguns casos)
	payload := map[string]interface{}{
		"op": 8,
		"d": map[string]interface{}{
			"guild_id":  guildID,
			"query":     "",    // query vazia (pode nao funcionar para user tokens)
			"limit":     100,   // limite de 100 membros por request
			"presences": false, // sem presences
		},
	}

	return conn.WriteJSON(payload)
}

// SendRequestGuildMembersWithQuery faz request de membros com query especifica
// Isso funciona melhor com user tokens, simulando busca na lista de membros
func (gc *GatewayConnection) SendRequestGuildMembersWithQuery(guildID, query string, limit int) error {
	return gc.SendRequestGuildMembersWithQueryAndNonce(guildID, query, limit, "")
}

// SendRequestGuildMembersWithQueryAndNonce faz request de membros com query e nonce especificos
// O nonce permite rastrear chunks de uma sessao de scraping especifica
func (gc *GatewayConnection) SendRequestGuildMembersWithQueryAndNonce(guildID, query string, limit int, nonce string) error {
	gc.mutex.RLock()
	conn := gc.Conn
	gc.mutex.RUnlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	d := map[string]interface{}{
		"guild_id":  guildID,
		"query":     query,
		"limit":     limit,
		"presences": false,
	}

	// Adicionar nonce se fornecido (permite rastrear chunks por sessao)
	if nonce != "" {
		d["nonce"] = nonce
	}

	payload := map[string]interface{}{
		"op": 8,
		"d":  d,
	}

	return conn.WriteJSON(payload)
}

// RequestGuildSubscriptions solicita subscricao para eventos de um guild
// Isso faz com que o Discord envie PRESENCE_UPDATE, MESSAGE_CREATE, etc
func (gc *GatewayConnection) RequestGuildSubscriptions(guildID string, channels map[string][][2]int) error {
	gc.mutex.RLock()
	conn := gc.Conn
	gc.mutex.RUnlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	// Opcode 14 = REQUEST_GUILD_SUBSCRIPTIONS
	// Isso faz o Discord enviar eventos de membros online em canais especificos
	payload := map[string]interface{}{
		"op": 14,
		"d": map[string]interface{}{
			"guild_id": guildID,
			"channels": channels,
		},
	}

	return conn.WriteJSON(payload)
}

func (gc *GatewayConnection) GetGuilds() []string {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()
	result := make([]string, len(gc.Guilds))
	copy(result, gc.Guilds)
	return result
}
