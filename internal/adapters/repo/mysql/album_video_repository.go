package mysql

import (
	"context"
	"database/sql"

	"github.com/media-vault-sync/internal/core/domain"
)

type AlbumVideoRepository struct {
	db *sql.DB
}

func NewAlbumVideoRepository(db *sql.DB) *AlbumVideoRepository {
	return &AlbumVideoRepository{db: db}
}

func (r *AlbumVideoRepository) FindByAlbumUID(ctx context.Context, providerID, databaseID, albumUID string) ([]domain.AlbumVideo, error) {
	query := `
		SELECT provider_id, database_id, album_uid, video_uid
		FROM album_videos
		WHERE provider_id = ? AND database_id = ? AND album_uid = ?
	`

	rows, err := r.db.QueryContext(ctx, query, providerID, databaseID, albumUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var videos []domain.AlbumVideo
	for rows.Next() {
		var av domain.AlbumVideo
		err := rows.Scan(&av.ProviderID, &av.DatabaseID, &av.AlbumUID, &av.VideoUID)
		if err != nil {
			return nil, err
		}
		videos = append(videos, av)
	}

	return videos, rows.Err()
}

func (r *AlbumVideoRepository) ReplaceForAlbum(ctx context.Context, providerID, databaseID, albumUID string, videos []domain.AlbumVideo) error {
	// TODO: add lock album that with SELECT FOR UPDATE

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	deleteQuery := `
		DELETE FROM album_videos
		WHERE provider_id = ? AND database_id = ? AND album_uid = ?
	`
	_, err = tx.ExecContext(ctx, deleteQuery, providerID, databaseID, albumUID)
	if err != nil {
		return err
	}

	insertQuery := `
		INSERT INTO album_videos (provider_id, database_id, album_uid, video_uid)
		VALUES (?, ?, ?, ?)
	`
	stmt, err := tx.PrepareContext(ctx, insertQuery)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, av := range videos {
		_, err = stmt.ExecContext(ctx, av.ProviderID, av.DatabaseID, av.AlbumUID, av.VideoUID)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *AlbumVideoRepository) Exists(ctx context.Context, providerID, databaseID, albumUID, videoUID string) (bool, error) {
	query := `
		SELECT 1 FROM album_videos
		WHERE provider_id = ? AND database_id = ? AND album_uid = ? AND video_uid = ?
		LIMIT 1
	`

	var exists int
	err := r.db.QueryRowContext(ctx, query, providerID, databaseID, albumUID, videoUID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, nil
}
