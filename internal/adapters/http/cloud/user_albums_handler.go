package cloud

import (
	"encoding/json"
	"net/http"

	"github.com/media-vault-sync/internal/core/services"
)

type UserAlbumsHandler struct {
	service *services.UserAlbumsService
}

func NewUserAlbumsHandler(service *services.UserAlbumsService) *UserAlbumsHandler {
	return &UserAlbumsHandler{service: service}
}

func (h *UserAlbumsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req services.UserAlbumsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.service.ProcessUserAlbums(r.Context(), req); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
