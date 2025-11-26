package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/hszk-dev/gostream/internal/domain/repository"
)

// objectReader abstracts minio.Object for testability.
// *minio.Object satisfies this interface.
type objectReader interface {
	io.ReadCloser
	Stat() (minio.ObjectInfo, error)
}

// minioClient defines the interface for MinIO operations.
// This abstraction allows for easier unit testing with mocks.
type minioClient interface {
	BucketExists(ctx context.Context, bucketName string) (bool, error)
	PresignedPutObject(ctx context.Context, bucketName, objectName string, expiry time.Duration) (*url.URL, error)
	PresignedGetObject(ctx context.Context, bucketName, objectName string, expiry time.Duration, reqParams url.Values) (*url.URL, error)
	PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, opts minio.PutObjectOptions) (minio.UploadInfo, error)
	GetObject(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (objectReader, error)
	RemoveObject(ctx context.Context, bucketName, objectName string, opts minio.RemoveObjectOptions) error
	StatObject(ctx context.Context, bucketName, objectName string, opts minio.StatObjectOptions) (minio.ObjectInfo, error)
}

// minioClientAdapter wraps *minio.Client to implement minioClient interface.
// This is necessary because *minio.Client.GetObject returns *minio.Object,
// but our interface returns objectReader for testability.
type minioClientAdapter struct {
	client *minio.Client
}

func (a *minioClientAdapter) BucketExists(ctx context.Context, bucketName string) (bool, error) {
	return a.client.BucketExists(ctx, bucketName)
}

func (a *minioClientAdapter) PresignedPutObject(ctx context.Context, bucketName, objectName string, expiry time.Duration) (*url.URL, error) {
	return a.client.PresignedPutObject(ctx, bucketName, objectName, expiry)
}

func (a *minioClientAdapter) PresignedGetObject(ctx context.Context, bucketName, objectName string, expiry time.Duration, reqParams url.Values) (*url.URL, error) {
	return a.client.PresignedGetObject(ctx, bucketName, objectName, expiry, reqParams)
}

func (a *minioClientAdapter) PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	return a.client.PutObject(ctx, bucketName, objectName, reader, objectSize, opts)
}

func (a *minioClientAdapter) GetObject(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (objectReader, error) {
	return a.client.GetObject(ctx, bucketName, objectName, opts)
}

func (a *minioClientAdapter) RemoveObject(ctx context.Context, bucketName, objectName string, opts minio.RemoveObjectOptions) error {
	return a.client.RemoveObject(ctx, bucketName, objectName, opts)
}

func (a *minioClientAdapter) StatObject(ctx context.Context, bucketName, objectName string, opts minio.StatObjectOptions) (minio.ObjectInfo, error) {
	return a.client.StatObject(ctx, bucketName, objectName, opts)
}

// ClientConfig holds configuration for the MinIO client.
type ClientConfig struct {
	Endpoint       string
	PublicEndpoint string // Optional: external-facing endpoint for presigned URLs
	AccessKey      string
	SecretKey      string
	Bucket         string
	UseSSL         bool
}

// Client wraps a MinIO client and implements repository.ObjectStorage.
type Client struct {
	client          minioClient
	presignedClient minioClient // Separate client for presigned URLs (may use public endpoint)
	bucket          string
}

// NewClient creates a new MinIO client.
// It verifies the bucket exists during initialization to fail fast on misconfiguration.
// If PublicEndpoint is set, a separate client is created for presigned URL generation.
func NewClient(ctx context.Context, cfg ClientConfig) (*Client, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create minio client: %w", err)
	}

	adapter := &minioClientAdapter{client: client}

	// Create a separate client for presigned URLs if public endpoint is configured
	var presignedAdapter minioClient = adapter
	if cfg.PublicEndpoint != "" {
		presignedClient, err := minio.New(cfg.PublicEndpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
			Secure: cfg.UseSSL,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create presigned minio client: %w", err)
		}
		presignedAdapter = &minioClientAdapter{client: presignedClient}
	}

	return newClientWithMinioClient(ctx, adapter, presignedAdapter, cfg.Bucket)
}

// newClientWithMinioClient creates a Client with a given minioClient implementation.
// This is used for dependency injection in tests.
func newClientWithMinioClient(ctx context.Context, client, presignedClient minioClient, bucket string) (*Client, error) {
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to check bucket existence: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("%w: %s", repository.ErrBucketNotFound, bucket)
	}

	return &Client{
		client:          client,
		presignedClient: presignedClient,
		bucket:          bucket,
	}, nil
}

// GeneratePresignedUploadURL creates a presigned URL for direct client upload.
// Uses presignedClient which may be configured with a public endpoint.
func (c *Client) GeneratePresignedUploadURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	presignedURL, err := c.presignedClient.PresignedPutObject(ctx, c.bucket, key, expiry)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned upload URL: %w", err)
	}
	return presignedURL.String(), nil
}

// GeneratePresignedDownloadURL creates a presigned URL for downloading an object.
// Uses presignedClient which may be configured with a public endpoint.
func (c *Client) GeneratePresignedDownloadURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	reqParams := make(url.Values)
	presignedURL, err := c.presignedClient.PresignedGetObject(ctx, c.bucket, key, expiry, reqParams)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned download URL: %w", err)
	}
	return presignedURL.String(), nil
}

// Upload stores an object in the storage.
func (c *Client) Upload(ctx context.Context, key string, reader io.Reader, contentType string) error {
	_, err := c.client.PutObject(ctx, c.bucket, key, reader, -1, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("failed to upload object: %w", err)
	}
	return nil
}

// Download retrieves an object from the storage.
// Caller is responsible for closing the returned ReadCloser.
func (c *Client) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := c.client.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get object: %w", err)
	}

	// Verify the object exists by checking its stat.
	// GetObject returns a lazy reader that doesn't fail until read.
	_, err = obj.Stat()
	if err != nil {
		_ = obj.Close() // Best effort close on error path
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil, repository.ErrObjectNotFound
		}
		return nil, fmt.Errorf("failed to stat object: %w", err)
	}

	return obj, nil
}

// Delete removes an object from the storage.
func (c *Client) Delete(ctx context.Context, key string) error {
	err := c.client.RemoveObject(ctx, c.bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}
	return nil
}

// Exists checks if an object exists in the storage.
func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	_, err := c.client.StatObject(ctx, c.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return false, nil
		}
		return false, fmt.Errorf("failed to check object existence: %w", err)
	}
	return true, nil
}

// Ping verifies the MinIO connection is alive by checking bucket access.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.client.BucketExists(ctx, c.bucket)
	if err != nil {
		return fmt.Errorf("failed to ping minio: %w", err)
	}
	return nil
}

// Bucket returns the configured bucket name.
func (c *Client) Bucket() string {
	return c.bucket
}
