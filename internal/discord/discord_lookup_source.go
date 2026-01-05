package discord

import (
	"context"
	"log/slog"

	"identity-archive/internal/external"
)

// DiscordLookupSource implementa external.DataSource usando a Discord HTTP API
// Prioriza USER TOKENS sobre bot token para ter acesso a mais dados (bio, banner, etc)
type DiscordLookupSource struct {
	userFetcher *UserFetcher
	logger      *slog.Logger
}

func NewDiscordLookupSource(logger *slog.Logger, userFetcher *UserFetcher) *DiscordLookupSource {
	return &DiscordLookupSource{
		userFetcher: userFetcher,
		logger:      logger,
	}
}

func (d *DiscordLookupSource) Name() string {
	return "discord_user_token"
}

func (d *DiscordLookupSource) Priority() int {
	return 0 // PRIORIDADE MÁXIMA - user tokens são preferidos
}

func (d *DiscordLookupSource) FetchUser(ctx context.Context, userID string) (*external.UserData, error) {
	user, err := d.userFetcher.FetchUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &external.UserData{
		UserID:        user.ID,
		Username:      user.Username,
		Discriminator: user.Discriminator,
		GlobalName:    user.GlobalName,
		Avatar:        user.Avatar,
		Banner:        user.Banner,
		Bio:           user.Bio,
		Source:        "discord_user_token",
		Confidence:    1.0, // máxima confiança - dados direto do Discord
	}, nil
}
