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
	"github.com/media-vault-sync/internal/core/services"
)

func TestAlbumManifestUpload_Idempotency(t *testing.T) {
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
									{AlbumUID: "album1", Videos: []string{"v1", "v2"}},
								},
							},
						},
					},
				},
			},
		},
	}
	writeConfig := func(cfg mediavault.Config) {
		data, _ := json.Marshal(cfg)
		os.WriteFile(configPath, data, 0644)
	}
	writeConfig(mediaVaultConfig)

	queue := memory.NewInMemoryQueue(clock)
	albumRepo := memoryrepo.NewAlbumRepository()
	albumVideoRepo := memoryrepo.NewAlbumVideoRepository()

	albumManifestUploadService := services.NewAlbumManifestUploadService(albumRepo, albumVideoRepo, queue, clock)
	albumManifestUploadHandler := cloud.NewAlbumManifestUploadHandler(albumManifestUploadService)

	mux := http.NewServeMux()
	mux.Handle("/v1/albummanifestupload", albumManifestUploadHandler)
	cloudServer := httptest.NewServer(mux)
	defer cloudServer.Close()

	cloudClient := onprem.NewHTTPCloudClient(cloudServer.URL, nil)
	mediaVaultRegistry := mediavault.NewFileSystemMediaVaultRegistry(configPath, nil)
	albumManifestUploadConsumer := services.NewAlbumManifestUploadConsumer("p1", mediaVaultRegistry, cloudClient, 1)

	queue.Subscribe(ctx, "onprem:p1", "albummanifestupload", "p1", albumManifestUploadConsumer.Handle)

	var videoUploadMessages []services.Message
	queue.Subscribe(ctx, "collector", "videoupload", "", func(ctx context.Context, msg services.Message) error {
		videoUploadMessages = append(videoUploadMessages, msg)
		return nil
	})

	albumManifestUploadPayload, _ := json.Marshal(services.AlbumManifestUploadPayload{
		DatabaseID: "db1",
		AlbumUID:   "album1",
	})
	queue.Publish(ctx, services.Message{
		MessageID: "upload-1",
		Topic:     "albummanifestupload",
		Payload:   albumManifestUploadPayload,
		Metadata:  map[string]string{"providerID": "p1"},
	})

	queue.Process(ctx)

	if len(videoUploadMessages) != 1 {
		t.Errorf("initial upload: expected 1 videoupload message, got %d", len(videoUploadMessages))
	}

	videoUploadMessages = nil

	queue.Publish(ctx, services.Message{
		MessageID: "upload-2",
		Topic:     "albummanifestupload",
		Payload:   albumManifestUploadPayload,
		Metadata:  map[string]string{"providerID": "p1"},
	})

	queue.Process(ctx)

	if len(videoUploadMessages) != 0 {
		t.Errorf("identical re-upload: expected 0 videoupload messages, got %d", len(videoUploadMessages))
	}

	mediaVaultConfig.Providers[0].Databases[0].Users[0].Albums[0].Videos = []string{"v1", "v2", "v3"}
	writeConfig(mediaVaultConfig)

	videoUploadMessages = nil

	queue.Publish(ctx, services.Message{
		MessageID: "upload-3",
		Topic:     "albummanifestupload",
		Payload:   albumManifestUploadPayload,
		Metadata:  map[string]string{"providerID": "p1"},
	})

	queue.Process(ctx)

	if len(videoUploadMessages) != 1 {
		t.Errorf("manifest changed: expected 1 videoupload message, got %d", len(videoUploadMessages))
	}

	album, _ := albumRepo.FindByAlbumUID(ctx, "p1", "db1", "album1")
	if album == nil {
		t.Fatal("album should exist")
	}
	if !album.Synced {
		t.Error("album should be synced after manifest update")
	}
}
