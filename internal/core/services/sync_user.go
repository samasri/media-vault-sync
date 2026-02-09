package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type SyncUserPayload struct {
	DatabaseID string `json:"databaseID"`
	UserID     string `json:"userID"`
}

type SyncUserConsumer struct {
	providerID         string
	mediaVaultRegistry MediaVaultRegistry
	cloudClient        CloudClient
	maxRetries         int
}

func NewSyncUserConsumer(providerID string, mediaVaultRegistry MediaVaultRegistry, cloudClient CloudClient, maxRetries int) *SyncUserConsumer {
	if maxRetries <= 0 {
		maxRetries = 3
	}
	return &SyncUserConsumer{
		providerID:         providerID,
		mediaVaultRegistry: mediaVaultRegistry,
		cloudClient:        cloudClient,
		maxRetries:         maxRetries,
	}
}

func (c *SyncUserConsumer) Handle(ctx context.Context, msg Message) error {
	var payload SyncUserPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return fmt.Errorf("parsing usersync payload: %w", err)
	}

	mediaVault, err := c.mediaVaultRegistry.Get(payload.DatabaseID)
	if err != nil {
		return fmt.Errorf("getting MediaVault for database %s: %w", payload.DatabaseID, err)
	}

	albumUIDs, err := mediaVault.ListAlbumUIDs(ctx, payload.UserID)
	if err != nil {
		return fmt.Errorf("listing album UIDs: %w", err)
	}

	if len(albumUIDs) == 0 {
		fmt.Println("Warning: no albums found for user with id=", payload.UserID)
		return nil
	}

	for attempt := 0; attempt < c.maxRetries; attempt++ {
		err = c.cloudClient.PostUserAlbums(ctx, UserAlbumsRequest{
			ProviderID: c.providerID,
			DatabaseID: payload.DatabaseID,
			UserID:     payload.UserID,
			AlbumUIDs:  albumUIDs,
		})
		if err == nil {
			break
		}
		if attempt == c.maxRetries-1 {
			return fmt.Errorf("posting user albums after %d attempts: %w", c.maxRetries, err)
		}
		time.Sleep(time.Duration(8<<attempt) * time.Second)
	}

	return nil
}
