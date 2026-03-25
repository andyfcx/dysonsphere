// Package types contains shared type constants used by both server and agent.
// These are kept in sync with the domain models but can be used without importing
// the full server or agent packages.
package types

// Known execution statuses — shared vocabulary.
const (
	StatusRunning = "running"
	StatusSuccess = "success"
	StatusFailed  = "failed"
	StatusUnknown = "unknown"
	StatusPartial = "partial"
	StatusMissed  = "missed"
)

// Known host statuses.
const (
	HostActive  = "active"
	HostStale   = "stale"
	HostOffline = "offline"
	HostUnknown = "unknown"
)

// Known metric statuses.
const (
	MetricOK       = "ok"
	MetricWarning  = "warning"
	MetricCritical = "critical"
	MetricUnknown  = "unknown"
)

// Known alert severities.
const (
	SeverityInfo     = "info"
	SeverityWarning  = "warning"
	SeverityCritical = "critical"
)
