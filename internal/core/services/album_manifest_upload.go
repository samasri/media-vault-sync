package services

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	"github.com/media-vault-sync/internal/core/domain"
)

var ErrUserIDMismatch = errors.New("user ID mismatch for existing album")

type AlbumManifestUploadRequest struct {
	ProviderID string   `json:"providerID"`
	DatabaseID string   `json:"databaseID"`
	UserID     string   `json:"userID"`
	AlbumUID   string   `json:"albumUID"`
	VideoUIDs  []string `json:"videoUIDs"`
}

type VideoUploadPayload struct {
	DatabaseID string `json:"databaseID"`
	AlbumUID   string `json:"albumUID"`
}

type AlbumManifestUploadService struct {
	albumRepo      AlbumRepository
	albumVideoRepo AlbumVideoRepository
	queue          Queue
	clock          Clock
}

func NewAlbumManifestUploadService(
	albumRepo AlbumRepository,
	albumVideoRepo AlbumVideoRepository,
	queue Queue,
	clock Clock,
) *AlbumManifestUploadService {
	return &AlbumManifestUploadService{
		albumRepo:      albumRepo,
		albumVideoRepo: albumVideoRepo,
		queue:          queue,
		clock:          clock,
	}
}

func (s *AlbumManifestUploadService) ProcessAlbumManifestUpload(ctx context.Context, req AlbumManifestUploadRequest) error {
	existing, err := s.albumRepo.FindByAlbumUID(ctx, req.ProviderID, req.DatabaseID, req.AlbumUID)
	if err != nil {
		return err
	}

	now := s.clock.Now()

	if existing == nil {
		album := &domain.Album{
			ProviderID: req.ProviderID,
			DatabaseID: req.DatabaseID,
			UserID:     req.UserID,
			AlbumUID:   req.AlbumUID,
			Synced:     true,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if err := s.albumRepo.Create(ctx, album); err != nil {
			return err
		}

		if err := s.storeManifest(ctx, req); err != nil {
			return err
		}

		return s.emitVideoUpload(ctx, req)
	}

	if existing.UserID != req.UserID {
		return ErrUserIDMismatch
	}

	currentVideos, err := s.albumVideoRepo.FindByAlbumUID(ctx, req.ProviderID, req.DatabaseID, req.AlbumUID)
	if err != nil {
		return err
	}

	existing.Synced = true // manifest sync status
	existing.UpdatedAt = now

	if s.manifestsEqual(currentVideos, req.VideoUIDs) {
		return s.albumRepo.Update(ctx, existing)
	}

	if err := s.storeManifest(ctx, req); err != nil {
		return err
	}

	if err := s.albumRepo.Update(ctx, existing); err != nil {
		return err
	}

	return s.emitVideoUpload(ctx, req)
}

func (s *AlbumManifestUploadService) storeManifest(ctx context.Context, req AlbumManifestUploadRequest) error {
	videos := make([]domain.AlbumVideo, len(req.VideoUIDs))
	for i, videoUID := range req.VideoUIDs {
		videos[i] = domain.AlbumVideo{
			ProviderID: req.ProviderID,
			DatabaseID: req.DatabaseID,
			AlbumUID:   req.AlbumUID,
			VideoUID:   videoUID,
		}
	}
	return s.albumVideoRepo.ReplaceForAlbum(ctx, req.ProviderID, req.DatabaseID, req.AlbumUID, videos)
}

func (s *AlbumManifestUploadService) manifestsEqual(current []domain.AlbumVideo, newUIDs []string) bool {
	if len(current) != len(newUIDs) {
		return false
	}

	currentUIDs := make([]string, len(current))
	for i, vid := range current {
		currentUIDs[i] = vid.VideoUID
	}
	sort.Strings(currentUIDs)

	sortedNew := make([]string, len(newUIDs))
	copy(sortedNew, newUIDs)
	sort.Strings(sortedNew)

	for i := range currentUIDs {
		if currentUIDs[i] != sortedNew[i] {
			return false
		}
	}
	return true
}

func (s *AlbumManifestUploadService) emitVideoUpload(ctx context.Context, req AlbumManifestUploadRequest) error {
	payload, err := json.Marshal(VideoUploadPayload{
		DatabaseID: req.DatabaseID,
		AlbumUID:   req.AlbumUID,
	})
	if err != nil {
		return err
	}

	return s.queue.Publish(ctx, Message{
		Topic:   "videoupload",
		Payload: payload,
		Metadata: map[string]string{
			"providerID": req.ProviderID,
		},
	})
}
