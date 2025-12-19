package discord

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"identity-archive/internal/db"
	"identity-archive/internal/logging"
)

// TokenStatus moved to token_manager.go

type Token struct {
	ID    int64
	Token string
}

type FailureReason string

const (
	FailureUnauthorized FailureReason = "unauthorized"
	FailureForbidden    FailureReason = "forbidden"
	FailureRateLimited  FailureReason = "rate_limited"
	FailureUnknown      FailureReason = "unknown"
)

// WorkerManager é um stub seguro: ele NÃO conecta no gateway do Discord.
// Ele existe para gerenciar rotação/failover de múltiplos tokens com política de log segura.
type WorkerManager struct {
	log *slog.Logger
	db  *db.DB
}

func NewWorkerManager(log *slog.Logger, dbConn *db.DB) *WorkerManager {
	return &WorkerManager{
		log: log,
		db:  dbConn,
	}
}

// NextToken retorna o próximo token ativo disponível.
func (m *WorkerManager) NextToken(ctx context.Context) (*Token, error) {
	var t Token
	err := m.db.Pool.QueryRow(ctx,
		`SELECT id, token
		 FROM tokens
		 WHERE status = $1
		 ORDER BY created_at ASC, id ASC
		 LIMIT 1`,
		"ativo",
	).Scan(&t.ID, &t.Token)
	if err != nil {
		return nil, errors.New("no_active_token_available")
	}
	return &t, nil
}

// ReportFailure marca token como suspenso/banido dependendo do motivo.
func (m *WorkerManager) ReportFailure(ctx context.Context, tok *Token, reason FailureReason) {
	if tok == nil {
		return
	}

	var newStatus string
	switch reason {
	case FailureUnauthorized, FailureForbidden:
		newStatus = "banido"
	case FailureRateLimited:
		newStatus = "suspenso"
	default:
		newStatus = "suspenso"
	}

	masked := logging.MaskToken(tok.Token)
	m.log.Warn("token_failover", "token_id", tok.ID, "token", masked, "reason", string(reason), "new_status", string(newStatus))

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, _ = m.db.Pool.Exec(ctx,
		`UPDATE tokens SET status = $1 WHERE id = $2`,
		string(newStatus),
		tok.ID,
	)
}

func SanitizeReason(s string) FailureReason {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "401", "unauthorized":
		return FailureUnauthorized
	case "403", "forbidden":
		return FailureForbidden
	case "429", "rate_limited":
		return FailureRateLimited
	default:
		return FailureUnknown
	}
}


