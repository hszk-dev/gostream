package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Status represents the processing state of a video.
type Status string

const (
	StatusPendingUpload Status = "PENDING_UPLOAD"
	StatusProcessing    Status = "PROCESSING"
	StatusReady         Status = "READY"
	StatusFailed        Status = "FAILED"
)

// Valid status transitions:
// PENDING_UPLOAD -> PROCESSING -> READY
//                            \-> FAILED
var validTransitions = map[Status][]Status{
	StatusPendingUpload: {StatusProcessing},
	StatusProcessing:    {StatusReady, StatusFailed},
	StatusReady:         {},
	StatusFailed:        {},
}

func (s Status) IsValid() bool {
	switch s {
	case StatusPendingUpload, StatusProcessing, StatusReady, StatusFailed:
		return true
	default:
		return false
	}
}

func (s Status) CanTransitionTo(next Status) bool {
	allowed, exists := validTransitions[s]
	if !exists {
		return false
	}
	for _, status := range allowed {
		if status == next {
			return true
		}
	}
	return false
}

func (s Status) String() string {
	return string(s)
}

// Video represents a video entity in the domain.
type Video struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	Title       string
	Status      Status
	OriginalURL string
	HLSURL      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

var (
	ErrEmptyTitle         = errors.New("title cannot be empty")
	ErrInvalidUserID      = errors.New("user ID cannot be nil")
	ErrInvalidTransition  = errors.New("invalid status transition")
	ErrTitleTooLong       = errors.New("title exceeds maximum length of 255 characters")
)

const maxTitleLength = 255

// NewVideo creates a new Video with PENDING_UPLOAD status.
func NewVideo(userID uuid.UUID, title string) (*Video, error) {
	if userID == uuid.Nil {
		return nil, ErrInvalidUserID
	}
	if title == "" {
		return nil, ErrEmptyTitle
	}
	if len(title) > maxTitleLength {
		return nil, ErrTitleTooLong
	}

	now := time.Now()
	return &Video{
		ID:        uuid.New(),
		UserID:    userID,
		Title:     title,
		Status:    StatusPendingUpload,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// TransitionTo attempts to change the video status.
// Returns error if the transition is not allowed.
func (v *Video) TransitionTo(next Status) error {
	if !next.IsValid() {
		return ErrInvalidTransition
	}
	if !v.Status.CanTransitionTo(next) {
		return ErrInvalidTransition
	}
	v.Status = next
	v.UpdatedAt = time.Now()
	return nil
}

// SetOriginalURL sets the original video URL after upload.
func (v *Video) SetOriginalURL(url string) {
	v.OriginalURL = url
	v.UpdatedAt = time.Now()
}

// SetHLSURL sets the HLS manifest URL after transcoding.
func (v *Video) SetHLSURL(url string) {
	v.HLSURL = url
	v.UpdatedAt = time.Now()
}

// IsReady returns true if the video is ready for streaming.
func (v *Video) IsReady() bool {
	return v.Status == StatusReady
}

// IsFailed returns true if the video processing failed.
func (v *Video) IsFailed() bool {
	return v.Status == StatusFailed
}
