package service

import (
	"context"
	"fmt"
	"time"

	"github.com/andyfcx/observer/server/internal/domain"
	"github.com/andyfcx/observer/server/internal/repository"
)

// CommandService manages immediate execution requests from the server to agents.
type CommandService struct {
	commands *repository.CommandRepo
	jobs     *repository.JobRepo
}

func NewCommandService(commands *repository.CommandRepo, jobs *repository.JobRepo) *CommandService {
	return &CommandService{commands: commands, jobs: jobs}
}

func (s *CommandService) EnqueueRuns(ctx context.Context, hostID string, jobIDs []string) ([]*domain.AgentCommand, error) {
	result := make([]*domain.AgentCommand, 0, len(jobIDs))
	for _, jobID := range jobIDs {
		job, err := s.jobs.GetByID(ctx, jobID)
		if err != nil {
			return nil, fmt.Errorf("get job %s: %w", jobID, err)
		}
		if job.HostID != hostID {
			return nil, fmt.Errorf("job %s does not belong to host %s", jobID, hostID)
		}
		cmd, err := s.commands.Insert(ctx, &domain.AgentCommand{
			HostID:      hostID,
			JobID:       job.ID,
			CommandHash: job.CommandHash,
			RawCommand:  job.RawCommand,
			Schedule:    job.Schedule,
			Status:      domain.AgentCommandStatusPending,
			Message:     "queued by server",
		})
		if err != nil {
			return nil, err
		}
		result = append(result, cmd)
	}
	return result, nil
}

func (s *CommandService) ClaimPending(ctx context.Context, hostID string, limit int) ([]*domain.AgentCommand, error) {
	return s.commands.ClaimPending(ctx, hostID, limit)
}

func (s *CommandService) Complete(ctx context.Context, id string, reqStatus, message string, startedAt, finishedAt *time.Time, exitCode *int) error {
	status := domain.AgentCommandStatus(reqStatus)
	if status != domain.AgentCommandStatusSuccess && status != domain.AgentCommandStatusFailed {
		return fmt.Errorf("invalid command status %q", reqStatus)
	}
	return s.commands.Complete(ctx, id, status, message, startedAt, finishedAt, exitCode)
}
