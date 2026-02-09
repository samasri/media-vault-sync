package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/media-vault-sync/internal/adapters/http/cloud"
	"github.com/media-vault-sync/internal/adapters/http/onprem"
	"github.com/media-vault-sync/internal/adapters/mediavault"
	"github.com/media-vault-sync/internal/adapters/queue/memory"
	memoryrepo "github.com/media-vault-sync/internal/adapters/repo/memory"
	"github.com/media-vault-sync/internal/core/domain"
	"github.com/media-vault-sync/internal/core/services"
)

func TestUserSync_NewAlbumsOnly(t *testing.T) {
	ctx := context.Background()
	clock := services.NewFakeClock(time.Now())

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "mediavault_config.json")

	mediaVaultConfig := mediavault.Config{
		Providers: []mediavault.ProviderConfig{
			{
				ProviderID: "p1",
				Databases: []mediavault.DatabaseConfig{
					{
						DatabaseID: "db1",
						Users: []mediavault.UserConfig{
							{
								UserID: "user1",
								Albums: []mediavault.AlbumConfig{
									{AlbumUID: "album1", Videos: []string{"v1"}},
									{AlbumUID: "album2", Videos: []string{"v2"}},
								},
							},
						},
					},
				},
			},
		},
	}
	configData, _ := json.Marshal(mediaVaultConfig)
	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		t.Fatalf("writing mediavault config: %v", err)
	}

	queue := memory.NewInMemoryQueue(clock)
	albumRepo := memoryrepo.NewAlbumRepository()

	userAlbumsService := services.NewUserAlbumsService(albumRepo, queue)
	userAlbumsHandler := cloud.NewUserAlbumsHandler(userAlbumsService)

	mux := http.NewServeMux()
	mux.Handle("/v1/useralbums", userAlbumsHandler)
	cloudServer := httptest.NewServer(mux)
	defer cloudServer.Close()

	cloudClient := onprem.NewHTTPCloudClient(cloudServer.URL, nil)
	mediaVaultRegistry := mediavault.NewFileSystemMediaVaultRegistry(configPath, nil)
	syncUserConsumer := services.NewSyncUserConsumer("p1", mediaVaultRegistry, cloudClient, 1)

	queue.Subscribe(ctx, "onprem:p1", "usersync", "p1", syncUserConsumer.Handle)

	var albumManifestUploadMessages []services.Message
	queue.Subscribe(ctx, "collector", "albummanifestupload", "", func(ctx context.Context, msg services.Message) error {
		albumManifestUploadMessages = append(albumManifestUploadMessages, msg)
		var payload services.AlbumManifestUploadPayload
		json.Unmarshal(msg.Payload, &payload)
		providerID := msg.Metadata["providerID"]
		albumRepo.Create(ctx, &domain.Album{
			ProviderID: providerID,
			DatabaseID: payload.DatabaseID,
			AlbumUID:   payload.AlbumUID,
			UserID:     "user1",
			Synced:     true,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		})
		return nil
	})

	syncPayload, _ := json.Marshal(services.SyncUserPayload{
		DatabaseID: "db1",
		UserID:     "user1",
	})
	queue.Publish(ctx, services.Message{
		MessageID: "sync-1",
		Topic:     "usersync",
		Payload:   syncPayload,
		Metadata:  map[string]string{"providerID": "p1"},
	})

	queue.Process(ctx)

	if len(albumManifestUploadMessages) != 2 {
		t.Errorf("first sync: expected 2 albummanifestupload messages, got %d", len(albumManifestUploadMessages))
	}

	var albumUIDs []string
	for _, msg := range albumManifestUploadMessages {
		var payload services.AlbumManifestUploadPayload
		json.Unmarshal(msg.Payload, &payload)
		albumUIDs = append(albumUIDs, payload.AlbumUID)
	}

	hasAlbum1 := false
	hasAlbum2 := false
	for _, uid := range albumUIDs {
		if uid == "album1" {
			hasAlbum1 = true
		}
		if uid == "album2" {
			hasAlbum2 = true
		}
	}
	if !hasAlbum1 || !hasAlbum2 {
		t.Errorf("expected albummanifestupload for album1 and album2, got %v", albumUIDs)
	}

	albumManifestUploadMessages = nil

	queue.Publish(ctx, services.Message{
		MessageID: "sync-2",
		Topic:     "usersync",
		Payload:   syncPayload,
		Metadata:  map[string]string{"providerID": "p1"},
	})

	queue.Process(ctx)

	if len(albumManifestUploadMessages) != 0 {
		t.Errorf("second sync: expected 0 albummanifestupload messages (albums already exist), got %d", len(albumManifestUploadMessages))
	}
}

func TestUserSync_OnlyMatchingProviderReceives(t *testing.T) {
	ctx := context.Background()
	clock := services.NewFakeClock(time.Now())

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "mediavault_config.json")

	mediaVaultConfig := mediavault.Config{
		Providers: []mediavault.ProviderConfig{
			{
				ProviderID: "p1",
				Databases: []mediavault.DatabaseConfig{
					{
						DatabaseID: "db1",
						Users: []mediavault.UserConfig{
							{
								UserID: "user1",
								Albums: []mediavault.AlbumConfig{
									{AlbumUID: "album1", Videos: []string{"v1"}},
								},
							},
						},
					},
				},
			},
		},
	}
	configData, _ := json.Marshal(mediaVaultConfig)
	os.WriteFile(configPath, configData, 0644)

	queue := memory.NewInMemoryQueue(clock)
	albumRepo := memoryrepo.NewAlbumRepository()

	userAlbumsService := services.NewUserAlbumsService(albumRepo, queue)
	userAlbumsHandler := cloud.NewUserAlbumsHandler(userAlbumsService)

	mux := http.NewServeMux()
	mux.Handle("/v1/useralbums", userAlbumsHandler)
	cloudServer := httptest.NewServer(mux)
	defer cloudServer.Close()

	cloudClient := onprem.NewHTTPCloudClient(cloudServer.URL, nil)
	mediaVaultRegistry := mediavault.NewFileSystemMediaVaultRegistry(configPath, nil)
	syncUserConsumer := services.NewSyncUserConsumer("p1", mediaVaultRegistry, cloudClient, 1)

	var p1Received int
	queue.Subscribe(ctx, "onprem:p1", "usersync", "p1", func(ctx context.Context, msg services.Message) error {
		p1Received++
		return syncUserConsumer.Handle(ctx, msg)
	})

	var p2Received int
	queue.Subscribe(ctx, "onprem:p2", "usersync", "p2", func(ctx context.Context, msg services.Message) error {
		p2Received++
		return nil
	})

	syncPayload, _ := json.Marshal(services.SyncUserPayload{
		DatabaseID: "db1",
		UserID:     "user1",
	})
	queue.Publish(ctx, services.Message{
		MessageID: "sync-p2",
		Topic:     "usersync",
		Payload:   syncPayload,
		Metadata:  map[string]string{"providerID": "p2"},
	})

	queue.Process(ctx)

	if p1Received != 0 {
		t.Errorf("p1 should not receive p2's message, got %d", p1Received)
	}
	if p2Received != 1 {
		t.Errorf("p2 should receive its own message, got %d", p2Received)
	}
}
