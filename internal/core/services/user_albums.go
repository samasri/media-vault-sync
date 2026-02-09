package services

import (
	"context"
	"encoding/json"
)

type UserAlbumsRequest struct {
	ProviderID string   `json:"providerID"`
	DatabaseID string   `json:"databaseID"`
	UserID     string   `json:"userID"`
	AlbumUIDs  []string `json:"albumUIDs"`
}

type AlbumManifestUploadPayload struct {
	DatabaseID string `json:"databaseID"`
	AlbumUID   string `json:"albumUID"`
}

type UserAlbumsService struct {
	albumRepo AlbumRepository
	queue     Queue
}

func NewUserAlbumsService(albumRepo AlbumRepository, queue Queue) *UserAlbumsService {
	return &UserAlbumsService{
		albumRepo: albumRepo,
		queue:     queue,
	}
}

func (s *UserAlbumsService) ProcessUserAlbums(ctx context.Context, req UserAlbumsRequest) error {
	for _, albumUID := range req.AlbumUIDs {
		existing, err := s.albumRepo.FindByAlbumUID(ctx, req.ProviderID, req.DatabaseID, albumUID)
		if err != nil {
			return err
		}

		if existing != nil {
			// existing albums are already processed, no need to reprocess them
			continue
		}

		payload, err := json.Marshal(AlbumManifestUploadPayload{
			DatabaseID: req.DatabaseID,
			AlbumUID:   albumUID,
		})
		if err != nil {
			return err
		}

		err = s.queue.Publish(ctx, Message{
			Topic:   "albummanifestupload",
			Payload: payload,
			Metadata: map[string]string{
				"providerID": req.ProviderID,
			},
		})
		if err != nil {
			return err
		}
	}

	return nil
}
