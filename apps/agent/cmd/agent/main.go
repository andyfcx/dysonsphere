// Command agent is the Observer agent CLI.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/andyfcx/observer/agent/internal/app"
	"github.com/andyfcx/observer/agent/internal/config"
	"github.com/andyfcx/observer/agent/internal/state"
)

const defaultConfigPath = "/etc/observer-agent/config.yaml"
const agentVersion = "0.1.0"

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	root := &cobra.Command{
		Use:   "observer-agent",
		Short: "Observer agent — monitors cronjobs and data probes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := ensureConfig(defaultConfigPath); err != nil {
				return err
			}
			cfg, err := config.Load(defaultConfigPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()
			return app.Run(ctx, cfg)
		},
	}

	root.AddCommand(initCmd(), runCmd(), daemonCmd(), statusCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func initCmd() *cobra.Command {
	var serverURL, token, env, tagsStr, configOut, stateFile string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Register this host with the central server and write agent config",
		RunE: func(cmd *cobra.Command, args []string) error {
			return interactiveInit(serverURL, token, env, tagsStr, configOut, stateFile)
		},
	}

	cmd.Flags().StringVar(&serverURL, "server", "", "Central server URL")
	cmd.Flags().StringVar(&token, "token", "", "Auth token")
	cmd.Flags().StringVar(&env, "env", "production", "Environment label")
	cmd.Flags().StringVar(&tagsStr, "tags", "", "Comma-separated tags")
	cmd.Flags().StringVar(&configOut, "config-out", defaultConfigPath, "Path to write config file")
	cmd.Flags().StringVar(&stateFile, "state-file", "/var/lib/observer-agent/state.db", "Path for local state db")
	return cmd
}

func runCmd() *cobra.Command {
	var configPath string
	var daemon bool
	var foreground bool
	var pidFile string
	var logFile string

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Start the agent main loop",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := ensureConfig(configPath); err != nil {
				return err
			}
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			if daemon && !foreground {
				pidPath, logPath := daemonPaths(cfg, pidFile, logFile)
				return startDetached(configPath, pidPath, logPath)
			}

			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			slog.Info("observer agent starting", "version", agentVersion, "config", configPath)
			return app.Run(ctx, cfg)
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", defaultConfigPath, "Config file path")
	cmd.Flags().BoolVar(&daemon, "daemon", false, "Run agent in background")
	cmd.Flags().BoolVar(&foreground, "foreground", false, "Internal flag used by --daemon")
	cmd.Flags().StringVar(&pidFile, "pid-file", "", "PID file path for daemon mode")
	cmd.Flags().StringVar(&logFile, "log-file", "", "Log file path for daemon mode")
	_ = cmd.Flags().MarkHidden("foreground")
	return cmd
}

func daemonCmd() *cobra.Command {
	var configPath string
	var pidFile string
	var logFile string

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Inspect or stop the background agent",
	}

	status := &cobra.Command{
		Use:   "status",
		Short: "Show background process status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfigIfExists(configPath)
			if err != nil {
				return err
			}
			pidPath, logPath := daemonPaths(cfg, pidFile, logFile)
			return printDaemonStatus(pidPath, logPath)
		},
	}

	stop := &cobra.Command{
		Use:   "stop",
		Short: "Stop the background agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfigIfExists(configPath)
			if err != nil {
				return err
			}
			pidPath, _ := daemonPaths(cfg, pidFile, logFile)
			return stopDaemon(pidPath)
		},
	}

	for _, sub := range []*cobra.Command{status, stop} {
		sub.Flags().StringVarP(&configPath, "config", "c", defaultConfigPath, "Config file path")
		sub.Flags().StringVar(&pidFile, "pid-file", "", "PID file path")
		sub.Flags().StringVar(&logFile, "log-file", "", "Log file path")
		cmd.AddCommand(sub)
	}
	return cmd
}

func statusCmd() *cobra.Command {
	var configPath string
	var since string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show local cronjob status history",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			window, err := time.ParseDuration(since)
			if err != nil {
				return fmt.Errorf("invalid --since: %w", err)
			}
			return printLocalStatus(cfg.Agent.StateFile, window)
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", defaultConfigPath, "Config file path")
	cmd.Flags().StringVar(&since, "since", "24h", "How far back to inspect local executions")
	return cmd
}

func ensureConfig(configPath string) error {
	if _, err := os.Stat(configPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	fmt.Printf("Config %s not found. Starting interactive setup.\n", configPath)
	return interactiveInit("", "", "production", "", configPath, "/var/lib/observer-agent/state.db")
}

func interactiveInit(serverURL, token, env, tagsStr, configOut, stateFile string) error {
	reader := bufio.NewReader(os.Stdin)
	serverURL = prompt(reader, "Server URL", serverURL)
	token = prompt(reader, "Token", token)
	env = prompt(reader, "Environment", defaultString(env, "production"))
	tagsStr = prompt(reader, "Tags (comma-separated)", tagsStr)
	configOut = prompt(reader, "Config path", defaultString(configOut, defaultConfigPath))
	stateFile = prompt(reader, "State DB path", defaultString(stateFile, "/var/lib/observer-agent/state.db"))

	if serverURL == "" || token == "" {
		return fmt.Errorf("server and token are required")
	}

	hostname, _ := os.Hostname()
	machineID := generateMachineID()
	tags := parseTags(tagsStr)

	hostID, err := registerHost(serverURL, token, machineID, hostname, env, tags)
	if err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}

	cfg := buildConfig(serverURL, token, machineID, hostname, env, tags, stateFile)
	if err := writeConfigFile(configOut, cfg); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	if err := os.MkdirAll(stateDir(stateFile), 0750); err != nil {
		slog.Warn("could not create state dir", "err", err)
	}
	st, err := state.Open(stateFile)
	if err != nil {
		return fmt.Errorf("open state: %w", err)
	}
	defer st.Close()
	if err := st.SetMeta("host_id", hostID); err != nil {
		return fmt.Errorf("save host_id: %w", err)
	}

	fmt.Printf("Registered host_id=%s\n", hostID)
	fmt.Printf("Config written to %s\n", configOut)
	return nil
}

func registerHost(serverURL, token, machineID, hostname, env string, tags []string) (string, error) {
	payload := map[string]any{
		"machine_id":    machineID,
		"hostname":      hostname,
		"ip_address":    "",
		"environment":   env,
		"tags":          tags,
		"agent_version": agentVersion,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", serverURL+"/api/v1/agents/register", strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	hostID, _ := result["id"].(string)
	if hostID == "" {
		return "", fmt.Errorf("server response missing id")
	}
	return hostID, nil
}

func generateMachineID() string {
	if data, err := os.ReadFile("/etc/machine-id"); err == nil {
		if id := strings.TrimSpace(string(data)); id != "" {
			return id
		}
	}
	return uuid.New().String()
}

func parseTags(s string) []string {
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	tags := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

func buildConfig(serverURL, token, machineID, hostname, env string, tags []string, stateFile string) map[string]any {
	return map[string]any{
		"server": map[string]any{
			"url":   serverURL,
			"token": token,
		},
		"agent": map[string]any{
			"machine_id":            machineID,
			"hostname":              hostname,
			"environment":           env,
			"tags":                  tags,
			"version":               agentVersion,
			"state_file":            stateFile,
			"heartbeat_interval":    "30s",
			"discovery_interval":    "5m",
			"process_scan_interval": "30s",
			"report_interval":       "1m",
			"command_poll_interval": "15s",
		},
		"probes": []any{},
	}
}

func writeConfigFile(path string, cfg any) error {
	if err := os.MkdirAll(stateDir(path), 0750); err != nil {
		slog.Warn("could not create config dir", "path", path, "err", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return yaml.NewEncoder(f).Encode(cfg)
}

func printLocalStatus(stateFile string, since time.Duration) error {
	st, err := state.Open(stateFile)
	if err != nil {
		return err
	}
	defer st.Close()

	records, err := st.ListLocalExecutionsSince(time.Now().Add(-since))
	if err != nil {
		return err
	}
	if len(records) == 0 {
		fmt.Println("No local cron executions found in the requested window.")
		return nil
	}

	type summary struct {
		Command    string
		Schedule   string
		Status     string
		Trigger    string
		LastRun    time.Time
		UpdatedAt  time.Time
		Success    int
		Failed     int
		Running    int
		Partial    int
	}
	grouped := map[string]*summary{}
	for _, rec := range records {
		key := rec.CommandHash
		if key == "" {
			key = rec.Command
		}
		item := grouped[key]
		if item == nil {
			item = &summary{Command: rec.Command, Schedule: rec.Schedule}
			grouped[key] = item
		}
		if rec.StartedAt.After(item.LastRun) {
			item.LastRun = rec.StartedAt
		}
		if rec.LastUpdatedAt.After(item.UpdatedAt) || item.UpdatedAt.IsZero() {
			item.UpdatedAt = rec.LastUpdatedAt
			item.Status = rec.Status
			item.Trigger = rec.Trigger
		}
		switch rec.Status {
		case "success":
			item.Success++
		case "failed":
			item.Failed++
		case "running":
			item.Running++
		default:
			item.Partial++
		}
	}

	rows := make([]*summary, 0, len(grouped))
	for _, item := range grouped {
		rows = append(rows, item)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].LastRun.After(rows[j].LastRun) })

	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(w, "LAST RUN\tSTATUS\tTRIGGER\tSUCCESS\tFAILED\tRUNNING\tPARTIAL\tSCHEDULE\tCOMMAND")
	for _, row := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\t%d\t%d\t%s\t%s\n",
			row.LastRun.Format("2006-01-02 15:04:05"),
			row.Status, row.Trigger, row.Success, row.Failed, row.Running, row.Partial,
			row.Schedule, truncate(row.Command, 80),
		)
	}
	return w.Flush()
}

func startDetached(configPath, pidPath, logPath string) error {
	if err := os.MkdirAll(stateDir(pidPath), 0750); err != nil {
		return err
	}
	if err := os.MkdirAll(stateDir(logPath), 0750); err != nil {
		return err
	}
	if running, pid := daemonRunning(pidPath); running {
		return fmt.Errorf("agent already running with pid %d", pid)
	}

	logFH, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
	if err != nil {
		return err
	}
	defer logFH.Close()

	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exePath, "run", "--config", configPath, "--foreground")
	cmd.Stdout = logFH
	cmd.Stderr = logFH
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return err
	}
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(cmd.Process.Pid)), 0640); err != nil {
		return err
	}

	fmt.Printf("observer-agent started in background\npid=%d\npid_file=%s\nlog_file=%s\n", cmd.Process.Pid, pidPath, logPath)
	return nil
}

func printDaemonStatus(pidPath, logPath string) error {
	running, pid := daemonRunning(pidPath)
	if !running {
		fmt.Printf("observer-agent is not running (pid file: %s)\n", pidPath)
		return nil
	}
	fmt.Printf("observer-agent is running\npid=%d\npid_file=%s\nlog_file=%s\n", pid, pidPath, logPath)
	return nil
}

func stopDaemon(pidPath string) error {
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return err
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	_ = os.Remove(pidPath)
	fmt.Printf("observer-agent stop signal sent to pid %d\n", pid)
	return nil
}

func daemonRunning(pidPath string) (bool, int) {
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return false, 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return false, 0
	}
	if err := syscall.Kill(pid, 0); err != nil {
		return false, pid
	}
	return true, pid
}

func daemonPaths(cfg *config.Config, pidOverride, logOverride string) (string, string) {
	baseDir := "/var/lib/observer-agent"
	if cfg != nil && cfg.Agent.StateFile != "" {
		baseDir = stateDir(cfg.Agent.StateFile)
	}
	pidPath := pidOverride
	if pidPath == "" {
		pidPath = baseDir + "/observer-agent.pid"
	}
	logPath := logOverride
	if logPath == "" {
		logPath = baseDir + "/observer-agent.log"
	}
	return pidPath, logPath
}

func loadConfigIfExists(path string) (*config.Config, error) {
	cfg, err := config.Load(path)
	if err == nil {
		return cfg, nil
	}
	if errors.Is(err, os.ErrNotExist) || strings.Contains(err.Error(), "open config") {
		return nil, nil
	}
	return nil, err
}

func prompt(reader *bufio.Reader, label, fallback string) string {
	if fallback != "" {
		fmt.Printf("%s [%s]: ", label, fallback)
	} else {
		fmt.Printf("%s: ", label)
	}
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return fallback
	}
	return line
}

func defaultString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func stateDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}
