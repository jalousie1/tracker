package models

import "time"

// DiscordUser representa a estrutura de um usuário do Discord vinda da API/Gateway
type DiscordUser struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	Discriminator string `json:"discriminator"`
	GlobalName    string `json:"global_name"`
	Avatar        string `json:"avatar"`
	Bot           bool   `json:"bot"`
	Bio           string `json:"bio"`
	System        bool   `json:"system"`
	AccentColor   int    `json:"accent_color"`
	Banner        string `json:"banner"`
}

// DiscordMember representa um membro de uma guilda
type DiscordMember struct {
	User     DiscordUser `json:"user"`
	Nick     *string     `json:"nick"`
	Roles    []string    `json:"roles"`
	JoinedAt string      `json:"joined_at"`
}

// DiscordActivity representa uma atividade de presença
type DiscordActivity struct {
	Type          int            `json:"type"`
	Name          string         `json:"name"`
	Details       string         `json:"details"`
	State         string         `json:"state"`
	URL           string         `json:"url"`
	ApplicationID string         `json:"application_id"`
	Assets        map[string]any `json:"assets"`
	SyncID        string         `json:"sync_id"`
}

// DiscordPresence representa um evento de presença
type DiscordPresence struct {
	User       DiscordUser       `json:"user"`
	GuildID    string            `json:"guild_id"`
	Status     string            `json:"status"`
	Activities []DiscordActivity `json:"activities"`
}

// DiscordMessage representa uma mensagem enviada
type DiscordMessage struct {
	ID              string          `json:"id"`
	ChannelID       string          `json:"channel_id"`
	GuildID         string          `json:"guild_id"`
	Author          DiscordUser     `json:"author"`
	Member          *DiscordMember  `json:"member"`
	Content         string          `json:"content"`
	Timestamp       string          `json:"timestamp"`
	EditedTimestamp string          `json:"edited_timestamp"`
	Attachments     []any           `json:"attachments"`
	Embeds          []any           `json:"embeds"`
	Mentions        []DiscordUser   `json:"mentions"`
	Reference       *MessageRef     `json:"message_reference"`
	ReferencedMsg   *DiscordMessage `json:"referenced_message"`
}

type MessageRef struct {
	MessageID string `json:"message_id"`
	ChannelID string `json:"channel_id"`
	GuildID   string `json:"guild_id"`
}

// DiscordVoiceState representa uma mudança no estado de voz
type DiscordVoiceState struct {
	UserID     string         `json:"user_id"`
	GuildID    string         `json:"guild_id"`
	ChannelID  string         `json:"channel_id"`
	SessionID  string         `json:"session_id"`
	SelfMute   bool           `json:"self_mute"`
	SelfDeaf   bool           `json:"self_deaf"`
	SelfVideo  bool           `json:"self_video"`
	SelfStream bool           `json:"self_stream"`
	Member     *DiscordMember `json:"member"`
}

// InternalEvent wrap para o processador
type InternalEvent struct {
	Type      string
	Data      any // Agora usaremos as structs tipadas aqui
	Timestamp time.Time
	TokenID   int64
}
