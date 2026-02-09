package memory

import (
	"context"
	"sync"

	"github.com/media-vault-sync/internal/core/domain"
	"github.com/media-vault-sync/internal/core/services"
)

type ObjectRepository struct {
	mu      sync.RWMutex
	objects map[string]*domain.Object
}

func NewObjectRepository() *ObjectRepository {
	return &ObjectRepository{
		objects: make(map[string]*domain.Object),
	}
}

func (r *ObjectRepository) makeKey(providerID, databaseID, videoUID string) string {
	return providerID + "|" + databaseID + "|" + videoUID
}

func (r *ObjectRepository) Upsert(ctx context.Context, object *domain.Object) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := r.makeKey(object.ProviderID, object.DatabaseID, object.VideoUID)
	copied := *object
	r.objects[key] = &copied
	return nil
}

func (r *ObjectRepository) FindByVideoUID(ctx context.Context, providerID, databaseID, videoUID string) (*domain.Object, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := r.makeKey(providerID, databaseID, videoUID)
	object, exists := r.objects[key]
	if !exists {
		return nil, nil
	}

	copied := *object
	return &copied, nil
}

func (r *ObjectRepository) CountByAlbumUID(ctx context.Context, providerID, databaseID, albumUID string, albumVideoRepo services.AlbumVideoRepository) (int, error) {
	videos, err := albumVideoRepo.FindByAlbumUID(ctx, providerID, databaseID, albumUID)
	if err != nil {
		return 0, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, vid := range videos {
		key := r.makeKey(providerID, databaseID, vid.VideoUID)
		if _, exists := r.objects[key]; exists {
			count++
		}
	}
	return count, nil
}
