package usecase

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hszk-dev/gostream/internal/domain/model"
)

// mockVideoService is a mock implementation of VideoService for testing.
type mockVideoService struct {
	createVideoFn    func(ctx context.Context, input CreateVideoInput) (*CreateVideoOutput, error)
	triggerProcessFn func(ctx context.Context, videoID uuid.UUID) error
	getVideoFn       func(ctx context.Context, videoID uuid.UUID) (*model.Video, error)
	getVideoCount    atomic.Int32
}

func (m *mockVideoService) CreateVideo(ctx context.Context, input CreateVideoInput) (*CreateVideoOutput, error) {
	if m.createVideoFn != nil {
		return m.createVideoFn(ctx, input)
	}
	return nil, nil
}

func (m *mockVideoService) TriggerProcess(ctx context.Context, videoID uuid.UUID) error {
	if m.triggerProcessFn != nil {
		return m.triggerProcessFn(ctx, videoID)
	}
	return nil
}

func (m *mockVideoService) GetVideo(ctx context.Context, videoID uuid.UUID) (*model.Video, error) {
	m.getVideoCount.Add(1)
	if m.getVideoFn != nil {
		return m.getVideoFn(ctx, videoID)
	}
	return nil, nil
}

// mockVideoCache is a mock implementation of VideoCache for testing.
type mockVideoCache struct {
	mu      sync.RWMutex
	data    map[uuid.UUID]*model.Video
	getFn   func(ctx context.Context, videoID uuid.UUID) (*model.Video, error)
	setFn   func(ctx context.Context, video *model.Video, ttl time.Duration) error
	deleteFn func(ctx context.Context, videoID uuid.UUID) error
}

func newMockVideoCache() *mockVideoCache {
	return &mockVideoCache{
		data: make(map[uuid.UUID]*model.Video),
	}
}

func (m *mockVideoCache) Get(ctx context.Context, videoID uuid.UUID) (*model.Video, error) {
	if m.getFn != nil {
		return m.getFn(ctx, videoID)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.data[videoID], nil
}

func (m *mockVideoCache) Set(ctx context.Context, video *model.Video, ttl time.Duration) error {
	if m.setFn != nil {
		return m.setFn(ctx, video, ttl)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[video.ID] = video
	return nil
}

func (m *mockVideoCache) Delete(ctx context.Context, videoID uuid.UUID) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, videoID)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, videoID)
	return nil
}

func TestCachedVideoService_GetVideo_CacheHit(t *testing.T) {
	videoID := uuid.New()
	cachedVideo := &model.Video{
		ID:        videoID,
		UserID:    uuid.New(),
		Title:     "Cached Video",
		Status:    model.StatusProcessing,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	mockSvc := &mockVideoService{}
	mockCache := newMockVideoCache()

	// Pre-populate cache
	mockCache.data[videoID] = cachedVideo

	svc := NewCachedVideoService(mockSvc, mockCache, DefaultCachedVideoServiceConfig())

	got, err := svc.GetVideo(context.Background(), videoID)
	if err != nil {
		t.Fatalf("GetVideo failed: %v", err)
	}

	if got.ID != videoID {
		t.Errorf("ID = %v, want %v", got.ID, videoID)
	}

	// Verify delegate was NOT called (cache hit)
	if mockSvc.getVideoCount.Load() != 0 {
		t.Errorf("delegate GetVideo called %d times, want 0", mockSvc.getVideoCount.Load())
	}
}

func TestCachedVideoService_GetVideo_CacheMiss(t *testing.T) {
	videoID := uuid.New()
	dbVideo := &model.Video{
		ID:        videoID,
		UserID:    uuid.New(),
		Title:     "DB Video",
		Status:    model.StatusProcessing,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	mockSvc := &mockVideoService{
		getVideoFn: func(ctx context.Context, id uuid.UUID) (*model.Video, error) {
			return dbVideo, nil
		},
	}
	mockCache := newMockVideoCache()

	svc := NewCachedVideoService(mockSvc, mockCache, DefaultCachedVideoServiceConfig())

	got, err := svc.GetVideo(context.Background(), videoID)
	if err != nil {
		t.Fatalf("GetVideo failed: %v", err)
	}

	if got.ID != videoID {
		t.Errorf("ID = %v, want %v", got.ID, videoID)
	}

	// Verify delegate was called (cache miss)
	if mockSvc.getVideoCount.Load() != 1 {
		t.Errorf("delegate GetVideo called %d times, want 1", mockSvc.getVideoCount.Load())
	}

	// Verify video was cached
	if mockCache.data[videoID] == nil {
		t.Error("video was not cached after cache miss")
	}
}

func TestCachedVideoService_GetVideo_CDNURLEnrichment(t *testing.T) {
	videoID := uuid.New()
	readyVideo := &model.Video{
		ID:        videoID,
		UserID:    uuid.New(),
		Title:     "Ready Video",
		Status:    model.StatusReady,
		HLSURL:    "hls/" + videoID.String() + "/master.m3u8",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	mockSvc := &mockVideoService{
		getVideoFn: func(ctx context.Context, id uuid.UUID) (*model.Video, error) {
			return readyVideo, nil
		},
	}
	mockCache := newMockVideoCache()

	cfg := CachedVideoServiceConfig{
		CacheTTL:   5 * time.Minute,
		CDNBaseURL: "http://cdn.example.com",
	}
	svc := NewCachedVideoService(mockSvc, mockCache, cfg)

	got, err := svc.GetVideo(context.Background(), videoID)
	if err != nil {
		t.Fatalf("GetVideo failed: %v", err)
	}

	expectedURL := "http://cdn.example.com/hls/" + videoID.String() + "/master.m3u8"
	if got.HLSURL != expectedURL {
		t.Errorf("HLSURL = %v, want %v", got.HLSURL, expectedURL)
	}
}

func TestCachedVideoService_GetVideo_NoCDNURLForNonReady(t *testing.T) {
	testCases := []struct {
		name   string
		status model.Status
	}{
		{"PendingUpload", model.StatusPendingUpload},
		{"Processing", model.StatusProcessing},
		{"Failed", model.StatusFailed},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			videoID := uuid.New()
			video := &model.Video{
				ID:        videoID,
				UserID:    uuid.New(),
				Title:     "Non-Ready Video",
				Status:    tc.status,
				HLSURL:    "hls/" + videoID.String() + "/master.m3u8",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}

			mockSvc := &mockVideoService{
				getVideoFn: func(ctx context.Context, id uuid.UUID) (*model.Video, error) {
					return video, nil
				},
			}
			mockCache := newMockVideoCache()

			cfg := CachedVideoServiceConfig{
				CacheTTL:   5 * time.Minute,
				CDNBaseURL: "http://cdn.example.com",
			}
			svc := NewCachedVideoService(mockSvc, mockCache, cfg)

			got, err := svc.GetVideo(context.Background(), videoID)
			if err != nil {
				t.Fatalf("GetVideo failed: %v", err)
			}

			// Should NOT have CDN URL for non-ready videos
			if got.HLSURL != video.HLSURL {
				t.Errorf("HLSURL = %v, want %v (original)", got.HLSURL, video.HLSURL)
			}
		})
	}
}

func TestCachedVideoService_TriggerProcess_InvalidatesCache(t *testing.T) {
	videoID := uuid.New()
	cachedVideo := &model.Video{
		ID:        videoID,
		UserID:    uuid.New(),
		Title:     "Cached Video",
		Status:    model.StatusPendingUpload,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	mockSvc := &mockVideoService{
		triggerProcessFn: func(ctx context.Context, id uuid.UUID) error {
			return nil
		},
	}
	mockCache := newMockVideoCache()
	mockCache.data[videoID] = cachedVideo

	svc := NewCachedVideoService(mockSvc, mockCache, DefaultCachedVideoServiceConfig())

	err := svc.TriggerProcess(context.Background(), videoID)
	if err != nil {
		t.Fatalf("TriggerProcess failed: %v", err)
	}

	// Verify cache was invalidated
	if mockCache.data[videoID] != nil {
		t.Error("cache was not invalidated after TriggerProcess")
	}
}

func TestCachedVideoService_GetVideo_Singleflight(t *testing.T) {
	videoID := uuid.New()
	video := &model.Video{
		ID:        videoID,
		UserID:    uuid.New(),
		Title:     "Test Video",
		Status:    model.StatusProcessing,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Add delay to simulate slow DB query
	mockSvc := &mockVideoService{
		getVideoFn: func(ctx context.Context, id uuid.UUID) (*model.Video, error) {
			time.Sleep(50 * time.Millisecond)
			return video, nil
		},
	}
	mockCache := newMockVideoCache()

	svc := NewCachedVideoService(mockSvc, mockCache, DefaultCachedVideoServiceConfig())

	// Launch multiple concurrent requests
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.GetVideo(context.Background(), videoID)
			if err != nil {
				t.Errorf("GetVideo failed: %v", err)
			}
		}()
	}

	wg.Wait()

	// Singleflight should coalesce requests - delegate should be called only once
	callCount := mockSvc.getVideoCount.Load()
	if callCount != 1 {
		t.Errorf("delegate GetVideo called %d times, want 1 (singleflight should coalesce)", callCount)
	}
}

func TestCachedVideoService_GetVideo_CacheErrorFallsBackToDB(t *testing.T) {
	videoID := uuid.New()
	dbVideo := &model.Video{
		ID:        videoID,
		UserID:    uuid.New(),
		Title:     "DB Video",
		Status:    model.StatusProcessing,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	mockSvc := &mockVideoService{
		getVideoFn: func(ctx context.Context, id uuid.UUID) (*model.Video, error) {
			return dbVideo, nil
		},
	}
	mockCache := &mockVideoCache{
		getFn: func(ctx context.Context, videoID uuid.UUID) (*model.Video, error) {
			return nil, errors.New("redis connection error")
		},
		setFn: func(ctx context.Context, video *model.Video, ttl time.Duration) error {
			return errors.New("redis connection error")
		},
	}

	svc := NewCachedVideoService(mockSvc, mockCache, DefaultCachedVideoServiceConfig())

	got, err := svc.GetVideo(context.Background(), videoID)
	if err != nil {
		t.Fatalf("GetVideo should not fail on cache error: %v", err)
	}

	if got.ID != videoID {
		t.Errorf("ID = %v, want %v", got.ID, videoID)
	}
}

func TestCachedVideoService_CreateVideo_Delegates(t *testing.T) {
	videoID := uuid.New()
	userID := uuid.New()
	output := &CreateVideoOutput{
		Video: &model.Video{
			ID:        videoID,
			UserID:    userID,
			Title:     "New Video",
			Status:    model.StatusPendingUpload,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		UploadURL: "http://example.com/upload",
	}

	mockSvc := &mockVideoService{
		createVideoFn: func(ctx context.Context, input CreateVideoInput) (*CreateVideoOutput, error) {
			return output, nil
		},
	}
	mockCache := newMockVideoCache()

	svc := NewCachedVideoService(mockSvc, mockCache, DefaultCachedVideoServiceConfig())

	got, err := svc.CreateVideo(context.Background(), CreateVideoInput{
		UserID:   userID,
		Title:    "New Video",
		FileName: "test.mp4",
	})

	if err != nil {
		t.Fatalf("CreateVideo failed: %v", err)
	}

	if got.Video.ID != videoID {
		t.Errorf("Video ID = %v, want %v", got.Video.ID, videoID)
	}
}
