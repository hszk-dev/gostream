package cache

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/hszk-dev/gostream/internal/domain/model"
)

// VideoCache defines the interface for caching video metadata.
// Implementations should handle serialization/deserialization transparently.
type VideoCache interface {
	// Get retrieves a video from cache by ID.
	// Returns nil, nil if the video is not found in cache (cache miss).
	Get(ctx context.Context, videoID uuid.UUID) (*model.Video, error)

	// Set stores a video in cache with the specified TTL.
	Set(ctx context.Context, video *model.Video, ttl time.Duration) error

	// Delete removes a video from cache by ID.
	// Returns nil if the video was not in cache.
	Delete(ctx context.Context, videoID uuid.UUID) error
}
