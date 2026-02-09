package services

import (
	"context"
	"encoding/json"
	"fmt"
)

type VideoUploadConsumer struct {
	mediaVaultRegistry MediaVaultRegistry
}

func NewVideoUploadConsumer(mediaVaultRegistry MediaVaultRegistry) *VideoUploadConsumer {
	return &VideoUploadConsumer{mediaVaultRegistry: mediaVaultRegistry}
}

func (c *VideoUploadConsumer) Handle(ctx context.Context, msg Message) error {
	var payload VideoUploadPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshaling payload: %w", err)
	}

	mediaVault, err := c.mediaVaultRegistry.Get(payload.DatabaseID)
	if err != nil {
		return fmt.Errorf("getting MediaVault for database %s: %w", payload.DatabaseID, err)
	}

	return mediaVault.CMove(ctx, payload.AlbumUID)
}
