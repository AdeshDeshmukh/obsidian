package api

import (
	"net/http"

	"obsidian/internal/api/handlers"
	"obsidian/internal/api/middleware"
	"obsidian/internal/api/ws"

	"github.com/go-chi/chi/v5"
	chi_middleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewRouter(pool *pgxpool.Pool) http.Handler {
	r := chi.NewRouter()

	// Base middleware
	r.Use(chi_middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(chi_middleware.Recoverer)
	r.Use(middleware.CORS)

	h := handlers.NewHandler(pool)

	// Health Check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy"}`))
	})

	// Public Routes
	r.Route("/api/auth", func(r chi.Router) {
		r.Post("/register", h.Register)
		r.Post("/login", h.Login)
	})

	// Authenticated Routes
	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth)

		r.Get("/api/auth/me", h.GetMe)
		r.Get("/api/ws", ws.ServeWS)

		// Projects
		r.Route("/api/projects", func(r chi.Router) {
			r.With(middleware.RequireRole("admin", "member")).Post("/", h.CreateProject)
			r.Get("/", h.ListProjects)
			
			// Queues under Projects
			r.With(middleware.RequireRole("admin", "member")).Post("/{projectId}/queues", h.CreateQueue)
			r.Get("/{projectId}/queues", h.ListQueues)
		})

		// Queues
		r.Route("/api/queues/{queueId}", func(r chi.Router) {
			r.With(middleware.RequireRole("admin")).Put("/", h.UpdateQueue) // Admin required for queue pause/config updates
			r.With(middleware.RequireRole("admin", "member")).Post("/jobs", h.CreateJob)
			r.With(middleware.RequireRole("admin", "member")).Post("/jobs/batch", h.CreateBatchJobs)
			r.Get("/jobs", h.ListJobs)
		})

		// Jobs
		r.Route("/api/jobs/{jobId}", func(r chi.Router) {
			r.Get("/", h.GetJob)
			r.With(middleware.RequireRole("admin", "member")).Post("/retry", h.RetryJob)
			r.With(middleware.RequireRole("admin")).Post("/cancel", h.CancelJob) // Admin required to cancel active job
		})

		// Workers
		r.Get("/api/workers", h.ListWorkers)

		// Dead Letter Queue
		r.Get("/api/dlq", h.ListDLQ)

		// Metrics
		r.Get("/api/metrics/system-health", h.GetSystemHealth)
		r.Get("/api/metrics/throughput", h.GetThroughput)
	})

	return r
}
