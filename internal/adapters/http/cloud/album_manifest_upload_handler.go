package cloud

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/media-vault-sync/internal/core/services"
)

type AlbumManifestUploadHandler struct {
	service *services.AlbumManifestUploadService
}

func NewAlbumManifestUploadHandler(service *services.AlbumManifestUploadService) *AlbumManifestUploadHandler {
	return &AlbumManifestUploadHandler{service: service}
}

func (h *AlbumManifestUploadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req services.AlbumManifestUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.service.ProcessAlbumManifestUpload(r.Context(), req); err != nil {
		if errors.Is(err, services.ErrUserIDMismatch) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
