// Package api defines HTTP request/response types and handler wiring.
package api

import (
	"encoding/json"
	"time"

	"github.com/andyfcx/observer/server/internal/domain"
)

// ── Agent register / heartbeat ─────────────────────────────────────────────

// RegisterRequest is the payload sent by an agent on first initialization.
type RegisterRequest struct {
	MachineID    string          `json:"machine_id"`
	Hostname     string          `json:"hostname"`
	IPAddress    string          `json:"ip_address"`
	Environment  string          `json:"environment"`
	Tags         []string        `json:"tags"`
	AgentVersion string          `json:"agent_version"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
}

func (r *RegisterRequest) toDomain() *domain.Host {
	return &domain.Host{
		MachineID:    r.MachineID,
		Hostname:     r.Hostname,
		IPAddress:    r.IPAddress,
		Environment:  r.Environment,
		Tags:         r.Tags,
		AgentVersion: r.AgentVersion,
		Metadata:     r.Metadata,
	}
}

// EnrollRequest is the payload sent by an agent during bootstrap enrollment.
type EnrollRequest struct {
	EnrollmentToken string          `json:"enrollment_token"`
	MachineID       string          `json:"machine_id"`
	Hostname        string          `json:"hostname"`
	IPAddress       string          `json:"ip_address"`
	AgentVersion    string          `json:"agent_version"`
	Metadata        json.RawMessage `json:"metadata,omitempty"`
}

func (r *EnrollRequest) toDomain() *domain.Host {
	return &domain.Host{
		MachineID:    r.MachineID,
		Hostname:     r.Hostname,
		IPAddress:    r.IPAddress,
		Environment:  "production",
		Tags:         []string{},
		AgentVersion: r.AgentVersion,
		Metadata:     r.Metadata,
	}
}

// HeartbeatRequest is a lightweight ping from an agent.
type HeartbeatRequest struct {
	IPAddress string `json:"ip_address"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// ── Job discovery ──────────────────────────────────────────────────────────

// DiscoveredJob is a single discovered job entry in the discovery payload.
type DiscoveredJob struct {
	SourceType        string          `json:"source_type"`
	Schedule          string          `json:"schedule"`
	Timezone          string          `json:"timezone"`
	User              string          `json:"user"`
	RawCommand        string          `json:"raw_command"`
	NormalizedCommand string          `json:"normalized_command"`
	CommandHash       string          `json:"command_hash"`
	Enabled           bool            `json:"enabled"`
	SourceFile        string          `json:"source_file"`
	Metadata          json.RawMessage `json:"metadata,omitempty"`
}

// DiscoveryRequest is the payload for POST /api/v1/jobs/discovery.
type DiscoveryRequest struct {
	Jobs []DiscoveredJob `json:"jobs"`
}

func (r *DiscoveryRequest) toDomain() []*domain.Job {
	jobs := make([]*domain.Job, 0, len(r.Jobs))
	for _, j := range r.Jobs {
		jobs = append(jobs, &domain.Job{
			SourceType:        domain.JobSourceType(j.SourceType),
			Schedule:          j.Schedule,
			Timezone:          j.Timezone,
			User:              j.User,
			RawCommand:        j.RawCommand,
			NormalizedCommand: j.NormalizedCommand,
			CommandHash:       j.CommandHash,
			Enabled:           j.Enabled,
			SourceFile:        j.SourceFile,
			Metadata:          j.Metadata,
		})
	}
	return jobs
}

// ── Executions ─────────────────────────────────────────────────────────────

// ExecutionEvent is a single execution entry in the batch payload.
type ExecutionEvent struct {
	JobID               *string         `json:"job_id,omitempty"`
	CommandHash         string          `json:"command_hash,omitempty"`
	ScheduledAt         *time.Time      `json:"scheduled_at,omitempty"`
	DetectedStartedAt   *time.Time      `json:"detected_started_at,omitempty"`
	DetectedFinishedAt  *time.Time      `json:"detected_finished_at,omitempty"`
	DurationSeconds     *float64        `json:"duration_seconds,omitempty"`
	Status              string          `json:"status"`
	ConfidenceScore     float64         `json:"confidence_score"`
	DetectionSources    []string        `json:"detection_sources"`
	Evidence            json.RawMessage `json:"evidence,omitempty"`
}

// ExecutionBatchRequest is the payload for POST /api/v1/executions/batch.
type ExecutionBatchRequest struct {
	Executions []ExecutionEvent `json:"executions"`
}

func (r *ExecutionBatchRequest) toDomain() []*domain.Execution {
	execs := make([]*domain.Execution, 0, len(r.Executions))
	for _, e := range r.Executions {
		ex := &domain.Execution{
			JobID:              e.JobID,
			ScheduledAt:        e.ScheduledAt,
			DetectedStartedAt:  e.DetectedStartedAt,
			DetectedFinishedAt: e.DetectedFinishedAt,
			DurationSeconds:    e.DurationSeconds,
			Status:             domain.ExecutionStatus(e.Status),
			ConfidenceScore:    e.ConfidenceScore,
			DetectionSources:   e.DetectionSources,
			Evidence:           e.Evidence,
		}
		if e.CommandHash != "" {
			ex.Evidence = mergeJSON(ex.Evidence, map[string]any{"command_hash": e.CommandHash})
		}
		if ex.DetectionSources == nil {
			ex.DetectionSources = []string{}
		}
		execs = append(execs, ex)
	}
	return execs
}

type TriggerJobsRequest struct {
	JobIDs []string `json:"job_ids"`
}

type CompleteCommandRequest struct {
	Status     string     `json:"status"`
	Message    string     `json:"message"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	ExitCode   *int       `json:"exit_code,omitempty"`
}

func mergeJSON(existing json.RawMessage, kv map[string]any) json.RawMessage {
	out := map[string]any{}
	if len(existing) > 0 {
		_ = json.Unmarshal(existing, &out)
	}
	for k, v := range kv {
		out[k] = v
	}
	b, _ := json.Marshal(out)
	return b
}

// ── Metrics ────────────────────────────────────────────────────────────────

// MetricEntry is a single metric in the batch payload.
type MetricEntry struct {
	MetricName string          `json:"metric_name"`
	MetricType string          `json:"metric_type"`
	Dimensions json.RawMessage `json:"dimensions,omitempty"`
	Value      float64         `json:"value"`
	MeasuredAt time.Time       `json:"measured_at"`
	Status     string          `json:"status"`
	Evidence   json.RawMessage `json:"evidence,omitempty"`
}

// MetricBatchRequest is the payload for POST /api/v1/metrics/batch.
type MetricBatchRequest struct {
	Metrics []MetricEntry `json:"metrics"`
}

func (r *MetricBatchRequest) toDomain() []*domain.DataMetric {
	metrics := make([]*domain.DataMetric, 0, len(r.Metrics))
	for _, m := range r.Metrics {
		metrics = append(metrics, &domain.DataMetric{
			MetricName: m.MetricName,
			MetricType: domain.MetricType(m.MetricType),
			Dimensions: m.Dimensions,
			Value:      m.Value,
			MeasuredAt: m.MeasuredAt,
			Status:     domain.MetricStatus(m.Status),
			Evidence:   m.Evidence,
		})
	}
	return metrics
}
