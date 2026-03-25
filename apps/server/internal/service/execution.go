package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/andyfcx/observer/server/internal/domain"
	"github.com/andyfcx/observer/server/internal/repository"
)

// ExecutionService processes execution events from agents.
type ExecutionService struct {
	executions *repository.ExecutionRepo
	jobs       *repository.JobRepo
}

func NewExecutionService(executions *repository.ExecutionRepo, jobs *repository.JobRepo) *ExecutionService {
	return &ExecutionService{executions: executions, jobs: jobs}
}

// BatchReport processes a batch of execution events and updates current job states.
func (s *ExecutionService) BatchReport(ctx context.Context, hostID string, events []*domain.Execution) ([]*domain.Execution, error) {
	result := make([]*domain.Execution, 0, len(events))
	for _, e := range events {
		e.HostID = hostID
		if e.JobID == nil {
			if commandHash := commandHashFromEvidence(e.Evidence); commandHash != "" {
				if job, err := s.jobs.GetByHostAndHash(ctx, hostID, commandHash); err == nil {
					e.JobID = &job.ID
				}
			}
		}
		saved, err := s.executions.Insert(ctx, e)
		if err != nil {
			slog.Warn("failed to insert execution", "host_id", hostID, "err", err)
			continue
		}
		result = append(result, saved)

		// Update current_job_states if the execution is linked to a known job.
		if saved.JobID != nil {
			if err := s.updateJobState(ctx, saved); err != nil {
				slog.Warn("failed to update job state", "job_id", *saved.JobID, "err", err)
			}
		}
	}
	slog.Info("batch executions processed", "host_id", hostID, "count", len(result))
	return result, nil
}

func commandHashFromEvidence(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	hash, _ := payload["command_hash"].(string)
	return hash
}

// updateJobState refreshes the current_job_states table after an execution event.
func (s *ExecutionService) updateJobState(ctx context.Context, e *domain.Execution) error {
	if e.JobID == nil {
		return nil
	}

	// Determine consecutive failure count.
	failCount, err := s.executions.CountRecentFailures(ctx, *e.JobID)
	if err != nil {
		return fmt.Errorf("count failures: %w", err)
	}

	var lastRunAt *time.Time
	if e.DetectedStartedAt != nil {
		lastRunAt = e.DetectedStartedAt
	} else if e.ScheduledAt != nil {
		lastRunAt = e.ScheduledAt
	}

	return s.jobs.UpsertCurrentState(
		ctx, *e.JobID, e.HostID,
		e.Status, lastRunAt, nil,
		int(failCount),
	)
}

// ListByJob returns recent executions for a job.
func (s *ExecutionService) ListByJob(ctx context.Context, jobID string, limit int) ([]*domain.Execution, error) {
	return s.executions.ListByJob(ctx, jobID, limit)
}

// List returns paginated executions.
func (s *ExecutionService) List(ctx context.Context, limit, offset int) ([]*domain.Execution, error) {
	return s.executions.List(ctx, limit, offset)
}
