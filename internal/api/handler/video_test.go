package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hszk-dev/gostream/internal/domain/model"
	"github.com/hszk-dev/gostream/internal/domain/repository"
	"github.com/hszk-dev/gostream/internal/usecase"
)

// Mock VideoService

type mockVideoService struct {
	createVideoFn    func(ctx context.Context, input usecase.CreateVideoInput) (*usecase.CreateVideoOutput, error)
	triggerProcessFn func(ctx context.Context, videoID uuid.UUID) error
	getVideoFn       func(ctx context.Context, videoID uuid.UUID) (*model.Video, error)
}

func (m *mockVideoService) CreateVideo(ctx context.Context, input usecase.CreateVideoInput) (*usecase.CreateVideoOutput, error) {
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
	if m.getVideoFn != nil {
		return m.getVideoFn(ctx, videoID)
	}
	return nil, nil
}

func TestVideoHandler_Create(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    interface{}
		setupMock      func(m *mockVideoService)
		wantStatusCode int
		checkResponse  func(t *testing.T, body []byte)
	}{
		{
			name: "successful creation",
			requestBody: CreateVideoRequest{
				UserID:   uuid.New().String(),
				Title:    "Test Video",
				FileName: "video.mp4",
			},
			setupMock: func(m *mockVideoService) {
				m.createVideoFn = func(ctx context.Context, input usecase.CreateVideoInput) (*usecase.CreateVideoOutput, error) {
					video := &model.Video{
						ID:        uuid.New(),
						UserID:    input.UserID,
						Title:     input.Title,
						Status:    model.StatusPendingUpload,
						CreatedAt: time.Now(),
						UpdatedAt: time.Now(),
					}
					return &usecase.CreateVideoOutput{
						Video:     video,
						UploadURL: "http://minio:9000/videos/upload?signature=xyz",
					}, nil
				}
			},
			wantStatusCode: http.StatusCreated,
			checkResponse: func(t *testing.T, body []byte) {
				var resp CreateVideoResponse
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}
				if resp.UploadURL == "" {
					t.Error("expected upload URL to be non-empty")
				}
				if resp.Status != "PENDING_UPLOAD" {
					t.Errorf("expected status PENDING_UPLOAD, got %s", resp.Status)
				}
			},
		},
		{
			name:           "invalid JSON body",
			requestBody:    "invalid json",
			setupMock:      func(m *mockVideoService) {},
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name: "invalid user ID",
			requestBody: CreateVideoRequest{
				UserID:   "not-a-uuid",
				Title:    "Test Video",
				FileName: "video.mp4",
			},
			setupMock:      func(m *mockVideoService) {},
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name: "empty title",
			requestBody: CreateVideoRequest{
				UserID:   uuid.New().String(),
				Title:    "",
				FileName: "video.mp4",
			},
			setupMock:      func(m *mockVideoService) {},
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name: "empty file name",
			requestBody: CreateVideoRequest{
				UserID:   uuid.New().String(),
				Title:    "Test Video",
				FileName: "",
			},
			setupMock:      func(m *mockVideoService) {},
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name: "service error - title too long",
			requestBody: CreateVideoRequest{
				UserID:   uuid.New().String(),
				Title:    "Test Video",
				FileName: "video.mp4",
			},
			setupMock: func(m *mockVideoService) {
				m.createVideoFn = func(ctx context.Context, input usecase.CreateVideoInput) (*usecase.CreateVideoOutput, error) {
					return nil, model.ErrTitleTooLong
				}
			},
			wantStatusCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockVideoService{}
			tt.setupMock(mock)
			h := NewVideoHandler(mock)

			var body []byte
			switch v := tt.requestBody.(type) {
			case string:
				body = []byte(v)
			default:
				var err error
				body, err = json.Marshal(v)
				if err != nil {
					t.Fatalf("failed to marshal request body: %v", err)
				}
			}

			req := httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			h.Create(rec, req)

			if rec.Code != tt.wantStatusCode {
				t.Errorf("expected status %d, got %d", tt.wantStatusCode, rec.Code)
			}

			if tt.checkResponse != nil {
				tt.checkResponse(t, rec.Body.Bytes())
			}
		})
	}
}

func TestVideoHandler_TriggerProcess(t *testing.T) {
	tests := []struct {
		name           string
		videoID        string
		setupMock      func(m *mockVideoService)
		wantStatusCode int
	}{
		{
			name:    "successful trigger",
			videoID: uuid.New().String(),
			setupMock: func(m *mockVideoService) {
				m.triggerProcessFn = func(ctx context.Context, videoID uuid.UUID) error {
					return nil
				}
			},
			wantStatusCode: http.StatusAccepted,
		},
		{
			name:           "invalid video ID",
			videoID:        "not-a-uuid",
			setupMock:      func(m *mockVideoService) {},
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name:    "video not found",
			videoID: uuid.New().String(),
			setupMock: func(m *mockVideoService) {
				m.triggerProcessFn = func(ctx context.Context, videoID uuid.UUID) error {
					return repository.ErrVideoNotFound
				}
			},
			wantStatusCode: http.StatusNotFound,
		},
		{
			name:    "video already completed",
			videoID: uuid.New().String(),
			setupMock: func(m *mockVideoService) {
				m.triggerProcessFn = func(ctx context.Context, videoID uuid.UUID) error {
					return usecase.ErrVideoAlreadyCompleted
				}
			},
			wantStatusCode: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockVideoService{}
			tt.setupMock(mock)
			h := NewVideoHandler(mock)

			r := chi.NewRouter()
			r.Post("/v1/videos/{id}/process", h.TriggerProcess)

			req := httptest.NewRequest(http.MethodPost, "/v1/videos/"+tt.videoID+"/process", nil)
			rec := httptest.NewRecorder()

			r.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatusCode {
				t.Errorf("expected status %d, got %d", tt.wantStatusCode, rec.Code)
			}
		})
	}
}

func TestVideoHandler_Get(t *testing.T) {
	tests := []struct {
		name           string
		videoID        string
		setupMock      func(m *mockVideoService)
		wantStatusCode int
		checkResponse  func(t *testing.T, body []byte)
	}{
		{
			name:    "successful get",
			videoID: uuid.New().String(),
			setupMock: func(m *mockVideoService) {
				m.getVideoFn = func(ctx context.Context, videoID uuid.UUID) (*model.Video, error) {
					return &model.Video{
						ID:        videoID,
						UserID:    uuid.New(),
						Title:     "Test Video",
						Status:    model.StatusReady,
						HLSURL:    "hls/video-id/master.m3u8",
						CreatedAt: time.Now(),
						UpdatedAt: time.Now(),
					}, nil
				}
			},
			wantStatusCode: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				var resp VideoResponse
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}
				if resp.Status != "READY" {
					t.Errorf("expected status READY, got %s", resp.Status)
				}
				if resp.HLSURL == "" {
					t.Error("expected HLS URL to be non-empty")
				}
			},
		},
		{
			name:           "invalid video ID",
			videoID:        "not-a-uuid",
			setupMock:      func(m *mockVideoService) {},
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name:    "video not found",
			videoID: uuid.New().String(),
			setupMock: func(m *mockVideoService) {
				m.getVideoFn = func(ctx context.Context, videoID uuid.UUID) (*model.Video, error) {
					return nil, repository.ErrVideoNotFound
				}
			},
			wantStatusCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockVideoService{}
			tt.setupMock(mock)
			h := NewVideoHandler(mock)

			r := chi.NewRouter()
			r.Get("/v1/videos/{id}", h.Get)

			req := httptest.NewRequest(http.MethodGet, "/v1/videos/"+tt.videoID, nil)
			rec := httptest.NewRecorder()

			r.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatusCode {
				t.Errorf("expected status %d, got %d", tt.wantStatusCode, rec.Code)
			}

			if tt.checkResponse != nil {
				tt.checkResponse(t, rec.Body.Bytes())
			}
		})
	}
}
