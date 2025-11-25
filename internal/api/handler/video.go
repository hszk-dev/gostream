package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hszk-dev/gostream/internal/domain/model"
	"github.com/hszk-dev/gostream/internal/domain/repository"
	"github.com/hszk-dev/gostream/internal/usecase"
)

// Request/Response types

type CreateVideoRequest struct {
	UserID   string `json:"user_id"`
	Title    string `json:"title"`
	FileName string `json:"file_name"`
}

type CreateVideoResponse struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	UploadURL string `json:"upload_url"`
	CreatedAt string `json:"created_at"`
}

type VideoResponse struct {
	ID          string `json:"id"`
	UserID      string `json:"user_id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	OriginalURL string `json:"original_url,omitempty"`
	HLSURL      string `json:"hls_url,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// VideoHandler handles video-related HTTP requests.
type VideoHandler struct {
	svc usecase.VideoService
}

// NewVideoHandler creates a new VideoHandler.
func NewVideoHandler(svc usecase.VideoService) *VideoHandler {
	return &VideoHandler{svc: svc}
}

// Create handles POST /v1/videos
func (h *VideoHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateVideoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Error(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		Error(w, http.StatusBadRequest, "invalid_user_id", "User ID must be a valid UUID")
		return
	}

	if req.Title == "" {
		Error(w, http.StatusBadRequest, "invalid_title", "Title is required")
		return
	}

	if req.FileName == "" {
		Error(w, http.StatusBadRequest, "invalid_file_name", "File name is required")
		return
	}

	output, err := h.svc.CreateVideo(r.Context(), usecase.CreateVideoInput{
		UserID:   userID,
		Title:    req.Title,
		FileName: req.FileName,
	})
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	JSON(w, http.StatusCreated, CreateVideoResponse{
		ID:        output.Video.ID.String(),
		UserID:    output.Video.UserID.String(),
		Title:     output.Video.Title,
		Status:    output.Video.Status.String(),
		UploadURL: output.UploadURL,
		CreatedAt: output.Video.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// TriggerProcess handles POST /v1/videos/{id}/process
func (h *VideoHandler) TriggerProcess(w http.ResponseWriter, r *http.Request) {
	videoID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		Error(w, http.StatusBadRequest, "invalid_video_id", "Video ID must be a valid UUID")
		return
	}

	if err := h.svc.TriggerProcess(r.Context(), videoID); err != nil {
		h.handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

// Get handles GET /v1/videos/{id}
func (h *VideoHandler) Get(w http.ResponseWriter, r *http.Request) {
	videoID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		Error(w, http.StatusBadRequest, "invalid_video_id", "Video ID must be a valid UUID")
		return
	}

	video, err := h.svc.GetVideo(r.Context(), videoID)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	JSON(w, http.StatusOK, toVideoResponse(video))
}

func (h *VideoHandler) handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, repository.ErrVideoNotFound):
		Error(w, http.StatusNotFound, "video_not_found", "Video not found")
	case errors.Is(err, model.ErrInvalidUserID):
		Error(w, http.StatusBadRequest, "invalid_user_id", "User ID cannot be empty")
	case errors.Is(err, model.ErrEmptyTitle):
		Error(w, http.StatusBadRequest, "invalid_title", "Title cannot be empty")
	case errors.Is(err, model.ErrTitleTooLong):
		Error(w, http.StatusBadRequest, "invalid_title", "Title exceeds maximum length")
	case errors.Is(err, usecase.ErrVideoAlreadyCompleted):
		Error(w, http.StatusConflict, "video_already_completed", "Video processing has already completed")
	default:
		Error(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")
	}
}

func toVideoResponse(v *model.Video) VideoResponse {
	return VideoResponse{
		ID:          v.ID.String(),
		UserID:      v.UserID.String(),
		Title:       v.Title,
		Status:      v.Status.String(),
		OriginalURL: v.OriginalURL,
		HLSURL:      v.HLSURL,
		CreatedAt:   v.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   v.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
