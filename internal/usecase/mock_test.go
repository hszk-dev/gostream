package usecase

import (
	"context"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/hszk-dev/gostream/internal/domain/model"
	"github.com/hszk-dev/gostream/internal/domain/repository"
	"github.com/hszk-dev/gostream/internal/transcoder"
)

// mockVideoRepository provides a configurable mock for VideoRepository.
type mockVideoRepository struct {
	createFn       func(ctx context.Context, video *model.Video) error
	getByIDFn      func(ctx context.Context, id uuid.UUID) (*model.Video, error)
	getByUserIDFn  func(ctx context.Context, userID uuid.UUID) ([]*model.Video, error)
	updateFn       func(ctx context.Context, video *model.Video) error
	updateStatusFn func(ctx context.Context, id uuid.UUID, status model.Status) error
}

func (m *mockVideoRepository) Create(ctx context.Context, video *model.Video) error {
	if m.createFn != nil {
		return m.createFn(ctx, video)
	}
	return nil
}

func (m *mockVideoRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Video, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, nil
}

func (m *mockVideoRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*model.Video, error) {
	if m.getByUserIDFn != nil {
		return m.getByUserIDFn(ctx, userID)
	}
	return nil, nil
}

func (m *mockVideoRepository) Update(ctx context.Context, video *model.Video) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, video)
	}
	return nil
}

func (m *mockVideoRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status model.Status) error {
	if m.updateStatusFn != nil {
		return m.updateStatusFn(ctx, id, status)
	}
	return nil
}

// mockObjectStorage provides a configurable mock for ObjectStorage.
type mockObjectStorage struct {
	generatePresignedUploadURLFn   func(ctx context.Context, key string, expiry time.Duration) (string, error)
	generatePresignedDownloadURLFn func(ctx context.Context, key string, expiry time.Duration) (string, error)
	uploadFn                       func(ctx context.Context, key string, reader io.Reader, contentType string) error
	downloadFn                     func(ctx context.Context, key string) (io.ReadCloser, error)
	deleteFn                       func(ctx context.Context, key string) error
	existsFn                       func(ctx context.Context, key string) (bool, error)
}

func (m *mockObjectStorage) GeneratePresignedUploadURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	if m.generatePresignedUploadURLFn != nil {
		return m.generatePresignedUploadURLFn(ctx, key, expiry)
	}
	return "http://example.com/upload", nil
}

func (m *mockObjectStorage) GeneratePresignedDownloadURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	if m.generatePresignedDownloadURLFn != nil {
		return m.generatePresignedDownloadURLFn(ctx, key, expiry)
	}
	return "http://example.com/download", nil
}

func (m *mockObjectStorage) Upload(ctx context.Context, key string, reader io.Reader, contentType string) error {
	if m.uploadFn != nil {
		return m.uploadFn(ctx, key, reader, contentType)
	}
	return nil
}

func (m *mockObjectStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	if m.downloadFn != nil {
		return m.downloadFn(ctx, key)
	}
	return nil, nil
}

func (m *mockObjectStorage) Delete(ctx context.Context, key string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, key)
	}
	return nil
}

func (m *mockObjectStorage) Exists(ctx context.Context, key string) (bool, error) {
	if m.existsFn != nil {
		return m.existsFn(ctx, key)
	}
	return false, nil
}

// mockMessageQueue provides a configurable mock for MessageQueue.
type mockMessageQueue struct {
	publishTranscodeTaskFn  func(ctx context.Context, task repository.TranscodeTask) error
	consumeTranscodeTasksFn func(ctx context.Context, handler func(task repository.TranscodeTask) error) error
}

func (m *mockMessageQueue) PublishTranscodeTask(ctx context.Context, task repository.TranscodeTask) error {
	if m.publishTranscodeTaskFn != nil {
		return m.publishTranscodeTaskFn(ctx, task)
	}
	return nil
}

func (m *mockMessageQueue) ConsumeTranscodeTasks(ctx context.Context, handler func(task repository.TranscodeTask) error) error {
	if m.consumeTranscodeTasksFn != nil {
		return m.consumeTranscodeTasksFn(ctx, handler)
	}
	return nil
}

func (m *mockMessageQueue) Close() error {
	return nil
}

// mockTranscoder provides a configurable mock for Transcoder.
type mockTranscoder struct {
	transcodeToHLSFn func(ctx context.Context, inputPath, outputDir string) (*transcoder.HLSOutput, error)
}

func (m *mockTranscoder) TranscodeToHLS(ctx context.Context, inputPath, outputDir string) (*transcoder.HLSOutput, error) {
	if m.transcodeToHLSFn != nil {
		return m.transcodeToHLSFn(ctx, inputPath, outputDir)
	}
	return nil, nil
}
