package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/media-vault-sync/internal/adapters/http/cloud"
	"github.com/media-vault-sync/internal/adapters/http/onprem"
	"github.com/media-vault-sync/internal/adapters/mediavault"
	"github.com/media-vault-sync/internal/adapters/queue/memory"
	memoryrepo "github.com/media-vault-sync/internal/adapters/repo/memory"
	"github.com/media-vault-sync/internal/adapters/storage/fs"
	"github.com/media-vault-sync/internal/core/services"
)

func TestRepairLoop_RecoversAfterConfigChange(t *testing.T) {
	ctx := context.Background()
	clock := services.NewFakeClock(time.Now())

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "mediavault_config.json")
	stagingPath := filepath.Join(tmpDir, "staging")

	writeConfig := func(videos []string) {
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
										{AlbumUID: "album1", Videos: videos},
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
	}

	writeConfig([]string{"v1"})

	queue := memory.NewInMemoryQueue(clock)
	albumRepo := memoryrepo.NewAlbumRepository()
	albumVideoRepo := memoryrepo.NewAlbumVideoRepository()
	videoRepo := memoryrepo.NewVideoRepository()
	objectRepo := memoryrepo.NewObjectRepository()

	userAlbumsService := services.NewUserAlbumsService(albumRepo, queue)
	userAlbumsHandler := cloud.NewUserAlbumsHandler(userAlbumsService)

	albumManifestUploadService := services.NewAlbumManifestUploadService(albumRepo, albumVideoRepo, queue, clock)
	albumManifestUploadHandler := cloud.NewAlbumManifestUploadHandler(albumManifestUploadService)

	videoUploadService := services.NewVideoUploadService(albumRepo, albumVideoRepo, videoRepo, objectRepo, clock)
	videoUploadHandler := cloud.NewVideoUploadHandler(videoUploadService)

	cloudMux := http.NewServeMux()
	cloudMux.Handle("/v1/useralbums", userAlbumsHandler)
	cloudMux.Handle("/v1/albummanifestupload", albumManifestUploadHandler)
	cloudMux.Handle("/v1/album/", videoUploadHandler)
	cloudServer := httptest.NewServer(cloudMux)
	defer cloudServer.Close()

	cloudClient := onprem.NewHTTPCloudClient(cloudServer.URL, nil)
	stagingStorage := fs.NewStagingStorage(stagingPath)

	var onpremReceiverServer *httptest.Server
	var mediaVaultRegistry *mediavault.FileSystemMediaVaultRegistry

	onpremMux := http.NewServeMux()
	mediaVaultRegistryProxy := &mediaVaultRegistryProxyForReceiver{
		getRegistry: func() services.MediaVaultRegistry { return mediaVaultRegistry },
	}
	onpremReceiver := onprem.NewVideoReceiver(stagingStorage, cloudClient, mediaVaultRegistryProxy, 1)
	onpremMux.Handle("/receive-video", onpremReceiver)
	onpremReceiverServer = httptest.NewServer(onpremMux)
	defer onpremReceiverServer.Close()

	videoSender := onprem.NewHTTPVideoSender(onpremReceiverServer.URL, "p1", nil)
	mediaVaultRegistry = mediavault.NewFileSystemMediaVaultRegistry(configPath, videoSender)

	syncUserConsumer := services.NewSyncUserConsumer("p1", mediaVaultRegistry, cloudClient, 1)
	albumManifestUploadConsumer := services.NewAlbumManifestUploadConsumer("p1", mediaVaultRegistry, cloudClient, 1)
	videoUploadConsumer := services.NewVideoUploadConsumer(mediaVaultRegistry)
	eventualConsistencyWorker := services.NewEventualConsistencyWorker(albumRepo, queue, clock)
	eventualConsistencyCheckConsumer := services.NewEventualConsistencyCheckConsumer(albumRepo, queue, clock)

	var firstAlbumManifestUpload int32
	wrappedAlbumManifestUploadConsumer := func(ctx context.Context, msg services.Message) error {
		err := albumManifestUploadConsumer.Handle(ctx, msg)
		count := atomic.AddInt32(&firstAlbumManifestUpload, 1)
		if count == 1 {
			writeConfig([]string{"v1", "v2"})
		}
		return err
	}

	queue.Subscribe(ctx, "onprem:p1:usersync", "usersync", "p1", syncUserConsumer.Handle)
	queue.Subscribe(ctx, "onprem:p1:albummanifestupload", "albummanifestupload", "p1", wrappedAlbumManifestUploadConsumer)
	queue.Subscribe(ctx, "onprem:p1:videoupload", "videoupload", "p1", videoUploadConsumer.Handle)
	queue.Subscribe(ctx, "cloud:syncconsistencycheck", "syncconsistencycheck", "", eventualConsistencyCheckConsumer.Handle)

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

	album, _ := albumRepo.FindByAlbumUID(ctx, "p1", "db1", "album1")
	if album == nil {
		t.Fatal("album should exist after initial sync")
	}
	if album.Synced {
		t.Fatal("album should be unsynced after v2 was rejected")
	}

	obj2, _ := objectRepo.FindByVideoUID(ctx, "p1", "db1", "v2")
	if obj2 != nil {
		t.Fatal("v2 should not be stored yet (it was rejected)")
	}

	err := eventualConsistencyWorker.Scan(ctx)
	if err != nil {
		t.Fatalf("worker scan failed: %v", err)
	}

	for i := 0; i < 10; i++ {
		queue.Process(ctx)
	}

	clock.Advance(2 * time.Second)

	for i := 0; i < 20; i++ {
		queue.Process(ctx)
	}

	album, _ = albumRepo.FindByAlbumUID(ctx, "p1", "db1", "album1")
	if album == nil {
		t.Fatal("album should still exist")
	}
	if !album.Synced {
		t.Error("album should be synced after repair loop")
	}

	obj1, _ := objectRepo.FindByVideoUID(ctx, "p1", "db1", "v1")
	obj2, _ = objectRepo.FindByVideoUID(ctx, "p1", "db1", "v2")

	if obj1 == nil {
		t.Error("object for v1 should exist")
	}
	if obj2 == nil {
		t.Error("object for v2 should exist after repair")
	}

	vid1, _ := videoRepo.FindByVideoUID(ctx, "p1", "db1", "v1")
	vid2, _ := videoRepo.FindByVideoUID(ctx, "p1", "db1", "v2")

	if vid1 == nil {
		t.Error("video metadata for v1 should exist")
	}
	if vid2 == nil {
		t.Error("video metadata for v2 should exist after repair")
	}
}
