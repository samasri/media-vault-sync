package services

import "context"

type CloudClient interface {
	PostUserAlbums(ctx context.Context, req UserAlbumsRequest) error
	PostAlbumManifestUpload(ctx context.Context, req AlbumManifestUploadRequest) error
	PostVideoUpload(ctx context.Context, req VideoUploadRequest) error
}
