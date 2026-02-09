package services

import (
	"context"

	"github.com/media-vault-sync/internal/core/domain"
)

type AlbumVideoRepository interface {
	FindByAlbumUID(ctx context.Context, providerID, databaseID, albumUID string) ([]domain.AlbumVideo, error)
	ReplaceForAlbum(ctx context.Context, providerID, databaseID, albumUID string, videos []domain.AlbumVideo) error
	Exists(ctx context.Context, providerID, databaseID, albumUID, videoUID string) (bool, error)
}
