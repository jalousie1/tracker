package db

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// BatchConfig holds configuration for batch processing operations.
type BatchConfig struct {
	BatchSize  int
	MaxRetries int
	RetryDelay time.Duration
	OnProgress func(processed, total int)
}

// DefaultBatchConfig returns sensible defaults for batch processing.
func DefaultBatchConfig() BatchConfig {
	return BatchConfig{
		BatchSize:  100,
		MaxRetries: 3,
		RetryDelay: 1 * time.Second,
		OnProgress: nil,
	}
}

// BatchInsert performs a batch insert operation with configurable chunk size.
// Returns the total number of rows inserted and any error encountered.
func (d *DB) BatchInsert(ctx context.Context, tableName string, columns []string, values [][]interface{}, cfg BatchConfig) (int, error) {
	if len(values) == 0 {
		return 0, nil
	}

	totalInserted := 0
	totalRows := len(values)

	// Process in batches
	for i := 0; i < len(values); i += cfg.BatchSize {
		end := i + cfg.BatchSize
		if end > len(values) {
			end = len(values)
		}

		batch := values[i:end]
		inserted, err := d.insertBatch(ctx, tableName, columns, batch, cfg.MaxRetries, cfg.RetryDelay)
		if err != nil {
			return totalInserted, fmt.Errorf("batch insert failed at offset %d: %w", i, err)
		}

		totalInserted += inserted

		// Report progress if handler provided
		if cfg.OnProgress != nil {
			cfg.OnProgress(totalInserted, totalRows)
		}
	}

	return totalInserted, nil
}

// insertBatch inserts a single batch with retry logic.
func (d *DB) insertBatch(ctx context.Context, tableName string, columns []string, batch [][]interface{}, maxRetries int, retryDelay time.Duration) (int, error) {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Check context before each attempt
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}

		rowCount, err := d.executeBatchInsert(ctx, tableName, columns, batch)
		if err == nil {
			return rowCount, nil
		}

		lastErr = err
		if attempt < maxRetries-1 {
			time.Sleep(retryDelay)
		}
	}

	return 0, lastErr
}

// executeBatchInsert performs the actual batch insert using COPY.
func (d *DB) executeBatchInsert(ctx context.Context, tableName string, columns []string, batch [][]interface{}) (int, error) {
	// Build column list
	colList := ""
	for i, col := range columns {
		if i > 0 {
			colList += ", "
		}
		colList += col
	}

	// Use CopyFrom for efficient bulk insert
	rowsCopied, err := d.Pool.CopyFrom(
		ctx,
		[]string{tableName},
		columns,
		&batchSource{rows: batch},
	)
	if err != nil {
		return 0, err
	}

	return int(rowsCopied), nil
}

// batchSource implements pgx.CopyFromSource for batch inserts.
type batchSource struct {
	rows  [][]interface{}
	index int
}

func (b *batchSource) Next() bool {
	b.index++
	return b.index <= len(b.rows)
}

func (b *batchSource) Values() ([]interface{}, error) {
	return b.rows[b.index-1], nil
}

func (b *batchSource) Err() error {
	return nil
}

// BatchProcessor provides a high-level API for batch processing with logging.
type BatchProcessor struct {
	db     *DB
	logger *slog.Logger
}

// NewBatchProcessor creates a new batch processor.
func NewBatchProcessor(db *DB, logger *slog.Logger) *BatchProcessor {
	return &BatchProcessor{
		db:     db,
		logger: logger,
	}
}

// ProcessHistoryBatch inserts history records in batches.
// Uses the configured batch size and provides progress logging.
func (bp *BatchProcessor) ProcessHistoryBatch(ctx context.Context, tableName string, columns []string, records [][]interface{}) error {
	if len(records) == 0 {
		return nil
	}

	cfg := DefaultBatchConfig()
	cfg.OnProgress = func(processed, total int) {
		bp.logger.Debug("batch_progress",
			"table", tableName,
			"processed", processed,
			"total", total,
			"percent", (processed*100)/total,
		)
	}

	startTime := time.Now()
	inserted, err := bp.db.BatchInsert(ctx, tableName, columns, records, cfg)
	elapsed := time.Since(startTime)

	if err != nil {
		bp.logger.Error("batch_insert_failed",
			"table", tableName,
			"error", err,
			"inserted", inserted,
			"elapsed", elapsed.String(),
		)
		return err
	}

	bp.logger.Info("batch_insert_complete",
		"table", tableName,
		"rows", inserted,
		"elapsed", elapsed.String(),
		"rate", fmt.Sprintf("%.1f rows/s", float64(inserted)/elapsed.Seconds()),
	)

	return nil
}
