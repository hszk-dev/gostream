package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/redis/go-redis/v9"

	"github.com/hszk-dev/gostream/internal/config"
	"github.com/hszk-dev/gostream/internal/domain/repository"
	"github.com/hszk-dev/gostream/internal/infrastructure/cache"
	"github.com/hszk-dev/gostream/internal/infrastructure/postgres"
	"github.com/hszk-dev/gostream/internal/infrastructure/queue"
	"github.com/hszk-dev/gostream/internal/infrastructure/storage"
	"github.com/hszk-dev/gostream/internal/transcoder"
	"github.com/hszk-dev/gostream/internal/usecase"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Ensure temp directory exists
	if err := os.MkdirAll(cfg.Worker.TempDir, 0755); err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}

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

	// Initialize Redis client for cache invalidation
	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr(),
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer redisClient.Close()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}
	logger.Info("connected to Redis")

	// Initialize transcoder
	tc := transcoder.NewFFmpegTranscoder(transcoder.DefaultFFmpegConfig())

	// Initialize repository and service
	videoRepo := postgres.NewVideoRepository(pgClient.Pool())
	videoCache := cache.NewRedisVideoCache(redisClient)
	transcodeSvc := usecase.NewTranscodeService(
		videoRepo,
		storageClient,
		tc,
		videoCache,
		usecase.TranscodeServiceConfig{
			TempDir:    cfg.Worker.TempDir,
			MaxRetries: cfg.Worker.MaxRetries,
		},
	)

	// Setup signal handling for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// WaitGroup to track in-flight tasks
	var wg sync.WaitGroup

	// Start consuming messages in a goroutine
	errCh := make(chan error, 1)
	go func() {
		logger.Info("starting worker, consuming transcode tasks")
		err := queueClient.ConsumeTranscodeTasks(ctx, func(task repository.TranscodeTask) error {
			wg.Add(1)
			defer wg.Done()

			logger.Info("processing task",
				slog.String("video_id", task.VideoID.String()),
				slog.Int("retry_count", task.RetryCount),
			)

			if err := transcodeSvc.ProcessTask(ctx, task); err != nil {
				logger.Error("task processing failed",
					slog.String("video_id", task.VideoID.String()),
					slog.Int("retry_count", task.RetryCount),
					slog.String("error", err.Error()),
				)
				return err
			}

			logger.Info("task completed successfully",
				slog.String("video_id", task.VideoID.String()),
			)
			return nil
		})
		if err != nil && ctx.Err() == nil {
			errCh <- fmt.Errorf("consumer error: %w", err)
		}
	}()

	// Wait for shutdown signal or error
	select {
	case err := <-errCh:
		return err
	case sig := <-quit:
		logger.Info("shutting down worker", slog.String("signal", sig.String()))
	}

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Worker.ShutdownTimeout)
	defer shutdownCancel()

	// Cancel the main context to stop consuming new messages
	cancel()

	// Wait for in-flight tasks to complete (or timeout)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("all in-flight tasks completed")
	case <-shutdownCtx.Done():
		logger.Warn("shutdown timeout exceeded, some tasks may not have completed")
	}

	logger.Info("worker stopped")
	return nil
}
