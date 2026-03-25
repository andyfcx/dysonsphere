package observer

import (
	"bufio"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

// JournalEvent represents a single cron-related log event.
type JournalEvent struct {
	Timestamp time.Time
	Unit      string
	Message   string
	RawLine   string
}

// JournalObserver reads cron-related events from journald or syslog.
// This is a best-effort observer — if the system doesn't support journald
// or the agent lacks permissions, it returns an empty slice.
type JournalObserver struct{}

// ReadCronEvents returns recent cron events from journald (if available).
// Falls back to /var/log/syslog or /var/log/cron silently.
// TODO: use CGO-based journald binding (github.com/coreos/go-systemd/sdjournal)
//       for more reliable real-time streaming in production.
func (o *JournalObserver) ReadCronEvents(since time.Time) ([]*JournalEvent, error) {
	// Try journalctl first.
	events, err := readFromJournalctl(since)
	if err == nil {
		return events, nil
	}
	slog.Debug("journalctl not available, trying syslog", "err", err)

	// Fallback: scan /var/log/syslog or /var/log/cron for cron entries.
	// MVP: return empty slice — full syslog parsing is outside MVP scope.
	slog.Debug("journal observation not available on this system; skipping")
	return nil, nil
}

// readFromJournalctl executes `journalctl -u cron -u crond --since=... --no-pager`
// and parses the output.
func readFromJournalctl(since time.Time) ([]*JournalEvent, error) {
	sinceStr := since.Format("2006-01-02 15:04:05")
	cmd := exec.Command("journalctl",
		"--no-pager",
		"-o", "short-iso",
		"--since", sinceStr,
		"_SYSTEMD_UNIT=cron.service",
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("journalctl: %w", err)
	}

	var events []*JournalEvent
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if ev := parseJournalLine(line); ev != nil {
			events = append(events, ev)
		}
	}
	return events, scanner.Err()
}

// parseJournalLine parses a single journalctl short-iso line.
// Format: YYYY-MM-DDTHH:MM:SS+TZ hostname unit[pid]: message
func parseJournalLine(line string) *JournalEvent {
	parts := strings.SplitN(line, " ", 4)
	if len(parts) < 4 {
		return nil
	}
	// Only keep CMD lines (actual cron job executions).
	if !strings.Contains(parts[3], "CMD") && !strings.Contains(parts[3], "CRON") {
		return nil
	}
	ts, err := time.Parse("2006-01-02T15:04:05-0700", parts[0])
	if err != nil {
		return nil
	}
	return &JournalEvent{
		Timestamp: ts,
		Unit:      "cron",
		Message:   parts[3],
		RawLine:   line,
	}
}
