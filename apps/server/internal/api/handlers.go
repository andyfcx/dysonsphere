package api

import (
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/andyfcx/observer/server/internal/repository"
	"github.com/andyfcx/observer/server/internal/service"
)

// parseWindow converts the ?window= query param to a since time and bucket size.
// Returns since time, label string, and bucket width in seconds.
func parseWindow(r *http.Request) (since time.Time, label string, bucketSeconds int) {
	switch r.URL.Query().Get("window") {
	case "7d":
		return time.Now().Add(-7 * 24 * time.Hour), "7d", 6 * 3600
	case "30d":
		return time.Now().Add(-30 * 24 * time.Hour), "30d", 86400
	default:
		return time.Now().Add(-24 * time.Hour), "24h", 3600
	}
}

// Handler holds all service dependencies for HTTP handlers.
type Handler struct {
	auth       *AuthManager
	agents     *service.AgentService
	commands   *service.CommandService
	discovery  *service.DiscoveryService
	executions *service.ExecutionService
	metrics    *service.MetricService
	hosts      *repository.HostRepo
	jobs       *repository.JobRepo
	alerts     *repository.AlertRepo
	execs      *repository.ExecutionRepo
	stats      *repository.StatsRepo
	publicURL  string
}

func NewHandler(
	auth *AuthManager,
	agents *service.AgentService,
	commands *service.CommandService,
	discovery *service.DiscoveryService,
	executions *service.ExecutionService,
	metrics *service.MetricService,
	hosts *repository.HostRepo,
	jobs *repository.JobRepo,
	alerts *repository.AlertRepo,
	execs *repository.ExecutionRepo,
	stats *repository.StatsRepo,
	publicURL string,
) *Handler {
	return &Handler{
		auth:       auth,
		agents:     agents,
		commands:   commands,
		discovery:  discovery,
		executions: executions,
		metrics:    metrics,
		hosts:      hosts,
		jobs:       jobs,
		alerts:     alerts,
		execs:      execs,
		stats:      stats,
		publicURL:  publicURL,
	}
}

// Login handles POST /api/v1/auth/login
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	resp, ok := h.auth.Login(req.Username, req.Password)
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// Logout handles POST /api/v1/auth/logout
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	h.auth.Logout(bearerToken(r.Header.Get("Authorization")))
	writeJSON(w, http.StatusOK, OKResponse{OK: true})
}

// ── Agent ──────────────────────────────────────────────────────────────────

// EnrollAgent handles POST /api/v1/agents/enroll
func (h *Handler) EnrollAgent(w http.ResponseWriter, r *http.Request) {
	var req EnrollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.EnrollmentToken == "" || req.MachineID == "" || req.Hostname == "" {
		writeError(w, http.StatusBadRequest, "enrollment_token, machine_id, and hostname are required")
		return
	}

	result, err := h.agents.Enroll(r.Context(), req.EnrollmentToken, req.toDomain())
	if err != nil {
		status := http.StatusUnauthorized
		if err.Error() == "enrollment token has already been used" {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, EnrollResponse{
		HostID: result.Host.ID,
		Credential: AgentCredentialBlock{
			Token: result.AgentToken,
		},
		Config: AgentConfigBlock{
			Environment:         result.Environment,
			Tags:                result.Tags,
			HeartbeatInterval:   "30s",
			DiscoveryInterval:   "5m",
			ProcessScanInterval: "30s",
			ReportInterval:      "1m",
			CommandPollInterval: "15s",
		},
	})
}

// RegisterAgent handles POST /api/v1/agents/register
func (h *Handler) RegisterAgent(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.MachineID == "" || req.Hostname == "" {
		writeError(w, http.StatusBadRequest, "machine_id and hostname are required")
		return
	}

	host, err := h.agents.Register(r.Context(), req.toDomain())
	if err != nil {
		slog.Error("register agent", "err", err)
		writeError(w, http.StatusInternalServerError, "registration failed")
		return
	}
	writeJSON(w, http.StatusCreated, host)
}

// AgentHeartbeat handles POST /api/v1/agents/heartbeat
func (h *Handler) AgentHeartbeat(w http.ResponseWriter, r *http.Request) {
	hostID := r.Header.Get("X-Host-ID")
	if hostID == "" {
		writeError(w, http.StatusBadRequest, "X-Host-ID header required")
		return
	}
	var req HeartbeatRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	if err := h.agents.Heartbeat(r.Context(), hostID, req.IPAddress); err != nil {
		slog.Error("heartbeat", "host_id", hostID, "err", err)
		writeError(w, http.StatusInternalServerError, "heartbeat failed")
		return
	}
	writeJSON(w, http.StatusOK, OKResponse{OK: true})
}

// ClaimAgentCommands handles GET /api/v1/agents/commands
func (h *Handler) ClaimAgentCommands(w http.ResponseWriter, r *http.Request) {
	hostID := r.Header.Get("X-Host-ID")
	if hostID == "" {
		writeError(w, http.StatusBadRequest, "X-Host-ID header required")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	commands, err := h.commands.ClaimPending(r.Context(), hostID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "claim commands failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"commands": commands})
}

// CompleteAgentCommand handles POST /api/v1/agents/commands/{id}/complete
func (h *Handler) CompleteAgentCommand(w http.ResponseWriter, r *http.Request) {
	var req CompleteCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := h.commands.Complete(r.Context(), chi.URLParam(r, "id"), req.Status, req.Message, req.StartedAt, req.FinishedAt, req.ExitCode); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, OKResponse{OK: true})
}

// ── Discovery ──────────────────────────────────────────────────────────────

// SubmitDiscovery handles POST /api/v1/jobs/discovery
func (h *Handler) SubmitDiscovery(w http.ResponseWriter, r *http.Request) {
	hostID := r.Header.Get("X-Host-ID")
	if hostID == "" {
		writeError(w, http.StatusBadRequest, "X-Host-ID header required")
		return
	}
	var req DiscoveryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	jobs, err := h.discovery.ProcessDiscovery(r.Context(), hostID, req.toDomain())
	if err != nil {
		slog.Error("process discovery", "host_id", hostID, "err", err)
		writeError(w, http.StatusInternalServerError, "discovery processing failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"upserted": len(jobs), "jobs": jobs})
}

// ── Executions ─────────────────────────────────────────────────────────────

// BatchExecutions handles POST /api/v1/executions/batch
func (h *Handler) BatchExecutions(w http.ResponseWriter, r *http.Request) {
	hostID := r.Header.Get("X-Host-ID")
	if hostID == "" {
		writeError(w, http.StatusBadRequest, "X-Host-ID header required")
		return
	}
	var req ExecutionBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	execs, err := h.executions.BatchReport(r.Context(), hostID, req.toDomain())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "batch report failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"accepted": len(execs)})
}

// ListExecutions handles GET /api/v1/executions
// Supports optional query params: status, host_id, job_id, window (24h|7d|30d).
func (h *Handler) ListExecutions(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePagination(r)

	f := repository.ExecutionFilter{
		HostID: r.URL.Query().Get("host_id"),
		JobID:  r.URL.Query().Get("job_id"),
		Status: r.URL.Query().Get("status"),
	}
	if window := r.URL.Query().Get("window"); window != "" {
		since, _, _ := parseWindow(r)
		f.Since = &since
	}

	execs, err := h.execs.ListFiltered(r.Context(), f, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list executions failed")
		return
	}
	writeJSON(w, http.StatusOK, execs)
}

// ── Metrics ────────────────────────────────────────────────────────────────

// BatchMetrics handles POST /api/v1/metrics/batch
func (h *Handler) BatchMetrics(w http.ResponseWriter, r *http.Request) {
	hostID := r.Header.Get("X-Host-ID")
	if hostID == "" {
		writeError(w, http.StatusBadRequest, "X-Host-ID header required")
		return
	}
	var req MetricBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	metrics, err := h.metrics.BatchReport(r.Context(), hostID, req.toDomain())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "metric batch failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"accepted": len(metrics)})
}

// ListMetrics handles GET /api/v1/metrics
func (h *Handler) ListMetrics(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePagination(r)
	metrics, err := h.metrics.List(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list metrics failed")
		return
	}
	writeJSON(w, http.StatusOK, metrics)
}

// ── Hosts ──────────────────────────────────────────────────────────────────

// ListHosts handles GET /api/v1/hosts
func (h *Handler) ListHosts(w http.ResponseWriter, r *http.Request) {
	hosts, err := h.hosts.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list hosts failed")
		return
	}
	writeJSON(w, http.StatusOK, hosts)
}

// ── Jobs ───────────────────────────────────────────────────────────────────

// ListJobs handles GET /api/v1/jobs
func (h *Handler) ListJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := h.jobs.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list jobs failed")
		return
	}
	writeJSON(w, http.StatusOK, jobs)
}

// GetJob handles GET /api/v1/jobs/{id}
func (h *Handler) GetJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job, err := h.discovery.GetJob(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}

	execs, _ := h.executions.ListByJob(r.Context(), id, 20)
	writeJSON(w, http.StatusOK, map[string]any{
		"job":        job,
		"executions": execs,
	})
}

// TriggerHostJobs handles POST /api/v1/hosts/{id}/commands/run
func (h *Handler) TriggerHostJobs(w http.ResponseWriter, r *http.Request) {
	var req TriggerJobsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if len(req.JobIDs) == 0 {
		writeError(w, http.StatusBadRequest, "job_ids is required")
		return
	}
	commands, err := h.commands.EnqueueRuns(r.Context(), chi.URLParam(r, "id"), req.JobIDs)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"queued": len(commands), "commands": commands})
}

// ── Alerts ─────────────────────────────────────────────────────────────────

// ListAlerts handles GET /api/v1/alerts
func (h *Handler) ListAlerts(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePagination(r)
	alerts, err := h.alerts.List(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list alerts failed")
		return
	}
	writeJSON(w, http.StatusOK, alerts)
}

// ── Stats ──────────────────────────────────────────────────────────────────

// GetStats handles GET /api/v1/stats (overview dashboard data)
func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	hosts, _ := h.hosts.List(ctx)
	var activeHosts int64
	for _, host := range hosts {
		if host.Status == "active" {
			activeHosts++
		}
	}

	jobs, _ := h.jobs.List(ctx)
	activeAlerts, _ := h.alerts.CountActive(ctx)

	ws, _ := h.stats.GetWindowStats(ctx, time.Now().Add(-24*time.Hour), "24h")
	var recentFailed int64
	if ws != nil {
		recentFailed = ws.Failed + ws.Unknown
	}

	writeJSON(w, http.StatusOK, StatsResponse{
		TotalHosts:   int64(len(hosts)),
		ActiveHosts:  activeHosts,
		TotalJobs:    int64(len(jobs)),
		ActiveAlerts: activeAlerts,
		RecentFailed: recentFailed,
	})
}

// ── Analytics stats ────────────────────────────────────────────────────────

// GetWindowStats handles GET /api/v1/stats/failures?window=24h|7d|30d
func (h *Handler) GetWindowStats(w http.ResponseWriter, r *http.Request) {
	since, label, _ := parseWindow(r)
	ws, err := h.stats.GetWindowStats(r.Context(), since, label)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "stats query failed")
		return
	}
	writeJSON(w, http.StatusOK, ws)
}

// GetFailureTrend handles GET /api/v1/stats/trend?window=24h|7d|30d
func (h *Handler) GetFailureTrend(w http.ResponseWriter, r *http.Request) {
	since, _, bucketSeconds := parseWindow(r)
	trend, err := h.stats.GetFailureTrend(r.Context(), since, bucketSeconds)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "trend query failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"trend": trend})
}

// GetJobStats handles GET /api/v1/stats/jobs?window=24h|7d|30d
func (h *Handler) GetJobStats(w http.ResponseWriter, r *http.Request) {
	since, _, _ := parseWindow(r)
	rates, err := h.stats.GetJobSuccessRates(r.Context(), since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "job stats query failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": rates})
}

// GetHostStats handles GET /api/v1/stats/hosts?window=24h|7d|30d
func (h *Handler) GetHostStats(w http.ResponseWriter, r *http.Request) {
	since, _, _ := parseWindow(r)
	counts, err := h.stats.GetHostFailureCounts(r.Context(), since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "host stats query failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"hosts": counts})
}

// GetRecoveryStats handles GET /api/v1/stats/recovery
func (h *Handler) GetRecoveryStats(w http.ResponseWriter, r *http.Request) {
	summaries, err := h.stats.GetJobRecoveryStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "recovery stats query failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": summaries})
}

// ── Enrollment tokens ──────────────────────────────────────────────────────

// CreateEnrollmentToken handles POST /api/v1/enrollment-tokens
func (h *Handler) CreateEnrollmentToken(w http.ResponseWriter, r *http.Request) {
	var req CreateEnrollmentTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	serverURL := req.ServerURL
	if serverURL == "" {
		serverURL = h.publicURL
	}
	if serverURL == "" {
		writeError(w, http.StatusBadRequest, "server_url is required (set SERVER_PUBLIC_URL or pass server_url in body)")
		return
	}

	rawToken, record, err := h.agents.GenerateEnrollmentToken(r.Context(), req.Label)
	if err != nil {
		slog.Error("generate enrollment token", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	type payloadJSON struct {
		ServerURL string `json:"server_url"`
		Token     string `json:"token"`
	}
	payloadBytes, _ := json.Marshal(payloadJSON{ServerURL: serverURL, Token: rawToken})
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)

	writeJSON(w, http.StatusCreated, EnrollmentTokenCreatedResponse{
		ID:        record.ID,
		Label:     record.Label,
		Payload:   payload,
		ExpiresAt: record.ExpiresAt.Format(time.RFC3339),
		CreatedAt: record.CreatedAt.Format(time.RFC3339),
	})
}

// ListEnrollmentTokens handles GET /api/v1/enrollment-tokens
func (h *Handler) ListEnrollmentTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := h.agents.ListEnrollmentTokens(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list enrollment tokens failed")
		return
	}
	items := make([]EnrollmentTokenItem, 0, len(tokens))
	for _, t := range tokens {
		item := EnrollmentTokenItem{
			ID:        t.ID,
			Label:     t.Label,
			Used:      t.Used,
			ExpiresAt: t.ExpiresAt.Format(time.RFC3339),
			CreatedAt: t.CreatedAt.Format(time.RFC3339),
		}
		if t.UsedAt != nil {
			s := t.UsedAt.Format(time.RFC3339)
			item.UsedAt = &s
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, items)
}

// ── helpers ────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write JSON response", "err", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Error: msg, Code: status})
}

func parsePagination(r *http.Request) (limit, offset int) {
	limit = 50
	offset = 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 500 {
			limit = v
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}
	return
}
