// Package discovery scans the local system for scheduled tasks.
// It reads cron configuration files without modifying them.
package discovery

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DiscoveredJob represents a single parsed cron entry.
type DiscoveredJob struct {
	SourceType        string
	Schedule          string
	Timezone          string
	User              string
	RawCommand        string
	NormalizedCommand string
	CommandHash       string
	Enabled           bool
	SourceFile        string
}

// CronScanner reads cron configuration from well-known locations.
// It does NOT modify any files.
type CronScanner struct {
	// ExtraCronDirs are additional directories to scan for cron.d files.
	ExtraCronDirs []string
	// SkipSystemCrons skips /etc/crontab and /etc/cron.d/* — useful for testing.
	SkipSystemCrons bool
}

// Scan returns all discovered jobs from system cron files.
func (s *CronScanner) Scan() ([]*DiscoveredJob, error) {
	var jobs []*DiscoveredJob

	if !s.SkipSystemCrons {
		// /etc/crontab (system crontab — has username field)
		if js, err := parseCrontab("/etc/crontab", true); err == nil {
			jobs = append(jobs, js...)
		}

		// /etc/cron.d/* (per-package crontabs — have username field)
		cronDFiles, _ := filepath.Glob("/etc/cron.d/*")
		for _, f := range cronDFiles {
			if js, err := parseCrontab(f, true); err == nil {
				jobs = append(jobs, js...)
			}
		}
	}

	// Extra dirs (e.g. for testing or custom locations)
	for _, dir := range s.ExtraCronDirs {
		files, _ := filepath.Glob(filepath.Join(dir, "*"))
		for _, f := range files {
			if js, err := parseCrontab(f, true); err == nil {
				jobs = append(jobs, js...)
			}
		}
	}

	if !s.SkipSystemCrons {
		// User crontabs — try via `crontab -l` output if readable.
		// NOTE: reading /var/spool/cron directly typically requires root.
		// We fall back gracefully if not available.
		userJobs := scanUserCrontabs()
		jobs = append(jobs, userJobs...)
	}

	return jobs, nil
}

// ParseCrontabFile is the exported version of parseCrontab for testing.
func ParseCrontabFile(path string, hasUserField bool) ([]*DiscoveredJob, error) {
	return parseCrontab(path, hasUserField)
}

// parseCrontab parses a crontab file.
// hasUserField: true for /etc/crontab and /etc/cron.d/* (format: min hr dom mon dow user cmd)
//               false for user crontabs (format: min hr dom mon dow cmd)
func parseCrontab(path string, hasUserField bool) ([]*DiscoveredJob, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var jobs []*DiscoveredJob
	currentTZ := "UTC"
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Detect CRON_TZ or TZ setting.
		if tz, ok := parseTZLine(line); ok {
			currentTZ = tz
			continue
		}

		// Skip @reboot and other non-periodic entries.
		if strings.HasPrefix(line, "@") {
			continue
		}

		job := parseCronLine(line, path, hasUserField, currentTZ)
		if job != nil {
			jobs = append(jobs, job)
		}
	}
	return jobs, scanner.Err()
}

// cronLineRe matches the 5-field schedule at the start of a cron line.
var cronLineRe = regexp.MustCompile(`^(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(.+)$`)

func parseCronLine(line, sourcePath string, hasUserField bool, tz string) *DiscoveredJob {
	m := cronLineRe.FindStringSubmatch(line)
	if m == nil {
		return nil
	}
	schedule := strings.Join(m[1:6], " ")
	rest := strings.TrimSpace(m[6])

	var user, rawCmd string
	if hasUserField {
		// rest = "username command..."
		parts := strings.SplitN(rest, " ", 2)
		if len(parts) < 2 {
			return nil
		}
		user = parts[0]
		rawCmd = strings.TrimSpace(parts[1])
	} else {
		user = ""
		rawCmd = rest
	}

	normalized := normalizeCommand(rawCmd)
	hash := commandHash(normalized)

	return &DiscoveredJob{
		SourceType:        "cron",
		Schedule:          schedule,
		Timezone:          tz,
		User:              user,
		RawCommand:        rawCmd,
		NormalizedCommand: normalized,
		CommandHash:       hash,
		Enabled:           true,
		SourceFile:        sourcePath,
	}
}

// parseTZLine extracts the timezone from CRON_TZ=... or TZ=... lines.
func parseTZLine(line string) (string, bool) {
	for _, prefix := range []string{"CRON_TZ=", "TZ="} {
		if strings.HasPrefix(line, prefix) {
			tz := strings.TrimPrefix(line, prefix)
			tz = strings.Trim(tz, `"' `)
			return tz, true
		}
	}
	return "", false
}

// normalizeCommand strips common variable expansions, redirections, and trims space.
// This is a best-effort normalization for job identity — not a full shell parser.
func normalizeCommand(cmd string) string {
	// Strip output redirections.
	re := regexp.MustCompile(`\s*>\s*/\S+`)
	cmd = re.ReplaceAllString(cmd, "")
	// Strip common no-output patterns.
	cmd = strings.ReplaceAll(cmd, "> /dev/null 2>&1", "")
	cmd = strings.ReplaceAll(cmd, "2>&1", "")
	cmd = strings.ReplaceAll(cmd, ">/dev/null", "")
	// Collapse whitespace.
	cmd = strings.Join(strings.Fields(cmd), " ")
	return strings.TrimSpace(cmd)
}

// commandHash returns a short deterministic hash of a normalized command.
func commandHash(normalizedCmd string) string {
	h := sha256.Sum256([]byte(normalizedCmd))
	return fmt.Sprintf("%x", h[:8])
}

// scanUserCrontabs attempts to read user crontabs from /var/spool/cron/crontabs.
// This requires read permission (typically root). Falls back silently.
func scanUserCrontabs() []*DiscoveredJob {
	dirs := []string{
		"/var/spool/cron/crontabs", // Debian/Ubuntu
		"/var/spool/cron",          // RHEL/CentOS
	}
	var jobs []*DiscoveredJob
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			path := filepath.Join(dir, e.Name())
			js, err := parseCrontab(path, false)
			if err != nil {
				continue
			}
			user := e.Name()
			for _, j := range js {
				j.User = user
			}
			jobs = append(jobs, js...)
		}
	}
	return jobs
}
