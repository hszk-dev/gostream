package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/hszk-dev/gostream/internal/domain/model"
	"github.com/hszk-dev/gostream/internal/domain/repository"
)

// DBTX is an interface that abstracts pgxpool.Pool and pgx.Tx for testability.
type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// VideoRepository implements repository.VideoRepository using PostgreSQL.
type VideoRepository struct {
	db DBTX
}

// NewVideoRepository creates a new VideoRepository instance.
func NewVideoRepository(db DBTX) *VideoRepository {
	return &VideoRepository{db: db}
}

// Create persists a new video entity.
func (r *VideoRepository) Create(ctx context.Context, video *model.Video) error {
	const query = `
		INSERT INTO videos (id, user_id, title, status, original_url, hls_url, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err := r.db.Exec(ctx, query,
		video.ID,
		video.UserID,
		video.Title,
		video.Status.String(),
		nullString(video.OriginalURL),
		nullString(video.HLSURL),
		video.CreatedAt,
		video.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return repository.ErrDuplicateVideo
		}
		return fmt.Errorf("failed to create video: %w", err)
	}

	return nil
}

// GetByID retrieves a video by its unique identifier.
func (r *VideoRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Video, error) {
	const query = `
		SELECT id, user_id, title, status, original_url, hls_url, created_at, updated_at
		FROM videos
		WHERE id = $1
	`

	video, err := r.scanVideo(r.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrVideoNotFound
		}
		return nil, fmt.Errorf("failed to get video by ID: %w", err)
	}

	return video, nil
}

// GetByUserID retrieves all videos belonging to a user.
func (r *VideoRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*model.Video, error) {
	const query = `
		SELECT id, user_id, title, status, original_url, hls_url, created_at, updated_at
		FROM videos
		WHERE user_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query videos by user ID: %w", err)
	}
	defer rows.Close()

	var videos []*model.Video
	for rows.Next() {
		video, err := r.scanVideoFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan video: %w", err)
		}
		videos = append(videos, video)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating videos: %w", err)
	}

	return videos, nil
}

// Update persists changes to an existing video entity.
func (r *VideoRepository) Update(ctx context.Context, video *model.Video) error {
	const query = `
		UPDATE videos
		SET title = $2, status = $3, original_url = $4, hls_url = $5, updated_at = $6
		WHERE id = $1
	`

	video.UpdatedAt = time.Now()

	tag, err := r.db.Exec(ctx, query,
		video.ID,
		video.Title,
		video.Status.String(),
		nullString(video.OriginalURL),
		nullString(video.HLSURL),
		video.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to update video: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return repository.ErrVideoNotFound
	}

	return nil
}

// UpdateStatus updates only the status field of a video.
func (r *VideoRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status model.Status) error {
	const query = `
		UPDATE videos
		SET status = $2, updated_at = $3
		WHERE id = $1
	`

	tag, err := r.db.Exec(ctx, query, id, status.String(), time.Now())
	if err != nil {
		return fmt.Errorf("failed to update video status: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return repository.ErrVideoNotFound
	}

	return nil
}

// scanVideo scans a single row into a Video model.
func (r *VideoRepository) scanVideo(row pgx.Row) (*model.Video, error) {
	var (
		video       model.Video
		status      string
		originalURL *string
		hlsURL      *string
	)

	err := row.Scan(
		&video.ID,
		&video.UserID,
		&video.Title,
		&status,
		&originalURL,
		&hlsURL,
		&video.CreatedAt,
		&video.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	video.Status = model.Status(status)
	if originalURL != nil {
		video.OriginalURL = *originalURL
	}
	if hlsURL != nil {
		video.HLSURL = *hlsURL
	}

	return &video, nil
}

// scanVideoFromRows scans from pgx.Rows into a Video model.
func (r *VideoRepository) scanVideoFromRows(rows pgx.Rows) (*model.Video, error) {
	var (
		video       model.Video
		status      string
		originalURL *string
		hlsURL      *string
	)

	err := rows.Scan(
		&video.ID,
		&video.UserID,
		&video.Title,
		&status,
		&originalURL,
		&hlsURL,
		&video.CreatedAt,
		&video.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	video.Status = model.Status(status)
	if originalURL != nil {
		video.OriginalURL = *originalURL
	}
	if hlsURL != nil {
		video.HLSURL = *hlsURL
	}

	return &video, nil
}

// nullString returns nil for empty strings, otherwise returns a pointer to the string.
func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Compile-time verification that VideoRepository implements repository.VideoRepository.
var _ repository.VideoRepository = (*VideoRepository)(nil)
