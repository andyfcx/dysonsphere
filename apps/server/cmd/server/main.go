// Command server is the Observer central API server.
// It accepts reports from agents, stores them in PostgreSQL,
// evaluates alerting rules, and exposes a REST API for the dashboard.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/andyfcx/observer/server/internal/alerting"
	"github.com/andyfcx/observer/server/internal/api"
	"github.com/andyfcx/observer/server/internal/config"
	"github.com/andyfcx/observer/server/internal/db"
	"github.com/andyfcx/observer/server/internal/repository"
	"github.com/andyfcx/observer/server/internal/service"
)

func main() {
	// Structured logging to stdout.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	if err := run(); err != nil {
		slog.Error("server exited with error", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Handle migrate subcommand: `server migrate up|down`
	if len(os.Args) >= 3 && os.Args[1] == "migrate" {
		return runMigrate(cfg, os.Args[2])
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	pool, err := db.Connect(ctx, db.Config{
		Host:     cfg.Database.Host,
		Port:     cfg.Database.Port,
		User:     cfg.Database.User,
		Password: cfg.Database.Password,
		DBName:   cfg.Database.DBName,
		SSLMode:  cfg.Database.SSLMode,
	})
	cancel()
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()

	// Run migrations automatically on startup.
	migrationsDir := "./migrations"
	if dir := os.Getenv("MIGRATIONS_DIR"); dir != "" {
		migrationsDir = dir
	}
	pgDSN := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.Database.User, cfg.Database.Password,
		cfg.Database.Host, cfg.Database.Port,
		cfg.Database.DBName, cfg.Database.SSLMode,
	)
	if err := db.MigrateUp(pgDSN, migrationsDir); err != nil {
		return fmt.Errorf("migrate up: %w", err)
	}

	// Wire up repositories.
	hostRepo := repository.NewHostRepo(pool)
	jobRepo := repository.NewJobRepo(pool)
	execRepo := repository.NewExecutionRepo(pool)
	metricRepo := repository.NewMetricRepo(pool)
	alertRepo := repository.NewAlertRepo(pool)
	commandRepo := repository.NewCommandRepo(pool)
	credentialRepo := repository.NewCredentialRepo(pool)

	// Wire up services.
	agentSvc := service.NewAgentService(hostRepo, credentialRepo, cfg.Server.EnrollmentToken)
	commandSvc := service.NewCommandService(commandRepo, jobRepo)
	discoverySvc := service.NewDiscoveryService(jobRepo)
	executionSvc := service.NewExecutionService(execRepo, jobRepo)
	metricSvc := service.NewMetricService(metricRepo)

	// Wire up alert evaluator.
	evaluator := alerting.NewEvaluator(hostRepo, execRepo, alertRepo, jobRepo)

	// Build HTTP handler.
	authManager := api.NewAuthManager(
		cfg.Server.Token,
		cfg.Server.LoginUsername,
		cfg.Server.LoginPassword,
		agentSvc,
	)
	handler := api.NewHandler(
		authManager,
		agentSvc, commandSvc, discoverySvc, executionSvc, metricSvc,
		hostRepo, jobRepo, alertRepo, execRepo,
	)
	router := api.NewRouter(handler)

	// Start background alert evaluation loop.
	stopEval := startAlertEvaluator(evaluator)
	defer close(stopEval)

	// Start HTTP server.
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("server starting", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("listen error", "err", err)
			quit <- syscall.SIGTERM
		}
	}()

	<-quit
	slog.Info("shutting down...")
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}

func runMigrate(cfg *config.Config, direction string) error {
	pgDSN := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.Database.User, cfg.Database.Password,
		cfg.Database.Host, cfg.Database.Port,
		cfg.Database.DBName, cfg.Database.SSLMode,
	)
	migrationsDir := "./migrations"
	switch direction {
	case "up":
		return db.MigrateUp(pgDSN, migrationsDir)
	case "down":
		return db.MigrateDown(pgDSN, migrationsDir)
	default:
		return fmt.Errorf("unknown migration direction %q (use up|down)", direction)
	}
}

// startAlertEvaluator runs the evaluator every minute in a goroutine.
func startAlertEvaluator(ev *alerting.Evaluator) chan struct{} {
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				ev.RunAll(context.Background())
			case <-stop:
				return
			}
		}
	}()
	return stop
}
