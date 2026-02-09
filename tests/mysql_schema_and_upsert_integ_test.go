package tests

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/media-vault-sync/internal/adapters/repo/mysql"
	"github.com/media-vault-sync/internal/core/domain"
)

func TestMySQL_SchemaAndUpsert(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		t.Skip("MYSQL_DSN not set, skipping integration test")
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("failed to connect to MySQL: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Fatalf("failed to ping MySQL: %v", err)
	}

	ctx := context.Background()

	runMigrations(t, db)
	cleanupTables(t, db)

	t.Run("album upsert uniqueness", func(t *testing.T) {
		albumRepo := mysql.NewAlbumRepository(db)

		album := &domain.Album{
			UID:        "album-uid-1",
			ProviderID: "p1",
			DatabaseID: "db1",
			UserID:     "user1",
			AlbumUID:   "album1",
			Synced:     false,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		err := albumRepo.Create(ctx, album)
		if err != nil {
			t.Fatalf("failed to create album: %v", err)
		}

		found, err := albumRepo.FindByAlbumUID(ctx, "p1", "db1", "album1")
		if err != nil {
			t.Fatalf("failed to find album: %v", err)
		}
		if found == nil {
			t.Fatal("album should exist")
		}
		if found.Synced {
			t.Error("album should not be synced")
		}

		found.Synced = true
		found.UpdatedAt = time.Now()
		err = albumRepo.Update(ctx, found)
		if err != nil {
			t.Fatalf("failed to update album: %v", err)
		}

		found, err = albumRepo.FindByAlbumUID(ctx, "p1", "db1", "album1")
		if err != nil {
			t.Fatalf("failed to find album after update: %v", err)
		}
		if !found.Synced {
			t.Error("album should be synced after update")
		}

		duplicateAlbum := &domain.Album{
			UID:        "album-uid-2",
			ProviderID: "p1",
			DatabaseID: "db1",
			UserID:     "user1",
			AlbumUID:   "album1",
			Synced:     false,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		err = albumRepo.Create(ctx, duplicateAlbum)
		if err == nil {
			t.Error("creating duplicate album should fail due to unique constraint")
		}
	})

	cleanupTables(t, db)

	t.Run("manifest membership updates correctly", func(t *testing.T) {
		albumVideoRepo := mysql.NewAlbumVideoRepository(db)

		videos := []domain.AlbumVideo{
			{ProviderID: "p1", DatabaseID: "db1", AlbumUID: "album1", VideoUID: "v1"},
			{ProviderID: "p1", DatabaseID: "db1", AlbumUID: "album1", VideoUID: "v2"},
		}
		err := albumVideoRepo.ReplaceForAlbum(ctx, "p1", "db1", "album1", videos)
		if err != nil {
			t.Fatalf("failed to replace album videos: %v", err)
		}

		found, err := albumVideoRepo.FindByAlbumUID(ctx, "p1", "db1", "album1")
		if err != nil {
			t.Fatalf("failed to find album videos: %v", err)
		}
		if len(found) != 2 {
			t.Fatalf("expected 2 videos, got %d", len(found))
		}

		exists, err := albumVideoRepo.Exists(ctx, "p1", "db1", "album1", "v1")
		if err != nil {
			t.Fatalf("failed to check exists: %v", err)
		}
		if !exists {
			t.Error("v1 should exist in manifest")
		}

		newVideos := []domain.AlbumVideo{
			{ProviderID: "p1", DatabaseID: "db1", AlbumUID: "album1", VideoUID: "v1"},
			{ProviderID: "p1", DatabaseID: "db1", AlbumUID: "album1", VideoUID: "v2"},
			{ProviderID: "p1", DatabaseID: "db1", AlbumUID: "album1", VideoUID: "v3"},
		}
		err = albumVideoRepo.ReplaceForAlbum(ctx, "p1", "db1", "album1", newVideos)
		if err != nil {
			t.Fatalf("failed to replace album videos: %v", err)
		}

		found, err = albumVideoRepo.FindByAlbumUID(ctx, "p1", "db1", "album1")
		if err != nil {
			t.Fatalf("failed to find album videos after update: %v", err)
		}
		if len(found) != 3 {
			t.Fatalf("expected 3 videos after update, got %d", len(found))
		}

		exists, err = albumVideoRepo.Exists(ctx, "p1", "db1", "album1", "v3")
		if err != nil {
			t.Fatalf("failed to check exists for v3: %v", err)
		}
		if !exists {
			t.Error("v3 should exist in manifest after update")
		}
	})

	cleanupTables(t, db)

	t.Run("video insert is idempotent", func(t *testing.T) {
		videoRepo := mysql.NewVideoRepository(db)

		video := &domain.Video{
			UID:        "vid-uid-1",
			ProviderID: "p1",
			DatabaseID: "db1",
			UserID:     "user1",
			VideoUID:   "v1",
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		err := videoRepo.Upsert(ctx, video)
		if err != nil {
			t.Fatalf("failed to insert video: %v", err)
		}

		found, err := videoRepo.FindByVideoUID(ctx, "p1", "db1", "v1")
		if err != nil {
			t.Fatalf("failed to find video: %v", err)
		}
		if found == nil {
			t.Fatal("video should exist")
		}
		if found.UserID != "user1" {
			t.Errorf("expected user ID user1, got %s", found.UserID)
		}

		video.UserID = "user2"
		video.UpdatedAt = time.Now()
		err = videoRepo.Upsert(ctx, video)
		if err != nil {
			t.Fatalf("failed to upsert video: %v", err)
		}

		found, err = videoRepo.FindByVideoUID(ctx, "p1", "db1", "v1")
		if err != nil {
			t.Fatalf("failed to find video after upsert: %v", err)
		}
		if found.UserID != "user2" {
			t.Errorf("expected user ID user2 after upsert, got %s", found.UserID)
		}
	})

	cleanupTables(t, db)

	t.Run("object insert is idempotent", func(t *testing.T) {
		objectRepo := mysql.NewObjectRepository(db)

		object := &domain.Object{
			UID:        "obj-uid-1",
			ProviderID: "p1",
			DatabaseID: "db1",
			VideoUID:   "v1",
			StorageKey: "key1",
			SizeBytes:  1024,
			Checksum:   "abc123",
			CreatedAt:  time.Now(),
		}

		err := objectRepo.Upsert(ctx, object)
		if err != nil {
			t.Fatalf("failed to insert object: %v", err)
		}

		found, err := objectRepo.FindByVideoUID(ctx, "p1", "db1", "v1")
		if err != nil {
			t.Fatalf("failed to find object: %v", err)
		}
		if found == nil {
			t.Fatal("object should exist")
		}
		if found.Checksum != "abc123" {
			t.Errorf("expected checksum abc123, got %s", found.Checksum)
		}

		object.Checksum = "def456"
		object.SizeBytes = 2048
		err = objectRepo.Upsert(ctx, object)
		if err != nil {
			t.Fatalf("failed to upsert object: %v", err)
		}

		found, err = objectRepo.FindByVideoUID(ctx, "p1", "db1", "v1")
		if err != nil {
			t.Fatalf("failed to find object after upsert: %v", err)
		}
		if found.Checksum != "def456" {
			t.Errorf("expected checksum def456 after upsert, got %s", found.Checksum)
		}
		if found.SizeBytes != 2048 {
			t.Errorf("expected size 2048 after upsert, got %d", found.SizeBytes)
		}
	})
}

func runMigrations(t *testing.T, db *sql.DB) {
	t.Helper()

	migrations := []string{
		`CREATE TABLE IF NOT EXISTS albums (
			uid VARCHAR(255) NOT NULL PRIMARY KEY,
			provider_id VARCHAR(255) NOT NULL,
			database_id VARCHAR(255) NOT NULL,
			user_id VARCHAR(255) NOT NULL,
			album_uid VARCHAR(255) NOT NULL,
			synced BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_album (provider_id, database_id, album_uid)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE IF NOT EXISTS album_videos (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			provider_id VARCHAR(255) NOT NULL,
			database_id VARCHAR(255) NOT NULL,
			album_uid VARCHAR(255) NOT NULL,
			video_uid VARCHAR(255) NOT NULL,
			UNIQUE KEY uk_album_video (provider_id, database_id, album_uid, video_uid),
			INDEX idx_album_lookup (provider_id, database_id, album_uid)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE IF NOT EXISTS videos (
			uid VARCHAR(255) NOT NULL PRIMARY KEY,
			provider_id VARCHAR(255) NOT NULL,
			database_id VARCHAR(255) NOT NULL,
			user_id VARCHAR(255) NOT NULL,
			video_uid VARCHAR(255) NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uk_video (provider_id, database_id, video_uid)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,

		`CREATE TABLE IF NOT EXISTS objects (
			uid VARCHAR(255) NOT NULL PRIMARY KEY,
			provider_id VARCHAR(255) NOT NULL,
			database_id VARCHAR(255) NOT NULL,
			video_uid VARCHAR(255) NOT NULL,
			storage_key VARCHAR(1024) NOT NULL,
			size_bytes BIGINT NOT NULL,
			checksum VARCHAR(255) NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE KEY uk_object (provider_id, database_id, video_uid)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}

	for _, m := range migrations {
		_, err := db.Exec(m)
		if err != nil {
			t.Fatalf("failed to run migration: %v", err)
		}
	}
}

func cleanupTables(t *testing.T, db *sql.DB) {
	t.Helper()

	tables := []string{"objects", "videos", "album_videos", "albums"}
	for _, table := range tables {
		_, err := db.Exec("DELETE FROM " + table)
		if err != nil {
			t.Fatalf("failed to clean table %s: %v", table, err)
		}
	}
}
