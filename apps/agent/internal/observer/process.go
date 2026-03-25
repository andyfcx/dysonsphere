// Package observer monitors running processes and attempts to match them
// to discovered cron jobs to infer execution lifecycle events.
package observer

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"
)

// ProcessSnapshot captures a single process observation.
type ProcessSnapshot struct {
	PID        int32
	PPID       int32
	Cmdline    string
	CreateTime time.Time
}

// ProcessObserver scans running processes and matches them against known job commands.
type ProcessObserver struct{}

// Scan returns all currently running process snapshots.
func (o *ProcessObserver) Scan() ([]*ProcessSnapshot, error) {
	pids, err := process.Pids()
	if err != nil {
		return nil, fmt.Errorf("list pids: %w", err)
	}

	var snapshots []*ProcessSnapshot
	for _, pid := range pids {
		p, err := process.NewProcess(pid)
		if err != nil {
			continue // process may have exited
		}
		cmdline, err := p.Cmdline()
		if err != nil || cmdline == "" {
			continue
		}
		ppid, _ := p.Ppid()
		createMS, _ := p.CreateTime()
		createTime := time.Unix(createMS/1000, (createMS%1000)*int64(time.Millisecond))

		snapshots = append(snapshots, &ProcessSnapshot{
			PID:        pid,
			PPID:       ppid,
			Cmdline:    cmdline,
			CreateTime: createTime,
		})
	}
	return snapshots, nil
}

// MatchedProcess represents a running process matched to a discovered job.
type MatchedProcess struct {
	Process    *ProcessSnapshot
	JobHash    string  // matched command hash
	Confidence float64 // 0.0–1.0
	Evidence   string
}

// MatchJobs attempts to match a list of process snapshots to known job command hashes.
// Matching is done by substring comparison of the normalized command.
// This is inherently imprecise — confidence reflects the quality of the match.
func MatchJobs(snapshots []*ProcessSnapshot, jobCommands map[string]string) []*MatchedProcess {
	var matched []*MatchedProcess
	for _, snap := range snapshots {
		for hash, normCmd := range jobCommands {
			if matchesCommand(snap.Cmdline, normCmd) {
				confidence := 0.75 // substring match is reasonably good but not exact
				if snap.Cmdline == normCmd {
					confidence = 0.95
				}
				matched = append(matched, &MatchedProcess{
					Process:    snap,
					JobHash:    hash,
					Confidence: confidence,
					Evidence: fmt.Sprintf("process cmdline %q matches job command %q",
						truncate(snap.Cmdline, 80), normCmd),
				})
				slog.Debug("matched process to job", "pid", snap.PID, "hash", hash, "confidence", confidence)
				break // first match wins
			}
		}
	}
	return matched
}

// matchesCommand returns true if the process cmdline contains the key parts of normCmd.
func matchesCommand(cmdline, normCmd string) bool {
	if normCmd == "" {
		return false
	}
	// Extract executable path (first token) for lightweight matching.
	exe := strings.Fields(normCmd)[0]
	return strings.Contains(cmdline, exe)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
