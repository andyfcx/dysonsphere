// Package service contains business logic that orchestrates repositories.
package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/andyfcx/observer/server/internal/domain"
	"github.com/andyfcx/observer/server/internal/repository"
)

// AgentService handles agent registration and heartbeat logic.
type AgentService struct {
	hosts *repository.HostRepo
}

func NewAgentService(hosts *repository.HostRepo) *AgentService {
	return &AgentService{hosts: hosts}
}

// Register upserts a host record and initializes its current state.
func (s *AgentService) Register(ctx context.Context, h *domain.Host) (*domain.Host, error) {
	host, err := s.hosts.Upsert(ctx, h)
	if err != nil {
		return nil, fmt.Errorf("register host: %w", err)
	}
	now := time.Now()
	if err := s.hosts.UpsertCurrentState(ctx, host.ID, domain.HostStatusActive, &now, 0); err != nil {
		slog.Warn("failed to upsert host state after register", "host_id", host.ID, "err", err)
	}
	slog.Info("host registered", "host_id", host.ID, "hostname", host.Hostname)
	return host, nil
}

// Heartbeat updates the host's last_heartbeat_at and refreshes current state.
func (s *AgentService) Heartbeat(ctx context.Context, hostID, ipAddress string) error {
	if err := s.hosts.UpdateHeartbeat(ctx, hostID, ipAddress); err != nil {
		return fmt.Errorf("heartbeat update: %w", err)
	}
	now := time.Now()
	return s.hosts.UpsertCurrentState(ctx, hostID, domain.HostStatusActive, &now, 0)
}
