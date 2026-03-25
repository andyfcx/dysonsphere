// Package config loads server configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all server configuration.
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
}

type ServerConfig struct {
	Host  string
	Port  int
	Token string
}

type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// Load reads configuration from environment variables.
// All settings have sensible defaults for local development.
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Host:  getenv("SERVER_HOST", "0.0.0.0"),
			Port:  getenvInt("SERVER_PORT", 8000),
			Token: getenv("SERVER_TOKEN", "dev-token"),
		},
		Database: DatabaseConfig{
			Host:     getenv("POSTGRES_HOST", "localhost"),
			Port:     getenvInt("POSTGRES_PORT", 5432),
			User:     getenv("POSTGRES_USER", "observer"),
			Password: getenv("POSTGRES_PASSWORD", "observer_secret"),
			DBName:   getenv("POSTGRES_DB", "observer"),
			SSLMode:  getenv("POSTGRES_SSLMODE", "disable"),
		},
	}
	if cfg.Server.Token == "" {
		return nil, fmt.Errorf("SERVER_TOKEN must be set")
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
