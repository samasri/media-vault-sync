package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/media-vault-sync/internal/core/domain"
)

var ErrVideoNotInManifest = errors.New("video not in manifest")

type VideoUploadRequest struct {
	ProviderID string `json:"providerID"`
	DatabaseID string `json:"databaseID"`
	UserID     string `json:"userID"`
	AlbumUID   string `json:"albumUID"`
	VideoUID   string `json:"videoUID"`
	Data       []byte `json:"-"`
}

type VideoUploadService struct {
	albumRepo      AlbumRepository
	albumVideoRepo AlbumVideoRepository
	videoRepo      VideoRepository
	objectRepo     ObjectRepository
	clock          Clock
}

func NewVideoUploadService(
	albumRepo AlbumRepository,
	albumVideoRepo AlbumVideoRepository,
	videoRepo VideoRepository,
	objectRepo ObjectRepository,
	clock Clock,
) *VideoUploadService {
	return &VideoUploadService{
		albumRepo:      albumRepo,
		albumVideoRepo: albumVideoRepo,
		videoRepo:      videoRepo,
		objectRepo:     objectRepo,
		clock:          clock,
	}
}

func (s *VideoUploadService) ProcessVideoUpload(ctx context.Context, req VideoUploadRequest) error {
	inManifest, err := s.albumVideoRepo.Exists(ctx, req.ProviderID, req.DatabaseID, req.AlbumUID, req.VideoUID)
	if err != nil {
		return err
	}

	if !inManifest {
		album, err := s.albumRepo.FindByAlbumUID(ctx, req.ProviderID, req.DatabaseID, req.AlbumUID)
		if err != nil {
			return err
		}
		if album != nil {
			album.Synced = false
			album.UpdatedAt = s.clock.Now()
			if err := s.albumRepo.Update(ctx, album); err != nil {
				return err
			}
		}
		return ErrVideoNotInManifest
	}

	now := s.clock.Now()

	// this entity includes more metadata, like the userID
	video := &domain.Video{
		UID:        fmt.Sprintf("%s-%s-%s", req.ProviderID, req.DatabaseID, req.VideoUID),
		ProviderID: req.ProviderID,
		DatabaseID: req.DatabaseID,
		UserID:     req.UserID,
		VideoUID:   req.VideoUID,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.videoRepo.Upsert(ctx, video); err != nil {
		return err
	}

	// hashing can be expensive if we have too many big files
	hash := sha256.Sum256(req.Data)
	checksum := hex.EncodeToString(hash[:])
	storageKey := fmt.Sprintf("objects/%s/%s/%s", req.ProviderID, req.DatabaseID, req.VideoUID)
	// TODO: upload the data to a Blob Storage and bind it to the storageKey
	// If the key changed, we should also delete the previous key before uploading the new one
	// Enhancement: make the object versioned for auditability

	// this entity includes storage details like the storageKey
	object := &domain.Object{
		UID:        fmt.Sprintf("%s-%s-%s-obj", req.ProviderID, req.DatabaseID, req.VideoUID),
		ProviderID: req.ProviderID,
		DatabaseID: req.DatabaseID,
		VideoUID:   req.VideoUID,
		StorageKey: storageKey,
		SizeBytes:  int64(len(req.Data)),
		Checksum:   checksum,
		CreatedAt:  now,
	}
	return s.objectRepo.Upsert(ctx, object)
}
