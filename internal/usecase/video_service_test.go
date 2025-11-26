package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hszk-dev/gostream/internal/domain/model"
	"github.com/hszk-dev/gostream/internal/domain/repository"
)

func TestVideoService_CreateVideo(t *testing.T) {
	tests := []struct {
		name      string
		input     CreateVideoInput
		setupMock func(repo *mockVideoRepository, storage *mockObjectStorage)
		wantErr   error
		checkFn   func(t *testing.T, output *CreateVideoOutput)
	}{
		{
			name: "successful creation",
			input: CreateVideoInput{
				UserID:   uuid.New(),
				Title:    "Test Video",
				FileName: "video.mp4",
			},
			setupMock: func(repo *mockVideoRepository, storage *mockObjectStorage) {
				storage.generatePresignedUploadURLFn = func(ctx context.Context, key string, expiry time.Duration) (string, error) {
					if !strings.HasPrefix(key, "originals/") {
						t.Errorf("unexpected key prefix: %s", key)
					}
					return "http://minio:9000/bucket/upload?signature=xyz", nil
				}
				repo.createFn = func(ctx context.Context, video *model.Video) error {
					return nil
				}
			},
			wantErr: nil,
			checkFn: func(t *testing.T, output *CreateVideoOutput) {
				if output.Video == nil {
					t.Error("expected video to be non-nil")
				}
				if output.Video.Status != model.StatusPendingUpload {
					t.Errorf("expected status %s, got %s", model.StatusPendingUpload, output.Video.Status)
				}
				if output.UploadURL == "" {
					t.Error("expected upload URL to be non-empty")
				}
			},
		},
		{
			name: "invalid user ID",
			input: CreateVideoInput{
				UserID:   uuid.Nil,
				Title:    "Test Video",
				FileName: "video.mp4",
			},
			setupMock: func(repo *mockVideoRepository, storage *mockObjectStorage) {},
			wantErr:   model.ErrInvalidUserID,
		},
		{
			name: "empty title",
			input: CreateVideoInput{
				UserID:   uuid.New(),
				Title:    "",
				FileName: "video.mp4",
			},
			setupMock: func(repo *mockVideoRepository, storage *mockObjectStorage) {},
			wantErr:   model.ErrEmptyTitle,
		},
		{
			name: "title too long",
			input: CreateVideoInput{
				UserID:   uuid.New(),
				Title:    strings.Repeat("a", 256),
				FileName: "video.mp4",
			},
			setupMock: func(repo *mockVideoRepository, storage *mockObjectStorage) {},
			wantErr:   model.ErrTitleTooLong,
		},
		{
			name: "storage error",
			input: CreateVideoInput{
				UserID:   uuid.New(),
				Title:    "Test Video",
				FileName: "video.mp4",
			},
			setupMock: func(repo *mockVideoRepository, storage *mockObjectStorage) {
				storage.generatePresignedUploadURLFn = func(ctx context.Context, key string, expiry time.Duration) (string, error) {
					return "", errors.New("storage unavailable")
				}
			},
			wantErr: errors.New("generate presigned upload URL"),
		},
		{
			name: "repository error",
			input: CreateVideoInput{
				UserID:   uuid.New(),
				Title:    "Test Video",
				FileName: "video.mp4",
			},
			setupMock: func(repo *mockVideoRepository, storage *mockObjectStorage) {
				storage.generatePresignedUploadURLFn = func(ctx context.Context, key string, expiry time.Duration) (string, error) {
					return "http://example.com/upload", nil
				}
				repo.createFn = func(ctx context.Context, video *model.Video) error {
					return errors.New("database error")
				}
			},
			wantErr: errors.New("create video"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockVideoRepository{}
			storage := &mockObjectStorage{}
			queue := &mockMessageQueue{}

			tt.setupMock(repo, storage)

			svc := NewVideoService(repo, storage, queue, DefaultVideoServiceConfig())

			output, err := svc.CreateVideo(context.Background(), tt.input)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !errors.Is(err, tt.wantErr) && !strings.Contains(err.Error(), tt.wantErr.Error()) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.checkFn != nil {
				tt.checkFn(t, output)
			}
		})
	}
}

func TestVideoService_TriggerProcess(t *testing.T) {
	tests := []struct {
		name      string
		videoID   uuid.UUID
		setupMock func(repo *mockVideoRepository, queue *mockMessageQueue) *model.Video
		wantErr   error
	}{
		{
			name:    "successful trigger from pending upload",
			videoID: uuid.New(),
			setupMock: func(repo *mockVideoRepository, queue *mockMessageQueue) *model.Video {
				video := &model.Video{
					ID:          uuid.New(),
					UserID:      uuid.New(),
					Title:       "Test Video",
					Status:      model.StatusPendingUpload,
					OriginalURL: "originals/video-id/video.mp4",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				}
				repo.getByIDFn = func(ctx context.Context, id uuid.UUID) (*model.Video, error) {
					return video, nil
				}
				repo.updateFn = func(ctx context.Context, v *model.Video) error {
					if v.Status != model.StatusProcessing {
						t.Errorf("expected status %s, got %s", model.StatusProcessing, v.Status)
					}
					return nil
				}
				queue.publishTranscodeTaskFn = func(ctx context.Context, task repository.TranscodeTask) error {
					if task.VideoID != video.ID {
						t.Errorf("expected video ID %s, got %s", video.ID, task.VideoID)
					}
					if task.OriginalKey != video.OriginalURL {
						t.Errorf("expected original key %s, got %s", video.OriginalURL, task.OriginalKey)
					}
					return nil
				}
				return video
			},
			wantErr: nil,
		},
		{
			name:    "idempotent - already processing",
			videoID: uuid.New(),
			setupMock: func(repo *mockVideoRepository, queue *mockMessageQueue) *model.Video {
				video := &model.Video{
					ID:          uuid.New(),
					UserID:      uuid.New(),
					Title:       "Test Video",
					Status:      model.StatusProcessing,
					OriginalURL: "originals/video-id/video.mp4",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				}
				repo.getByIDFn = func(ctx context.Context, id uuid.UUID) (*model.Video, error) {
					return video, nil
				}
				return video
			},
			wantErr: nil,
		},
		{
			name:    "error - already ready",
			videoID: uuid.New(),
			setupMock: func(repo *mockVideoRepository, queue *mockMessageQueue) *model.Video {
				video := &model.Video{
					ID:        uuid.New(),
					UserID:    uuid.New(),
					Title:     "Test Video",
					Status:    model.StatusReady,
					HLSURL:    "hls/video-id/master.m3u8",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				repo.getByIDFn = func(ctx context.Context, id uuid.UUID) (*model.Video, error) {
					return video, nil
				}
				return video
			},
			wantErr: ErrVideoAlreadyCompleted,
		},
		{
			name:    "error - already failed",
			videoID: uuid.New(),
			setupMock: func(repo *mockVideoRepository, queue *mockMessageQueue) *model.Video {
				video := &model.Video{
					ID:        uuid.New(),
					UserID:    uuid.New(),
					Title:     "Test Video",
					Status:    model.StatusFailed,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				repo.getByIDFn = func(ctx context.Context, id uuid.UUID) (*model.Video, error) {
					return video, nil
				}
				return video
			},
			wantErr: ErrVideoAlreadyCompleted,
		},
		{
			name:    "error - video not found",
			videoID: uuid.New(),
			setupMock: func(repo *mockVideoRepository, queue *mockMessageQueue) *model.Video {
				repo.getByIDFn = func(ctx context.Context, id uuid.UUID) (*model.Video, error) {
					return nil, repository.ErrVideoNotFound
				}
				return nil
			},
			wantErr: repository.ErrVideoNotFound,
		},
		{
			name:    "error - repository update fails",
			videoID: uuid.New(),
			setupMock: func(repo *mockVideoRepository, queue *mockMessageQueue) *model.Video {
				video := &model.Video{
					ID:          uuid.New(),
					UserID:      uuid.New(),
					Title:       "Test Video",
					Status:      model.StatusPendingUpload,
					OriginalURL: "originals/video-id/video.mp4",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				}
				repo.getByIDFn = func(ctx context.Context, id uuid.UUID) (*model.Video, error) {
					return video, nil
				}
				repo.updateFn = func(ctx context.Context, v *model.Video) error {
					return errors.New("database error")
				}
				return video
			},
			wantErr: errors.New("update video status"),
		},
		{
			name:    "error - queue publish fails",
			videoID: uuid.New(),
			setupMock: func(repo *mockVideoRepository, queue *mockMessageQueue) *model.Video {
				video := &model.Video{
					ID:          uuid.New(),
					UserID:      uuid.New(),
					Title:       "Test Video",
					Status:      model.StatusPendingUpload,
					OriginalURL: "originals/video-id/video.mp4",
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				}
				repo.getByIDFn = func(ctx context.Context, id uuid.UUID) (*model.Video, error) {
					return video, nil
				}
				repo.updateFn = func(ctx context.Context, v *model.Video) error {
					return nil
				}
				queue.publishTranscodeTaskFn = func(ctx context.Context, task repository.TranscodeTask) error {
					return errors.New("queue unavailable")
				}
				return video
			},
			wantErr: errors.New("publish transcode task"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockVideoRepository{}
			storage := &mockObjectStorage{}
			queue := &mockMessageQueue{}

			tt.setupMock(repo, queue)

			svc := NewVideoService(repo, storage, queue, DefaultVideoServiceConfig())

			err := svc.TriggerProcess(context.Background(), tt.videoID)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !errors.Is(err, tt.wantErr) && !strings.Contains(err.Error(), tt.wantErr.Error()) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestVideoService_GetVideo(t *testing.T) {
	tests := []struct {
		name      string
		videoID   uuid.UUID
		setupMock func(repo *mockVideoRepository) *model.Video
		wantErr   error
	}{
		{
			name:    "successful retrieval",
			videoID: uuid.New(),
			setupMock: func(repo *mockVideoRepository) *model.Video {
				video := &model.Video{
					ID:        uuid.New(),
					UserID:    uuid.New(),
					Title:     "Test Video",
					Status:    model.StatusReady,
					HLSURL:    "hls/video-id/master.m3u8",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				repo.getByIDFn = func(ctx context.Context, id uuid.UUID) (*model.Video, error) {
					return video, nil
				}
				return video
			},
			wantErr: nil,
		},
		{
			name:    "video not found",
			videoID: uuid.New(),
			setupMock: func(repo *mockVideoRepository) *model.Video {
				repo.getByIDFn = func(ctx context.Context, id uuid.UUID) (*model.Video, error) {
					return nil, repository.ErrVideoNotFound
				}
				return nil
			},
			wantErr: repository.ErrVideoNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockVideoRepository{}
			storage := &mockObjectStorage{}
			queue := &mockMessageQueue{}

			expectedVideo := tt.setupMock(repo)

			svc := NewVideoService(repo, storage, queue, DefaultVideoServiceConfig())

			video, err := svc.GetVideo(context.Background(), tt.videoID)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tt.wantErr)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if video.ID != expectedVideo.ID {
				t.Errorf("expected video ID %s, got %s", expectedVideo.ID, video.ID)
			}
		})
	}
}
