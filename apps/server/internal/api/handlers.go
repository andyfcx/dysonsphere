package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/andyfcx/observer/server/internal/repository"
	"github.com/andyfcx/observer/server/internal/service"
)

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
func (h *Handler) ListExecutions(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePagination(r)
	execs, err := h.executions.List(r.Context(), limit, offset)
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

	// Count recent failed executions in last 24h
	execs, _ := h.execs.List(ctx, 200, 0)
	var recentFailed int64
	for _, e := range execs {
		if e.Status == "failed" || e.Status == "unknown" {
			recentFailed++
		}
	}

	writeJSON(w, http.StatusOK, StatsResponse{
		TotalHosts:   int64(len(hosts)),
		ActiveHosts:  activeHosts,
		TotalJobs:    int64(len(jobs)),
		ActiveAlerts: activeAlerts,
		RecentFailed: recentFailed,
	})
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
