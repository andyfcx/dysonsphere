// Package config loads and validates the agent configuration from a YAML file.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the top-level agent configuration structure.
type Config struct {
	Server ServerConfig  `yaml:"server"`
	Agent  AgentConfig   `yaml:"agent"`
	Probes []ProbeConfig `yaml:"probes"`
}

// ServerConfig points to the central observer server.
type ServerConfig struct {
	URL       string `yaml:"url"`
	Token     string `yaml:"token,omitempty"`
	TokenFile string `yaml:"token_file,omitempty"`
}

// AgentConfig contains self-identification and behaviour settings.
type AgentConfig struct {
	MachineID   string   `yaml:"machine_id"`
	Hostname    string   `yaml:"hostname"`
	Environment string   `yaml:"environment"`
	Tags        []string `yaml:"tags"`
	Version     string   `yaml:"version"`
	StateFile   string   `yaml:"state_file"`

	HeartbeatInterval   string `yaml:"heartbeat_interval"`    // e.g. "30s"
	DiscoveryInterval   string `yaml:"discovery_interval"`    // e.g. "5m"
	ProcessScanInterval string `yaml:"process_scan_interval"` // e.g. "30s"
	ReportInterval      string `yaml:"report_interval"`       // e.g. "1m"
	CommandPollInterval string `yaml:"command_poll_interval"` // e.g. "15s"
	MailScanInterval    string `yaml:"mail_scan_interval"`    // e.g. "2m"

	// MailSpoolPaths lists Unix mbox files to scan for cron stdout/stderr.
	// Defaults to ["/var/mail/root", "/var/spool/mail/root"] when empty.
	MailSpoolPaths []string `yaml:"mail_spool_paths"`
}

// ProbeConfig defines a single data probe.
type ProbeConfig struct {
	Name     string        `yaml:"name"`
	Type     string        `yaml:"type"`     // "sql_count", "file_freshness", "command"
	Schedule string        `yaml:"schedule"` // cron expression
	SQL      *SQLProbe     `yaml:"sql,omitempty"`
	File     *FileProbe    `yaml:"file,omitempty"`
	Command  *CommandProbe `yaml:"command,omitempty"`
}

type SQLProbe struct {
	DSN   string `yaml:"dsn"`
	Query string `yaml:"query"`
}

type FileProbe struct {
	Path          string `yaml:"path"`
	MaxAgeSeconds int    `yaml:"max_age_seconds"`
}

type CommandProbe struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
}

type credentialFile struct {
	Token string `yaml:"token"`
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
	if cfg.Server.Token == "" && cfg.Server.TokenFile != "" {
		token, err := loadTokenFile(cfg.Server.TokenFile)
		if err != nil {
			return nil, err
		}
		cfg.Server.Token = token
	}
	return &cfg, validate(&cfg)
}

func validate(cfg *Config) error {
	if cfg.Server.URL == "" {
		return fmt.Errorf("server.url is required")
	}
	if cfg.Server.Token == "" {
		return fmt.Errorf("server.token or server.token_file is required")
	}
	if cfg.Agent.MachineID == "" {
		return fmt.Errorf("agent.machine_id is required (run 'observer-agent init' first)")
	}
	return nil
}

func loadTokenFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open credential file %q: %w", path, err)
	}
	defer f.Close()

	var creds credentialFile
	if err := yaml.NewDecoder(f).Decode(&creds); err != nil {
		return "", fmt.Errorf("parse credential file: %w", err)
	}
	if creds.Token == "" {
		return "", fmt.Errorf("credential file %q missing token", path)
	}
	return creds.Token, nil
}
