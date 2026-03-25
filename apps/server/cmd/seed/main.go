// Command seed inserts sample data via the central API for local development.
// Run: go run ./cmd/seed or via docker-compose --profile seed up seed
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"
)

func main() {
	serverURL := getenv("SERVER_URL", "http://localhost:8000")
	token := getenv("SERVER_TOKEN", "dev-token")

	slog.Info("seeding sample data", "server", serverURL)

	// Wait for server to be ready.
	waitForServer(serverURL)

	hosts := []map[string]any{
		{
			"machine_id": "seed-host-001",
			"hostname":   "web-prod-01",
			"ip_address": "10.0.1.10",
			"environment": "production",
			"tags":       []string{"web", "nginx", "prod"},
			"agent_version": "0.1.0",
		},
		{
			"machine_id": "seed-host-002",
			"hostname":   "db-prod-01",
			"ip_address": "10.0.1.20",
			"environment": "production",
			"tags":       []string{"database", "postgres", "prod"},
			"agent_version": "0.1.0",
		},
		{
			"machine_id": "seed-host-003",
			"hostname":   "crawler-dev-01",
			"ip_address": "192.168.1.5",
			"environment": "development",
			"tags":       []string{"crawler", "dev"},
			"agent_version": "0.1.0",
		},
	}

	hostIDs := make([]string, 0, len(hosts))
	for _, h := range hosts {
		resp := post(serverURL+"/api/v1/agents/register", token, h)
		id, _ := resp["id"].(string)
		hostIDs = append(hostIDs, id)
		slog.Info("registered host", "id", id, "hostname", h["hostname"])
	}

	// Send heartbeats.
	for _, id := range hostIDs {
		postWithHostID(serverURL+"/api/v1/agents/heartbeat", token, id, map[string]any{})
	}

	if len(hostIDs) == 0 {
		slog.Error("no hosts registered, exiting")
		os.Exit(1)
	}

	// Seed jobs for first host.
	jobPayload := map[string]any{
		"jobs": []map[string]any{
			{
				"source_type":        "cron",
				"schedule":           "0 2 * * *",
				"timezone":           "UTC",
				"user":               "root",
				"raw_command":        "/usr/local/bin/backup.sh --full",
				"normalized_command": "/usr/local/bin/backup.sh",
				"command_hash":       "abc123backup",
				"enabled":            true,
				"source_file":        "/etc/cron.d/backup",
			},
			{
				"source_type":        "cron",
				"schedule":           "*/5 * * * *",
				"timezone":           "UTC",
				"user":               "www-data",
				"raw_command":        "/usr/bin/python3 /opt/app/collect.py",
				"normalized_command": "/usr/bin/python3 /opt/app/collect.py",
				"command_hash":       "def456collect",
				"enabled":            true,
				"source_file":        "/etc/crontab",
			},
			{
				"source_type":        "cron",
				"schedule":           "0 0 * * *",
				"timezone":           "Asia/Taipei",
				"user":               "app",
				"raw_command":        "/opt/app/daily_report.sh",
				"normalized_command": "/opt/app/daily_report.sh",
				"command_hash":       "ghi789report",
				"enabled":            true,
				"source_file":        "/var/spool/cron/crontabs/app",
			},
		},
	}
	postWithHostID(serverURL+"/api/v1/jobs/discovery", token, hostIDs[0], jobPayload)
	slog.Info("submitted job discovery for host", "host_id", hostIDs[0])

	// Seed executions.
	now := time.Now()
	execPayload := map[string]any{
		"executions": []map[string]any{
			{
				"scheduled_at":       now.Add(-2 * time.Hour),
				"detected_started_at": now.Add(-2*time.Hour + 5*time.Second),
				"detected_finished_at": now.Add(-2*time.Hour + 45*time.Second),
				"duration_seconds":   40.5,
				"status":             "success",
				"confidence_score":   0.85,
				"detection_sources":  []string{"process"},
				"evidence":           map[string]any{"pid": 12345, "ppid": 1},
			},
			{
				"scheduled_at":       now.Add(-1 * time.Hour),
				"detected_started_at": now.Add(-1 * time.Hour),
				"status":             "unknown",
				"confidence_score":   0.4,
				"detection_sources":  []string{"cron_log"},
				"evidence":           map[string]any{"log_line": "CRON[12349]: CMD (/usr/local/bin/backup.sh)"},
			},
		},
	}
	postWithHostID(serverURL+"/api/v1/executions/batch", token, hostIDs[0], execPayload)

	// Seed metrics.
	metricPayload := map[string]any{
		"metrics": []map[string]any{
			{
				"metric_name": "daily_records_inserted",
				"metric_type": "count",
				"dimensions":  map[string]any{"table": "events", "db": "app_prod"},
				"value":       4821.0,
				"measured_at": now.Add(-30 * time.Minute),
				"status":      "ok",
				"evidence":    map[string]any{"query": "SELECT COUNT(*) FROM events WHERE created_at > NOW() - INTERVAL '1 day'"},
			},
			{
				"metric_name": "daily_records_inserted",
				"metric_type": "count",
				"dimensions":  map[string]any{"table": "events", "db": "app_prod"},
				"value":       0.0,
				"measured_at": now.Add(-24*time.Hour - 30*time.Minute),
				"status":      "critical",
				"evidence":    map[string]any{"query": "SELECT COUNT(*) FROM events WHERE created_at > NOW() - INTERVAL '1 day'"},
			},
		},
	}
	postWithHostID(serverURL+"/api/v1/metrics/batch", token, hostIDs[1], metricPayload)

	slog.Info("seed complete")
}

func post(url, token string, body any) map[string]any {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Warn("request failed", "url", url, "err", err)
		return nil
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return result
}

func postWithHostID(url, token, hostID string, body any) map[string]any {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Host-ID", hostID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Warn("request failed", "url", url, "err", err)
		return nil
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return result
}

func waitForServer(serverURL string) {
	for i := 0; i < 30; i++ {
		resp, err := http.Get(serverURL + "/health")
		if err == nil && resp.StatusCode == 200 {
			return
		}
		slog.Info("waiting for server...", "attempt", i+1)
		time.Sleep(2 * time.Second)
	}
	slog.Error("server did not become ready in time")
	os.Exit(1)
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

var _ = fmt.Sprintf // avoid unused import
