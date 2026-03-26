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

type AgentConfigBlock struct {
	Environment         string   `json:"environment"`
	Tags                []string `json:"tags"`
	HeartbeatInterval   string   `json:"heartbeat_interval"`
	DiscoveryInterval   string   `json:"discovery_interval"`
	ProcessScanInterval string   `json:"process_scan_interval"`
	ReportInterval      string   `json:"report_interval"`
	CommandPollInterval string   `json:"command_poll_interval"`
}
