package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/media-vault-sync/internal/adapters/http/cloud"
	"github.com/media-vault-sync/internal/adapters/http/onprem"
	"github.com/media-vault-sync/internal/adapters/queue/memory"
	memoryrepo "github.com/media-vault-sync/internal/adapters/repo/memory"
	"github.com/media-vault-sync/internal/core/services"
)

func TestAlbumManifestUpload_UserImmutability(t *testing.T) {
	ctx := context.Background()
	clock := services.NewFakeClock(time.Now())

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

	err := cloudClient.PostAlbumManifestUpload(ctx, services.AlbumManifestUploadRequest{
		ProviderID: "p1",
		DatabaseID: "db1",
		UserID:     "user1",
		AlbumUID:   "album1",
		VideoUIDs:  []string{"v1", "v2"},
	})
	if err != nil {
		t.Fatalf("first upload failed: %v", err)
	}

	album, _ := albumRepo.FindByAlbumUID(ctx, "p1", "db1", "album1")
	if album == nil {
		t.Fatal("album should exist after first upload")
	}
	if album.UserID != "user1" {
		t.Errorf("expected userID user1, got %s", album.UserID)
	}

	err = cloudClient.PostAlbumManifestUpload(ctx, services.AlbumManifestUploadRequest{
		ProviderID: "p1",
		DatabaseID: "db1",
		UserID:     "user2",
		AlbumUID:   "album1",
		VideoUIDs:  []string{"v1", "v2"},
	})

	if err == nil {
		t.Error("expected error when changing userID, got nil")
	}

	album, _ = albumRepo.FindByAlbumUID(ctx, "p1", "db1", "album1")
	if album.UserID != "user1" {
		t.Errorf("userID should remain user1 after rejected update, got %s", album.UserID)
	}
}
