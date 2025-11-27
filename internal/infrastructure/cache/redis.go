package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hszk-dev/gostream/internal/domain/model"
	"github.com/redis/go-redis/v9"
)

const (
	// videoCacheKeyPrefix is the prefix for video cache keys in Redis.
	videoCacheKeyPrefix = "video:"
)

// videoJSON is the JSON representation of a Video for caching.
// Using explicit struct avoids coupling to domain model's JSON tags.
type videoJSON struct {
	ID          string `json:"id"`
	UserID      string `json:"user_id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	OriginalURL string `json:"original_url"`
	HLSURL      string `json:"hls_url"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// RedisVideoCache implements VideoCache using Redis as the backing store.
type RedisVideoCache struct {
	client *redis.Client
}

// NewRedisVideoCache creates a new Redis-backed video cache.
func NewRedisVideoCache(client *redis.Client) *RedisVideoCache {
	return &RedisVideoCache{
		client: client,
	}
}

// Get retrieves a video from Redis cache.
// Returns nil, nil on cache miss.
func (c *RedisVideoCache) Get(ctx context.Context, videoID uuid.UUID) (*model.Video, error) {
	key := c.buildKey(videoID)

	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil // Cache miss
		}
		return nil, fmt.Errorf("redis get: %w", err)
	}

	video, err := c.deserialize(data)
	if err != nil {
		return nil, fmt.Errorf("deserialize video: %w", err)
	}

	return video, nil
}

// Set stores a video in Redis cache with the specified TTL.
func (c *RedisVideoCache) Set(ctx context.Context, video *model.Video, ttl time.Duration) error {
	key := c.buildKey(video.ID)

	data, err := c.serialize(video)
	if err != nil {
		return fmt.Errorf("serialize video: %w", err)
	}

	if err := c.client.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("redis set: %w", err)
	}

	return nil
}

// Delete removes a video from Redis cache.
func (c *RedisVideoCache) Delete(ctx context.Context, videoID uuid.UUID) error {
	key := c.buildKey(videoID)

	if err := c.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("redis del: %w", err)
	}

	return nil
}

// buildKey constructs the Redis key for a video.
func (c *RedisVideoCache) buildKey(videoID uuid.UUID) string {
	return videoCacheKeyPrefix + videoID.String()
}

// serialize converts a Video to JSON bytes.
func (c *RedisVideoCache) serialize(video *model.Video) ([]byte, error) {
	v := videoJSON{
		ID:          video.ID.String(),
		UserID:      video.UserID.String(),
		Title:       video.Title,
		Status:      string(video.Status),
		OriginalURL: video.OriginalURL,
		HLSURL:      video.HLSURL,
		CreatedAt:   video.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt:   video.UpdatedAt.Format(time.RFC3339Nano),
	}
	return json.Marshal(v)
}

// deserialize converts JSON bytes to a Video.
func (c *RedisVideoCache) deserialize(data []byte) (*model.Video, error) {
	var v videoJSON
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}

	id, err := uuid.Parse(v.ID)
	if err != nil {
		return nil, fmt.Errorf("parse video ID: %w", err)
	}

	userID, err := uuid.Parse(v.UserID)
	if err != nil {
		return nil, fmt.Errorf("parse user ID: %w", err)
	}

	createdAt, err := time.Parse(time.RFC3339Nano, v.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}

	updatedAt, err := time.Parse(time.RFC3339Nano, v.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}

	return &model.Video{
		ID:          id,
		UserID:      userID,
		Title:       v.Title,
		Status:      model.Status(v.Status),
		OriginalURL: v.OriginalURL,
		HLSURL:      v.HLSURL,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}, nil
}
