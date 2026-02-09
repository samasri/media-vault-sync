package memory

import (
	"context"
	"sync"

	"github.com/media-vault-sync/internal/core/domain"
)

type AlbumRepository struct {
	mu     sync.RWMutex
	albums map[string]*domain.Album
}

func NewAlbumRepository() *AlbumRepository {
	return &AlbumRepository{
		albums: make(map[string]*domain.Album),
	}
}

func (r *AlbumRepository) makeKey(providerID, databaseID, albumUID string) string {
	return providerID + "|" + databaseID + "|" + albumUID
}

func (r *AlbumRepository) FindByAlbumUID(ctx context.Context, providerID, databaseID, albumUID string) (*domain.Album, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := r.makeKey(providerID, databaseID, albumUID)
	album, exists := r.albums[key]
	if !exists {
		return nil, nil
	}
	copy := *album
	return &copy, nil
}

func (r *AlbumRepository) Create(ctx context.Context, album *domain.Album) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := r.makeKey(album.ProviderID, album.DatabaseID, album.AlbumUID)
	copy := *album
	r.albums[key] = &copy
	return nil
}

func (r *AlbumRepository) Update(ctx context.Context, album *domain.Album) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := r.makeKey(album.ProviderID, album.DatabaseID, album.AlbumUID)
	copy := *album
	r.albums[key] = &copy
	return nil
}

func (r *AlbumRepository) FindNeedingRepair(ctx context.Context) ([]*domain.Album, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*domain.Album
	for _, album := range r.albums {
		if !album.Synced {
			copy := *album
			result = append(result, &copy)
		}
	}
	return result, nil
}
