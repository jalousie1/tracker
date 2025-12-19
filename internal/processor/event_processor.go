package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"identity-archive/internal/db"
	"identity-archive/internal/models"
	"identity-archive/internal/redis"
	"identity-archive/internal/storage"
)

type StorageClient interface {
	UploadAvatar(userID string, avatarHash string, imageData []byte) (string, error)
}

type Event struct {
	Type      string
	Data      map[string]interface{}
	Timestamp time.Time
	TokenID   int64
}

type Worker struct {
	ID        int
	processor *EventProcessor
	stopChan  chan bool
}

type EventProcessor struct {
	log        *slog.Logger
	db         *db.DB
	redis      *redis.Client
	storage    storage.StorageClient
	eventQueue chan Event
	workerPool []*Worker
	wg         sync.WaitGroup
	mu         sync.RWMutex
}


func NewEventProcessor(log *slog.Logger, dbConn *db.DB, redisClient *redis.Client, storageClient storage.StorageClient) *EventProcessor {
	ep := &EventProcessor{
		log:        log,
		db:         dbConn,
		redis:      redisClient,
		storage:   storageClient,
		eventQueue: make(chan Event, 10000),
		workerPool: make([]*Worker, 0),
	}

	return ep
}

func (ep *EventProcessor) GetEventQueue() chan Event {
	return ep.eventQueue
}

func (ep *EventProcessor) StartWorkers(workerCount int) {
	if workerCount < 1 {
		workerCount = 5
	}
	if workerCount > 10 {
		workerCount = 10
	}

	ep.mu.Lock()
	defer ep.mu.Unlock()

	for i := 0; i < workerCount; i++ {
		worker := &Worker{
			ID:        i + 1,
			processor: ep,
			stopChan:  make(chan bool, 1),
		}
		ep.workerPool = append(ep.workerPool, worker)

		ep.wg.Add(1)
		go ep.runWorker(worker)
	}

	ep.log.Info("event_workers_started", "count", workerCount)
}

func (ep *EventProcessor) runWorker(worker *Worker) {
	defer ep.wg.Done()

	for {
		select {
		case event := <-ep.eventQueue:
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := ep.ProcessEvent(ctx, event); err != nil {
				ep.log.Warn("event_processing_failed",
					"worker_id", worker.ID,
					"event_type", event.Type,
					"token_id", event.TokenID,
					"error", err,
				)
				// Send to dead letter queue
				ep.sendToDLQ(ctx, event, err.Error())
			}
			cancel()
		case <-worker.stopChan:
			ep.log.Info("worker_stopped", "worker_id", worker.ID)
			return
		}
	}
}

func (ep *EventProcessor) StopWorkers() {
	ep.mu.Lock()
	defer ep.mu.Unlock()

	for _, worker := range ep.workerPool {
		select {
		case worker.stopChan <- true:
		default:
		}
	}

	ep.wg.Wait()
	ep.log.Info("all_workers_stopped")
}

func (ep *EventProcessor) ProcessEvent(ctx context.Context, event Event) error {
	// Check for duplicate events (deduplication)
	dedupKey := fmt.Sprintf("event:dedup:%s:%s:%d", event.Data["user_id"], event.Type, event.TokenID)
	exists, _ := ep.redis.RDB().Exists(ctx, dedupKey).Result()
	if exists > 0 {
		return nil // Duplicate, skip
	}

	// Set dedup key with 2s TTL
	ep.redis.RDB().Set(ctx, dedupKey, "1", 2*time.Second)

	switch event.Type {
	case "USER_UPDATE":
		return ep.HandleUserUpdate(ctx, event)
	case "GUILD_MEMBER_UPDATE":
		return ep.HandleGuildMemberUpdate(ctx, event)
	case "PRESENCE_UPDATE":
		return ep.HandlePresenceUpdate(ctx, event)
	case "GUILD_MEMBERS_CHUNK":
		return ep.HandleGuildMembersChunk(ctx, event)
	case "MESSAGE_CREATE":
		return ep.HandleMessageCreate(ctx, event)
	case "VOICE_STATE_UPDATE":
		return ep.HandleVoiceStateUpdate(ctx, event)
	case "TYPING_START":
		return ep.HandleTypingStart(ctx, event)
	case "GUILD_MEMBER_ADD":
		return ep.HandleGuildMemberAdd(ctx, event)
	default:
		ep.log.Debug("unknown_event_type", "type", event.Type)
		return nil
	}
}

func (ep *EventProcessor) sendToDLQ(ctx context.Context, event Event, errorMsg string) {
	data, _ := json.Marshal(map[string]interface{}{
		"event":     event,
		"error":     errorMsg,
		"timestamp": time.Now(),
	})
	ep.redis.RDB().LPush(ctx, "dlq:events", data)
	ep.redis.RDB().Expire(ctx, "dlq:events", 24*time.Hour)
}

// ProcessUserUpdate aplica diffs e grava no user_history apenas se mudou algo relevante.
func (p *EventProcessor) ProcessUserUpdate(ctx context.Context, ev models.UserUpdateEvent) error {
	if ev.ObservedAt.IsZero() {
		ev.ObservedAt = time.Now()
	}

	// garantir existência do user
	_, err := p.db.Pool.Exec(ctx,
		`INSERT INTO users (id) VALUES ($1)
		 ON CONFLICT (id) DO NOTHING`,
		ev.UserID,
	)
	if err != nil {
		return err
	}

	// buscar último snapshot
	var last models.UserHistoryRow
	row := p.db.Pool.QueryRow(ctx,
		`SELECT id, user_id, username, discriminator, global_name, nickname, avatar_hash, avatar_url, bio_content, observed_at
		 FROM user_history
		 WHERE user_id = $1
		 ORDER BY observed_at DESC, id DESC
		 LIMIT 1`,
		ev.UserID,
	)

	lastFound := true
	if scanErr := row.Scan(
		&last.ID,
		&last.UserID,
		&last.Username,
		&last.Discriminator,
		&last.GlobalName,
		&last.Nickname,
		&last.AvatarHash,
		&last.AvatarURL,
		&last.BioContent,
		&last.ObservedAt,
	); scanErr != nil {
		lastFound = false
	}

	avatarURL := (*string)(nil)
	if ev.AvatarHash != nil && *ev.AvatarHash != "" {
		// se mudou avatar, fazer upload (será implementado nos handlers)
		if !lastFound || last.AvatarHash == nil || *last.AvatarHash != *ev.AvatarHash {
			// Upload será feito nos handlers de eventos
			avatarURL = last.AvatarURL
		} else {
			avatarURL = last.AvatarURL
		}
	}

	changed := !lastFound ||
		!eqPtr(last.Username, ev.Username) ||
		!eqPtr(last.Discriminator, ev.Discriminator) ||
		!eqPtr(last.GlobalName, ev.GlobalName) ||
		!eqPtr(last.Nickname, ev.Nickname) ||
		!eqPtr(last.AvatarHash, ev.AvatarHash) ||
		!eqPtr(last.BioContent, ev.BioContent)

	if !changed {
		return nil
	}

	_, err = p.db.Pool.Exec(ctx,
		`INSERT INTO user_history (user_id, username, discriminator, global_name, nickname, avatar_hash, avatar_url, bio_content, observed_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		ev.UserID,
		ev.Username,
		ev.Discriminator,
		ev.GlobalName,
		ev.Nickname,
		ev.AvatarHash,
		avatarURL,
		ev.BioContent,
		ev.ObservedAt,
	)
	if err != nil {
		return err
	}

	p.log.Info("user_history_recorded", "user_id", ev.UserID)
	return nil
}

func eqPtr(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}


