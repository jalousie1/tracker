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
	UserID            string // ID do usuário do token
	Conn              *websocket.Conn
	SessionID         string
	ResumeGatewayURL  string
	LastSequence      int64
	HeartbeatInterval time.Duration
	Connected         bool
	Guilds            []GuildInfo
	ReadyData         *ReadyData // Dados completos do READY para processar depois
	// When scraping guild members, request presence snapshots in chunks (heavier payload).
	RequestPresences bool
	heartbeatTicker  *time.Ticker
	stopChan         chan bool
	mutex            sync.RWMutex
	logger           *slog.Logger
}

type GuildInfo struct {
	ID                       string
	Name                     string
	Icon                     string
	Banner                   string
	OwnerID                  string
	Description              string
	MemberCount              int
	PresenceCount            int
	PremiumTier              int
	PremiumSubscriptionCount int
	Features                 []string
	Channels                 []ChannelInfo
	Roles                    []RoleInfo
	VoiceStates              []VoiceStateInfo
}

type ChannelInfo struct {
	ID        string
	Type      int // 0=text, 2=voice, 4=category, etc
	Name      string
	ParentID  string
	Position  int
	Topic     string
	NSFW      bool
	UserLimit int
}

type RoleInfo struct {
	ID          string
	Name        string
	Color       int
	Position    int
	Permissions string
	Hoist       bool
	Mentionable bool
}

type VoiceStateInfo struct {
	ChannelID  string
	UserID     string
	SelfMute   bool
	SelfDeaf   bool
	SelfStream bool
	SelfVideo  bool
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

// ReadyData contém todos os dados do evento READY para user tokens
// User tokens recebem MUITO mais dados que bots no READY
type ReadyData struct {
	SessionID        string `json:"session_id"`
	ResumeGatewayURL string `json:"resume_gateway_url"`
	User             struct {
		ID            string `json:"id"`
		Username      string `json:"username"`
		Discriminator string `json:"discriminator"`
		GlobalName    string `json:"global_name"`
		Avatar        string `json:"avatar"`
		Banner        string `json:"banner"`
		AccentColor   int    `json:"accent_color"`
		Bio           string `json:"bio"`
		PremiumType   int    `json:"premium_type"`
		PublicFlags   int    `json:"public_flags"`
		Email         string `json:"email"`
		Verified      bool   `json:"verified"`
		Phone         string `json:"phone"`
		MfaEnabled    bool   `json:"mfa_enabled"`
	} `json:"user"`
	Guilds []ReadyGuild `json:"guilds"`
	// User tokens recebem relacionamentos (amigos)
	Relationships []struct {
		ID       string `json:"id"`
		Type     int    `json:"type"` // 1=friend, 2=blocked, 3=incoming_request, 4=outgoing_request
		Nickname string `json:"nickname"`
		User     struct {
			ID            string `json:"id"`
			Username      string `json:"username"`
			Discriminator string `json:"discriminator"`
			GlobalName    string `json:"global_name"`
			Avatar        string `json:"avatar"`
			PublicFlags   int    `json:"public_flags"`
		} `json:"user"`
	} `json:"relationships"`
	// Presences dos amigos
	Presences []struct {
		UserID     string `json:"user_id"`
		Status     string `json:"status"`
		Activities []struct {
			Name    string `json:"name"`
			Type    int    `json:"type"`
			Details string `json:"details"`
			State   string `json:"state"`
		} `json:"activities"`
	} `json:"presences"`
	// Private channels (DMs)
	PrivateChannels []struct {
		ID         string   `json:"id"`
		Type       int      `json:"type"`
		Recipients []string `json:"recipient_ids"`
	} `json:"private_channels"`
	// Connected accounts
	ConnectedAccounts []struct {
		Type       string `json:"type"`
		ID         string `json:"id"`
		Name       string `json:"name"`
		Verified   bool   `json:"verified"`
		Visibility int    `json:"visibility"`
	} `json:"connected_accounts"`
}

// ReadyGuild representa um guild completo recebido no READY
type ReadyGuild struct {
	ID                       string            `json:"id"`
	Name                     string            `json:"name"`
	Icon                     string            `json:"icon"`
	Banner                   string            `json:"banner"`
	Splash                   string            `json:"splash"`
	OwnerID                  string            `json:"owner_id"`
	Description              string            `json:"description"`
	VanityURLCode            string            `json:"vanity_url_code"`
	PremiumTier              int               `json:"premium_tier"`
	PremiumSubscriptionCount int               `json:"premium_subscription_count"`
	MemberCount              int               `json:"member_count"`
	PresenceCount            int               `json:"presence_count"`
	MaxMembers               int               `json:"max_members"`
	Features                 []string          `json:"features"`
	Channels                 []ReadyChannel    `json:"channels"`
	Roles                    []ReadyRole       `json:"roles"`
	Emojis                   []ReadyEmoji      `json:"emojis"`
	Members                  []ReadyMember     `json:"members"` // membros iniciais (pode ser parcial)
	VoiceStates              []ReadyVoiceState `json:"voice_states"`
	Presences                []ReadyPresence   `json:"presences"`
	Threads                  []ReadyChannel    `json:"threads"`
}

type ReadyChannel struct {
	ID                   string `json:"id"`
	Type                 int    `json:"type"`
	Name                 string `json:"name"`
	Position             int    `json:"position"`
	ParentID             string `json:"parent_id"`
	Topic                string `json:"topic"`
	NSFW                 bool   `json:"nsfw"`
	Bitrate              int    `json:"bitrate"`
	UserLimit            int    `json:"user_limit"`
	RateLimitPerUser     int    `json:"rate_limit_per_user"`
	PermissionOverwrites []struct {
		ID    string `json:"id"`
		Type  int    `json:"type"`
		Allow string `json:"allow"`
		Deny  string `json:"deny"`
	} `json:"permission_overwrites"`
}

type ReadyRole struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Color       int    `json:"color"`
	Hoist       bool   `json:"hoist"`
	Position    int    `json:"position"`
	Permissions string `json:"permissions"`
	Managed     bool   `json:"managed"`
	Mentionable bool   `json:"mentionable"`
	Icon        string `json:"icon"`
}

type ReadyEmoji struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Roles         []string `json:"roles"`
	RequireColons bool     `json:"require_colons"`
	Managed       bool     `json:"managed"`
	Animated      bool     `json:"animated"`
	Available     bool     `json:"available"`
}

type ReadyMember struct {
	User struct {
		ID            string `json:"id"`
		Username      string `json:"username"`
		Discriminator string `json:"discriminator"`
		GlobalName    string `json:"global_name"`
		Avatar        string `json:"avatar"`
		Bot           bool   `json:"bot"`
		PublicFlags   int    `json:"public_flags"`
	} `json:"user"`
	Nick         string   `json:"nick"`
	Roles        []string `json:"roles"`
	JoinedAt     string   `json:"joined_at"`
	PremiumSince string   `json:"premium_since"`
	Deaf         bool     `json:"deaf"`
	Mute         bool     `json:"mute"`
	Avatar       string   `json:"avatar"` // guild-specific avatar
}

type ReadyVoiceState struct {
	ChannelID  string `json:"channel_id"`
	UserID     string `json:"user_id"`
	SessionID  string `json:"session_id"`
	Deaf       bool   `json:"deaf"`
	Mute       bool   `json:"mute"`
	SelfDeaf   bool   `json:"self_deaf"`
	SelfMute   bool   `json:"self_mute"`
	SelfStream bool   `json:"self_stream"`
	SelfVideo  bool   `json:"self_video"`
	Suppress   bool   `json:"suppress"`
}

type ReadyPresence struct {
	User struct {
		ID string `json:"id"`
	} `json:"user"`
	Status     string `json:"status"`
	Activities []struct {
		Name          string `json:"name"`
		Type          int    `json:"type"`
		Details       string `json:"details"`
		State         string `json:"state"`
		URL           string `json:"url"`
		ApplicationID string `json:"application_id"`
	} `json:"activities"`
	ClientStatus struct {
		Desktop string `json:"desktop"`
		Mobile  string `json:"mobile"`
		Web     string `json:"web"`
	} `json:"client_status"`
}

func NewGatewayConnection(tokenID int64, token string, logger *slog.Logger) *GatewayConnection {
	return &GatewayConnection{
		TokenID:          tokenID,
		Token:            token,
		logger:           logger,
		stopChan:         make(chan bool, 1),
		Guilds:           make([]GuildInfo, 0),
		RequestPresences: true,
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

	// Send IDENTIFY - para user tokens, precisa parecer um cliente Discord real
	// NÃO usamos intents (isso é só para bots), mas precisamos das capabilities corretas
	identifyPayload := map[string]interface{}{
		"op": 2,
		"d": map[string]interface{}{
			"token":        gc.Token,
			"capabilities": 16381, // Capabilities do cliente Discord real (habilita todos os eventos)
			"properties": map[string]interface{}{
				"os":                       "Windows",
				"browser":                  "Chrome",
				"device":                   "",
				"system_locale":            "en-US",
				"browser_user_agent":       "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
				"browser_version":          "120.0.0.0",
				"os_version":               "10",
				"referrer":                 "",
				"referring_domain":         "",
				"referrer_current":         "",
				"referring_domain_current": "",
				"release_channel":          "stable",
				"client_build_number":      254573,
				"client_event_source":      nil,
			},
			"presence": map[string]interface{}{
				"status":     "online",
				"since":      0,
				"activities": []interface{}{},
				"afk":        false,
			},
			"compress": false,
			"client_state": map[string]interface{}{
				"guild_versions":              map[string]interface{}{},
				"highest_last_message_id":     "0",
				"read_state_version":          0,
				"user_guild_settings_version": -1,
				"user_settings_version":       -1,
				"private_channels_version":    "0",
				"api_code_version":            0,
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
	gc.UserID = readyData.User.ID
	gc.Connected = true
	gc.ReadyData = &readyData // Salvar dados completos do READY
	gc.Guilds = make([]GuildInfo, 0, len(readyData.Guilds))

	for _, guild := range readyData.Guilds {
		// Processar canais
		channels := make([]ChannelInfo, 0, len(guild.Channels))
		for _, ch := range guild.Channels {
			channels = append(channels, ChannelInfo{
				ID:        ch.ID,
				Type:      ch.Type,
				Name:      ch.Name,
				ParentID:  ch.ParentID,
				Position:  ch.Position,
				Topic:     ch.Topic,
				NSFW:      ch.NSFW,
				UserLimit: ch.UserLimit,
			})
		}

		// Processar roles
		roles := make([]RoleInfo, 0, len(guild.Roles))
		for _, r := range guild.Roles {
			roles = append(roles, RoleInfo{
				ID:          r.ID,
				Name:        r.Name,
				Color:       r.Color,
				Position:    r.Position,
				Permissions: r.Permissions,
				Hoist:       r.Hoist,
				Mentionable: r.Mentionable,
			})
		}

		// Processar voice states atuais
		voiceStates := make([]VoiceStateInfo, 0, len(guild.VoiceStates))
		for _, vs := range guild.VoiceStates {
			voiceStates = append(voiceStates, VoiceStateInfo{
				ChannelID:  vs.ChannelID,
				UserID:     vs.UserID,
				SelfMute:   vs.SelfMute,
				SelfDeaf:   vs.SelfDeaf,
				SelfStream: vs.SelfStream,
				SelfVideo:  vs.SelfVideo,
			})
		}

		gc.Guilds = append(gc.Guilds, GuildInfo{
			ID:                       guild.ID,
			Name:                     guild.Name,
			Icon:                     guild.Icon,
			Banner:                   guild.Banner,
			OwnerID:                  guild.OwnerID,
			Description:              guild.Description,
			MemberCount:              guild.MemberCount,
			PresenceCount:            guild.PresenceCount,
			PremiumTier:              guild.PremiumTier,
			PremiumSubscriptionCount: guild.PremiumSubscriptionCount,
			Features:                 guild.Features,
			Channels:                 channels,
			Roles:                    roles,
			VoiceStates:              voiceStates,
		})
	}
	gc.mutex.Unlock()

	masked := logging.MaskToken(gc.Token)

	// Extrair nomes das guilds para log
	guildNames := make([]string, 0, len(gc.Guilds))
	for _, g := range gc.Guilds {
		if g.Name != "" {
			guildNames = append(guildNames, g.Name)
		}
	}
	// Mostrar até 5 guilds no log
	displayNames := guildNames
	if len(displayNames) > 5 {
		displayNames = displayNames[:5]
	}

	gc.logger.Info("gateway_connected",
		"token_id", gc.TokenID,
		"token", masked,
		"session_id", gc.SessionID,
		"user_id", gc.UserID,
		"guilds_count", len(gc.Guilds),
		"guild_names", displayNames,
		"relationships_count", len(readyData.Relationships),
		"private_channels_count", len(readyData.PrivateChannels),
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
		gc.logger.Debug("heartbeat_send_failed", "token_id", gc.TokenID, "error", err)
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
			"query":     "",  // query vazia (pode nao funcionar para user tokens)
			"limit":     100, // limite de 100 membros por request
			"presences": gc.RequestPresences,
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
		"presences": gc.RequestPresences,
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
	for i, g := range gc.Guilds {
		result[i] = g.ID
	}
	return result
}

func (gc *GatewayConnection) GetGuildInfos() []GuildInfo {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()
	result := make([]GuildInfo, len(gc.Guilds))
	copy(result, gc.Guilds)
	return result
}

// GetGuildChannels retorna os canais de um guild especifico
func (gc *GatewayConnection) GetGuildChannels(guildID string) []ChannelInfo {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()
	for _, g := range gc.Guilds {
		if g.ID == guildID {
			result := make([]ChannelInfo, len(g.Channels))
			copy(result, g.Channels)
			return result
		}
	}
	return nil
}
