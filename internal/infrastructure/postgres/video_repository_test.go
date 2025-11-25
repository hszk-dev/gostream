package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/hszk-dev/gostream/internal/domain/model"
	"github.com/hszk-dev/gostream/internal/domain/repository"
)

func TestVideoRepository_Create(t *testing.T) {
	tests := []struct {
		name    string
		video   *model.Video
		mockFn  func(mock pgxmock.PgxPoolIface, video *model.Video)
		wantErr error
	}{
		{
			name: "successful creation",
			video: &model.Video{
				ID:        uuid.New(),
				UserID:    uuid.New(),
				Title:     "Test Video",
				Status:    model.StatusPendingUpload,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			mockFn: func(mock pgxmock.PgxPoolIface, video *model.Video) {
				mock.ExpectExec("INSERT INTO videos").
					WithArgs(
						video.ID,
						video.UserID,
						video.Title,
						video.Status.String(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
					).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))
			},
			wantErr: nil,
		},
		{
			name: "duplicate video error",
			video: &model.Video{
				ID:        uuid.New(),
				UserID:    uuid.New(),
				Title:     "Test Video",
				Status:    model.StatusPendingUpload,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			mockFn: func(mock pgxmock.PgxPoolIface, video *model.Video) {
				mock.ExpectExec("INSERT INTO videos").
					WithArgs(
						video.ID,
						video.UserID,
						video.Title,
						video.Status.String(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
					).
					WillReturnError(&pgconn.PgError{Code: "23505"})
			},
			wantErr: repository.ErrDuplicateVideo,
		},
		{
			name: "database error",
			video: &model.Video{
				ID:        uuid.New(),
				UserID:    uuid.New(),
				Title:     "Test Video",
				Status:    model.StatusPendingUpload,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			mockFn: func(mock pgxmock.PgxPoolIface, video *model.Video) {
				mock.ExpectExec("INSERT INTO videos").
					WithArgs(
						video.ID,
						video.UserID,
						video.Title,
						video.Status.String(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
					).
					WillReturnError(errors.New("connection refused"))
			},
			wantErr: errors.New("failed to create video"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock, err := pgxmock.NewPool()
			if err != nil {
				t.Fatalf("failed to create mock: %v", err)
			}
			defer mock.Close()

			tt.mockFn(mock, tt.video)

			repo := NewVideoRepository(mock)
			err = repo.Create(context.Background(), tt.video)

			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("Create() expected error, got nil")
					return
				}
				if !errors.Is(err, tt.wantErr) && !containsError(err, tt.wantErr) {
					t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("Create() unexpected error = %v", err)
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestVideoRepository_GetByID(t *testing.T) {
	now := time.Now()
	videoID := uuid.New()
	userID := uuid.New()

	tests := []struct {
		name    string
		id      uuid.UUID
		mockFn  func(mock pgxmock.PgxPoolIface)
		want    *model.Video
		wantErr error
	}{
		{
			name: "successful retrieval",
			id:   videoID,
			mockFn: func(mock pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{
					"id", "user_id", "title", "status", "original_url", "hls_url", "created_at", "updated_at",
				}).AddRow(
					videoID, userID, "Test Video", "PENDING_UPLOAD", nil, nil, now, now,
				)
				mock.ExpectQuery("SELECT .* FROM videos WHERE id").
					WithArgs(videoID).
					WillReturnRows(rows)
			},
			want: &model.Video{
				ID:        videoID,
				UserID:    userID,
				Title:     "Test Video",
				Status:    model.StatusPendingUpload,
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantErr: nil,
		},
		{
			name: "video not found",
			id:   videoID,
			mockFn: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectQuery("SELECT .* FROM videos WHERE id").
					WithArgs(videoID).
					WillReturnError(pgx.ErrNoRows)
			},
			want:    nil,
			wantErr: repository.ErrVideoNotFound,
		},
		{
			name: "with original and hls urls",
			id:   videoID,
			mockFn: func(mock pgxmock.PgxPoolIface) {
				originalURL := "s3://bucket/original.mp4"
				hlsURL := "s3://bucket/hls/master.m3u8"
				rows := pgxmock.NewRows([]string{
					"id", "user_id", "title", "status", "original_url", "hls_url", "created_at", "updated_at",
				}).AddRow(
					videoID, userID, "Test Video", "READY", &originalURL, &hlsURL, now, now,
				)
				mock.ExpectQuery("SELECT .* FROM videos WHERE id").
					WithArgs(videoID).
					WillReturnRows(rows)
			},
			want: &model.Video{
				ID:          videoID,
				UserID:      userID,
				Title:       "Test Video",
				Status:      model.StatusReady,
				OriginalURL: "s3://bucket/original.mp4",
				HLSURL:      "s3://bucket/hls/master.m3u8",
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock, err := pgxmock.NewPool()
			if err != nil {
				t.Fatalf("failed to create mock: %v", err)
			}
			defer mock.Close()

			tt.mockFn(mock)

			repo := NewVideoRepository(mock)
			got, err := repo.GetByID(context.Background(), tt.id)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("GetByID() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("GetByID() unexpected error = %v", err)
				return
			}

			if got.ID != tt.want.ID ||
				got.UserID != tt.want.UserID ||
				got.Title != tt.want.Title ||
				got.Status != tt.want.Status ||
				got.OriginalURL != tt.want.OriginalURL ||
				got.HLSURL != tt.want.HLSURL {
				t.Errorf("GetByID() = %+v, want %+v", got, tt.want)
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestVideoRepository_GetByUserID(t *testing.T) {
	now := time.Now()
	userID := uuid.New()
	videoID1 := uuid.New()
	videoID2 := uuid.New()

	tests := []struct {
		name    string
		userID  uuid.UUID
		mockFn  func(mock pgxmock.PgxPoolIface)
		want    int
		wantErr bool
	}{
		{
			name:   "returns multiple videos",
			userID: userID,
			mockFn: func(mock pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{
					"id", "user_id", "title", "status", "original_url", "hls_url", "created_at", "updated_at",
				}).
					AddRow(videoID1, userID, "Video 1", "READY", nil, nil, now, now).
					AddRow(videoID2, userID, "Video 2", "PENDING_UPLOAD", nil, nil, now, now)
				mock.ExpectQuery("SELECT .* FROM videos WHERE user_id").
					WithArgs(userID).
					WillReturnRows(rows)
			},
			want:    2,
			wantErr: false,
		},
		{
			name:   "returns empty slice when no videos",
			userID: userID,
			mockFn: func(mock pgxmock.PgxPoolIface) {
				rows := pgxmock.NewRows([]string{
					"id", "user_id", "title", "status", "original_url", "hls_url", "created_at", "updated_at",
				})
				mock.ExpectQuery("SELECT .* FROM videos WHERE user_id").
					WithArgs(userID).
					WillReturnRows(rows)
			},
			want:    0,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock, err := pgxmock.NewPool()
			if err != nil {
				t.Fatalf("failed to create mock: %v", err)
			}
			defer mock.Close()

			tt.mockFn(mock)

			repo := NewVideoRepository(mock)
			got, err := repo.GetByUserID(context.Background(), tt.userID)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByUserID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(got) != tt.want {
				t.Errorf("GetByUserID() returned %d videos, want %d", len(got), tt.want)
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestVideoRepository_Update(t *testing.T) {
	videoID := uuid.New()

	tests := []struct {
		name    string
		video   *model.Video
		mockFn  func(mock pgxmock.PgxPoolIface)
		wantErr error
	}{
		{
			name: "successful update",
			video: &model.Video{
				ID:          videoID,
				UserID:      uuid.New(),
				Title:       "Updated Title",
				Status:      model.StatusProcessing,
				OriginalURL: "s3://bucket/original.mp4",
			},
			mockFn: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectExec("UPDATE videos").
					WithArgs(
						videoID,
						"Updated Title",
						"PROCESSING",
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
					).
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))
			},
			wantErr: nil,
		},
		{
			name: "video not found",
			video: &model.Video{
				ID:     videoID,
				UserID: uuid.New(),
				Title:  "Updated Title",
				Status: model.StatusProcessing,
			},
			mockFn: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectExec("UPDATE videos").
					WithArgs(
						videoID,
						"Updated Title",
						"PROCESSING",
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
						pgxmock.AnyArg(),
					).
					WillReturnResult(pgxmock.NewResult("UPDATE", 0))
			},
			wantErr: repository.ErrVideoNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock, err := pgxmock.NewPool()
			if err != nil {
				t.Fatalf("failed to create mock: %v", err)
			}
			defer mock.Close()

			tt.mockFn(mock)

			repo := NewVideoRepository(mock)
			err = repo.Update(context.Background(), tt.video)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("Update() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("Update() unexpected error = %v", err)
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestVideoRepository_UpdateStatus(t *testing.T) {
	videoID := uuid.New()

	tests := []struct {
		name    string
		id      uuid.UUID
		status  model.Status
		mockFn  func(mock pgxmock.PgxPoolIface)
		wantErr error
	}{
		{
			name:   "successful status update",
			id:     videoID,
			status: model.StatusProcessing,
			mockFn: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectExec("UPDATE videos").
					WithArgs(videoID, "PROCESSING", pgxmock.AnyArg()).
					WillReturnResult(pgxmock.NewResult("UPDATE", 1))
			},
			wantErr: nil,
		},
		{
			name:   "video not found",
			id:     videoID,
			status: model.StatusProcessing,
			mockFn: func(mock pgxmock.PgxPoolIface) {
				mock.ExpectExec("UPDATE videos").
					WithArgs(videoID, "PROCESSING", pgxmock.AnyArg()).
					WillReturnResult(pgxmock.NewResult("UPDATE", 0))
			},
			wantErr: repository.ErrVideoNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock, err := pgxmock.NewPool()
			if err != nil {
				t.Fatalf("failed to create mock: %v", err)
			}
			defer mock.Close()

			tt.mockFn(mock)

			repo := NewVideoRepository(mock)
			err = repo.UpdateStatus(context.Background(), tt.id, tt.status)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("UpdateStatus() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("UpdateStatus() unexpected error = %v", err)
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}

// containsError checks if err's message contains the expected error's message.
func containsError(err, expected error) bool {
	if err == nil || expected == nil {
		return false
	}
	return err.Error() != "" && expected.Error() != "" &&
		len(err.Error()) >= len(expected.Error()) &&
		err.Error()[:len(expected.Error())] == expected.Error()[:len(expected.Error())]
}
