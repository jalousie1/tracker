package external

import (
	"context"
	"fmt"
	"log/slog"
)

// UserData representa dados coletados de uma fonte externa
type UserData struct {
	UserID        string
	Username      string
	Discriminator string
	GlobalName    string
	Avatar        string
	Banner        string
	Bio           string
	Source        string
	Confidence    float64 // 0.0 a 1.0
}

// DataSource interface para diferentes fontes de dados
type DataSource interface {
	Name() string
	FetchUser(ctx context.Context, userID string) (*UserData, error)
	Priority() int // menor numero = maior prioridade
}

// SourceManager gerencia múltiplas fontes de dados
type SourceManager struct {
	sources []DataSource
	logger  *slog.Logger
}

func NewSourceManager(logger *slog.Logger) *SourceManager {
	return &SourceManager{
		sources: make([]DataSource, 0),
		logger:  logger,
	}
}

// RegisterSource adiciona uma fonte de dados
func (sm *SourceManager) RegisterSource(source DataSource) {
	sm.sources = append(sm.sources, source)
	// ordenar por prioridade
	for i := 0; i < len(sm.sources)-1; i++ {
		for j := i + 1; j < len(sm.sources); j++ {
			if sm.sources[i].Priority() > sm.sources[j].Priority() {
				sm.sources[i], sm.sources[j] = sm.sources[j], sm.sources[i]
			}
		}
	}
}

// FetchUser tenta buscar usuário em todas as fontes, em ordem de prioridade
func (sm *SourceManager) FetchUser(ctx context.Context, userID string) (*UserData, error) {
	var lastErr error

	for _, source := range sm.sources {
		sm.logger.Debug("trying_source", "source", source.Name(), "user_id", userID)
		data, err := source.FetchUser(ctx, userID)
		if err == nil && data != nil {
			sm.logger.Info("user_found_in_source", "source", source.Name(), "user_id", userID)
			return data, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("user_not_found_in_any_source: %w", lastErr)
}

// PlaceholderSource para futuras implementacoes
type PlaceholderSource struct {
	name     string
	priority int
	logger   *slog.Logger
}

func NewPlaceholderSource(name string, priority int, logger *slog.Logger) *PlaceholderSource {
	return &PlaceholderSource{
		name:     name,
		priority: priority,
		logger:   logger,
	}
}

func (p *PlaceholderSource) Name() string {
	return p.name
}

func (p *PlaceholderSource) Priority() int {
	return p.priority
}

func (p *PlaceholderSource) FetchUser(ctx context.Context, userID string) (*UserData, error) {
	// placeholder - retorna erro para indicar que nao implementado
	return nil, fmt.Errorf("source_not_implemented: %s", p.name)
}
