// Package reporter handles batching and uploading events to the central server.
package reporter

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/andyfcx/observer/agent/internal/client"
	"github.com/andyfcx/observer/agent/internal/state"
)

// Reporter batches pending events from local state and uploads them.
type Reporter struct {
	store  *state.Store
	client *client.Client
}

// NewReporter creates a Reporter.
func NewReporter(store *state.Store, client *client.Client) *Reporter {
	return &Reporter{store: store, client: client}
}

// FlushExecutions uploads all buffered execution events.
func (r *Reporter) FlushExecutions(ctx context.Context) {
	pending, err := r.store.PendingExecutions()
	if err != nil || len(pending) == 0 {
		return
	}

	events := make([]json.RawMessage, 0, len(pending))
	for _, p := range pending {
		events = append(events, p.Payload)
	}

	payload := map[string]any{"executions": events}
	if err := r.client.BatchExecutions(ctx, payload); err != nil {
		slog.Warn("flush executions failed", "count", len(pending), "err", err)
		return
	}

	for _, p := range pending {
		_ = r.store.DeleteExecution(p.ID)
	}
	slog.Info("executions flushed", "count", len(pending))
}

// FlushMetrics uploads all buffered metric events.
func (r *Reporter) FlushMetrics(ctx context.Context) {
	pending, err := r.store.PendingMetrics()
	if err != nil || len(pending) == 0 {
		return
	}

	metrics := make([]json.RawMessage, 0, len(pending))
	for _, p := range pending {
		metrics = append(metrics, p.Payload)
	}

	payload := map[string]any{"metrics": metrics}
	if err := r.client.BatchMetrics(ctx, payload); err != nil {
		slog.Warn("flush metrics failed", "count", len(pending), "err", err)
		return
	}

	for _, p := range pending {
		_ = r.store.DeleteMetric(p.ID)
	}
	slog.Info("metrics flushed", "count", len(pending))
}

// BufferExecution saves an execution event to local state for later upload.
func (r *Reporter) BufferExecution(id string, event any) {
	if err := r.store.SaveExecution(id, event); err != nil {
		slog.Error("buffer execution", "id", id, "err", err)
	}
}

// BufferMetric saves a metric to local state for later upload.
func (r *Reporter) BufferMetric(id string, metric any) {
	if err := r.store.SaveMetric(id, metric); err != nil {
		slog.Error("buffer metric", "id", id, "err", err)
	}
}

// ReportTime is a small helper to produce ISO8601 timestamps.
func ReportTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}
