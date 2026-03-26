// Package client provides an HTTP client for communicating with the central server.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Client is an HTTP client for the observer central server.
type Client struct {
	baseURL    string
	token      string
	hostID     string
	httpClient *http.Client
}

// EnrollRequest is sent by the agent during one-time enrollment.
type EnrollRequest struct {
	EnrollmentToken string `json:"enrollment_token"`
	MachineID       string `json:"machine_id"`
	Hostname        string `json:"hostname"`
	IPAddress       string `json:"ip_address"`
	AgentVersion    string `json:"agent_version"`
}

// EnrollResponse returns the issued credential and initial config.
type EnrollResponse struct {
	HostID     string             `json:"host_id"`
	Credential AgentCredential    `json:"credential"`
	Config     AgentBootstrapConfig `json:"config"`
}

type AgentCredential struct {
	Token string `json:"token"`
}

type AgentBootstrapConfig struct {
	Environment         string   `json:"environment"`
	Tags                []string `json:"tags"`
	HeartbeatInterval   string   `json:"heartbeat_interval"`
	DiscoveryInterval   string   `json:"discovery_interval"`
	ProcessScanInterval string   `json:"process_scan_interval"`
	ReportInterval      string   `json:"report_interval"`
	CommandPollInterval string   `json:"command_poll_interval"`
}

// RunCommand is a server-dispatched job execution request for this host.
type RunCommand struct {
	ID          string    `json:"id"`
	HostID      string    `json:"host_id"`
	JobID       string    `json:"job_id"`
	CommandHash string    `json:"command_hash"`
	RawCommand  string    `json:"raw_command"`
	Schedule    string    `json:"schedule"`
	RequestedAt time.Time `json:"requested_at"`
}

// NewClient creates a new server client.
func NewClient(baseURL, token, hostID string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		hostID:  hostID,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Enroll exchanges an enrollment token for a formal agent credential.
func (c *Client) Enroll(ctx context.Context, reqBody *EnrollRequest) (*EnrollResponse, error) {
	respBody, err := c.postRaw(ctx, "/api/v1/agents/enroll", "", reqBody)
	if err != nil {
		return nil, err
	}
	var resp EnrollResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("decode enroll response: %w", err)
	}
	return &resp, nil
}

// Heartbeat sends a heartbeat.
func (c *Client) Heartbeat(ctx context.Context, ipAddress string) error {
	_, err := c.post(ctx, "/api/v1/agents/heartbeat", c.hostID, map[string]any{
		"ip_address": ipAddress,
	})
	return err
}

// SubmitDiscovery sends a cron job discovery batch.
func (c *Client) SubmitDiscovery(ctx context.Context, jobs any) error {
	_, err := c.post(ctx, "/api/v1/jobs/discovery", c.hostID, jobs)
	return err
}

// BatchExecutions sends execution events.
func (c *Client) BatchExecutions(ctx context.Context, events any) error {
	_, err := c.post(ctx, "/api/v1/executions/batch", c.hostID, events)
	return err
}

// BatchMetrics sends metric results.
func (c *Client) BatchMetrics(ctx context.Context, metrics any) error {
	_, err := c.post(ctx, "/api/v1/metrics/batch", c.hostID, metrics)
	return err
}

// ClaimRunCommands claims any pending remote execution requests for this host.
func (c *Client) ClaimRunCommands(ctx context.Context, limit int) ([]*RunCommand, error) {
	var resp struct {
		Commands []*RunCommand `json:"commands"`
	}
	if err := c.get(ctx, fmt.Sprintf("/api/v1/agents/commands?limit=%d", limit), c.hostID, &resp); err != nil {
		return nil, err
	}
	return resp.Commands, nil
}

// CompleteRunCommand acknowledges remote execution completion back to the server.
func (c *Client) CompleteRunCommand(ctx context.Context, id, status, message string, startedAt, finishedAt time.Time, exitCode *int) error {
	payload := map[string]any{
		"status":      status,
		"message":     message,
		"started_at":  startedAt.UTC().Format(time.RFC3339),
		"finished_at": finishedAt.UTC().Format(time.RFC3339),
		"exit_code":   exitCodeValue(exitCode),
	}
	_, err := c.post(ctx, "/api/v1/agents/commands/"+id+"/complete", c.hostID, payload)
	return err
}

// post is the generic POST helper.
func (c *Client) post(ctx context.Context, path, hostID string, body any) (map[string]any, error) {
	respBody, err := c.postRaw(ctx, path, hostID, body)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	_ = json.Unmarshal(respBody, &result)
	slog.Debug("api call ok", "path", path)
	return result, nil
}

func (c *Client) postRaw(ctx context.Context, path, hostID string, body any) ([]byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if hostID != "" {
		req.Header.Set("X-Host-ID", hostID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("server error %d: %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

func (c *Client) get(ctx context.Context, path, hostID string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if hostID != "" {
		req.Header.Set("X-Host-ID", hostID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server error %d: %s", resp.StatusCode, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func exitCodeValue(code *int) any {
	if code == nil {
		return nil
	}
	return *code
}
