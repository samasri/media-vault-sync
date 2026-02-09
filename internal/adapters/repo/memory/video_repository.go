package memory

import (
	"context"
	"sync"

	"github.com/media-vault-sync/internal/core/domain"
)

type VideoRepository struct {
	mu     sync.RWMutex
	videos map[string]*domain.Video
}

func NewVideoRepository() *VideoRepository {
	return &VideoRepository{
		videos: make(map[string]*domain.Video),
	}
}

func (r *VideoRepository) makeKey(providerID, databaseID, videoUID string) string {
	return providerID + "|" + databaseID + "|" + videoUID
}

func (r *VideoRepository) Upsert(ctx context.Context, video *domain.Video) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := r.makeKey(video.ProviderID, video.DatabaseID, video.VideoUID)
	existing, exists := r.videos[key]
	if exists {
		existing.UserID = video.UserID
		existing.UpdatedAt = video.UpdatedAt
	} else {
		copied := *video
		r.videos[key] = &copied
	}
	return nil
}

func (r *VideoRepository) FindByVideoUID(ctx context.Context, providerID, databaseID, videoUID string) (*domain.Video, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := r.makeKey(providerID, databaseID, videoUID)
	video, exists := r.videos[key]
	if !exists {
		return nil, nil
	}

	copied := *video
	return &copied, nil
}
