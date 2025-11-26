package usecase

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/hszk-dev/gostream/internal/domain/model"
	"github.com/hszk-dev/gostream/internal/domain/repository"
	"github.com/hszk-dev/gostream/internal/transcoder"
)

const (
	// DefaultMaxRetries is the default maximum number of retry attempts before marking as failed.
	DefaultMaxRetries = 3
)

// TranscodeServiceConfig holds configuration for TranscodeService.
type TranscodeServiceConfig struct {
	// TempDir is the base directory for temporary files during transcoding.
	TempDir string
	// MaxRetries is the maximum number of retry attempts before marking video as failed.
	MaxRetries int
}

// DefaultTranscodeServiceConfig returns the default configuration.
func DefaultTranscodeServiceConfig() TranscodeServiceConfig {
	return TranscodeServiceConfig{
		TempDir:    os.TempDir(),
		MaxRetries: DefaultMaxRetries,
	}
}

// TranscodeService defines the interface for video transcoding operations.
type TranscodeService interface {
	// ProcessTask handles a transcoding task from the message queue.
	// Returns nil on success or permanent failure (max retries exceeded).
	// Returns error for transient failures that should trigger a retry.
	ProcessTask(ctx context.Context, task repository.TranscodeTask) error
}

type transcodeService struct {
	repo       repository.VideoRepository
	storage    repository.ObjectStorage
	transcoder transcoder.Transcoder

	tempDir    string
	maxRetries int
}

// NewTranscodeService creates a new TranscodeService instance.
func NewTranscodeService(
	repo repository.VideoRepository,
	storage repository.ObjectStorage,
	tc transcoder.Transcoder,
	cfg TranscodeServiceConfig,
) TranscodeService {
	return &transcodeService{
		repo:       repo,
		storage:    storage,
		transcoder: tc,
		tempDir:    cfg.TempDir,
		maxRetries: cfg.MaxRetries,
	}
}

// ProcessTask handles a transcoding task.
// It downloads the original video, transcodes to HLS, uploads the results,
// and updates the video status in the database.
func (s *transcodeService) ProcessTask(ctx context.Context, task repository.TranscodeTask) error {
	// Check if max retries exceeded - mark as failed and return nil (ack the message)
	if task.RetryCount >= s.maxRetries {
		if err := s.markVideoFailed(ctx, task.VideoID); err != nil {
			slog.Error("failed to mark video as failed",
				"video_id", task.VideoID,
				"retry_count", task.RetryCount,
				"error", err,
			)
			// Still return nil to ack the message
			// The video remains in PROCESSING state, which is acceptable
			return nil
		}
		return nil
	}

	// Create temporary working directory for this task
	workDir, err := s.createWorkDir(task.VideoID)
	if err != nil {
		return fmt.Errorf("create work directory: %w", err)
	}
	defer s.cleanup(workDir)

	// Download original video
	inputPath, err := s.downloadOriginal(ctx, task.OriginalKey, workDir)
	if err != nil {
		return fmt.Errorf("download original: %w", err)
	}

	// Create output directory for HLS files
	outputDir := filepath.Join(workDir, "hls")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	// Transcode to HLS
	hlsOutput, err := s.transcoder.TranscodeToHLS(ctx, inputPath, outputDir)
	if err != nil {
		return fmt.Errorf("transcode: %w", err)
	}

	// Upload HLS files to object storage
	manifestKey, err := s.uploadHLSFiles(ctx, task.OutputKey, hlsOutput)
	if err != nil {
		return fmt.Errorf("upload HLS files: %w", err)
	}

	// Update video status to READY
	if err := s.markVideoReady(ctx, task.VideoID, manifestKey); err != nil {
		return fmt.Errorf("update video status: %w", err)
	}

	return nil
}

// createWorkDir creates a temporary directory for processing a specific video.
func (s *transcodeService) createWorkDir(videoID uuid.UUID) (string, error) {
	workDir := filepath.Join(s.tempDir, "gostream", videoID.String())
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}
	return workDir, nil
}

// cleanup removes the temporary working directory.
func (s *transcodeService) cleanup(workDir string) {
	_ = os.RemoveAll(workDir)
}

// downloadOriginal downloads the original video from object storage to a local file.
func (s *transcodeService) downloadOriginal(ctx context.Context, key, workDir string) (string, error) {
	reader, err := s.storage.Download(ctx, key)
	if err != nil {
		return "", fmt.Errorf("storage download: %w", err)
	}
	defer func() { _ = reader.Close() }()

	// Extract filename from key or use default
	filename := filepath.Base(key)
	if filename == "." || filename == "/" {
		filename = "original.mp4"
	}

	localPath := filepath.Join(workDir, filename)
	file, err := os.Create(localPath)
	if err != nil {
		return "", fmt.Errorf("create local file: %w", err)
	}

	if _, err := io.Copy(file, reader); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("copy to local file: %w", err)
	}

	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close local file: %w", err)
	}

	return localPath, nil
}

// uploadHLSFiles uploads all HLS files (manifest and segments) to object storage.
// Returns the full key path to the manifest file.
func (s *transcodeService) uploadHLSFiles(ctx context.Context, outputKeyPrefix string, hlsOutput *transcoder.HLSOutput) (string, error) {
	// Upload manifest
	manifestKey := outputKeyPrefix + filepath.Base(hlsOutput.ManifestPath)
	if err := s.uploadFile(ctx, hlsOutput.ManifestPath, manifestKey, "application/vnd.apple.mpegurl"); err != nil {
		return "", fmt.Errorf("upload manifest: %w", err)
	}

	// Upload segments
	for _, segmentPath := range hlsOutput.SegmentPaths {
		segmentKey := outputKeyPrefix + filepath.Base(segmentPath)
		if err := s.uploadFile(ctx, segmentPath, segmentKey, "video/mp2t"); err != nil {
			return "", fmt.Errorf("upload segment %s: %w", filepath.Base(segmentPath), err)
		}
	}

	return manifestKey, nil
}

// uploadFile uploads a single file to object storage.
func (s *transcodeService) uploadFile(ctx context.Context, localPath, key, contentType string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = file.Close() }()

	if err := s.storage.Upload(ctx, key, file, contentType); err != nil {
		return fmt.Errorf("storage upload: %w", err)
	}

	return nil
}

// markVideoReady updates the video status to READY and sets the HLS URL.
func (s *transcodeService) markVideoReady(ctx context.Context, videoID uuid.UUID, hlsKey string) error {
	video, err := s.repo.GetByID(ctx, videoID)
	if err != nil {
		return fmt.Errorf("get video: %w", err)
	}

	// Validate current status
	if video.Status != model.StatusProcessing {
		// Video is not in expected state - log but don't fail
		return nil
	}

	video.SetHLSURL(hlsKey)
	if err := video.TransitionTo(model.StatusReady); err != nil {
		return fmt.Errorf("transition to ready: %w", err)
	}

	if err := s.repo.Update(ctx, video); err != nil {
		return fmt.Errorf("update video: %w", err)
	}

	return nil
}

// markVideoFailed updates the video status to FAILED.
func (s *transcodeService) markVideoFailed(ctx context.Context, videoID uuid.UUID) error {
	video, err := s.repo.GetByID(ctx, videoID)
	if err != nil {
		return fmt.Errorf("get video: %w", err)
	}

	// Only transition if in PROCESSING state
	if video.Status != model.StatusProcessing {
		return nil
	}

	if err := video.TransitionTo(model.StatusFailed); err != nil {
		return fmt.Errorf("transition to failed: %w", err)
	}

	if err := s.repo.Update(ctx, video); err != nil {
		return fmt.Errorf("update video: %w", err)
	}

	return nil
}
