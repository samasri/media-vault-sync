package onprem

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/media-vault-sync/internal/core/services"
)

type VideoReceiver struct {
	staging            services.StagingStorage
	cloudClient        services.CloudClient
	mediaVaultRegistry services.MediaVaultRegistry
	maxRetries         int
}

func NewVideoReceiver(staging services.StagingStorage, cloudClient services.CloudClient, mediaVaultRegistry services.MediaVaultRegistry, maxRetries int) *VideoReceiver {
	if maxRetries <= 0 {
		maxRetries = 3
	}
	return &VideoReceiver{
		staging:            staging,
		cloudClient:        cloudClient,
		mediaVaultRegistry: mediaVaultRegistry,
		maxRetries:         maxRetries,
	}
}

func (h *VideoReceiver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	providerID := r.Header.Get("X-Provider-ID")
	databaseID := r.Header.Get("X-Database-ID")
	albumUID := r.Header.Get("X-Album-UID")
	videoUID := r.Header.Get("X-Video-UID")

	if providerID == "" || databaseID == "" || albumUID == "" || videoUID == "" {
		http.Error(w, "missing required headers", http.StatusBadRequest)
		return
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	stagingKey := fmt.Sprintf("%s/%s/%s/%s", providerID, databaseID, albumUID, videoUID)
	// saving on disk to store and be able to retry in case of an error
	if err := h.staging.Store(ctx, stagingKey, data); err != nil {
		http.Error(w, fmt.Sprintf("failed to store in staging: %v", err), http.StatusInternalServerError)
		return
	}
	data = nil

	mediaVault, err := h.mediaVaultRegistry.Get(databaseID)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get MediaVault for database: %v", err), http.StatusInternalServerError)
		return
	}

	userID, err := mediaVault.GetUserIDForAlbum(ctx, albumUID)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get userID: %v", err), http.StatusInternalServerError)
		return
	}

	data, err = h.staging.Load(ctx, stagingKey)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to load from staging: %v", err), http.StatusInternalServerError)
		return
	}

	// this sends the data as octet-stream and all metadata as headers
	for attempt := 0; attempt < h.maxRetries; attempt++ {
		err = h.cloudClient.PostVideoUpload(ctx, services.VideoUploadRequest{
			ProviderID: providerID,
			DatabaseID: databaseID,
			UserID:     userID,
			AlbumUID:   albumUID,
			VideoUID:   videoUID,
			Data:       data,
		})
		if err == nil {
			break
		}
		if attempt == h.maxRetries-1 {
			// this is effectively a memory leak (we never delete the file)
			http.Error(w, fmt.Sprintf("failed to upload to cloud after %d attempts: %v", h.maxRetries, err), http.StatusInternalServerError)
			return // returning the error here will cause the C-MOVE issued against the MediaVault to fail
		}
		time.Sleep(time.Duration(8<<attempt) * time.Second)
	}

	h.staging.Delete(ctx, stagingKey)

	w.WriteHeader(http.StatusOK)
}
