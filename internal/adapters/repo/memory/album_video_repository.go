package memory

import (
	"context"
	"sync"

	"github.com/media-vault-sync/internal/core/domain"
)

type AlbumVideoRepository struct {
	mu     sync.RWMutex
	videos map[string][]domain.AlbumVideo
}

func NewAlbumVideoRepository() *AlbumVideoRepository {
	return &AlbumVideoRepository{
		videos: make(map[string][]domain.AlbumVideo),
	}
}

func (r *AlbumVideoRepository) makeAlbumKey(providerID, databaseID, albumUID string) string {
	return providerID + "|" + databaseID + "|" + albumUID
}

func (r *AlbumVideoRepository) FindByAlbumUID(ctx context.Context, providerID, databaseID, albumUID string) ([]domain.AlbumVideo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := r.makeAlbumKey(providerID, databaseID, albumUID)
	videos, exists := r.videos[key]
	if !exists {
		return nil, nil
	}

	result := make([]domain.AlbumVideo, len(videos))
	copy(result, videos)
	return result, nil
}

func (r *AlbumVideoRepository) ReplaceForAlbum(ctx context.Context, providerID, databaseID, albumUID string, videos []domain.AlbumVideo) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := r.makeAlbumKey(providerID, databaseID, albumUID)
	copied := make([]domain.AlbumVideo, len(videos))
	copy(copied, videos)
	r.videos[key] = copied
	return nil
}

func (r *AlbumVideoRepository) Exists(ctx context.Context, providerID, databaseID, albumUID, videoUID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := r.makeAlbumKey(providerID, databaseID, albumUID)
	videos, exists := r.videos[key]
	if !exists {
		return false, nil
	}

	for _, vid := range videos {
		if vid.VideoUID == videoUID {
			return true, nil
		}
	}
	return false, nil
}
