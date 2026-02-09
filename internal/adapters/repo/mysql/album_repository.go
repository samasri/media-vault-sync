package mysql

import (
	"context"
	"database/sql"

	"github.com/media-vault-sync/internal/core/domain"
)

type AlbumRepository struct {
	db *sql.DB
}

func NewAlbumRepository(db *sql.DB) *AlbumRepository {
	return &AlbumRepository{db: db}
}

func (r *AlbumRepository) FindByAlbumUID(ctx context.Context, providerID, databaseID, albumUID string) (*domain.Album, error) {
	query := `
		SELECT uid, provider_id, database_id, user_id, album_uid, synced, created_at, updated_at
		FROM albums
		WHERE provider_id = ? AND database_id = ? AND album_uid = ?
	`

	var album domain.Album
	err := r.db.QueryRowContext(ctx, query, providerID, databaseID, albumUID).Scan(
		&album.UID,
		&album.ProviderID,
		&album.DatabaseID,
		&album.UserID,
		&album.AlbumUID,
		&album.Synced,
		&album.CreatedAt,
		&album.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &album, nil
}

func (r *AlbumRepository) Create(ctx context.Context, album *domain.Album) error {
	query := `
		INSERT INTO albums (uid, provider_id, database_id, user_id, album_uid, synced, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := r.db.ExecContext(ctx, query,
		album.UID,
		album.ProviderID,
		album.DatabaseID,
		album.UserID,
		album.AlbumUID,
		album.Synced,
		album.CreatedAt,
		album.UpdatedAt,
	)

	return err
}

func (r *AlbumRepository) Update(ctx context.Context, album *domain.Album) error {
	query := `
		UPDATE albums
		SET user_id = ?, synced = ?, updated_at = ?
		WHERE provider_id = ? AND database_id = ? AND album_uid = ?
	`

	_, err := r.db.ExecContext(ctx, query,
		album.UserID,
		album.Synced,
		album.UpdatedAt,
		album.ProviderID,
		album.DatabaseID,
		album.AlbumUID,
	)

	return err
}

func (r *AlbumRepository) FindNeedingRepair(ctx context.Context) ([]*domain.Album, error) {
	query := `
		SELECT uid, provider_id, database_id, user_id, album_uid, synced, created_at, updated_at
		FROM albums
		WHERE synced = FALSE
	` // TODO: add an "or instances in the db don't match the ones in the manifest"

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var albums []*domain.Album
	for rows.Next() {
		var album domain.Album
		err := rows.Scan(
			&album.UID,
			&album.ProviderID,
			&album.DatabaseID,
			&album.UserID,
			&album.AlbumUID,
			&album.Synced,
			&album.CreatedAt,
			&album.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		albums = append(albums, &album)
	}

	return albums, rows.Err()
}
