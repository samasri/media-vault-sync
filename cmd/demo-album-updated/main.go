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

	tmpDir, err := os.MkdirTemp("", "demo2-*")
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

	printHeader("UNSYNCED ALBUM RECOVERY DEMO")
	fmt.Println()
	fmt.Println("This demo simulates an album getting additional videos after the")
	fmt.Println("initial manifest is built, triggering the 'unsynced' state, then")
	fmt.Println("showing recovery via a new albummanifestupload event.")
	fmt.Println()
	fmt.Println("Flow:")
	fmt.Printf("  %s1.%s Initial sync with 2 videos\n", colorBold, colorReset)
	fmt.Printf("  %s2.%s MediaVault adds 2 more videos (simulating real-world changes)\n", colorBold, colorReset)
	fmt.Printf("  %s3.%s CMove sends 4 videos, 2 rejected → album UNSYNCED\n", colorBold, colorReset)
	fmt.Printf("  %s4.%s Repair via new albummanifestupload → album SYNCED with all 4\n", colorBold, colorReset)
	fmt.Println()
	waitForEnter()

	printHeader("STEP 0: Initial State")
	fmt.Println()
	fmt.Println("Single album with 2 videos:")
	fmt.Println()
	fmt.Printf("  %sProvider:%s provider-1\n", colorBold, colorReset)
	fmt.Printf("  └── %sDatabase:%s database-1\n", colorBold, colorReset)
	fmt.Printf("      └── %sUser:%s user-123\n", colorBold, colorReset)
	fmt.Printf("          └── %sAlbum:%s %salbum-CT-001%s\n", colorBold, colorReset, colorCyan, colorReset)
	fmt.Printf("              └── Videos: %s[vid-001, vid-002]%s\n", colorYellow, colorReset)
	fmt.Println()

	writeConfig(configPath, []string{"vid-001", "vid-002"})
	fmt.Printf("Config written to: %s%s%s\n", colorDim, configPath, colorReset)
	fmt.Println()
	waitForEnter()

	fmt.Println("Setting up components...")
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

	fmt.Printf("  %s✓%s Cloud API at: %s%s%s\n", colorGreen, colorReset, colorBlue, cloudServer.URL, colorReset)
	fmt.Printf("  %s✓%s On-prem app at: %s%s%s\n", colorGreen, colorReset, colorBlue, onpremServer.URL, colorReset)
	fmt.Println()

	fmt.Printf("%sInitial database state:%s\n", colorBold, colorReset)
	fmt.Println()
	printDatabaseState(ctx, cloud)
	fmt.Println()
	waitForEnter()

	printHeader("STEP 1: Publishing usersync Message")
	fmt.Println()
	fmt.Printf("Publishing: %susersync%s for user-123\n", colorCyan, colorReset)
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

	fmt.Printf("Queue has %s%d%s pending message(s)\n", colorYellow, queue.PendingCount(), colorReset)
	fmt.Println()
	waitForEnter()

	printHeader("STEP 2: Processing usersync")
	fmt.Println()
	fmt.Println("On-prem discovers 1 album, emits albummanifestupload...")
	fmt.Println()
	printCodeRef("sync_user.go", "user_albums.go")
	fmt.Println()

	queue.Tick(ctx)
	fmt.Printf("Queue now has %s%d%s pending message(s)\n", colorYellow, queue.PendingCount(), colorReset)
	fmt.Println()
	waitForEnter()

	printHeader("STEP 3: Processing albummanifestupload")
	fmt.Println()
	fmt.Println("Building manifest with current MediaVault config (2 videos)...")
	fmt.Println()
	printCodeRef("album_manifest_upload_consumer.go", "album_manifest_upload.go")
	fmt.Println()

	queue.Tick(ctx)

	fmt.Printf("%sDatabase after albummanifestupload:%s\n", colorBold, colorReset)
	fmt.Println()
	printDatabaseState(ctx, cloud)
	fmt.Println()
	fmt.Printf("%sNote:%s Manifest locked at 2 videos. videoupload message pending.\n", colorYellow, colorReset)
	fmt.Printf("Queue has %s%d%s pending message(s)\n", colorYellow, queue.PendingCount(), colorReset)
	fmt.Println()
	waitForEnter()

	printHeader("STEP 4: MediaVault Config Changes (Key Step)")
	fmt.Println()
	fmt.Printf("%s⚠ BEFORE processing videoupload, MediaVault gets 2 more videos!%s\n", colorYellow, colorReset)
	fmt.Println()
	fmt.Println("This simulates a real-world scenario where new videos arrive at")
	fmt.Println("the MediaVault system between manifest creation and video transfer.")
	fmt.Println()
	fmt.Println("Updating config:")
	fmt.Printf("  Old: %s[vid-001, vid-002]%s\n", colorDim, colorReset)
	fmt.Printf("  New: %s[vid-001, vid-002, vid-003, vid-004]%s\n", colorYellow, colorReset)
	fmt.Println()

	writeConfig(configPath, []string{"vid-001", "vid-002", "vid-003", "vid-004"})
	fmt.Printf("%s✓%s Config updated with 4 videos\n", colorGreen, colorReset)
	fmt.Println()
	waitForEnter()

	printHeader("STEP 5: Processing videoupload")
	fmt.Println()
	fmt.Println("CMove reads UPDATED config, sends ALL 4 videos to cloud...")
	fmt.Println()
	printCodeRef("video_upload_consumer.go", "mediavault.go", "video_receiver.go", "video_upload.go")
	fmt.Println()
	fmt.Printf("%sExpected behavior:%s\n", colorDim, colorReset)
	fmt.Printf("  %s•%s vid-001: in manifest → %saccepted%s\n", colorGreen, colorReset, colorGreen, colorReset)
	fmt.Printf("  %s•%s vid-002: in manifest → %saccepted%s\n", colorGreen, colorReset, colorGreen, colorReset)
	fmt.Printf("  %s•%s vid-003: NOT in manifest → %srejected (409)%s\n", colorRed, colorReset, colorRed, colorReset)
	fmt.Printf("  %s•%s vid-004: NOT in manifest → %srejected (409)%s\n", colorRed, colorReset, colorRed, colorReset)
	fmt.Println()
	fmt.Printf("Album will be marked as %sUNSYNCED%s because not all videos were accepted.\n", colorRed, colorReset)
	fmt.Println()
	waitForEnter()

	// Process until queue is drained (videoupload will retry and fail until max attempts)
	queue.Process(ctx)

	fmt.Printf("%sActual database state:%s\n", colorBold, colorReset)
	fmt.Println()
	printDatabaseState(ctx, cloud)
	fmt.Println()
	fmt.Printf("%s⚠ Album is UNSYNCED!%s Only 2 of 4 videos stored.\n", colorRed, colorReset)
	fmt.Println()
	waitForEnter()

	printHeader("STEP 6: Automatic Repair via Sync Consistency Worker")
	fmt.Println()
	fmt.Println("The EventualConsistencyWorker periodically scans for unsynced albums")
	fmt.Println("and triggers repair by publishing albummanifestupload messages.")
	fmt.Println()
	printCodeRef("eventual_consistency.go")
	fmt.Println()

	eventualConsistencyWorker := services.NewEventualConsistencyWorker(cloud.AlbumRepo, queue, clock)

	fmt.Println("Running worker.Scan()...")
	eventualConsistencyWorker.Scan(ctx)
	fmt.Printf("Queue has %s%d%s pending message(s)\n", colorYellow, queue.PendingCount(), colorReset)
	fmt.Printf("  %sexpected: syncconsistencycheck%s\n", colorDim, colorReset)
	fmt.Println()
	waitForEnter()

	fmt.Println("Processing syncconsistencycheck → emits albummanifestupload...")
	queue.Tick(ctx)
	fmt.Printf("Queue has %s%d%s pending message(s)\n", colorYellow, queue.PendingCount(), colorReset)
	fmt.Printf("  %sexpected: albummanifestupload + syncconsistencycheck (scheduled retry)%s\n", colorDim, colorReset)
	fmt.Println()

	fmt.Println("Processing albummanifestupload (rebuilds manifest with 4 videos)...")
	queue.Tick(ctx)
	fmt.Printf("Queue has %s%d%s pending message(s)\n", colorYellow, queue.PendingCount(), colorReset)
	fmt.Printf("  %sexpected: videoupload + syncconsistencycheck (scheduled retry)%s\n", colorDim, colorReset)
	fmt.Println()

	fmt.Println("Processing videoupload (CMove sends all 4 again)...")
	fmt.Printf("%sExpected behavior:%s\n", colorDim, colorReset)
	fmt.Printf("  %s•%s vid-001: already exists → %supserted%s\n", colorGreen, colorReset, colorGreen, colorReset)
	fmt.Printf("  %s•%s vid-002: already exists → %supserted%s\n", colorGreen, colorReset, colorGreen, colorReset)
	fmt.Printf("  %s•%s vid-003: now in manifest → %saccepted%s\n", colorGreen, colorReset, colorGreen, colorReset)
	fmt.Printf("  %s•%s vid-004: now in manifest → %saccepted%s\n", colorGreen, colorReset, colorGreen, colorReset)
	fmt.Println()
	waitForEnter()

	queue.Process(ctx)

	fmt.Printf("%sActual database state:%s\n", colorBold, colorReset)
	fmt.Println()
	printDatabaseState(ctx, cloud)
	fmt.Println()

	printHeader("DEMO COMPLETE")
	fmt.Println()
	fmt.Println("The demo successfully showed:")
	fmt.Printf("  %s✓%s Manifest locked at 2 videos during initial sync\n", colorGreen, colorReset)
	fmt.Printf("  %s✓%s MediaVault config change added 2 more videos\n", colorGreen, colorReset)
	fmt.Printf("  %s✓%s CMove sent 4, but 2 were rejected → album UNSYNCED\n", colorGreen, colorReset)
	fmt.Printf("  %s✓%s EventualConsistencyWorker detected unsynced album\n", colorGreen, colorReset)
	fmt.Printf("  %s✓%s Automatic repair rebuilt manifest with all 4\n", colorGreen, colorReset)
	fmt.Printf("  %s✓%s All 4 videos stored → album SYNCED\n", colorGreen, colorReset)
	fmt.Println()
}

func writeConfig(configPath string, videos []string) {
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
									{AlbumUID: "album-CT-001", Videos: videos},
								},
							},
						},
					},
				},
			},
		},
	}
	data, _ := json.MarshalIndent(mediaVaultConfig, "", "  ")
	os.WriteFile(configPath, data, 0644)
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
	album, _ := cloud.AlbumRepo.FindByAlbumUID(ctx, "provider-1", "database-1", "album-CT-001")
	if album == nil {
		fmt.Printf("    %s(album not found)%s\n", colorDim, colorReset)
		return
	}

	syncStatus := fmt.Sprintf("%s✗ UNSYNCED%s", colorRed, colorReset)
	if album.Synced {
		syncStatus = fmt.Sprintf("%s✓ SYNCED%s", colorGreen, colorReset)
	}
	fmt.Printf("  %sAlbum:%s %salbum-CT-001%s [%s]\n", colorBold, colorReset, colorCyan, colorReset, syncStatus)

	videos, _ := cloud.AlbumVideoRepo.FindByAlbumUID(ctx, "provider-1", "database-1", "album-CT-001")
	if len(videos) > 0 {
		fmt.Printf("    Manifest: %s%d video(s)%s\n", colorYellow, len(videos), colorReset)
		for _, vid := range videos {
			obj, _ := cloud.ObjectRepo.FindByVideoUID(ctx, "provider-1", "database-1", vid.VideoUID)
			if obj != nil {
				fmt.Printf("      %s•%s %s %s[stored, %d bytes]%s\n",
					colorGreen, colorReset, vid.VideoUID, colorGreen, obj.SizeBytes, colorReset)
			} else {
				fmt.Printf("      %s•%s %s %s[not yet stored]%s\n",
					colorDim, colorReset, vid.VideoUID, colorDim, colorReset)
			}
		}
	}
}

type deferredMediaVaultRegistry struct {
	getRegistry func() services.MediaVaultRegistry
}

func (r *deferredMediaVaultRegistry) Get(databaseID string) (services.MediaVault, error) {
	return r.getRegistry().Get(databaseID)
}
