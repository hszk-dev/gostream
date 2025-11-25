package model

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestStatus_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		want   bool
	}{
		{"PENDING_UPLOAD is valid", StatusPendingUpload, true},
		{"PROCESSING is valid", StatusProcessing, true},
		{"READY is valid", StatusReady, true},
		{"FAILED is valid", StatusFailed, true},
		{"empty string is invalid", Status(""), false},
		{"unknown status is invalid", Status("UNKNOWN"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsValid(); got != tt.want {
				t.Errorf("Status.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatus_CanTransitionTo(t *testing.T) {
	tests := []struct {
		name    string
		current Status
		next    Status
		want    bool
	}{
		// Valid transitions
		{"PENDING_UPLOAD -> PROCESSING", StatusPendingUpload, StatusProcessing, true},
		{"PROCESSING -> READY", StatusProcessing, StatusReady, true},
		{"PROCESSING -> FAILED", StatusProcessing, StatusFailed, true},

		// Invalid transitions
		{"PENDING_UPLOAD -> READY (skip)", StatusPendingUpload, StatusReady, false},
		{"PENDING_UPLOAD -> FAILED (skip)", StatusPendingUpload, StatusFailed, false},
		{"READY -> PROCESSING (reverse)", StatusReady, StatusProcessing, false},
		{"FAILED -> READY (terminal)", StatusFailed, StatusReady, false},
		{"READY -> PENDING_UPLOAD (reverse)", StatusReady, StatusPendingUpload, false},

		// Self transitions
		{"PENDING_UPLOAD -> PENDING_UPLOAD", StatusPendingUpload, StatusPendingUpload, false},
		{"PROCESSING -> PROCESSING", StatusProcessing, StatusProcessing, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.current.CanTransitionTo(tt.next); got != tt.want {
				t.Errorf("Status.CanTransitionTo() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewVideo(t *testing.T) {
	validUserID := uuid.New()

	tests := []struct {
		name    string
		userID  uuid.UUID
		title   string
		wantErr error
	}{
		{
			name:    "valid video creation",
			userID:  validUserID,
			title:   "My Video",
			wantErr: nil,
		},
		{
			name:    "nil user ID",
			userID:  uuid.Nil,
			title:   "My Video",
			wantErr: ErrInvalidUserID,
		},
		{
			name:    "empty title",
			userID:  validUserID,
			title:   "",
			wantErr: ErrEmptyTitle,
		},
		{
			name:    "title too long",
			userID:  validUserID,
			title:   strings.Repeat("a", 256),
			wantErr: ErrTitleTooLong,
		},
		{
			name:    "title at max length",
			userID:  validUserID,
			title:   strings.Repeat("a", 255),
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			video, err := NewVideo(tt.userID, tt.title)

			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Errorf("NewVideo() error = %v, wantErr %v", err, tt.wantErr)
				}
				if video != nil {
					t.Error("NewVideo() should return nil video on error")
				}
				return
			}

			if err != nil {
				t.Errorf("NewVideo() unexpected error = %v", err)
				return
			}

			if video.ID == uuid.Nil {
				t.Error("NewVideo() should generate non-nil ID")
			}
			if video.UserID != tt.userID {
				t.Errorf("NewVideo() UserID = %v, want %v", video.UserID, tt.userID)
			}
			if video.Title != tt.title {
				t.Errorf("NewVideo() Title = %v, want %v", video.Title, tt.title)
			}
			if video.Status != StatusPendingUpload {
				t.Errorf("NewVideo() Status = %v, want %v", video.Status, StatusPendingUpload)
			}
			if video.CreatedAt.IsZero() {
				t.Error("NewVideo() should set CreatedAt")
			}
			if video.UpdatedAt.IsZero() {
				t.Error("NewVideo() should set UpdatedAt")
			}
		})
	}
}

func TestVideo_TransitionTo(t *testing.T) {
	tests := []struct {
		name        string
		setup       func() *Video
		nextStatus  Status
		wantErr     bool
		wantStatus  Status
	}{
		{
			name: "valid transition PENDING_UPLOAD -> PROCESSING",
			setup: func() *Video {
				v, _ := NewVideo(uuid.New(), "test")
				return v
			},
			nextStatus: StatusProcessing,
			wantErr:    false,
			wantStatus: StatusProcessing,
		},
		{
			name: "valid transition PROCESSING -> READY",
			setup: func() *Video {
				v, _ := NewVideo(uuid.New(), "test")
				v.Status = StatusProcessing
				return v
			},
			nextStatus: StatusReady,
			wantErr:    false,
			wantStatus: StatusReady,
		},
		{
			name: "valid transition PROCESSING -> FAILED",
			setup: func() *Video {
				v, _ := NewVideo(uuid.New(), "test")
				v.Status = StatusProcessing
				return v
			},
			nextStatus: StatusFailed,
			wantErr:    false,
			wantStatus: StatusFailed,
		},
		{
			name: "invalid transition PENDING_UPLOAD -> READY",
			setup: func() *Video {
				v, _ := NewVideo(uuid.New(), "test")
				return v
			},
			nextStatus: StatusReady,
			wantErr:    true,
			wantStatus: StatusPendingUpload,
		},
		{
			name: "invalid status value",
			setup: func() *Video {
				v, _ := NewVideo(uuid.New(), "test")
				return v
			},
			nextStatus: Status("INVALID"),
			wantErr:    true,
			wantStatus: StatusPendingUpload,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			video := tt.setup()
			oldUpdatedAt := video.UpdatedAt

			err := video.TransitionTo(tt.nextStatus)

			if (err != nil) != tt.wantErr {
				t.Errorf("Video.TransitionTo() error = %v, wantErr %v", err, tt.wantErr)
			}
			if video.Status != tt.wantStatus {
				t.Errorf("Video.Status = %v, want %v", video.Status, tt.wantStatus)
			}
			if !tt.wantErr && !video.UpdatedAt.After(oldUpdatedAt) {
				t.Error("Video.TransitionTo() should update UpdatedAt on success")
			}
		})
	}
}

func TestVideo_SetOriginalURL(t *testing.T) {
	video, _ := NewVideo(uuid.New(), "test")
	oldUpdatedAt := video.UpdatedAt

	video.SetOriginalURL("s3://bucket/video.mp4")

	if video.OriginalURL != "s3://bucket/video.mp4" {
		t.Errorf("Video.OriginalURL = %v, want %v", video.OriginalURL, "s3://bucket/video.mp4")
	}
	if !video.UpdatedAt.After(oldUpdatedAt) {
		t.Error("Video.SetOriginalURL() should update UpdatedAt")
	}
}

func TestVideo_SetHLSURL(t *testing.T) {
	video, _ := NewVideo(uuid.New(), "test")
	oldUpdatedAt := video.UpdatedAt

	video.SetHLSURL("s3://bucket/hls/master.m3u8")

	if video.HLSURL != "s3://bucket/hls/master.m3u8" {
		t.Errorf("Video.HLSURL = %v, want %v", video.HLSURL, "s3://bucket/hls/master.m3u8")
	}
	if !video.UpdatedAt.After(oldUpdatedAt) {
		t.Error("Video.SetHLSURL() should update UpdatedAt")
	}
}

func TestVideo_IsReady(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		want   bool
	}{
		{"READY returns true", StatusReady, true},
		{"PENDING_UPLOAD returns false", StatusPendingUpload, false},
		{"PROCESSING returns false", StatusProcessing, false},
		{"FAILED returns false", StatusFailed, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			video, _ := NewVideo(uuid.New(), "test")
			video.Status = tt.status

			if got := video.IsReady(); got != tt.want {
				t.Errorf("Video.IsReady() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVideo_IsFailed(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		want   bool
	}{
		{"FAILED returns true", StatusFailed, true},
		{"PENDING_UPLOAD returns false", StatusPendingUpload, false},
		{"PROCESSING returns false", StatusProcessing, false},
		{"READY returns false", StatusReady, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			video, _ := NewVideo(uuid.New(), "test")
			video.Status = tt.status

			if got := video.IsFailed(); got != tt.want {
				t.Errorf("Video.IsFailed() = %v, want %v", got, tt.want)
			}
		})
	}
}
