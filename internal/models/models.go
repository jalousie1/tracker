package models

import "time"

type User struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type UserHistoryRow struct {
	ID           int64     `json:"id"`
	UserID       string    `json:"user_id"`
	Username     *string   `json:"username,omitempty"`
	Discriminator *string  `json:"discriminator,omitempty"`
	GlobalName   *string   `json:"global_name,omitempty"`
	Nickname     *string   `json:"nickname,omitempty"`
	AvatarHash   *string   `json:"avatar_hash,omitempty"`
	AvatarURL    *string   `json:"avatar_url,omitempty"`
	BioContent   *string   `json:"bio_content,omitempty"`
	ObservedAt   time.Time `json:"observed_at"`
}

type ConnectedAccount struct {
	ID         int64     `json:"id"`
	UserID     string    `json:"user_id"`
	Type       string    `json:"type"`
	ExternalID *string   `json:"external_id,omitempty"`
	Name       *string   `json:"name,omitempty"`
	ObservedAt time.Time `json:"observed_at"`
}

// eventos internos (stub)
type UserUpdateEvent struct {
	UserID       string
	Username     *string
	Discriminator *string
	GlobalName   *string
	Nickname     *string
	AvatarHash   *string
	BioContent   *string
	ObservedAt   time.Time
}


