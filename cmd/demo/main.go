package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"github.com/media-vault-sync/internal/adapters/http/onprem"
	"github.com/media-vault-sync/internal/adapters/mediavault"
	"github.com/media-vault-sync/internal/adapters/queue/memory"
	"github.com/media-vault-sync/internal/adapters/storage/fs"
	cloudapp "github.com/media-vault-sync/internal/app/cloud"
	onpremapp "github.com/media-vault-sync/internal/app/onprem"
	"github.com/media-vault-sync/internal/core/services"
)

const (
	colorReset   = "\033[0m"
	colorBold    = "\033[1m"
	colorDim     = "\033[2m"
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorBlue    = "\033[34m"
	colorMagenta = "\033[35m"
	colorCyan    = "\033[36m"
)

var (
	reader      = bufio.NewReader(os.Stdin)
	interactive = true
)

func main() {
	nonInteractive := flag.Bool("y", false, "Run non-interactively (skip prompts)")
	flag.Parse()
	interactive = !*nonInteractive

	tmpDir, err := os.MkdirTemp("", "demo-*")
	if err != nil {
		fmt.Printf("Failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	cleanup := func() {
		os.RemoveAll(tmpDir)
	}
	defer cleanup()

	exitCode := 0
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("\nDemo failed with panic: %v\n", r)
			cleanup()
			os.Exit(1)
		}
		os.Exit(exitCode)
	}()

	ctx := context.Background()
	clock := services.NewFakeClock(time.Now())

	configPath := filepath.Join(tmpDir, "mediavault_config.json")
	stagingPath := filepath.Join(tmpDir, "staging")

	printHeader("MEDIA SYNC ENGINE - INTERACTIVE DEMO")
	fmt.Println()
	fmt.Println("This demo simulates the full sync pipeline:")
	fmt.Printf("  %susersync%s → %salbummanifestupload%s → %svideoupload%s → %sstored in cloud%s\n",
		colorCyan, colorReset, colorCyan, colorReset, colorCyan, colorReset, colorGreen, colorReset)
	fmt.Println()
	waitForEnter()

	printHeader("STEP 0: Initial State")
	fmt.Println()
	fmt.Println("The MediaVault system has the following data (read from JSON config JIT):")
	fmt.Println()

	mediaVaultConfig := mediavault.Config{
		Providers: []mediavault.ProviderConfig{
			{
				ProviderID: "provider-1",
				Databases: []mediavault.DatabaseConfig{
					{
						DatabaseID: "database-1",
						Users: []mediavault.UserConfig{
							{
								UserID: "user-123",
								Albums: []mediavault.AlbumConfig{
									{AlbumUID: "album-CT-001", Videos: []string{"vid-001", "vid-002", "vid-003"}},
									{AlbumUID: "album-MRI-002", Videos: []string{"vid-004", "vid-005"}},
								},
							},
						},
					},
				},
			},
		},
	}

	fmt.Printf("  %sProvider:%s provider-1\n", colorBold, colorReset)
	fmt.Printf("  └── %sDatabase:%s database-1\n", colorBold, colorReset)
	fmt.Printf("      └── %sUser:%s user-123\n", colorBold, colorReset)
	fmt.Printf("          ├── %sAlbum:%s %salbum-CT-001%s\n", colorBold, colorReset, colorCyan, colorReset)
	fmt.Printf("          │   └── Videos: %s[vid-001, vid-002, vid-003]%s\n", colorYellow, colorReset)
	fmt.Printf("          └── %sAlbum:%s %salbum-MRI-002%s\n", colorBold, colorReset, colorCyan, colorReset)
	fmt.Printf("              └── Videos: %s[vid-004, vid-005]%s\n", colorYellow, colorReset)
	fmt.Println()

	data, _ := json.MarshalIndent(mediaVaultConfig, "", "  ")
	os.WriteFile(configPath, data, 0644)
	fmt.Printf("Config written to: %s%s%s\n", colorDim, configPath, colorReset)
	fmt.Println()
	waitForEnter()
	fmt.Println("Setting up the following components:")
	fmt.Printf("  %s•%s In-memory message queue (shared between cloud and on-prem)\n", colorGreen, colorReset)
	fmt.Printf("  %s•%s Cloud API server\n", colorGreen, colorReset)
	fmt.Printf("  %s•%s On-prem application\n", colorGreen, colorReset)
	fmt.Printf("  %s•%s MediaVault adapter (reads config JIT to return album shapes)\n", colorGreen, colorReset)
	fmt.Printf("  %s•%s In-memory repositories for albums, videos, objects\n", colorGreen, colorReset)
	fmt.Println()

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
		ProviderID:           "provider-1",
	}
	onpremOpts := &onpremapp.WireOptions{
		Clock:              clock,
		Queue:              queue,
		CloudClient:        cloudClient,
		StagingStorage:     stagingStorage,
		MediaVaultRegistry: mediaVaultRegistryProxy,
		MaxRetries:         1,
	}
	onpremApp := onpremapp.Wire(onpremCfg, onpremOpts)

	onpremServer = httptest.NewServer(onpremApp.Handler)
	defer onpremServer.Close()

	videoSender := onprem.NewHTTPVideoSender(onpremServer.URL, "provider-1", nil)
	mediaVaultRegistry = mediavault.NewFileSystemMediaVaultRegistry(configPath, videoSender)

	cloud.SubscribeEventualConsistencyCheck(ctx)
	onpremApp.SubscribeAll(ctx)

	fmt.Printf("  Cloud API running at:    %s%s%s\n", colorBlue, cloudServer.URL, colorReset)
	fmt.Printf("  On-prem application at:  %s%s%s\n", colorBlue, onpremServer.URL, colorReset)
	fmt.Println()
	fmt.Println("Subscriptions active:")
	fmt.Printf("  %s•%s on-prem subscribed to: %susersync%s, %salbummanifestupload%s, %svideoupload%s\n",
		colorGreen, colorReset, colorCyan, colorReset, colorCyan, colorReset, colorCyan, colorReset)
	fmt.Println()
	waitForEnter()

	fmt.Printf("%sInitial Database State:%s\n", colorBold, colorReset)
	fmt.Println()
	printDatabaseState(ctx, cloud)
	waitForEnter()

	printHeader("STEP 1: Publishing usersync Message")
	fmt.Println()
	fmt.Println("Publishing message to queue:")
	fmt.Printf("  Topic:    %susersync%s\n", colorCyan, colorReset)
	fmt.Printf("  Payload:  %s{databaseID: database-1, userID: user-123}%s\n", colorYellow, colorReset)
	fmt.Printf("  Metadata: %s{providerID: provider-1}%s\n", colorYellow, colorReset)
	fmt.Println()

	syncPayload, _ := json.Marshal(services.SyncUserPayload{
		DatabaseID: "database-1",
		UserID:     "user-123",
	})
	queue.Publish(ctx, services.Message{
		MessageID: "demo-sync-1",
		Topic:     "usersync",
		Payload:   syncPayload,
		Metadata:  map[string]string{"providerID": "provider-1"},
	})

	fmt.Printf("Queue now has %s%d%s pending message(s)\n", colorYellow, queue.PendingCount(), colorReset)
	fmt.Println()
	waitForEnter()

	printHeader("STEP 2: Processing usersync")
	fmt.Println()
	fmt.Println("The on-prem app will:")
	fmt.Printf("  %s1.%s Call MediaVault.ListAlbumUIDs(user-123)\n", colorBold, colorReset)
	fmt.Printf("  %s2.%s POST to cloud /v1/useralbums with the album list\n", colorBold, colorReset)
	fmt.Printf("  %s3.%s Cloud will emit albummanifestupload for each NEW album\n", colorBold, colorReset)
	fmt.Println()
	printCodeRef("sync_user.go", "user_albums.go")
	waitForEnter()

	delivered, _ := queue.Tick(ctx)
	fmt.Printf("Queue delivered %s%d%s message(s)\n", colorGreen, delivered, colorReset)
	fmt.Println()
	fmt.Printf("%sWhat happens next:%s\n", colorBold, colorReset)
	fmt.Println()
	fmt.Printf("  %s•%s 2 albummanifestupload messages have been published (one per album)\n", colorYellow, colorReset)
	fmt.Println("  For each albummanifestupload event, the on-prem app will:")
	fmt.Printf("    %s→%s Build the manifest by calling MediaVault.ListVideoUIDs(albumUID)\n", colorDim, colorReset)
	fmt.Printf("    %s→%s POST to cloud /v1/albummanifestupload with manifest\n", colorDim, colorReset)
	fmt.Printf("    %s→%s Cloud stores album + manifest, emits videoupload\n", colorDim, colorReset)
	fmt.Println()
	fmt.Printf("Queue now has %s%d%s pending message(s)\n", colorYellow, queue.PendingCount(), colorReset)
	fmt.Println()
	printCodeRef("album_manifest_upload_consumer.go", "album_manifest_upload.go")
	waitForEnter()

	delivered, _ = queue.Tick(ctx)
	fmt.Printf("Processed %s%d%s albummanifestupload message(s)\n", colorGreen, delivered, colorReset)
	fmt.Printf("Queue now has %s%d%s pending message(s) (videoupload messages)\n", colorYellow, queue.PendingCount(), colorReset)
	fmt.Println()

	fmt.Printf("%sDatabase after albummanifestupload processing:%s\n", colorBold, colorReset)
	fmt.Println()
	printDatabaseState(ctx, cloud)
	fmt.Println()
	fmt.Printf("%sNote:%s Albums are synced and manifests stored, but videos show\n", colorYellow, colorReset)
	fmt.Printf("      'not yet stored' because videoupload hasn't run yet.\n")
	fmt.Println()
	waitForEnter()

	printHeader("STEP 3: Processing videoupload Messages")
	fmt.Println()
	fmt.Println("For each videoupload, on-prem will:")
	fmt.Printf("  %s1.%s Call MediaVault.CMove(albumUID) - simulates media transfer\n", colorBold, colorReset)
	fmt.Printf("  %s2.%s MediaVault sends each video to on-prem /receive-video\n", colorBold, colorReset)
	fmt.Printf("  %s3.%s On-prem stores locally, then uploads to cloud\n", colorBold, colorReset)
	fmt.Printf("  %s4.%s Cloud validates video is in manifest, stores object\n", colorBold, colorReset)
	fmt.Println()
	fmt.Printf("Processing %s%d%s videoupload message(s)...\n", colorYellow, queue.PendingCount(), colorReset)
	fmt.Println()
	printCodeRef("video_upload_consumer.go", "mediavault.go", "video_receiver.go", "video_upload.go")
	fmt.Println()
	waitForEnter()

	for queue.PendingCount() > 0 {
		delivered, _ := queue.Tick(ctx)
		if delivered > 0 {
			fmt.Printf("  %s✓%s Processed %d message(s), %s%d%s remaining\n",
				colorGreen, colorReset, delivered, colorYellow, queue.PendingCount(), colorReset)
		}
	}
	fmt.Println()
	fmt.Printf("%sAll videos transferred! Final database state:%s\n", colorGreen, colorReset)
	fmt.Println()
	printDatabaseState(ctx, cloud)

	printHeader("DEMO COMPLETE")
	fmt.Println()
	fmt.Println("The sync pipeline successfully:")
	fmt.Printf("  %s✓%s Discovered 2 albums from MediaVault\n", colorGreen, colorReset)
	fmt.Printf("  %s✓%s Built manifests with video lists\n", colorGreen, colorReset)
	fmt.Printf("  %s✓%s Transferred 5 videos via simulated C-MOVE\n", colorGreen, colorReset)
	fmt.Printf("  %s✓%s Stored all objects in cloud database\n", colorGreen, colorReset)
	fmt.Println()
}

func printHeader(title string) {
	fmt.Printf("%s════════════════════════════════════════════════════════════════%s\n", colorCyan, colorReset)
	fmt.Printf("%s  %s%s\n", colorBold, title, colorReset)
	fmt.Printf("%s════════════════════════════════════════════════════════════════%s\n", colorCyan, colorReset)
}

func printCodeRef(files ...string) {
	fmt.Printf("  %s┌─ Code:%s\n", colorMagenta, colorReset)
	for i, f := range files {
		prefix := "├"
		if i == len(files)-1 {
			prefix = "└"
		}
		fmt.Printf("  %s%s %s%s%s\n", colorMagenta, prefix, colorReset, f, colorReset)
	}
}

func waitForEnter() {
	if interactive {
		fmt.Printf("%sPress Enter to continue...%s", colorDim, colorReset)
		reader.ReadString('\n')
	}
	fmt.Println()
}

func printDatabaseState(ctx context.Context, cloud *cloudapp.App) {
	albums := []struct {
		albumUID string
	}{
		{"album-CT-001"},
		{"album-MRI-002"},
	}

	fmt.Printf("  %sAlbums:%s\n", colorBold, colorReset)
	foundAlbums := 0
	for _, a := range albums {
		album, _ := cloud.AlbumRepo.FindByAlbumUID(ctx, "provider-1", "database-1", a.albumUID)
		if album != nil {
			foundAlbums++
			syncStatus := fmt.Sprintf("%s✗ unsynced%s", colorRed, colorReset)
			if album.Synced {
				syncStatus = fmt.Sprintf("%s✓ synced%s", colorGreen, colorReset)
			}
			fmt.Printf("    %s%s%s [%s]\n", colorCyan, album.AlbumUID, colorReset, syncStatus)

			videos, _ := cloud.AlbumVideoRepo.FindByAlbumUID(ctx, "provider-1", "database-1", a.albumUID)
			if len(videos) > 0 {
				fmt.Printf("      Manifest: %s%d video(s)%s\n", colorYellow, len(videos), colorReset)
				for _, vid := range videos {
					obj, _ := cloud.ObjectRepo.FindByVideoUID(ctx, "provider-1", "database-1", vid.VideoUID)
					if obj != nil {
						fmt.Printf("        %s•%s %s %s[stored, %d bytes]%s\n",
							colorGreen, colorReset, vid.VideoUID, colorGreen, obj.SizeBytes, colorReset)
					} else {
						fmt.Printf("        %s•%s %s %s[not yet stored]%s\n",
							colorDim, colorReset, vid.VideoUID, colorDim, colorReset)
					}
				}
			}
		}
	}

	if foundAlbums == 0 {
		fmt.Printf("    %s(no albums found)%s\n", colorDim, colorReset)
	}
	fmt.Println()
}

type deferredMediaVaultRegistry struct {
	getRegistry func() services.MediaVaultRegistry
}

func (r *deferredMediaVaultRegistry) Get(databaseID string) (services.MediaVault, error) {
	return r.getRegistry().Get(databaseID)
}
