package mysql

import (
	"context"
	"database/sql"

	"github.com/media-vault-sync/internal/core/domain"
	"github.com/media-vault-sync/internal/core/services"
)

type ObjectRepository struct {
	db *sql.DB
}

func NewObjectRepository(db *sql.DB) *ObjectRepository {
	return &ObjectRepository{db: db}
}

func (r *ObjectRepository) Upsert(ctx context.Context, object *domain.Object) error {
	query := `
		INSERT INTO objects (uid, provider_id, database_id, video_uid, storage_key, size_bytes, checksum, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			storage_key = VALUES(storage_key),
			size_bytes = VALUES(size_bytes),
			checksum = VALUES(checksum)
	`

	_, err := r.db.ExecContext(ctx, query,
		object.UID,
		object.ProviderID,
		object.DatabaseID,
		object.VideoUID,
		object.StorageKey,
		object.SizeBytes,
		object.Checksum,
		object.CreatedAt,
	)

	return err
}

func (r *ObjectRepository) FindByVideoUID(ctx context.Context, providerID, databaseID, videoUID string) (*domain.Object, error) {
	query := `
		SELECT uid, provider_id, database_id, video_uid, storage_key, size_bytes, checksum, created_at
		FROM objects
		WHERE provider_id = ? AND database_id = ? AND video_uid = ?
	`

	var object domain.Object
	err := r.db.QueryRowContext(ctx, query, providerID, databaseID, videoUID).Scan(
		&object.UID,
		&object.ProviderID,
		&object.DatabaseID,
		&object.VideoUID,
		&object.StorageKey,
		&object.SizeBytes,
		&object.Checksum,
		&object.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &object, nil
}

func (r *ObjectRepository) CountByAlbumUID(ctx context.Context, providerID, databaseID, albumUID string, albumVideoRepo services.AlbumVideoRepository) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM objects o
		INNER JOIN album_videos av ON
			o.provider_id = av.provider_id AND
			o.database_id = av.database_id AND
			o.video_uid = av.video_uid
		WHERE av.provider_id = ? AND av.database_id = ? AND av.album_uid = ?
	`

	var count int
	err := r.db.QueryRowContext(ctx, query, providerID, databaseID, albumUID).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}
