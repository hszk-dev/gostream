package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/hszk-dev/gostream/internal/domain/model"
	"github.com/redis/go-redis/v9"
)

func setupTestRedis(t *testing.T) (*redis.Client, func()) {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	cleanup := func() {
		client.Close()
		mr.Close()
	}

	return client, cleanup
}

func TestRedisVideoCache_Get_CacheHit(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	cache := NewRedisVideoCache(client)
	ctx := context.Background()

	video := &model.Video{
		ID:          uuid.New(),
		UserID:      uuid.New(),
		Title:       "Test Video",
		Status:      model.StatusReady,
		OriginalURL: "originals/test.mp4",
		HLSURL:      "hls/test/master.m3u8",
		CreatedAt:   time.Now().Truncate(time.Microsecond),
		UpdatedAt:   time.Now().Truncate(time.Microsecond),
	}

	// Set the video in cache
	err := cache.Set(ctx, video, 5*time.Minute)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get the video from cache
	got, err := cache.Get(ctx, video.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got == nil {
		t.Fatal("expected video, got nil")
	}

	// Verify fields
	if got.ID != video.ID {
		t.Errorf("ID = %v, want %v", got.ID, video.ID)
	}
	if got.UserID != video.UserID {
		t.Errorf("UserID = %v, want %v", got.UserID, video.UserID)
	}
	if got.Title != video.Title {
		t.Errorf("Title = %v, want %v", got.Title, video.Title)
	}
	if got.Status != video.Status {
		t.Errorf("Status = %v, want %v", got.Status, video.Status)
	}
	if got.OriginalURL != video.OriginalURL {
		t.Errorf("OriginalURL = %v, want %v", got.OriginalURL, video.OriginalURL)
	}
	if got.HLSURL != video.HLSURL {
		t.Errorf("HLSURL = %v, want %v", got.HLSURL, video.HLSURL)
	}
}

func TestRedisVideoCache_Get_CacheMiss(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	cache := NewRedisVideoCache(client)
	ctx := context.Background()

	// Try to get a non-existent video
	got, err := cache.Get(ctx, uuid.New())
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got != nil {
		t.Errorf("expected nil for cache miss, got %v", got)
	}
}

func TestRedisVideoCache_Delete(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	cache := NewRedisVideoCache(client)
	ctx := context.Background()

	video := &model.Video{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		Title:     "Test Video",
		Status:    model.StatusReady,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Set the video in cache
	err := cache.Set(ctx, video, 5*time.Minute)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Delete the video from cache
	err = cache.Delete(ctx, video.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify it's gone
	got, err := cache.Get(ctx, video.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got != nil {
		t.Errorf("expected nil after delete, got %v", got)
	}
}

func TestRedisVideoCache_Delete_NonExistent(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	cache := NewRedisVideoCache(client)
	ctx := context.Background()

	// Delete non-existent video should not error
	err := cache.Delete(ctx, uuid.New())
	if err != nil {
		t.Fatalf("Delete failed for non-existent key: %v", err)
	}
}

func TestRedisVideoCache_Set_AllStatuses(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	cache := NewRedisVideoCache(client)
	ctx := context.Background()

	statuses := []model.Status{
		model.StatusPendingUpload,
		model.StatusProcessing,
		model.StatusReady,
		model.StatusFailed,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			video := &model.Video{
				ID:        uuid.New(),
				UserID:    uuid.New(),
				Title:     "Test Video",
				Status:    status,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}

			err := cache.Set(ctx, video, 5*time.Minute)
			if err != nil {
				t.Fatalf("Set failed: %v", err)
			}

			got, err := cache.Get(ctx, video.ID)
			if err != nil {
				t.Fatalf("Get failed: %v", err)
			}

			if got.Status != status {
				t.Errorf("Status = %v, want %v", got.Status, status)
			}
		})
	}
}

func TestRedisVideoCache_buildKey(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	cache := NewRedisVideoCache(client)
	videoID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

	key := cache.buildKey(videoID)
	expected := "video:550e8400-e29b-41d4-a716-446655440000"

	if key != expected {
		t.Errorf("buildKey() = %v, want %v", key, expected)
	}
}
