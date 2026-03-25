// Package config loads and validates the agent configuration from a YAML file.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the top-level agent configuration structure.
type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Agent   AgentConfig   `yaml:"agent"`
	Probes  []ProbeConfig `yaml:"probes"`
}

// ServerConfig points to the central observer server.
type ServerConfig struct {
	URL   string `yaml:"url"`
	Token string `yaml:"token"`
}

// AgentConfig contains self-identification and behaviour settings.
type AgentConfig struct {
	MachineID   string   `yaml:"machine_id"`
	Hostname    string   `yaml:"hostname"`
	Environment string   `yaml:"environment"`
	Tags        []string `yaml:"tags"`
	Version     string   `yaml:"version"`
	StateFile   string   `yaml:"state_file"`

	HeartbeatInterval  string `yaml:"heartbeat_interval"`  // e.g. "30s"
	DiscoveryInterval  string `yaml:"discovery_interval"`  // e.g. "5m"
	ProcessScanInterval string `yaml:"process_scan_interval"` // e.g. "30s"
	ReportInterval     string `yaml:"report_interval"`     // e.g. "1m"
	CommandPollInterval string `yaml:"command_poll_interval"` // e.g. "15s"
}

// ProbeConfig defines a single data probe.
type ProbeConfig struct {
	Name     string         `yaml:"name"`
	Type     string         `yaml:"type"`     // "sql_count", "file_freshness", "command"
	Schedule string         `yaml:"schedule"` // cron expression
	SQL      *SQLProbe      `yaml:"sql,omitempty"`
	File     *FileProbe     `yaml:"file,omitempty"`
	Command  *CommandProbe  `yaml:"command,omitempty"`
}

type SQLProbe struct {
	DSN   string `yaml:"dsn"`
	Query string `yaml:"query"`
}

type FileProbe struct {
	Path           string `yaml:"path"`
	MaxAgeSeconds  int    `yaml:"max_age_seconds"`
}

type CommandProbe struct {
	Command string `yaml:"command"`
	Args    []string `yaml:"args"`
}

// Load reads and parses a YAML config file.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config %q: %w", path, err)
	}
	defer f.Close()

	var cfg Config
	if err := yaml.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, validate(&cfg)
}

func validate(cfg *Config) error {
	if cfg.Server.URL == "" {
		return fmt.Errorf("server.url is required")
	}
	if cfg.Server.Token == "" {
		return fmt.Errorf("server.token is required")
	}
	if cfg.Agent.MachineID == "" {
		return fmt.Errorf("agent.machine_id is required (run 'observer-agent init' first)")
	}
	return nil
}
