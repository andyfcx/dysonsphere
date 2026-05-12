package api

// ErrorResponse is returned for any API error.
type ErrorResponse struct {
	Error string `json:"error"`
	Code  int    `json:"code,omitempty"`
}

// OKResponse is a simple success acknowledgment.
type OKResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

type LoginResponse struct {
	Token    string `json:"token"`
	Username string `json:"username"`
}

// StatsResponse is the overview dashboard stats payload.
type StatsResponse struct {
	TotalHosts    int64 `json:"total_hosts"`
	ActiveHosts   int64 `json:"active_hosts"`
	TotalJobs     int64 `json:"total_jobs"`
	ActiveAlerts  int64 `json:"active_alerts"`
	RecentFailed  int64 `json:"recent_failed"`
}

type EnrollResponse struct {
	HostID      string               `json:"host_id"`
	Credential  AgentCredentialBlock `json:"credential"`
	Config      AgentConfigBlock     `json:"config"`
}

type AgentCredentialBlock struct {
	Token string `json:"token"`
}

// EnrollmentTokenCreatedResponse is returned when a new enrollment token is generated.
type EnrollmentTokenCreatedResponse struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Payload   string `json:"payload"`   // base64url-encoded JSON for agent --payload flag
	ExpiresAt string `json:"expires_at"` // RFC3339
	CreatedAt string `json:"created_at"`
}

// EnrollmentTokenItem is a list entry (never exposes the raw token).
type EnrollmentTokenItem struct {
	ID        string  `json:"id"`
	Label     string  `json:"label"`
	Used      bool    `json:"used"`
	UsedAt    *string `json:"used_at,omitempty"`
	ExpiresAt string  `json:"expires_at"`
	CreatedAt string  `json:"created_at"`
}

type AgentConfigBlock struct {
	Environment         string   `json:"environment"`
	Tags                []string `json:"tags"`
	HeartbeatInterval   string   `json:"heartbeat_interval"`
	DiscoveryInterval   string   `json:"discovery_interval"`
	ProcessScanInterval string   `json:"process_scan_interval"`
	ReportInterval      string   `json:"report_interval"`
	CommandPollInterval string   `json:"command_poll_interval"`
}
