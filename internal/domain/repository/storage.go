package repository

import (
	"context"
	"io"
	"time"
)

// ObjectStorage defines the interface for object storage operations.
// Implementations should be provided by the infrastructure layer (e.g., MinIO, S3).
type ObjectStorage interface {
	// GeneratePresignedUploadURL creates a presigned URL for direct client upload.
	// The URL is valid for the specified duration.
	// key is the object path within the bucket (e.g., "uploads/{video_id}/original.mp4").
	GeneratePresignedUploadURL(ctx context.Context, key string, expiry time.Duration) (string, error)

	// GeneratePresignedDownloadURL creates a presigned URL for downloading an object.
	// The URL is valid for the specified duration.
	GeneratePresignedDownloadURL(ctx context.Context, key string, expiry time.Duration) (string, error)

	// Upload stores an object in the storage.
	// This is used by the worker service for uploading transcoded segments.
	Upload(ctx context.Context, key string, reader io.Reader, contentType string) error

	// Download retrieves an object from the storage.
	// Caller is responsible for closing the returned ReadCloser.
	Download(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete removes an object from the storage.
	Delete(ctx context.Context, key string) error

	// Exists checks if an object exists in the storage.
	Exists(ctx context.Context, key string) (bool, error)
}

// ObjectInfo contains metadata about a stored object.
type ObjectInfo struct {
	Key          string
	Size         int64
	ContentType  string
	LastModified time.Time
}
