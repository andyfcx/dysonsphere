// Package service contains business logic that orchestrates repositories.
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/andyfcx/observer/server/internal/domain"
	"github.com/andyfcx/observer/server/internal/repository"
)

// EnrollmentResult is returned when an agent successfully enrolls.
type EnrollmentResult struct {
	Host        *domain.Host
	AgentToken  string
	Environment string
	Tags        []string
}

// AgentService handles agent registration, enrollment, and heartbeat logic.
type AgentService struct {
	hosts           *repository.HostRepo
	credentials     *repository.CredentialRepo
	enrollmentToken string
	mu              sync.Mutex
}

func NewAgentService(hosts *repository.HostRepo, credentials *repository.CredentialRepo, enrollmentToken string) *AgentService {
	return &AgentService{
		hosts:           hosts,
		credentials:     credentials,
		enrollmentToken: enrollmentToken,
	}
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

// Enroll validates a one-time enrollment token, registers the host, and issues a formal credential.
func (s *AgentService) Enroll(ctx context.Context, enrollmentToken string, h *domain.Host) (*EnrollmentResult, error) {
	if err := s.consumeEnrollmentToken(enrollmentToken); err != nil {
		return nil, err
	}

	host, err := s.Register(ctx, h)
	if err != nil {
		return nil, err
	}

	agentToken, err := newAgentToken()
	if err != nil {
		return nil, fmt.Errorf("generate agent credential: %w", err)
	}
	if err := s.credentials.Upsert(ctx, host.ID, hashToken(agentToken)); err != nil {
		return nil, fmt.Errorf("store agent credential: %w", err)
	}

	return &EnrollmentResult{
		Host:        host,
		AgentToken:  agentToken,
		Environment: defaultEnvironment(host.Environment),
		Tags:        host.Tags,
	}, nil
}

// Heartbeat updates the host's last_heartbeat_at and refreshes current state.
func (s *AgentService) Heartbeat(ctx context.Context, hostID, ipAddress string) error {
	if err := s.hosts.UpdateHeartbeat(ctx, hostID, ipAddress); err != nil {
		return fmt.Errorf("heartbeat update: %w", err)
	}
	now := time.Now()
	return s.hosts.UpsertCurrentState(ctx, hostID, domain.HostStatusActive, &now, 0)
}

func (s *AgentService) ValidateCredential(ctx context.Context, hostID, agentToken string) bool {
	if s.credentials == nil {
		return false
	}
	ok, err := s.credentials.Validate(ctx, hostID, hashToken(agentToken))
	return err == nil && ok
}

func (s *AgentService) consumeEnrollmentToken(token string) error {
	if token == "" {
		return fmt.Errorf("enrollment token is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if token != s.enrollmentToken {
		return fmt.Errorf("invalid enrollment token")
	}
	return nil
}

func newAgentToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func defaultEnvironment(value string) string {
	if value == "" || value == "unknown" {
		return "production"
	}
	return value
}
