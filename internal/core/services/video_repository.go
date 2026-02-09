package services

import (
	"context"

	"github.com/media-vault-sync/internal/core/domain"
)

type VideoRepository interface {
	Upsert(ctx context.Context, video *domain.Video) error
	FindByVideoUID(ctx context.Context, providerID, databaseID, videoUID string) (*domain.Video, error)
}
