package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// NewRouter builds the chi router with all routes wired up.
func NewRouter(h *Handler) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Host-ID"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// Health check (no auth required).
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Unauthenticated endpoints.
	r.Post("/api/v1/auth/login", h.Login)
	r.Post("/api/v1/agents/enroll", h.EnrollAgent)

	// All /api/v1 routes require either the agent bearer token or a dashboard login session.
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(h.auth.APIAuthMiddleware)

		r.Post("/auth/logout", h.Logout)

		// Agent endpoints
		r.Post("/agents/register", h.RegisterAgent)
		r.Post("/agents/heartbeat", h.AgentHeartbeat)
		r.Get("/agents/commands", h.ClaimAgentCommands)
		r.Post("/agents/commands/{id}/complete", h.CompleteAgentCommand)

		// Discovery
		r.Post("/jobs/discovery", h.SubmitDiscovery)

		// Executions
		r.Post("/executions/batch", h.BatchExecutions)
		r.Get("/executions", h.ListExecutions)

		// Metrics
		r.Post("/metrics/batch", h.BatchMetrics)
		r.Get("/metrics", h.ListMetrics)

		// Read-only resource endpoints
		r.Get("/hosts", h.ListHosts)
		r.Post("/hosts/{id}/commands/run", h.TriggerHostJobs)
		r.Get("/jobs", h.ListJobs)
		r.Get("/jobs/{id}", h.GetJob)
		r.Get("/alerts", h.ListAlerts)
		r.Get("/stats", h.GetStats)
		r.Get("/stats/failures", h.GetWindowStats)
		r.Get("/stats/trend", h.GetFailureTrend)
		r.Get("/stats/jobs", h.GetJobStats)
		r.Get("/stats/hosts", h.GetHostStats)
		r.Get("/stats/recovery", h.GetRecoveryStats)

		// Enrollment token management (dashboard only)
		r.Post("/enrollment-tokens", h.CreateEnrollmentToken)
		r.Get("/enrollment-tokens", h.ListEnrollmentTokens)
	})

	return r
}
