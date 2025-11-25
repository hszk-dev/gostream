package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/hszk-dev/gostream/internal/domain/model"
)

// VideoRepository defines the interface for video persistence operations.
// Implementations should be provided by the infrastructure layer (e.g., PostgreSQL).
type VideoRepository interface {
	// Create persists a new video entity.
	// Returns error if the video already exists or persistence fails.
	Create(ctx context.Context, video *model.Video) error

	// GetByID retrieves a video by its unique identifier.
	// Returns nil and ErrVideoNotFound if the video does not exist.
	GetByID(ctx context.Context, id uuid.UUID) (*model.Video, error)

	// GetByUserID retrieves all videos belonging to a user.
	// Returns empty slice if no videos exist for the user.
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*model.Video, error)

	// Update persists changes to an existing video entity.
	// Returns ErrVideoNotFound if the video does not exist.
	Update(ctx context.Context, video *model.Video) error

	// UpdateStatus updates only the status field of a video.
	// This is optimized for status transitions without full entity update.
	// Returns ErrVideoNotFound if the video does not exist.
	UpdateStatus(ctx context.Context, id uuid.UUID, status model.Status) error
}
