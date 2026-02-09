package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type AlbumManifestUploadConsumer struct {
	providerID         string
	mediaVaultRegistry MediaVaultRegistry
	cloudClient        CloudClient
	maxRetries         int
}

func NewAlbumManifestUploadConsumer(providerID string, mediaVaultRegistry MediaVaultRegistry, cloudClient CloudClient, maxRetries int) *AlbumManifestUploadConsumer {
	if maxRetries <= 0 {
		maxRetries = 3
	}
	return &AlbumManifestUploadConsumer{
		providerID:         providerID,
		mediaVaultRegistry: mediaVaultRegistry,
		cloudClient:        cloudClient,
		maxRetries:         maxRetries,
	}
}

func (c *AlbumManifestUploadConsumer) Handle(ctx context.Context, msg Message) error {
	var payload AlbumManifestUploadPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return fmt.Errorf("parsing albummanifestupload payload: %w", err)
	}

	mediaVault, err := c.mediaVaultRegistry.Get(payload.DatabaseID)
	if err != nil {
		return fmt.Errorf("getting MediaVault for database %s: %w", payload.DatabaseID, err)
	}

	videoUIDs, err := mediaVault.ListVideoUIDs(ctx, payload.AlbumUID)
	if err != nil {
		return fmt.Errorf("listing video UIDs: %w", err)
	}

	userID, err := mediaVault.GetUserIDForAlbum(ctx, payload.AlbumUID)
	if err != nil {
		return fmt.Errorf("getting user ID: %w", err)
	}

	for attempt := 0; attempt < c.maxRetries; attempt++ {
		err = c.cloudClient.PostAlbumManifestUpload(ctx, AlbumManifestUploadRequest{
			ProviderID: c.providerID,
			DatabaseID: payload.DatabaseID,
			UserID:     userID,
			AlbumUID:   payload.AlbumUID,
			VideoUIDs:  videoUIDs,
		})
		if err == nil {
			break
		}
		if attempt == c.maxRetries-1 {
			return fmt.Errorf("posting album manifest upload after %d attempts: %w", c.maxRetries, err)
		}
		time.Sleep(time.Duration(8<<attempt) * time.Second)
	}

	return nil
}
