// Package alerting contains the state evaluation and alert generation logic.
// MVP implementation: simple rule-based checks run periodically.
package alerting

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/andyfcx/observer/server/internal/domain"
	"github.com/andyfcx/observer/server/internal/repository"
)

const (
	// RuleStaleHeartbeat fires when a host hasn't sent a heartbeat in 5 minutes.
	RuleStaleHeartbeat = "stale_heartbeat"
	// RuleRepeatedFailures fires when a job has ≥3 failures in 24 hours.
	RuleRepeatedFailures = "repeated_failures"
)

// Evaluator runs alerting rules against the current system state.
type Evaluator struct {
	hosts      *repository.HostRepo
	executions *repository.ExecutionRepo
	alerts     *repository.AlertRepo
	jobs       *repository.JobRepo
}

func NewEvaluator(
	hosts *repository.HostRepo,
	executions *repository.ExecutionRepo,
	alerts *repository.AlertRepo,
	jobs *repository.JobRepo,
) *Evaluator {
	return &Evaluator{
		hosts:      hosts,
		executions: executions,
		alerts:     alerts,
		jobs:       jobs,
	}
}

// RunAll executes all alert rules. Meant to be called periodically (e.g. every minute).
func (e *Evaluator) RunAll(ctx context.Context) {
	e.checkStaleHeartbeats(ctx)
	e.checkRepeatedFailures(ctx)
}

// checkStaleHeartbeats marks hosts as stale and fires alerts.
func (e *Evaluator) checkStaleHeartbeats(ctx context.Context) {
	threshold := 5 * time.Minute
	count, err := e.hosts.MarkStaleHosts(ctx, threshold)
	if err != nil {
		slog.Error("stale heartbeat check failed", "err", err)
		return
	}
	if count > 0 {
		slog.Info("marked hosts as stale", "count", count)
	}

	hosts, err := e.hosts.List(ctx)
	if err != nil {
		return
	}
	for _, h := range hosts {
		isStale := h.Status == domain.HostStatusStale ||
			(h.LastHeartbeatAt != nil && time.Since(*h.LastHeartbeatAt) > threshold)

		if isStale {
			e.fireOrUpdate(ctx, &domain.Alert{
				TargetType: "host",
				TargetID:   h.ID,
				RuleName:   RuleStaleHeartbeat,
				Severity:   domain.AlertSeverityWarning,
				Message:    fmt.Sprintf("Host %q has not sent a heartbeat in over %v", h.Hostname, threshold),
			})
		} else {
			// Resolve if host is now healthy.
			_ = e.alerts.Resolve(ctx, "host", h.ID, RuleStaleHeartbeat)
		}
	}
}

// checkRepeatedFailures fires alerts for jobs with many recent failures.
func (e *Evaluator) checkRepeatedFailures(ctx context.Context) {
	jobs, err := e.jobs.List(ctx)
	if err != nil {
		return
	}
	const failThreshold = 3
	for _, j := range jobs {
		count, err := e.executions.CountRecentFailures(ctx, j.ID)
		if err != nil {
			continue
		}
		if count >= failThreshold {
			e.fireOrUpdate(ctx, &domain.Alert{
				TargetType: "job",
				TargetID:   j.ID,
				RuleName:   RuleRepeatedFailures,
				Severity:   domain.AlertSeverityWarning,
				Message: fmt.Sprintf(
					"Job %q has had %d failed/unknown executions in the past 24 hours",
					j.NormalizedCommand, count,
				),
			})
		} else {
			_ = e.alerts.Resolve(ctx, "job", j.ID, RuleRepeatedFailures)
		}
	}
}

// fireOrUpdate inserts a new alert only if one isn't already active for the same target+rule.
func (e *Evaluator) fireOrUpdate(ctx context.Context, a *domain.Alert) {
	existing, err := e.alerts.GetActiveByTarget(ctx, a.TargetType, a.TargetID, a.RuleName)
	if err != nil {
		slog.Error("check existing alert", "err", err)
		return
	}
	if existing != nil {
		return // already active, no duplicate
	}
	if _, err := e.alerts.Insert(ctx, a); err != nil {
		slog.Error("insert alert", "rule", a.RuleName, "target", a.TargetID, "err", err)
		return
	}
	slog.Info("alert fired", "rule", a.RuleName, "target_type", a.TargetType, "target_id", a.TargetID)
}
