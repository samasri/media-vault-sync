package mysql

import (
	"context"
	"database/sql"

	"github.com/media-vault-sync/internal/core/domain"
)

type VideoRepository struct {
	db *sql.DB
}

func NewVideoRepository(db *sql.DB) *VideoRepository {
	return &VideoRepository{db: db}
}

func (r *VideoRepository) Upsert(ctx context.Context, video *domain.Video) error {
	query := `
		INSERT INTO videos (uid, provider_id, database_id, user_id, video_uid, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			user_id = VALUES(user_id),
			updated_at = VALUES(updated_at)
	`

	_, err := r.db.ExecContext(ctx, query,
		video.UID,
		video.ProviderID,
		video.DatabaseID,
		video.UserID,
		video.VideoUID,
		video.CreatedAt,
		video.UpdatedAt,
	)

	return err
}

func (r *VideoRepository) FindByVideoUID(ctx context.Context, providerID, databaseID, videoUID string) (*domain.Video, error) {
	query := `
		SELECT uid, provider_id, database_id, user_id, video_uid, created_at, updated_at
		FROM videos
		WHERE provider_id = ? AND database_id = ? AND video_uid = ?
	`

	var video domain.Video
	err := r.db.QueryRowContext(ctx, query, providerID, databaseID, videoUID).Scan(
		&video.UID,
		&video.ProviderID,
		&video.DatabaseID,
		&video.UserID,
		&video.VideoUID,
		&video.CreatedAt,
		&video.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &video, nil
}
