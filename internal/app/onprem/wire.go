package onprem

import (
	"context"
	"net/http"

	"github.com/media-vault-sync/internal/adapters/http/onprem"
	"github.com/media-vault-sync/internal/adapters/mediavault"
	"github.com/media-vault-sync/internal/adapters/queue/memory"
	"github.com/media-vault-sync/internal/adapters/storage/fs"
	"github.com/media-vault-sync/internal/core/services"
)

type TickableQueue interface {
	services.Queue
	Tick(ctx context.Context) (delivered int, requeued int)
	Process(ctx context.Context) int
	PendingCount() int
}

type App struct {
	Handler                     http.Handler
	Queue                       TickableQueue
	Clock                       services.Clock
	MediaVaultRegistry          services.MediaVaultRegistry
	CloudClient                 services.CloudClient
	StagingStorage              services.StagingStorage
	SyncUserConsumer            *services.SyncUserConsumer
	AlbumManifestUploadConsumer *services.AlbumManifestUploadConsumer
	VideoUploadConsumer         *services.VideoUploadConsumer
	ProviderID                  string
}

type WireOptions struct {
	Clock              services.Clock
	Queue              TickableQueue
	MediaVaultRegistry services.MediaVaultRegistry
	CloudClient        services.CloudClient
	StagingStorage     services.StagingStorage
	VideoSender        mediavault.VideoSender
	ReceiverURL        string
	MaxRetries         int
}

func Wire(cfg Config, opts *WireOptions) *App {
	var clock services.Clock
	var queue TickableQueue
	var mediaVaultRegistry services.MediaVaultRegistry
	var cloudClient services.CloudClient
	var stagingStorage services.StagingStorage
	var videoSender mediavault.VideoSender

	if opts != nil && opts.Clock != nil {
		clock = opts.Clock
	} else {
		clock = services.RealClock{}
	}

	if opts != nil && opts.Queue != nil {
		queue = opts.Queue
	} else {
		queue = memory.NewInMemoryQueue(clock)
	}

	if opts != nil && opts.StagingStorage != nil {
		stagingStorage = opts.StagingStorage
	} else {
		stagingStorage = fs.NewStagingStorage(cfg.StagingDir)
	}

	if opts != nil && opts.CloudClient != nil {
		cloudClient = opts.CloudClient
	} else {
		cloudClient = onprem.NewHTTPCloudClient(cfg.CloudBaseURL, nil)
	}

	receiverURL := cfg.ReceiverURL
	if opts != nil && opts.ReceiverURL != "" {
		receiverURL = opts.ReceiverURL
	}
	if receiverURL == "" {
		receiverURL = "http://localhost:" + cfg.Port
	}

	if opts != nil && opts.VideoSender != nil {
		videoSender = opts.VideoSender
	} else {
		videoSender = onprem.NewHTTPVideoSender(receiverURL, cfg.ProviderID, nil)
	}

	if opts != nil && opts.MediaVaultRegistry != nil {
		mediaVaultRegistry = opts.MediaVaultRegistry
	} else {
		mediaVaultRegistry = mediavault.NewFileSystemMediaVaultRegistry(cfg.MediaVaultConfigPath, videoSender)
	}

	maxRetries := 0
	if opts != nil && opts.MaxRetries > 0 {
		maxRetries = opts.MaxRetries
	}

	syncUserConsumer := services.NewSyncUserConsumer(cfg.ProviderID, mediaVaultRegistry, cloudClient, maxRetries)
	albumManifestUploadConsumer := services.NewAlbumManifestUploadConsumer(cfg.ProviderID, mediaVaultRegistry, cloudClient, maxRetries)
	videoUploadConsumer := services.NewVideoUploadConsumer(mediaVaultRegistry)

	videoReceiver := onprem.NewVideoReceiver(stagingStorage, cloudClient, mediaVaultRegistry, maxRetries)

	mux := http.NewServeMux()
	mux.Handle("/receive-video", videoReceiver)

	return &App{
		Handler:                     mux,
		Queue:                       queue,
		Clock:                       clock,
		MediaVaultRegistry:          mediaVaultRegistry,
		CloudClient:                 cloudClient,
		StagingStorage:              stagingStorage,
		SyncUserConsumer:            syncUserConsumer,
		AlbumManifestUploadConsumer: albumManifestUploadConsumer,
		VideoUploadConsumer:         videoUploadConsumer,
		ProviderID:                  cfg.ProviderID,
	}
}

func (a *App) SubscribeAll(ctx context.Context) error {
	providerID := a.ProviderID

	if err := a.Queue.Subscribe(ctx, "onprem:"+providerID+":usersync", "usersync", providerID, a.SyncUserConsumer.Handle); err != nil {
		return err
	}
	if err := a.Queue.Subscribe(ctx, "onprem:"+providerID+":albummanifestupload", "albummanifestupload", providerID, a.AlbumManifestUploadConsumer.Handle); err != nil {
		return err
	}
	if err := a.Queue.Subscribe(ctx, "onprem:"+providerID+":videoupload", "videoupload", providerID, a.VideoUploadConsumer.Handle); err != nil {
		return err
	}
	return nil
}
