package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/andyfcx/observer/server/internal/domain"
	"github.com/andyfcx/observer/server/internal/repository"
)

// DiscoveryService processes job discovery reports from agents.
type DiscoveryService struct {
	jobs *repository.JobRepo
}

func NewDiscoveryService(jobs *repository.JobRepo) *DiscoveryService {
	return &DiscoveryService{jobs: jobs}
}

// ProcessDiscovery upserts all discovered jobs for a host.
// Returns the list of resulting job records.
func (s *DiscoveryService) ProcessDiscovery(ctx context.Context, hostID string, discovered []*domain.Job) ([]*domain.Job, error) {
	result := make([]*domain.Job, 0, len(discovered))
	for _, j := range discovered {
		j.HostID = hostID
		saved, err := s.jobs.Upsert(ctx, j)
		if err != nil {
			slog.Warn("failed to upsert job", "host_id", hostID, "hash", j.CommandHash, "err", err)
			continue
		}
		result = append(result, saved)
	}
	slog.Info("processed discovery", "host_id", hostID, "count", len(result))
	return result, nil
}

// GetJob returns a job by ID with basic validation.
func (s *DiscoveryService) GetJob(ctx context.Context, jobID string) (*domain.Job, error) {
	job, err := s.jobs.GetByID(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("get job %q: %w", jobID, err)
	}
	return job, nil
}
