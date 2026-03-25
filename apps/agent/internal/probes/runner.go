// Package probes implements the data probe runner.
// Probes measure external data quality/volume and report results as DataMetrics.
package probes

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	_ "github.com/lib/pq" // postgres driver — optional; only used by sql probes

	"github.com/andyfcx/observer/agent/internal/config"
)

// Result is the output of a single probe run.
type Result struct {
	ProbeName  string
	MetricType string
	Value      float64
	Status     string // "ok", "warning", "critical", "unknown"
	MeasuredAt time.Time
	Evidence   map[string]any
	Error      error
}

// Runner executes all configured probes.
type Runner struct {
	probes []config.ProbeConfig
}

// NewRunner creates a Runner for the given probe configs.
func NewRunner(probes []config.ProbeConfig) *Runner {
	return &Runner{probes: probes}
}

// RunAll executes all probes and returns results. Errors are included per-result.
func (r *Runner) RunAll(ctx context.Context) []*Result {
	results := make([]*Result, 0, len(r.probes))
	for _, p := range r.probes {
		result := r.runOne(ctx, p)
		results = append(results, result)
	}
	return results
}

func (r *Runner) runOne(ctx context.Context, p config.ProbeConfig) *Result {
	slog.Debug("running probe", "name", p.Name, "type", p.Type)
	switch p.Type {
	case "sql_count":
		return runSQLCountProbe(ctx, p)
	case "file_freshness":
		return runFileFreshnessProbe(p)
	case "command":
		return runCommandProbe(ctx, p)
	default:
		return &Result{
			ProbeName:  p.Name,
			MetricType: "gauge",
			Status:     "unknown",
			MeasuredAt: time.Now(),
			Error:      fmt.Errorf("unknown probe type %q", p.Type),
		}
	}
}

// runSQLCountProbe connects to a database and runs a COUNT query.
// The query should return a single integer row.
// NOTE: requires the database driver to be importable. For MVP, only PostgreSQL
// is supported. Import "github.com/lib/pq" in go.mod to enable.
func runSQLCountProbe(ctx context.Context, p config.ProbeConfig) *Result {
	res := &Result{
		ProbeName:  p.Name,
		MetricType: "count",
		MeasuredAt: time.Now(),
	}
	if p.SQL == nil {
		res.Status = "unknown"
		res.Error = fmt.Errorf("sql probe %q missing sql config", p.Name)
		return res
	}

	db, err := sql.Open("postgres", p.SQL.DSN)
	if err != nil {
		res.Status = "critical"
		res.Error = fmt.Errorf("open db: %w", err)
		return res
	}
	defer db.Close()
	db.SetConnMaxLifetime(10 * time.Second)

	ctx2, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var count float64
	if err := db.QueryRowContext(ctx2, p.SQL.Query).Scan(&count); err != nil {
		res.Status = "critical"
		res.Error = fmt.Errorf("query failed: %w", err)
		return res
	}

	res.Value = count
	res.Status = "ok"
	if count == 0 {
		res.Status = "warning"
	}
	res.Evidence = map[string]any{"query": p.SQL.Query}
	return res
}

// runFileFreshnessProbe checks whether a file exists and is recent enough.
func runFileFreshnessProbe(p config.ProbeConfig) *Result {
	res := &Result{
		ProbeName:  p.Name,
		MetricType: "gauge",
		MeasuredAt: time.Now(),
	}
	if p.File == nil {
		res.Status = "unknown"
		res.Error = fmt.Errorf("file probe %q missing file config", p.Name)
		return res
	}

	info, err := os.Stat(p.File.Path)
	if os.IsNotExist(err) {
		res.Status = "critical"
		res.Value = -1
		res.Evidence = map[string]any{"path": p.File.Path, "error": "file not found"}
		return res
	}
	if err != nil {
		res.Status = "unknown"
		res.Error = err
		return res
	}

	ageSeconds := time.Since(info.ModTime()).Seconds()
	res.Value = ageSeconds
	res.Evidence = map[string]any{
		"path":         p.File.Path,
		"mod_time":     info.ModTime().Format(time.RFC3339),
		"age_seconds":  ageSeconds,
		"max_age":      p.File.MaxAgeSeconds,
	}

	if p.File.MaxAgeSeconds > 0 && ageSeconds > float64(p.File.MaxAgeSeconds) {
		res.Status = "warning"
	} else {
		res.Status = "ok"
	}
	return res
}

// runCommandProbe executes a shell command and parses the first line as a float.
func runCommandProbe(ctx context.Context, p config.ProbeConfig) *Result {
	res := &Result{
		ProbeName:  p.Name,
		MetricType: "gauge",
		MeasuredAt: time.Now(),
	}
	if p.Command == nil {
		res.Status = "unknown"
		res.Error = fmt.Errorf("command probe %q missing command config", p.Name)
		return res
	}

	ctx2, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx2, p.Command.Command, p.Command.Args...)
	out, err := cmd.Output()
	if err != nil {
		res.Status = "critical"
		res.Error = fmt.Errorf("command failed: %w", err)
		return res
	}

	output := strings.TrimSpace(string(out))
	var value float64
	if _, err := fmt.Sscanf(output, "%f", &value); err != nil {
		res.Status = "unknown"
		res.Evidence = map[string]any{"output": output, "parse_error": err.Error()}
		return res
	}

	res.Value = value
	res.Status = "ok"
	res.Evidence = map[string]any{
		"command": p.Command.Command,
		"output":  output,
	}
	return res
}
