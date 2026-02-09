package services

import (
	"context"

	"github.com/media-vault-sync/internal/core/domain"
)

type AlbumRepository interface {
	FindByAlbumUID(ctx context.Context, providerID, databaseID, albumUID string) (*domain.Album, error)
	Create(ctx context.Context, album *domain.Album) error
	Update(ctx context.Context, album *domain.Album) error
	FindNeedingRepair(ctx context.Context) ([]*domain.Album, error)
}
