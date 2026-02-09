package tests

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	cloudapp "github.com/media-vault-sync/internal/app/cloud"
	onpremapp "github.com/media-vault-sync/internal/app/onprem"
	"github.com/media-vault-sync/internal/adapters/http/onprem"
	"github.com/media-vault-sync/internal/adapters/mediavault"
	"github.com/media-vault-sync/internal/adapters/queue/memory"
	"github.com/media-vault-sync/internal/adapters/storage/fs"
	"github.com/media-vault-sync/internal/core/services"
)

func TestWiring_EndToEnd_VideoIngest(t *testing.T) {
	ctx := context.Background()
	clock := services.NewFakeClock(time.Now())

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "mediavault_config.json")
	stagingPath := filepath.Join(tmpDir, "staging")

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
	data, _ := json.Marshal(mediaVaultConfig)
	os.WriteFile(configPath, data, 0644)

	queue := memory.NewInMemoryQueue(clock)

	cloudCfg := cloudapp.Config{}
	cloudOpts := &cloudapp.WireOptions{
		Clock: clock,
		Queue: queue,
	}
	cloud := cloudapp.Wire(cloudCfg, cloudOpts)

	cloudServer := httptest.NewServer(cloud.Handler)
	defer cloudServer.Close()

	cloudClient := onprem.NewHTTPCloudClient(cloudServer.URL, nil)
	stagingStorage := fs.NewStagingStorage(stagingPath)

	var onpremServer *httptest.Server
	var mediaVaultRegistry *mediavault.FileSystemMediaVaultRegistry

	mediaVaultRegistryProxy := &deferredMediaVaultRegistry{getRegistry: func() services.MediaVaultRegistry { return mediaVaultRegistry }}

	onpremCfg := onpremapp.Config{
		MediaVaultConfigPath: configPath,
		ProviderID:           "p1",
	}
	onpremOpts := &onpremapp.WireOptions{
		Clock:              clock,
		Queue:              queue,
		CloudClient:        cloudClient,
		StagingStorage:     stagingStorage,
		MediaVaultRegistry: mediaVaultRegistryProxy,
	}
	onpremApp := onpremapp.Wire(onpremCfg, onpremOpts)

	onpremServer = httptest.NewServer(onpremApp.Handler)
	defer onpremServer.Close()

	videoSender := onprem.NewHTTPVideoSender(onpremServer.URL, "p1", nil)
	mediaVaultRegistry = mediavault.NewFileSystemMediaVaultRegistry(configPath, videoSender)

	if err := cloud.SubscribeEventualConsistencyCheck(ctx); err != nil {
		t.Fatalf("failed to subscribe cloud: %v", err)
	}
	if err := onpremApp.SubscribeAll(ctx); err != nil {
		t.Fatalf("failed to subscribe onprem: %v", err)
	}

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

	for i := 0; i < 10; i++ {
		queue.Process(ctx)
	}

	album, err := cloud.AlbumRepo.FindByAlbumUID(ctx, "p1", "db1", "album1")
	if err != nil {
		t.Fatalf("failed to find album: %v", err)
	}
	if album == nil {
		t.Fatal("album should exist")
	}
	if !album.Synced {
		t.Error("album should be synced")
	}

	obj1, _ := cloud.ObjectRepo.FindByVideoUID(ctx, "p1", "db1", "v1")
	obj2, _ := cloud.ObjectRepo.FindByVideoUID(ctx, "p1", "db1", "v2")

	if obj1 == nil {
		t.Error("object for v1 should exist")
	}
	if obj2 == nil {
		t.Error("object for v2 should exist")
	}
}

type deferredMediaVaultRegistry struct {
	getRegistry func() services.MediaVaultRegistry
}

func (r *deferredMediaVaultRegistry) Get(databaseID string) (services.MediaVault, error) {
	return r.getRegistry().Get(databaseID)
}
