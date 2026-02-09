package cloud

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/media-vault-sync/internal/core/services"
)

type VideoUploadHandler struct {
	service *services.VideoUploadService
}

func NewVideoUploadHandler(service *services.VideoUploadService) *VideoUploadHandler {
	return &VideoUploadHandler{service: service}
}

type videoUploadHTTPRequest struct {
	ProviderID string `json:"providerID"`
	DatabaseID string `json:"databaseID"`
	UserID     string `json:"userID"`
	VideoUID   string `json:"videoUID"`
}

func (h *VideoUploadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := r.URL.Path
	prefix := "/v1/album/"
	suffix := "/videoupload"

	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	albumUID := strings.TrimPrefix(path, prefix)
	albumUID = strings.TrimSuffix(albumUID, suffix)
	if albumUID == "" {
		http.Error(w, "missing albumUID", http.StatusBadRequest)
		return
	}

	contentType := r.Header.Get("Content-Type")
	if contentType == "application/octet-stream" {
		h.handleBinary(w, r, albumUID)
		return
	}
	if strings.HasPrefix(contentType, "multipart/form-data") {
		h.handleMultipart(w, r, albumUID)
		return
	}

	h.handleJSON(w, r, albumUID)
}

func (h *VideoUploadHandler) handleBinary(w http.ResponseWriter, r *http.Request, albumUID string) {
	providerID := r.Header.Get("X-Provider-ID")
	databaseID := r.Header.Get("X-Database-ID")
	userID := r.Header.Get("X-User-ID")
	videoUID := r.Header.Get("X-Video-UID")

	if providerID == "" || databaseID == "" || userID == "" || videoUID == "" {
		http.Error(w, "missing required headers", http.StatusBadRequest)
		return
	}

	// storing the whole file in memory can crash the service
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	err = h.service.ProcessVideoUpload(r.Context(), services.VideoUploadRequest{
		ProviderID: providerID,
		DatabaseID: databaseID,
		UserID:     userID,
		AlbumUID:   albumUID,
		VideoUID:   videoUID,
		Data:       data,
	})

	if errors.Is(err, services.ErrVideoNotInManifest) {
		http.Error(w, "video not in manifest", http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *VideoUploadHandler) handleJSON(w http.ResponseWriter, r *http.Request, albumUID string) {
	var req videoUploadHTTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	data := r.Header.Get("X-Video-Data")
	var dataBytes []byte
	if data != "" {
		dataBytes = []byte(data)
	}

	err := h.service.ProcessVideoUpload(r.Context(), services.VideoUploadRequest{
		ProviderID: req.ProviderID,
		DatabaseID: req.DatabaseID,
		UserID:     req.UserID,
		AlbumUID:   albumUID,
		VideoUID:   req.VideoUID,
		Data:       dataBytes,
	})

	if errors.Is(err, services.ErrVideoNotInManifest) {
		http.Error(w, "video not in manifest", http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *VideoUploadHandler) handleMultipart(w http.ResponseWriter, r *http.Request, albumUID string) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "failed to parse multipart form", http.StatusBadRequest)
		return
	}

	providerID := r.FormValue("providerID")
	databaseID := r.FormValue("databaseID")
	userID := r.FormValue("userID")
	videoUID := r.FormValue("videoUID")

	var data []byte
	file, _, err := r.FormFile("data")
	if err == nil {
		defer file.Close()
		data, _ = io.ReadAll(file)
	}

	err = h.service.ProcessVideoUpload(r.Context(), services.VideoUploadRequest{
		ProviderID: providerID,
		DatabaseID: databaseID,
		UserID:     userID,
		AlbumUID:   albumUID,
		VideoUID:   videoUID,
		Data:       data,
	})

	if errors.Is(err, services.ErrVideoNotInManifest) {
		http.Error(w, "video not in manifest", http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
