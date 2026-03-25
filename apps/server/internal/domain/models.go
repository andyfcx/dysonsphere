// Package domain defines the core domain types for the observer system.
// These are the canonical models shared across API, service, and repository layers.
package domain

import (
	"encoding/json"
	"time"
)

// HostStatus represents the operational status of a monitored host.
type HostStatus string

const (
	HostStatusActive  HostStatus = "active"
	HostStatusStale   HostStatus = "stale"
	HostStatusOffline HostStatus = "offline"
	HostStatusUnknown HostStatus = "unknown"
)

// Host represents a monitored Linux machine.
type Host struct {
	ID              string          `json:"id"`
	MachineID       string          `json:"machine_id"`
	Hostname        string          `json:"hostname"`
	IPAddress       string          `json:"ip_address"`
	Environment     string          `json:"environment"`
	Tags            []string        `json:"tags"`
	RegisteredAt    time.Time       `json:"registered_at"`
	LastHeartbeatAt *time.Time      `json:"last_heartbeat_at,omitempty"`
	Status          HostStatus      `json:"status"`
	AgentVersion    string          `json:"agent_version"`
	Metadata        json.RawMessage `json:"metadata,omitempty"`
}

// JobSourceType describes how a job was discovered.
type JobSourceType string

const (
	JobSourceCron          JobSourceType = "cron"
	JobSourceSystemdTimer  JobSourceType = "systemd_timer"
	JobSourceManual        JobSourceType = "manual"
	JobSourceDiscovered    JobSourceType = "discovered"
)

// Job represents a discovered scheduled task on a host.
type Job struct {
	ID                string          `json:"id"`
	HostID            string          `json:"host_id"`
	SourceType        JobSourceType   `json:"source_type"`
	Schedule          string          `json:"schedule"`          // cron expression
	Timezone          string          `json:"timezone"`
	User              string          `json:"user"`
	RawCommand        string          `json:"raw_command"`
	NormalizedCommand string          `json:"normalized_command"`
	CommandHash       string          `json:"command_hash"`
	Enabled           bool            `json:"enabled"`
	SourceFile        string          `json:"source_file"`
	Metadata          json.RawMessage `json:"metadata,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// ExecutionStatus describes the observed status of a job execution.
// Because we observe externally without modifying the cronjob, status may be inferred.
type ExecutionStatus string

const (
	ExecutionStatusRunning   ExecutionStatus = "running"
	ExecutionStatusSuccess   ExecutionStatus = "success"   // inferred: process exited cleanly
	ExecutionStatusFailed    ExecutionStatus = "failed"    // inferred: process exited with error
	ExecutionStatusUnknown   ExecutionStatus = "unknown"   // cannot determine
	ExecutionStatusPartial   ExecutionStatus = "partial"   // started but unclear finish
	ExecutionStatusMissed    ExecutionStatus = "missed"    // expected but not seen
)

// Execution represents an observed run of a scheduled job.
// NOTE: Without modifying the original cronjob, exit codes are not directly available.
// Status here is inferred from process lifecycle observations. confidence_score
// (0.0–1.0) and evidence_json document the basis for the inferred status.
type Execution struct {
	ID                  string          `json:"id"`
	HostID              string          `json:"host_id"`
	JobID               *string         `json:"job_id,omitempty"`  // nil if unmatched
	ScheduledAt         *time.Time      `json:"scheduled_at,omitempty"`
	DetectedStartedAt   *time.Time      `json:"detected_started_at,omitempty"`
	DetectedFinishedAt  *time.Time      `json:"detected_finished_at,omitempty"`
	DurationSeconds     *float64        `json:"duration_seconds,omitempty"`
	Status              ExecutionStatus `json:"status"`
	ConfidenceScore     float64         `json:"confidence_score"` // 0.0–1.0
	DetectionSources    []string        `json:"detection_sources"`
	Evidence            json.RawMessage `json:"evidence,omitempty"`
	CreatedAt           time.Time       `json:"created_at"`
}

// MetricType categorizes what kind of data metric was collected.
type MetricType string

const (
	MetricTypeCount     MetricType = "count"
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeBoolean   MetricType = "boolean"
	MetricTypeString    MetricType = "string"
)

// MetricStatus describes whether the metric value is within expected bounds.
type MetricStatus string

const (
	MetricStatusOK       MetricStatus = "ok"
	MetricStatusWarning  MetricStatus = "warning"
	MetricStatusCritical MetricStatus = "critical"
	MetricStatusUnknown  MetricStatus = "unknown"
)

// DataMetric represents a data quality or volume probe result.
type DataMetric struct {
	ID         string          `json:"id"`
	HostID     string          `json:"host_id"`
	MetricName string          `json:"metric_name"`
	MetricType MetricType      `json:"metric_type"`
	Dimensions json.RawMessage `json:"dimensions,omitempty"` // arbitrary k/v labels
	Value      float64         `json:"value"`
	MeasuredAt time.Time       `json:"measured_at"`
	Status     MetricStatus    `json:"status"`
	Evidence   json.RawMessage `json:"evidence,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}

// AlertSeverity represents the urgency of an alert.
type AlertSeverity string

const (
	AlertSeverityInfo     AlertSeverity = "info"
	AlertSeverityWarning  AlertSeverity = "warning"
	AlertSeverityCritical AlertSeverity = "critical"
)

// AlertStatus is the lifecycle state of an alert.
type AlertStatus string

const (
	AlertStatusActive   AlertStatus = "active"
	AlertStatusResolved AlertStatus = "resolved"
)

// Alert represents a triggered monitoring alert.
type Alert struct {
	ID          string        `json:"id"`
	TargetType  string        `json:"target_type"`  // "host", "job", "metric"
	TargetID    string        `json:"target_id"`
	RuleName    string        `json:"rule_name"`
	Severity    AlertSeverity `json:"severity"`
	Status      AlertStatus   `json:"status"`
	Message     string        `json:"message"`
	TriggeredAt time.Time     `json:"triggered_at"`
	ResolvedAt  *time.Time    `json:"resolved_at,omitempty"`
}

// CurrentJobState is a materialized view of a job's latest observed state,
// used for fast dashboard queries without scanning all execution records.
type CurrentJobState struct {
	JobID           string          `json:"job_id"`
	HostID          string          `json:"host_id"`
	LastStatus      ExecutionStatus `json:"last_status"`
	LastRunAt       *time.Time      `json:"last_run_at,omitempty"`
	NextExpectedAt  *time.Time      `json:"next_expected_at,omitempty"`
	ConsecutiveFail int             `json:"consecutive_fail"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// CurrentHostState is a materialized view of a host's latest state.
type CurrentHostState struct {
	HostID          string     `json:"host_id"`
	Status          HostStatus `json:"status"`
	LastHeartbeatAt *time.Time `json:"last_heartbeat_at,omitempty"`
	ActiveJobs      int        `json:"active_jobs"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// AgentCommandStatus tracks a remotely requested immediate job run.
type AgentCommandStatus string

const (
	AgentCommandStatusPending    AgentCommandStatus = "pending"
	AgentCommandStatusDispatched AgentCommandStatus = "dispatched"
	AgentCommandStatusSuccess    AgentCommandStatus = "success"
	AgentCommandStatusFailed     AgentCommandStatus = "failed"
)

// AgentCommand represents a server-side request for an agent to execute a job immediately.
type AgentCommand struct {
	ID           string             `json:"id"`
	HostID       string             `json:"host_id"`
	JobID        string             `json:"job_id"`
	CommandHash  string             `json:"command_hash"`
	RawCommand   string             `json:"raw_command"`
	Schedule     string             `json:"schedule"`
	Status       AgentCommandStatus `json:"status"`
	Message      string             `json:"message,omitempty"`
	ExitCode     *int               `json:"exit_code,omitempty"`
	RequestedAt  time.Time          `json:"requested_at"`
	DispatchedAt *time.Time         `json:"dispatched_at,omitempty"`
	StartedAt    *time.Time         `json:"started_at,omitempty"`
	FinishedAt   *time.Time         `json:"finished_at,omitempty"`
}
