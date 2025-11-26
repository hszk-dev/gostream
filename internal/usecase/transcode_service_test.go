package usecase

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hszk-dev/gostream/internal/domain/model"
	"github.com/hszk-dev/gostream/internal/domain/repository"
	"github.com/hszk-dev/gostream/internal/transcoder"
)

func TestDefaultTranscodeServiceConfig(t *testing.T) {
	cfg := DefaultTranscodeServiceConfig()

	if cfg.TempDir == "" {
		t.Error("TempDir should not be empty")
	}
	if cfg.MaxRetries != DefaultMaxRetries {
		t.Errorf("MaxRetries: got %d, expected %d", cfg.MaxRetries, DefaultMaxRetries)
	}
}

func TestTranscodeService_ProcessTask_Success(t *testing.T) {
	ctx := context.Background()
	videoID := uuid.New()
	tempDir := t.TempDir()

	// Track uploaded files
	uploadedFiles := make(map[string][]byte)

	video := &model.Video{
		ID:          videoID,
		UserID:      uuid.New(),
		Title:       "Test Video",
		Status:      model.StatusProcessing,
		OriginalURL: "originals/" + videoID.String() + "/video.mp4",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	repo := &mockVideoRepository{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (*model.Video, error) {
			if id == videoID {
				return video, nil
			}
			return nil, repository.ErrVideoNotFound
		},
		updateFn: func(ctx context.Context, v *model.Video) error {
			video = v
			return nil
		},
	}

	storage := &mockObjectStorage{
		downloadFn: func(ctx context.Context, key string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("fake video data")), nil
		},
		uploadFn: func(ctx context.Context, key string, reader io.Reader, contentType string) error {
			data, _ := io.ReadAll(reader)
			uploadedFiles[key] = data
			return nil
		},
	}

	tc := &mockTranscoder{
		transcodeToHLSFn: func(ctx context.Context, inputPath, outputDir string) (*transcoder.HLSOutput, error) {
			// Create mock output files
			manifestPath := filepath.Join(outputDir, "playlist.m3u8")
			segmentPath := filepath.Join(outputDir, "segment_000.ts")

			os.WriteFile(manifestPath, []byte("#EXTM3U\n"), 0644)
			os.WriteFile(segmentPath, []byte("mock segment data"), 0644)

			return &transcoder.HLSOutput{
				ManifestPath: manifestPath,
				SegmentPaths: []string{segmentPath},
			}, nil
		},
	}

	cfg := TranscodeServiceConfig{
		TempDir:    tempDir,
		MaxRetries: 3,
	}
	svc := NewTranscodeService(repo, storage, tc, cfg)

	task := repository.TranscodeTask{
		VideoID:     videoID,
		OriginalKey: "originals/" + videoID.String() + "/video.mp4",
		OutputKey:   "hls/" + videoID.String() + "/",
		RetryCount:  0,
	}

	err := svc.ProcessTask(ctx, task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify video status is READY
	if video.Status != model.StatusReady {
		t.Errorf("video status: got %s, expected %s", video.Status, model.StatusReady)
	}

	// Verify HLS URL is set
	if video.HLSURL == "" {
		t.Error("HLS URL should be set")
	}

	// Verify HLS files were uploaded
	if _, ok := uploadedFiles["hls/"+videoID.String()+"/playlist.m3u8"]; !ok {
		t.Error("manifest should be uploaded")
	}
	if _, ok := uploadedFiles["hls/"+videoID.String()+"/segment_000.ts"]; !ok {
		t.Error("segment should be uploaded")
	}
}

func TestTranscodeService_ProcessTask_MaxRetriesExceeded(t *testing.T) {
	ctx := context.Background()
	videoID := uuid.New()

	video := &model.Video{
		ID:        videoID,
		UserID:    uuid.New(),
		Title:     "Test Video",
		Status:    model.StatusProcessing,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	repo := &mockVideoRepository{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (*model.Video, error) {
			return video, nil
		},
		updateFn: func(ctx context.Context, v *model.Video) error {
			video = v
			return nil
		},
	}

	storage := &mockObjectStorage{}
	tc := &mockTranscoder{}

	cfg := TranscodeServiceConfig{
		TempDir:    t.TempDir(),
		MaxRetries: 3,
	}
	svc := NewTranscodeService(repo, storage, tc, cfg)

	task := repository.TranscodeTask{
		VideoID:    videoID,
		RetryCount: 3, // Already at max retries
	}

	// Should return nil (ack the message) but mark video as FAILED
	err := svc.ProcessTask(ctx, task)
	if err != nil {
		t.Fatalf("expected nil error for max retries, got: %v", err)
	}

	// Verify video status is FAILED
	if video.Status != model.StatusFailed {
		t.Errorf("video status: got %s, expected %s", video.Status, model.StatusFailed)
	}
}

func TestTranscodeService_ProcessTask_DownloadError(t *testing.T) {
	ctx := context.Background()
	videoID := uuid.New()

	video := &model.Video{
		ID:        videoID,
		UserID:    uuid.New(),
		Title:     "Test Video",
		Status:    model.StatusProcessing,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	repo := &mockVideoRepository{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (*model.Video, error) {
			return video, nil
		},
	}

	storage := &mockObjectStorage{
		downloadFn: func(ctx context.Context, key string) (io.ReadCloser, error) {
			return nil, errors.New("download failed")
		},
	}

	tc := &mockTranscoder{}

	cfg := TranscodeServiceConfig{
		TempDir:    t.TempDir(),
		MaxRetries: 3,
	}
	svc := NewTranscodeService(repo, storage, tc, cfg)

	task := repository.TranscodeTask{
		VideoID:     videoID,
		OriginalKey: "originals/" + videoID.String() + "/video.mp4",
		OutputKey:   "hls/" + videoID.String() + "/",
		RetryCount:  0,
	}

	// Should return error to trigger retry
	err := svc.ProcessTask(ctx, task)
	if err == nil {
		t.Error("expected error for download failure")
	}

	// Video should still be in PROCESSING state
	if video.Status != model.StatusProcessing {
		t.Error("video status should remain PROCESSING on transient error")
	}
}

func TestTranscodeService_ProcessTask_TranscodeError(t *testing.T) {
	ctx := context.Background()
	videoID := uuid.New()

	video := &model.Video{
		ID:          videoID,
		UserID:      uuid.New(),
		Title:       "Test Video",
		Status:      model.StatusProcessing,
		OriginalURL: "originals/" + videoID.String() + "/video.mp4",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	repo := &mockVideoRepository{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (*model.Video, error) {
			return video, nil
		},
	}

	storage := &mockObjectStorage{
		downloadFn: func(ctx context.Context, key string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("fake video data")), nil
		},
	}

	tc := &mockTranscoder{
		transcodeToHLSFn: func(ctx context.Context, inputPath, outputDir string) (*transcoder.HLSOutput, error) {
			return nil, errors.New("transcode failed")
		},
	}

	cfg := TranscodeServiceConfig{
		TempDir:    t.TempDir(),
		MaxRetries: 3,
	}
	svc := NewTranscodeService(repo, storage, tc, cfg)

	task := repository.TranscodeTask{
		VideoID:     videoID,
		OriginalKey: "originals/" + videoID.String() + "/video.mp4",
		OutputKey:   "hls/" + videoID.String() + "/",
		RetryCount:  0,
	}

	// Should return error to trigger retry
	err := svc.ProcessTask(ctx, task)
	if err == nil {
		t.Error("expected error for transcode failure")
	}
}

func TestTranscodeService_ProcessTask_UploadError(t *testing.T) {
	ctx := context.Background()
	videoID := uuid.New()

	video := &model.Video{
		ID:          videoID,
		UserID:      uuid.New(),
		Title:       "Test Video",
		Status:      model.StatusProcessing,
		OriginalURL: "originals/" + videoID.String() + "/video.mp4",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	repo := &mockVideoRepository{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (*model.Video, error) {
			return video, nil
		},
	}

	storage := &mockObjectStorage{
		downloadFn: func(ctx context.Context, key string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("fake video data")), nil
		},
		uploadFn: func(ctx context.Context, key string, reader io.Reader, contentType string) error {
			return errors.New("upload failed")
		},
	}

	tc := &mockTranscoder{
		transcodeToHLSFn: func(ctx context.Context, inputPath, outputDir string) (*transcoder.HLSOutput, error) {
			manifestPath := filepath.Join(outputDir, "playlist.m3u8")
			os.WriteFile(manifestPath, []byte("#EXTM3U\n"), 0644)
			return &transcoder.HLSOutput{
				ManifestPath: manifestPath,
				SegmentPaths: []string{},
			}, nil
		},
	}

	cfg := TranscodeServiceConfig{
		TempDir:    t.TempDir(),
		MaxRetries: 3,
	}
	svc := NewTranscodeService(repo, storage, tc, cfg)

	task := repository.TranscodeTask{
		VideoID:     videoID,
		OriginalKey: "originals/" + videoID.String() + "/video.mp4",
		OutputKey:   "hls/" + videoID.String() + "/",
		RetryCount:  0,
	}

	// Should return error to trigger retry
	err := svc.ProcessTask(ctx, task)
	if err == nil {
		t.Error("expected error for upload failure")
	}
}

func TestTranscodeService_ProcessTask_VideoNotInProcessingState(t *testing.T) {
	ctx := context.Background()
	videoID := uuid.New()

	video := &model.Video{
		ID:          videoID,
		UserID:      uuid.New(),
		Title:       "Test Video",
		Status:      model.StatusReady, // Already completed
		OriginalURL: "originals/" + videoID.String() + "/video.mp4",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	repo := &mockVideoRepository{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (*model.Video, error) {
			return video, nil
		},
		updateFn: func(ctx context.Context, v *model.Video) error {
			video = v
			return nil
		},
	}

	storage := &mockObjectStorage{
		downloadFn: func(ctx context.Context, key string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("fake video data")), nil
		},
		uploadFn: func(ctx context.Context, key string, reader io.Reader, contentType string) error {
			return nil
		},
	}

	tc := &mockTranscoder{
		transcodeToHLSFn: func(ctx context.Context, inputPath, outputDir string) (*transcoder.HLSOutput, error) {
			manifestPath := filepath.Join(outputDir, "playlist.m3u8")
			segmentPath := filepath.Join(outputDir, "segment_000.ts")
			os.WriteFile(manifestPath, []byte("#EXTM3U\n"), 0644)
			os.WriteFile(segmentPath, []byte("mock segment"), 0644)
			return &transcoder.HLSOutput{
				ManifestPath: manifestPath,
				SegmentPaths: []string{segmentPath},
			}, nil
		},
	}

	cfg := TranscodeServiceConfig{
		TempDir:    t.TempDir(),
		MaxRetries: 3,
	}
	svc := NewTranscodeService(repo, storage, tc, cfg)

	task := repository.TranscodeTask{
		VideoID:     videoID,
		OriginalKey: "originals/" + videoID.String() + "/video.mp4",
		OutputKey:   "hls/" + videoID.String() + "/",
		RetryCount:  0,
	}

	// Should succeed without error (idempotent)
	err := svc.ProcessTask(ctx, task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Video should remain in READY state
	if video.Status != model.StatusReady {
		t.Error("video status should remain READY")
	}
}

func TestGetFileExtension(t *testing.T) {
	tests := []struct {
		key      string
		expected string
	}{
		{"video.mp4", ".mp4"},
		{"video.MP4", ".mp4"},
		{"path/to/video.mov", ".mov"},
		{"no-extension", ".mp4"},
		{"", ".mp4"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := getFileExtension(tt.key)
			if got != tt.expected {
				t.Errorf("getFileExtension(%q): got %q, expected %q", tt.key, got, tt.expected)
			}
		})
	}
}
