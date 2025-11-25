package usecase

import (
	"context"
	"errors"
	"fmt"
	"path"
	"time"

	"github.com/google/uuid"
	"github.com/hszk-dev/gostream/internal/domain/model"
	"github.com/hszk-dev/gostream/internal/domain/repository"
)

var (
	// ErrVideoAlreadyCompleted is returned when attempting to process a video that has already completed.
	ErrVideoAlreadyCompleted = errors.New("video processing has already completed")
)

// CreateVideoInput contains the input parameters for creating a video.
type CreateVideoInput struct {
	UserID   uuid.UUID
	Title    string
	FileName string
}

// CreateVideoOutput contains the result of creating a video.
type CreateVideoOutput struct {
	Video     *model.Video
	UploadURL string
}

// VideoService defines the interface for video business logic operations.
type VideoService interface {
	// CreateVideo creates video metadata and returns a presigned upload URL.
	CreateVideo(ctx context.Context, input CreateVideoInput) (*CreateVideoOutput, error)

	// TriggerProcess initiates transcoding for an uploaded video.
	// This operation is idempotent - calling it on an already processing video returns nil.
	TriggerProcess(ctx context.Context, videoID uuid.UUID) error

	// GetVideo retrieves video information by ID.
	GetVideo(ctx context.Context, videoID uuid.UUID) (*model.Video, error)
}

// VideoServiceConfig holds configuration for VideoService.
type VideoServiceConfig struct {
	UploadURLExpiry time.Duration
}

// DefaultVideoServiceConfig returns the default configuration.
func DefaultVideoServiceConfig() VideoServiceConfig {
	return VideoServiceConfig{
		UploadURLExpiry: 15 * time.Minute,
	}
}

type videoService struct {
	repo    repository.VideoRepository
	storage repository.ObjectStorage
	queue   repository.MessageQueue

	uploadURLExpiry time.Duration
}

// NewVideoService creates a new VideoService instance.
func NewVideoService(
	repo repository.VideoRepository,
	storage repository.ObjectStorage,
	queue repository.MessageQueue,
	cfg VideoServiceConfig,
) VideoService {
	return &videoService{
		repo:            repo,
		storage:         storage,
		queue:           queue,
		uploadURLExpiry: cfg.UploadURLExpiry,
	}
}

// CreateVideo creates video metadata and generates a presigned upload URL.
func (s *videoService) CreateVideo(ctx context.Context, input CreateVideoInput) (*CreateVideoOutput, error) {
	video, err := model.NewVideo(input.UserID, input.Title)
	if err != nil {
		return nil, err
	}

	key := s.generateOriginalKey(video.ID, input.FileName)

	uploadURL, err := s.storage.GeneratePresignedUploadURL(ctx, key, s.uploadURLExpiry)
	if err != nil {
		return nil, fmt.Errorf("generate presigned upload URL: %w", err)
	}

	video.SetOriginalURL(key)

	if err := s.repo.Create(ctx, video); err != nil {
		return nil, fmt.Errorf("create video: %w", err)
	}

	return &CreateVideoOutput{
		Video:     video,
		UploadURL: uploadURL,
	}, nil
}

// TriggerProcess initiates async transcoding for a video.
// Idempotency: returns nil if video is already processing.
func (s *videoService) TriggerProcess(ctx context.Context, videoID uuid.UUID) error {
	video, err := s.repo.GetByID(ctx, videoID)
	if err != nil {
		return err
	}

	if video.Status == model.StatusProcessing {
		return nil
	}

	if video.Status == model.StatusReady || video.Status == model.StatusFailed {
		return ErrVideoAlreadyCompleted
	}

	if err := video.TransitionTo(model.StatusProcessing); err != nil {
		return err
	}

	if err := s.repo.Update(ctx, video); err != nil {
		return fmt.Errorf("update video status: %w", err)
	}

	task := repository.TranscodeTask{
		VideoID:     video.ID,
		OriginalKey: video.OriginalURL,
		OutputKey:   s.generateHLSOutputKey(video.ID),
	}

	if err := s.queue.PublishTranscodeTask(ctx, task); err != nil {
		return fmt.Errorf("publish transcode task: %w", err)
	}

	return nil
}

// GetVideo retrieves video information by ID.
func (s *videoService) GetVideo(ctx context.Context, videoID uuid.UUID) (*model.Video, error) {
	return s.repo.GetByID(ctx, videoID)
}

// generateOriginalKey creates the storage key for original video files.
// Format: originals/{video_id}/{filename}
func (s *videoService) generateOriginalKey(videoID uuid.UUID, filename string) string {
	return path.Join("originals", videoID.String(), filename)
}

// generateHLSOutputKey creates the storage key prefix for HLS output.
// Format: hls/{video_id}/
func (s *videoService) generateHLSOutputKey(videoID uuid.UUID) string {
	return path.Join("hls", videoID.String()) + "/"
}
