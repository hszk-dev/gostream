package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/hszk-dev/gostream/internal/api/handler"
	"github.com/hszk-dev/gostream/internal/api/middleware"
	"github.com/hszk-dev/gostream/internal/config"
	"github.com/hszk-dev/gostream/internal/infrastructure/postgres"
	"github.com/hszk-dev/gostream/internal/infrastructure/queue"
	"github.com/hszk-dev/gostream/internal/infrastructure/storage"
	"github.com/hszk-dev/gostream/internal/usecase"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Initialize infrastructure clients
	pgClient, err := postgres.NewClient(ctx, postgres.DefaultClientConfig(cfg.Database.DSN()))
	if err != nil {
		return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}
	defer pgClient.Close()
	logger.Info("connected to PostgreSQL")

	storageClient, err := storage.NewClient(ctx, storage.ClientConfig{
		Endpoint:  cfg.MinIO.Endpoint,
		AccessKey: cfg.MinIO.AccessKey,
		SecretKey: cfg.MinIO.SecretKey,
		Bucket:    cfg.MinIO.Bucket,
		UseSSL:    cfg.MinIO.UseSSL,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to MinIO: %w", err)
	}
	logger.Info("connected to MinIO")

	queueClient, err := queue.NewClient(ctx, queue.DefaultClientConfig(cfg.RabbitMQ.URL()))
	if err != nil {
		return fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}
	defer queueClient.Close()
	logger.Info("connected to RabbitMQ")

	// Initialize repositories and services
	videoRepo := postgres.NewVideoRepository(pgClient.Pool())
	videoSvc := usecase.NewVideoService(videoRepo, storageClient, queueClient, usecase.DefaultVideoServiceConfig())

	// Initialize handlers
	videoHandler := handler.NewVideoHandler(videoSvc)

	r := setupRouter(logger, videoHandler)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      r,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("starting server", slog.Int("port", cfg.Server.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("server error: %w", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case sig := <-quit:
		logger.Info("shutting down server", slog.String("signal", sig.String()))
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown error: %w", err)
	}

	logger.Info("server stopped")
	return nil
}

func setupRouter(logger *slog.Logger, videoHandler *handler.VideoHandler) *chi.Mux {
	r := chi.NewRouter()

	r.Use(chimw.RequestID)
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger(logger))
	r.Use(middleware.Recoverer(logger))

	r.Get("/health", handler.Health)

	r.Route("/v1", func(r chi.Router) {
		r.Route("/videos", func(r chi.Router) {
			r.Post("/", videoHandler.Create)
			r.Post("/{id}/process", videoHandler.TriggerProcess)
			r.Get("/{id}", videoHandler.Get)
		})
	})

	return r
}
