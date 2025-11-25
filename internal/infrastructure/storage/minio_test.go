package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"

	"github.com/hszk-dev/gostream/internal/domain/repository"
)

// mockObjectReader implements objectReader interface for testing.
type mockObjectReader struct {
	readFunc  func(p []byte) (n int, err error)
	closeFunc func() error
	statFunc  func() (minio.ObjectInfo, error)
	data      []byte
	offset    int
}

func (m *mockObjectReader) Read(p []byte) (n int, err error) {
	if m.readFunc != nil {
		return m.readFunc(p)
	}
	if m.offset >= len(m.data) {
		return 0, io.EOF
	}
	n = copy(p, m.data[m.offset:])
	m.offset += n
	return n, nil
}

func (m *mockObjectReader) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func (m *mockObjectReader) Stat() (minio.ObjectInfo, error) {
	if m.statFunc != nil {
		return m.statFunc()
	}
	return minio.ObjectInfo{}, nil
}

// mockMinioClient implements minioClient interface for testing.
type mockMinioClient struct {
	bucketExistsFunc       func(ctx context.Context, bucketName string) (bool, error)
	presignedPutObjectFunc func(ctx context.Context, bucketName, objectName string, expiry time.Duration) (*url.URL, error)
	presignedGetObjectFunc func(ctx context.Context, bucketName, objectName string, expiry time.Duration, reqParams url.Values) (*url.URL, error)
	putObjectFunc          func(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, opts minio.PutObjectOptions) (minio.UploadInfo, error)
	getObjectFunc          func(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (objectReader, error)
	removeObjectFunc       func(ctx context.Context, bucketName, objectName string, opts minio.RemoveObjectOptions) error
	statObjectFunc         func(ctx context.Context, bucketName, objectName string, opts minio.StatObjectOptions) (minio.ObjectInfo, error)
}

func (m *mockMinioClient) BucketExists(ctx context.Context, bucketName string) (bool, error) {
	if m.bucketExistsFunc != nil {
		return m.bucketExistsFunc(ctx, bucketName)
	}
	return true, nil
}

func (m *mockMinioClient) PresignedPutObject(ctx context.Context, bucketName, objectName string, expiry time.Duration) (*url.URL, error) {
	if m.presignedPutObjectFunc != nil {
		return m.presignedPutObjectFunc(ctx, bucketName, objectName, expiry)
	}
	return nil, nil
}

func (m *mockMinioClient) PresignedGetObject(ctx context.Context, bucketName, objectName string, expiry time.Duration, reqParams url.Values) (*url.URL, error) {
	if m.presignedGetObjectFunc != nil {
		return m.presignedGetObjectFunc(ctx, bucketName, objectName, expiry, reqParams)
	}
	return nil, nil
}

func (m *mockMinioClient) PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	if m.putObjectFunc != nil {
		return m.putObjectFunc(ctx, bucketName, objectName, reader, objectSize, opts)
	}
	return minio.UploadInfo{}, nil
}

func (m *mockMinioClient) GetObject(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (objectReader, error) {
	if m.getObjectFunc != nil {
		return m.getObjectFunc(ctx, bucketName, objectName, opts)
	}
	return nil, nil
}

func (m *mockMinioClient) RemoveObject(ctx context.Context, bucketName, objectName string, opts minio.RemoveObjectOptions) error {
	if m.removeObjectFunc != nil {
		return m.removeObjectFunc(ctx, bucketName, objectName, opts)
	}
	return nil
}

func (m *mockMinioClient) StatObject(ctx context.Context, bucketName, objectName string, opts minio.StatObjectOptions) (minio.ObjectInfo, error) {
	if m.statObjectFunc != nil {
		return m.statObjectFunc(ctx, bucketName, objectName, opts)
	}
	return minio.ObjectInfo{}, nil
}

func TestNewClientWithMinioClient(t *testing.T) {
	tests := []struct {
		name       string
		bucket     string
		mockClient *mockMinioClient
		wantErr    error
	}{
		{
			name:   "successful initialization",
			bucket: "test-bucket",
			mockClient: &mockMinioClient{
				bucketExistsFunc: func(ctx context.Context, bucketName string) (bool, error) {
					return true, nil
				},
			},
			wantErr: nil,
		},
		{
			name:   "bucket does not exist",
			bucket: "non-existent-bucket",
			mockClient: &mockMinioClient{
				bucketExistsFunc: func(ctx context.Context, bucketName string) (bool, error) {
					return false, nil
				},
			},
			wantErr: repository.ErrBucketNotFound,
		},
		{
			name:   "bucket check error",
			bucket: "test-bucket",
			mockClient: &mockMinioClient{
				bucketExistsFunc: func(ctx context.Context, bucketName string) (bool, error) {
					return false, errors.New("connection refused")
				},
			},
			wantErr: errors.New("failed to check bucket existence"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := newClientWithMinioClient(context.Background(), tt.mockClient, tt.bucket)

			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("newClientWithMinioClient() expected error, got nil")
					return
				}
				if !errors.Is(err, tt.wantErr) && !strings.Contains(err.Error(), tt.wantErr.Error()) {
					t.Errorf("newClientWithMinioClient() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("newClientWithMinioClient() unexpected error = %v", err)
				return
			}

			if client.bucket != tt.bucket {
				t.Errorf("client.bucket = %v, want %v", client.bucket, tt.bucket)
			}
		})
	}
}

func TestClient_GeneratePresignedUploadURL(t *testing.T) {
	tests := []struct {
		name       string
		key        string
		expiry     time.Duration
		mockClient *mockMinioClient
		wantURL    string
		wantErr    bool
	}{
		{
			name:   "successful presigned upload URL generation",
			key:    "uploads/video-123/original.mp4",
			expiry: 15 * time.Minute,
			mockClient: &mockMinioClient{
				presignedPutObjectFunc: func(ctx context.Context, bucketName, objectName string, expiry time.Duration) (*url.URL, error) {
					u, _ := url.Parse("http://localhost:9000/videos/uploads/video-123/original.mp4?X-Amz-Signature=abc123")
					return u, nil
				},
			},
			wantURL: "http://localhost:9000/videos/uploads/video-123/original.mp4?X-Amz-Signature=abc123",
			wantErr: false,
		},
		{
			name:   "error generating presigned URL",
			key:    "uploads/video-123/original.mp4",
			expiry: 15 * time.Minute,
			mockClient: &mockMinioClient{
				presignedPutObjectFunc: func(ctx context.Context, bucketName, objectName string, expiry time.Duration) (*url.URL, error) {
					return nil, errors.New("signing error")
				},
			},
			wantURL: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				client: tt.mockClient,
				bucket: "videos",
			}

			got, err := client.GeneratePresignedUploadURL(context.Background(), tt.key, tt.expiry)

			if (err != nil) != tt.wantErr {
				t.Errorf("GeneratePresignedUploadURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if got != tt.wantURL {
				t.Errorf("GeneratePresignedUploadURL() = %v, want %v", got, tt.wantURL)
			}
		})
	}
}

func TestClient_GeneratePresignedDownloadURL(t *testing.T) {
	tests := []struct {
		name       string
		key        string
		expiry     time.Duration
		mockClient *mockMinioClient
		wantURL    string
		wantErr    bool
	}{
		{
			name:   "successful presigned download URL generation",
			key:    "hls/video-123/master.m3u8",
			expiry: 1 * time.Hour,
			mockClient: &mockMinioClient{
				presignedGetObjectFunc: func(ctx context.Context, bucketName, objectName string, expiry time.Duration, reqParams url.Values) (*url.URL, error) {
					u, _ := url.Parse("http://localhost:9000/videos/hls/video-123/master.m3u8?X-Amz-Signature=xyz789")
					return u, nil
				},
			},
			wantURL: "http://localhost:9000/videos/hls/video-123/master.m3u8?X-Amz-Signature=xyz789",
			wantErr: false,
		},
		{
			name:   "error generating presigned URL",
			key:    "hls/video-123/master.m3u8",
			expiry: 1 * time.Hour,
			mockClient: &mockMinioClient{
				presignedGetObjectFunc: func(ctx context.Context, bucketName, objectName string, expiry time.Duration, reqParams url.Values) (*url.URL, error) {
					return nil, errors.New("signing error")
				},
			},
			wantURL: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				client: tt.mockClient,
				bucket: "videos",
			}

			got, err := client.GeneratePresignedDownloadURL(context.Background(), tt.key, tt.expiry)

			if (err != nil) != tt.wantErr {
				t.Errorf("GeneratePresignedDownloadURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if got != tt.wantURL {
				t.Errorf("GeneratePresignedDownloadURL() = %v, want %v", got, tt.wantURL)
			}
		})
	}
}

func TestClient_Upload(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		content     string
		contentType string
		mockClient  *mockMinioClient
		wantErr     bool
	}{
		{
			name:        "successful upload",
			key:         "uploads/video-123/original.mp4",
			content:     "video content",
			contentType: "video/mp4",
			mockClient: &mockMinioClient{
				putObjectFunc: func(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
					if opts.ContentType != "video/mp4" {
						t.Errorf("expected content type video/mp4, got %s", opts.ContentType)
					}
					return minio.UploadInfo{Bucket: bucketName, Key: objectName}, nil
				},
			},
			wantErr: false,
		},
		{
			name:        "upload error",
			key:         "uploads/video-123/original.mp4",
			content:     "video content",
			contentType: "video/mp4",
			mockClient: &mockMinioClient{
				putObjectFunc: func(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
					return minio.UploadInfo{}, errors.New("upload failed")
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				client: tt.mockClient,
				bucket: "videos",
			}

			reader := bytes.NewReader([]byte(tt.content))
			err := client.Upload(context.Background(), tt.key, reader, tt.contentType)

			if (err != nil) != tt.wantErr {
				t.Errorf("Upload() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestClient_Download(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		mockClient  *mockMinioClient
		wantContent string
		wantErr     error
	}{
		{
			name: "successful download",
			key:  "uploads/video-123/original.mp4",
			mockClient: &mockMinioClient{
				getObjectFunc: func(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (objectReader, error) {
					return &mockObjectReader{
						data: []byte("video content"),
						statFunc: func() (minio.ObjectInfo, error) {
							return minio.ObjectInfo{Key: objectName, Size: 13}, nil
						},
					}, nil
				},
			},
			wantContent: "video content",
			wantErr:     nil,
		},
		{
			name: "object not found",
			key:  "uploads/video-123/nonexistent.mp4",
			mockClient: &mockMinioClient{
				getObjectFunc: func(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (objectReader, error) {
					return &mockObjectReader{
						statFunc: func() (minio.ObjectInfo, error) {
							return minio.ObjectInfo{}, minio.ErrorResponse{Code: "NoSuchKey"}
						},
					}, nil
				},
			},
			wantContent: "",
			wantErr:     repository.ErrObjectNotFound,
		},
		{
			name: "get object error",
			key:  "uploads/video-123/original.mp4",
			mockClient: &mockMinioClient{
				getObjectFunc: func(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (objectReader, error) {
					return nil, errors.New("connection refused")
				},
			},
			wantContent: "",
			wantErr:     errors.New("failed to get object"),
		},
		{
			name: "stat error",
			key:  "uploads/video-123/original.mp4",
			mockClient: &mockMinioClient{
				getObjectFunc: func(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (objectReader, error) {
					return &mockObjectReader{
						statFunc: func() (minio.ObjectInfo, error) {
							return minio.ObjectInfo{}, errors.New("stat failed")
						},
					}, nil
				},
			},
			wantContent: "",
			wantErr:     errors.New("failed to stat object"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				client: tt.mockClient,
				bucket: "videos",
			}

			reader, err := client.Download(context.Background(), tt.key)

			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("Download() expected error, got nil")
					return
				}
				if !errors.Is(err, tt.wantErr) && !strings.Contains(err.Error(), tt.wantErr.Error()) {
					t.Errorf("Download() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("Download() unexpected error = %v", err)
				return
			}

			defer reader.Close()

			content, err := io.ReadAll(reader)
			if err != nil {
				t.Errorf("failed to read content: %v", err)
				return
			}

			if string(content) != tt.wantContent {
				t.Errorf("Download() content = %v, want %v", string(content), tt.wantContent)
			}
		})
	}
}

func TestClient_Delete(t *testing.T) {
	tests := []struct {
		name       string
		key        string
		mockClient *mockMinioClient
		wantErr    bool
	}{
		{
			name: "successful delete",
			key:  "uploads/video-123/original.mp4",
			mockClient: &mockMinioClient{
				removeObjectFunc: func(ctx context.Context, bucketName, objectName string, opts minio.RemoveObjectOptions) error {
					return nil
				},
			},
			wantErr: false,
		},
		{
			name: "delete error",
			key:  "uploads/video-123/original.mp4",
			mockClient: &mockMinioClient{
				removeObjectFunc: func(ctx context.Context, bucketName, objectName string, opts minio.RemoveObjectOptions) error {
					return errors.New("delete failed")
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				client: tt.mockClient,
				bucket: "videos",
			}

			err := client.Delete(context.Background(), tt.key)

			if (err != nil) != tt.wantErr {
				t.Errorf("Delete() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestClient_Exists(t *testing.T) {
	tests := []struct {
		name       string
		key        string
		mockClient *mockMinioClient
		want       bool
		wantErr    bool
	}{
		{
			name: "object exists",
			key:  "uploads/video-123/original.mp4",
			mockClient: &mockMinioClient{
				statObjectFunc: func(ctx context.Context, bucketName, objectName string, opts minio.StatObjectOptions) (minio.ObjectInfo, error) {
					return minio.ObjectInfo{Key: objectName, Size: 1024}, nil
				},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "object does not exist",
			key:  "uploads/video-123/original.mp4",
			mockClient: &mockMinioClient{
				statObjectFunc: func(ctx context.Context, bucketName, objectName string, opts minio.StatObjectOptions) (minio.ObjectInfo, error) {
					return minio.ObjectInfo{}, minio.ErrorResponse{Code: "NoSuchKey"}
				},
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "stat error",
			key:  "uploads/video-123/original.mp4",
			mockClient: &mockMinioClient{
				statObjectFunc: func(ctx context.Context, bucketName, objectName string, opts minio.StatObjectOptions) (minio.ObjectInfo, error) {
					return minio.ObjectInfo{}, errors.New("connection error")
				},
			},
			want:    false,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				client: tt.mockClient,
				bucket: "videos",
			}

			got, err := client.Exists(context.Background(), tt.key)

			if (err != nil) != tt.wantErr {
				t.Errorf("Exists() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if got != tt.want {
				t.Errorf("Exists() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClient_Ping(t *testing.T) {
	tests := []struct {
		name       string
		mockClient *mockMinioClient
		wantErr    bool
	}{
		{
			name: "successful ping",
			mockClient: &mockMinioClient{
				bucketExistsFunc: func(ctx context.Context, bucketName string) (bool, error) {
					return true, nil
				},
			},
			wantErr: false,
		},
		{
			name: "ping error",
			mockClient: &mockMinioClient{
				bucketExistsFunc: func(ctx context.Context, bucketName string) (bool, error) {
					return false, errors.New("connection refused")
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				client: tt.mockClient,
				bucket: "videos",
			}

			err := client.Ping(context.Background())

			if (err != nil) != tt.wantErr {
				t.Errorf("Ping() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestClient_Bucket(t *testing.T) {
	client := &Client{
		bucket: "test-bucket",
	}

	if got := client.Bucket(); got != "test-bucket" {
		t.Errorf("Bucket() = %v, want %v", got, "test-bucket")
	}
}
