package mediavault

import (
	"sync"

	"github.com/media-vault-sync/internal/core/services"
)

type FileSystemMediaVaultRegistry struct {
	mu         sync.RWMutex
	vaults     map[string]services.MediaVault
	configPath string
	sender     VideoSender
}

func NewFileSystemMediaVaultRegistry(configPath string, sender VideoSender) *FileSystemMediaVaultRegistry {
	return &FileSystemMediaVaultRegistry{
		vaults:     make(map[string]services.MediaVault),
		configPath: configPath,
		sender:     sender,
	}
}

func (r *FileSystemMediaVaultRegistry) Get(databaseID string) (services.MediaVault, error) {
	r.mu.RLock()
	if vault, ok := r.vaults[databaseID]; ok {
		r.mu.RUnlock()
		return vault, nil
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	if vault, ok := r.vaults[databaseID]; ok {
		return vault, nil
	}

	vault := NewDatabaseScopedMediaVault(r.configPath, databaseID, r.sender)
	r.vaults[databaseID] = vault
	return vault, nil
}
