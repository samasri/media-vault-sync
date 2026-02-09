package services

import (
	"context"

	"github.com/media-vault-sync/internal/core/domain"
)

type ObjectRepository interface {
	Upsert(ctx context.Context, object *domain.Object) error
	FindByVideoUID(ctx context.Context, providerID, databaseID, videoUID string) (*domain.Object, error)
	CountByAlbumUID(ctx context.Context, providerID, databaseID, albumUID string, albumVideoRepo AlbumVideoRepository) (int, error)
}
