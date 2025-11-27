package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"path"
	"time"

	"github.com/google/uuid"
	"github.com/hszk-dev/gostream/internal/domain/model"
	"github.com/hszk-dev/gostream/internal/infrastructure/cache"
	"github.com/hszk-dev/gostream/internal/infrastructure/metrics"
	"golang.org/x/sync/singleflight"
)

// CachedVideoServiceConfig holds configuration for CachedVideoService.
type CachedVideoServiceConfig struct {
	// CacheTTL is the TTL for cached video metadata.
	CacheTTL time.Duration
	// CDNBaseURL is the base URL for CDN-served HLS content.
	CDNBaseURL string
}

// DefaultCachedVideoServiceConfig returns the default configuration.
func DefaultCachedVideoServiceConfig() CachedVideoServiceConfig {
	return CachedVideoServiceConfig{
		CacheTTL:   5 * time.Minute,
		CDNBaseURL: "http://localhost:8081",
	}
}

// cachedVideoService wraps VideoService with caching capabilities.
// It implements the decorator pattern to add caching without modifying the original service.
type cachedVideoService struct {
	delegate VideoService
	cache    cache.VideoCache
	sfGroup  singleflight.Group

	cacheTTL   time.Duration
	cdnBaseURL string
}

// NewCachedVideoService creates a new CachedVideoService wrapping the provided VideoService.
func NewCachedVideoService(
	delegate VideoService,
	videoCache cache.VideoCache,
	cfg CachedVideoServiceConfig,
) VideoService {
	return &cachedVideoService{
		delegate:   delegate,
		cache:      videoCache,
		cacheTTL:   cfg.CacheTTL,
		cdnBaseURL: cfg.CDNBaseURL,
	}
}

// CreateVideo delegates to the underlying service.
// No caching for create operations - the video is immediately returned.
func (s *cachedVideoService) CreateVideo(ctx context.Context, input CreateVideoInput) (*CreateVideoOutput, error) {
	return s.delegate.CreateVideo(ctx, input)
}

// TriggerProcess invalidates the cache and delegates to the underlying service.
// Cache invalidation happens before processing to ensure stale data is not served
// during the transition to PROCESSING status.
func (s *cachedVideoService) TriggerProcess(ctx context.Context, videoID uuid.UUID) error {
	// Invalidate cache before triggering process
	// This ensures the next GetVideo call fetches fresh data
	if err := s.cache.Delete(ctx, videoID); err != nil {
		// Log but don't fail - cache invalidation failure is non-critical
		slog.Warn("failed to invalidate cache on trigger process",
			"video_id", videoID,
			"error", err,
		)
	}

	return s.delegate.TriggerProcess(ctx, videoID)
}

// GetVideo retrieves video information with caching and CDN URL enrichment.
// Uses singleflight to prevent cache stampede on concurrent requests for the same video.
func (s *cachedVideoService) GetVideo(ctx context.Context, videoID uuid.UUID) (*model.Video, error) {
	// Use singleflight to coalesce concurrent requests
	key := videoID.String()
	result, err, shared := s.sfGroup.Do(key, func() (any, error) {
		return s.getVideoWithCache(ctx, videoID)
	})

	// Record singleflight metrics
	if shared {
		metrics.SingleflightRequestsTotal.WithLabelValues(metrics.SingleflightShared).Inc()
	} else {
		metrics.SingleflightRequestsTotal.WithLabelValues(metrics.SingleflightInitiated).Inc()
	}

	if err != nil {
		return nil, err
	}

	video := result.(*model.Video)
	return s.enrichWithCDNURL(video), nil
}

// getVideoWithCache implements the cache-aside pattern.
func (s *cachedVideoService) getVideoWithCache(ctx context.Context, videoID uuid.UUID) (*model.Video, error) {
	// Try cache first
	video, err := s.cache.Get(ctx, videoID)
	if err != nil {
		// Log cache error but continue to database
		slog.Warn("cache get failed, falling back to database",
			"video_id", videoID,
			"error", err,
		)
	}

	if video != nil {
		return video, nil // Cache hit
	}

	// Cache miss - fetch from database
	video, err = s.delegate.GetVideo(ctx, videoID)
	if err != nil {
		return nil, err
	}

	// Store in cache (async-safe: errors logged but not propagated)
	if err := s.cache.Set(ctx, video, s.cacheTTL); err != nil {
		slog.Warn("failed to cache video",
			"video_id", videoID,
			"error", err,
		)
	}

	return video, nil
}

// enrichWithCDNURL transforms the HLS URL to CDN URL for READY videos.
// Returns a copy to avoid mutating cached data.
func (s *cachedVideoService) enrichWithCDNURL(video *model.Video) *model.Video {
	if video.Status != model.StatusReady || video.HLSURL == "" {
		return video
	}

	// Create a copy to avoid mutating cached data
	enriched := *video
	enriched.HLSURL = s.buildCDNURL(video.ID)
	return &enriched
}

// buildCDNURL constructs the CDN URL for a video's HLS manifest.
// Format: {CDN_BASE_URL}/hls/{videoID}/master.m3u8
func (s *cachedVideoService) buildCDNURL(videoID uuid.UUID) string {
	return fmt.Sprintf("%s/%s", s.cdnBaseURL, path.Join("hls", videoID.String(), "master.m3u8"))
}

// InvalidateCache removes a video from the cache.
// This is exposed for use by TranscodeService when video status changes.
func (s *cachedVideoService) InvalidateCache(ctx context.Context, videoID uuid.UUID) error {
	return s.cache.Delete(ctx, videoID)
}
