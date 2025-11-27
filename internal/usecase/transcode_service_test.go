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

// mustWriteFile is a test helper that writes a file and fails the test on error.
func mustWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write test file %s: %v", path, err)
	}
}

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
		transcodeToABRFn: func(ctx context.Context, inputPath, outputDir string, variants []transcoder.Variant) (*transcoder.ABROutput, error) {
			// Create mock output files for ABR
			masterPath := filepath.Join(outputDir, "master.m3u8")
			mustWriteFile(t, masterPath, []byte("#EXTM3U\n#EXT-X-VERSION:3\n"))

			var variantOutputs []transcoder.VariantOutput
			for _, v := range variants {
				variantDir := filepath.Join(outputDir, v.Name)
				if err := os.MkdirAll(variantDir, 0755); err != nil {
					return nil, err
				}

				manifestPath := filepath.Join(variantDir, "playlist.m3u8")
				segmentPath := filepath.Join(variantDir, "segment_000.ts")

				mustWriteFile(t, manifestPath, []byte("#EXTM3U\n"))
				mustWriteFile(t, segmentPath, []byte("mock segment data"))

				variantOutputs = append(variantOutputs, transcoder.VariantOutput{
					Variant:      v,
					ManifestPath: manifestPath,
					SegmentPaths: []string{segmentPath},
				})
			}

			return &transcoder.ABROutput{
				MasterManifestPath: masterPath,
				Variants:           variantOutputs,
			}, nil
		},
	}

	cfg := TranscodeServiceConfig{
		TempDir:    tempDir,
		MaxRetries: 3,
	}
	svc := NewTranscodeService(repo, storage, tc, nil, cfg)

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

	// Verify HLS URL is set (should point to master.m3u8)
	if video.HLSURL == "" {
		t.Error("HLS URL should be set")
	}
	expectedHLSURL := "hls/" + videoID.String() + "/master.m3u8"
	if video.HLSURL != expectedHLSURL {
		t.Errorf("HLS URL: got %s, expected %s", video.HLSURL, expectedHLSURL)
	}

	// Verify master manifest was uploaded
	if _, ok := uploadedFiles["hls/"+videoID.String()+"/master.m3u8"]; !ok {
		t.Error("master manifest should be uploaded")
	}

	// Verify variant files were uploaded (check one variant)
	if _, ok := uploadedFiles["hls/"+videoID.String()+"/720p/playlist.m3u8"]; !ok {
		t.Error("720p playlist should be uploaded")
	}
	if _, ok := uploadedFiles["hls/"+videoID.String()+"/720p/segment_000.ts"]; !ok {
		t.Error("720p segment should be uploaded")
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
	svc := NewTranscodeService(repo, storage, tc, nil, cfg)

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
	svc := NewTranscodeService(repo, storage, tc, nil, cfg)

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
		transcodeToABRFn: func(ctx context.Context, inputPath, outputDir string, variants []transcoder.Variant) (*transcoder.ABROutput, error) {
			return nil, errors.New("transcode failed")
		},
	}

	cfg := TranscodeServiceConfig{
		TempDir:    t.TempDir(),
		MaxRetries: 3,
	}
	svc := NewTranscodeService(repo, storage, tc, nil, cfg)

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
		transcodeToABRFn: func(ctx context.Context, inputPath, outputDir string, variants []transcoder.Variant) (*transcoder.ABROutput, error) {
			masterPath := filepath.Join(outputDir, "master.m3u8")
			mustWriteFile(t, masterPath, []byte("#EXTM3U\n"))
			return &transcoder.ABROutput{
				MasterManifestPath: masterPath,
				Variants:           []transcoder.VariantOutput{},
			}, nil
		},
	}

	cfg := TranscodeServiceConfig{
		TempDir:    t.TempDir(),
		MaxRetries: 3,
	}
	svc := NewTranscodeService(repo, storage, tc, nil, cfg)

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
		transcodeToABRFn: func(ctx context.Context, inputPath, outputDir string, variants []transcoder.Variant) (*transcoder.ABROutput, error) {
			masterPath := filepath.Join(outputDir, "master.m3u8")
			mustWriteFile(t, masterPath, []byte("#EXTM3U\n"))

			var variantOutputs []transcoder.VariantOutput
			for _, v := range variants {
				variantDir := filepath.Join(outputDir, v.Name)
				os.MkdirAll(variantDir, 0755)
				manifestPath := filepath.Join(variantDir, "playlist.m3u8")
				segmentPath := filepath.Join(variantDir, "segment_000.ts")
				mustWriteFile(t, manifestPath, []byte("#EXTM3U\n"))
				mustWriteFile(t, segmentPath, []byte("mock segment"))
				variantOutputs = append(variantOutputs, transcoder.VariantOutput{
					Variant:      v,
					ManifestPath: manifestPath,
					SegmentPaths: []string{segmentPath},
				})
			}

			return &transcoder.ABROutput{
				MasterManifestPath: masterPath,
				Variants:           variantOutputs,
			}, nil
		},
	}

	cfg := TranscodeServiceConfig{
		TempDir:    t.TempDir(),
		MaxRetries: 3,
	}
	svc := NewTranscodeService(repo, storage, tc, nil, cfg)

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

func TestTranscodeService_ProcessTask_DBUpdateError(t *testing.T) {
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
		updateFn: func(ctx context.Context, v *model.Video) error {
			return errors.New("database connection lost")
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
		transcodeToABRFn: func(ctx context.Context, inputPath, outputDir string, variants []transcoder.Variant) (*transcoder.ABROutput, error) {
			masterPath := filepath.Join(outputDir, "master.m3u8")
			mustWriteFile(t, masterPath, []byte("#EXTM3U\n"))

			var variantOutputs []transcoder.VariantOutput
			for _, v := range variants {
				variantDir := filepath.Join(outputDir, v.Name)
				os.MkdirAll(variantDir, 0755)
				manifestPath := filepath.Join(variantDir, "playlist.m3u8")
				segmentPath := filepath.Join(variantDir, "segment_000.ts")
				mustWriteFile(t, manifestPath, []byte("#EXTM3U\n"))
				mustWriteFile(t, segmentPath, []byte("mock segment"))
				variantOutputs = append(variantOutputs, transcoder.VariantOutput{
					Variant:      v,
					ManifestPath: manifestPath,
					SegmentPaths: []string{segmentPath},
				})
			}

			return &transcoder.ABROutput{
				MasterManifestPath: masterPath,
				Variants:           variantOutputs,
			}, nil
		},
	}

	cfg := TranscodeServiceConfig{
		TempDir:    t.TempDir(),
		MaxRetries: 3,
	}
	svc := NewTranscodeService(repo, storage, tc, nil, cfg)

	task := repository.TranscodeTask{
		VideoID:     videoID,
		OriginalKey: "originals/" + videoID.String() + "/video.mp4",
		OutputKey:   "hls/" + videoID.String() + "/",
		RetryCount:  0,
	}

	// Should return error to trigger retry
	err := svc.ProcessTask(ctx, task)
	if err == nil {
		t.Error("expected error for DB update failure")
	}

	// Verify error message contains context
	if !strings.Contains(err.Error(), "update video status") {
		t.Errorf("error should indicate update failure, got: %v", err)
	}
}
