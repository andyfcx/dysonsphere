// Package app wires all agent subsystems together and runs the main loop.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"time"

	"github.com/google/uuid"

	"github.com/andyfcx/observer/agent/internal/client"
	"github.com/andyfcx/observer/agent/internal/config"
	"github.com/andyfcx/observer/agent/internal/discovery"
	"github.com/andyfcx/observer/agent/internal/observer"
	"github.com/andyfcx/observer/agent/internal/probes"
	"github.com/andyfcx/observer/agent/internal/reporter"
	"github.com/andyfcx/observer/agent/internal/state"
)

// Agent is the main runtime for the observer agent.
type Agent struct {
	cfg      *config.Config
	hostID   string
	store    *state.Store
	client   *client.Client
	reporter *reporter.Reporter
	scanner  *discovery.CronScanner
	procObs  *observer.ProcessObserver
	jrnlObs  *observer.JournalObserver
	probeRun *probes.Runner

	knownJobs  map[string]*discovery.DiscoveredJob
	activeJobs map[int32]*activeExecution
}

type activeExecution struct {
	LocalID    string
	JobHash    string
	Command    string
	Schedule   string
	StartedAt  time.Time
	Confidence float64
	PID        int32
}

// Run starts the agent main loop. Blocks until ctx is cancelled.
func Run(ctx context.Context, cfg *config.Config) error {
	stateFile := cfg.Agent.StateFile
	if stateFile == "" {
		stateFile = "/var/lib/observer-agent/state.db"
	}
	if err := os.MkdirAll(stateFilDir(stateFile), 0750); err != nil {
		slog.Warn("could not create state dir", "err", err)
	}

	store, err := state.Open(stateFile)
	if err != nil {
		return err
	}
	defer store.Close()

	hostID, err := store.GetMeta("host_id")
	if err != nil || hostID == "" {
		return fmt.Errorf("host_id not found in state; run 'observer-agent init' first")
	}

	apiClient := client.NewClient(cfg.Server.URL, cfg.Server.Token, hostID)
	rep := reporter.NewReporter(store, apiClient)

	a := &Agent{
		cfg:        cfg,
		hostID:     hostID,
		store:      store,
		client:     apiClient,
		reporter:   rep,
		scanner:    &discovery.CronScanner{},
		procObs:    &observer.ProcessObserver{},
		jrnlObs:    &observer.JournalObserver{},
		probeRun:   probes.NewRunner(cfg.Probes),
		knownJobs:  make(map[string]*discovery.DiscoveredJob),
		activeJobs: make(map[int32]*activeExecution),
	}

	return a.mainLoop(ctx)
}

func (a *Agent) mainLoop(ctx context.Context) error {
	heartbeatD := parseDuration(a.cfg.Agent.HeartbeatInterval, 30*time.Second)
	discoveryD := parseDuration(a.cfg.Agent.DiscoveryInterval, 5*time.Minute)
	processScanD := parseDuration(a.cfg.Agent.ProcessScanInterval, 30*time.Second)
	reportD := parseDuration(a.cfg.Agent.ReportInterval, time.Minute)
	commandPollD := parseDuration(a.cfg.Agent.CommandPollInterval, 15*time.Second)

	heartbeatTick := time.NewTicker(heartbeatD)
	discoveryTick := time.NewTicker(discoveryD)
	processTick := time.NewTicker(processScanD)
	reportTick := time.NewTicker(reportD)
	commandTick := time.NewTicker(commandPollD)
	defer heartbeatTick.Stop()
	defer discoveryTick.Stop()
	defer processTick.Stop()
	defer reportTick.Stop()
	defer commandTick.Stop()

	a.sendHeartbeat(ctx)
	a.runDiscovery(ctx)
	a.pollCommands(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-heartbeatTick.C:
			a.sendHeartbeat(ctx)
		case <-discoveryTick.C:
			a.runDiscovery(ctx)
		case <-processTick.C:
			a.runProcessScan(ctx)
		case <-reportTick.C:
			a.reporter.FlushExecutions(ctx)
			a.reporter.FlushMetrics(ctx)
			a.runProbes(ctx)
		case <-commandTick.C:
			a.pollCommands(ctx)
		}
	}
}

func (a *Agent) sendHeartbeat(ctx context.Context) {
	if err := a.client.Heartbeat(ctx, ""); err != nil {
		slog.Warn("heartbeat failed", "err", err)
	}
}

func (a *Agent) runDiscovery(ctx context.Context) {
	jobs, err := a.scanner.Scan()
	if err != nil {
		slog.Warn("cron scan failed", "err", err)
		return
	}

	for _, j := range jobs {
		a.knownJobs[j.CommandHash] = j
		_ = a.store.SaveJob(&state.DiscoveredJob{
			CommandHash:       j.CommandHash,
			Schedule:          j.Schedule,
			Timezone:          j.Timezone,
			User:              j.User,
			RawCommand:        j.RawCommand,
			NormalizedCommand: j.NormalizedCommand,
			SourceFile:        j.SourceFile,
		})
	}

	payload := map[string]any{"jobs": toDiscoveryPayload(jobs)}
	if err := a.client.SubmitDiscovery(ctx, payload); err != nil {
		slog.Warn("submit discovery failed", "err", err)
		return
	}
	slog.Info("discovery submitted", "count", len(jobs))
}

func (a *Agent) runProcessScan(ctx context.Context) {
	snapshots, err := a.procObs.Scan()
	if err != nil {
		slog.Warn("process scan failed", "err", err)
		return
	}

	matched := observer.MatchJobs(snapshots, a.knownJobsForMatch())
	current := make(map[int32]bool, len(matched))

	for _, m := range matched {
		current[m.Process.PID] = true
		if _, exists := a.activeJobs[m.Process.PID]; exists {
			continue
		}

		job := a.knownJobs[m.JobHash]
		if job == nil {
			continue
		}
		id := uuid.New().String()
		a.activeJobs[m.Process.PID] = &activeExecution{
			LocalID:    id,
			JobHash:    m.JobHash,
			Command:    job.RawCommand,
			Schedule:   job.Schedule,
			StartedAt:  m.Process.CreateTime,
			Confidence: m.Confidence,
			PID:        m.Process.PID,
		}

		pid := m.Process.PID
		a.saveLocalExecution(&state.LocalExecution{
			ID:              id,
			CommandHash:     m.JobHash,
			Command:         job.RawCommand,
			Schedule:        job.Schedule,
			Status:          "running",
			Trigger:         "detected-process",
			ConfidenceScore: m.Confidence,
			StartedAt:       m.Process.CreateTime,
			PID:             &pid,
			Evidence:        mustJSON(map[string]any{"ppid": m.Process.PPID, "note": m.Evidence}),
		})

		a.reporter.BufferExecution(id, map[string]any{
			"command_hash":        m.JobHash,
			"detected_started_at": m.Process.CreateTime.UTC().Format(time.RFC3339),
			"status":              "running",
			"confidence_score":    m.Confidence,
			"detection_sources":   []string{"process"},
			"evidence": map[string]any{
				"pid":  m.Process.PID,
				"ppid": m.Process.PPID,
				"note": m.Evidence,
			},
		})
	}

	now := time.Now()
	for pid, active := range a.activeJobs {
		if current[pid] {
			continue
		}
		finishedAt := now
		duration := finishedAt.Sub(active.StartedAt).Seconds()
		a.saveLocalExecution(&state.LocalExecution{
			ID:              active.LocalID,
			CommandHash:     active.JobHash,
			Command:         active.Command,
			Schedule:        active.Schedule,
			Status:          "partial",
			Trigger:         "detected-process",
			ConfidenceScore: active.Confidence,
			StartedAt:       active.StartedAt,
			FinishedAt:      &finishedAt,
			DurationSeconds: &duration,
			Evidence:        mustJSON(map[string]any{"note": "process disappeared; completion inferred"}),
		})
		delete(a.activeJobs, pid)
	}

	slog.Debug("process scan done", "total", len(snapshots), "matched", len(matched))
}

func (a *Agent) runProbes(ctx context.Context) {
	results := a.probeRun.RunAll(ctx)
	for _, r := range results {
		id := uuid.New().String()
		a.reporter.BufferMetric(id, map[string]any{
			"metric_name": r.ProbeName,
			"metric_type": r.MetricType,
			"value":       r.Value,
			"measured_at": r.MeasuredAt.UTC().Format(time.RFC3339),
			"status":      r.Status,
			"evidence":    r.Evidence,
		})
	}
}

func (a *Agent) pollCommands(ctx context.Context) {
	commands, err := a.client.ClaimRunCommands(ctx, 10)
	if err != nil {
		slog.Warn("poll commands failed", "err", err)
		return
	}
	for _, cmd := range commands {
		go a.executeRunCommand(context.Background(), cmd)
	}
}

func (a *Agent) executeRunCommand(ctx context.Context, cmd *client.RunCommand) {
	startedAt := time.Now()
	localID := uuid.New().String()
	a.saveLocalExecution(&state.LocalExecution{
		ID:          localID,
		JobID:       cmd.JobID,
		CommandHash: cmd.CommandHash,
		Command:     cmd.RawCommand,
		Schedule:    cmd.Schedule,
		Status:      "running",
		Trigger:     "server-triggered",
		StartedAt:   startedAt,
		Evidence:    mustJSON(map[string]any{"command_id": cmd.ID, "requested_at": cmd.RequestedAt}),
	})

	execCmd := exec.CommandContext(ctx, "/bin/sh", "-lc", cmd.RawCommand)
	output, err := execCmd.CombinedOutput()
	finishedAt := time.Now()
	duration := finishedAt.Sub(startedAt).Seconds()

	status := "success"
	message := "completed"
	var exitCode *int
	if err != nil {
		status = "failed"
		message = err.Error()
		if exitErr, ok := err.(*exec.ExitError); ok {
			code := exitErr.ExitCode()
			exitCode = &code
		}
	} else {
		code := 0
		exitCode = &code
	}

	a.saveLocalExecution(&state.LocalExecution{
		ID:              localID,
		JobID:           cmd.JobID,
		CommandHash:     cmd.CommandHash,
		Command:         cmd.RawCommand,
		Schedule:        cmd.Schedule,
		Status:          status,
		Trigger:         "server-triggered",
		StartedAt:       startedAt,
		FinishedAt:      &finishedAt,
		DurationSeconds: &duration,
		ExitCode:        exitCode,
		Evidence: mustJSON(map[string]any{
			"command_id":    cmd.ID,
			"stdout_stderr": truncateString(string(output), 2000),
		}),
	})

	a.reporter.BufferExecution(uuid.New().String(), map[string]any{
		"job_id":               cmd.JobID,
		"command_hash":         cmd.CommandHash,
		"detected_started_at":  startedAt.UTC().Format(time.RFC3339),
		"detected_finished_at": finishedAt.UTC().Format(time.RFC3339),
		"duration_seconds":     duration,
		"status":               status,
		"confidence_score":     1.0,
		"detection_sources":    []string{"remote_command"},
		"evidence": map[string]any{
			"command_id":    cmd.ID,
			"exit_code":     exitCodeValue(exitCode),
			"stdout_stderr": truncateString(string(output), 2000),
		},
	})
	a.reporter.FlushExecutions(ctx)

	if err := a.client.CompleteRunCommand(ctx, cmd.ID, status, message, startedAt, finishedAt, exitCode); err != nil {
		slog.Warn("complete run command failed", "command_id", cmd.ID, "err", err)
	}
}

func (a *Agent) saveLocalExecution(exec *state.LocalExecution) {
	if err := a.store.SaveLocalExecution(exec); err != nil {
		slog.Warn("save local execution failed", "id", exec.ID, "err", err)
	}
}

func (a *Agent) knownJobsForMatch() map[string]string {
	out := make(map[string]string, len(a.knownJobs))
	for hash, job := range a.knownJobs {
		out[hash] = job.NormalizedCommand
	}
	return out
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func toDiscoveryPayload(jobs []*discovery.DiscoveredJob) []map[string]any {
	out := make([]map[string]any, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, map[string]any{
			"source_type":        j.SourceType,
			"schedule":           j.Schedule,
			"timezone":           j.Timezone,
			"user":               j.User,
			"raw_command":        j.RawCommand,
			"normalized_command": j.NormalizedCommand,
			"command_hash":       j.CommandHash,
			"enabled":            j.Enabled,
			"source_file":        j.SourceFile,
		})
	}
	return out
}

func parseDuration(s string, fallback time.Duration) time.Duration {
	if s == "" {
		return fallback
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return fallback
	}
	return d
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func exitCodeValue(code *int) any {
	if code == nil {
		return nil
	}
	return *code
}

func stateFilDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i]
		}
	}
	return "."
}
