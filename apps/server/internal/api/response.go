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

// StatsResponse is the overview dashboard stats payload.
type StatsResponse struct {
	TotalHosts    int64 `json:"total_hosts"`
	ActiveHosts   int64 `json:"active_hosts"`
	TotalJobs     int64 `json:"total_jobs"`
	ActiveAlerts  int64 `json:"active_alerts"`
	RecentFailed  int64 `json:"recent_failed"`
}
