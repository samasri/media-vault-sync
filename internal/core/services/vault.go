package services

import "context"

type MediaVault interface {
	ListAlbumUIDs(ctx context.Context, userID string) ([]string, error)
	ListVideoUIDs(ctx context.Context, albumUID string) ([]string, error)
	GetUserIDForAlbum(ctx context.Context, albumUID string) (string, error)
	CMove(ctx context.Context, albumUID string) error
}

type MediaVaultRegistry interface {
	Get(databaseID string) (MediaVault, error)
}
