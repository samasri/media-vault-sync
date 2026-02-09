package cloud

import (
	"context"
	"net/http"

	"github.com/media-vault-sync/internal/adapters/http/cloud"
	"github.com/media-vault-sync/internal/adapters/queue/memory"
	memoryrepo "github.com/media-vault-sync/internal/adapters/repo/memory"
	"github.com/media-vault-sync/internal/core/services"
)

type TickableQueue interface {
	services.Queue
	Tick(ctx context.Context) (delivered int, requeued int)
	Process(ctx context.Context) int
	PendingCount() int
}

type App struct {
	Handler                    http.Handler
	Queue                      TickableQueue
	Clock                      services.Clock
	AlbumRepo                  services.AlbumRepository
	AlbumVideoRepo             services.AlbumVideoRepository
	VideoRepo                  services.VideoRepository
	ObjectRepo                 services.ObjectRepository
	EventualConsistencyWorker      *services.EventualConsistencyWorker
	EventualConsistencyCheckConsumer *services.EventualConsistencyCheckConsumer
}

type WireOptions struct {
	Clock          services.Clock
	Queue          TickableQueue
	AlbumRepo      services.AlbumRepository
	AlbumVideoRepo services.AlbumVideoRepository
	VideoRepo      services.VideoRepository
	ObjectRepo     services.ObjectRepository
}

func Wire(cfg Config, opts *WireOptions) *App {
	var clock services.Clock
	var queue TickableQueue
	var albumRepo services.AlbumRepository
	var albumVideoRepo services.AlbumVideoRepository
	var videoRepo services.VideoRepository
	var objectRepo services.ObjectRepository

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

	if opts != nil && opts.AlbumRepo != nil {
		albumRepo = opts.AlbumRepo
	} else {
		albumRepo = memoryrepo.NewAlbumRepository()
	}

	if opts != nil && opts.AlbumVideoRepo != nil {
		albumVideoRepo = opts.AlbumVideoRepo
	} else {
		albumVideoRepo = memoryrepo.NewAlbumVideoRepository()
	}

	if opts != nil && opts.VideoRepo != nil {
		videoRepo = opts.VideoRepo
	} else {
		videoRepo = memoryrepo.NewVideoRepository()
	}

	if opts != nil && opts.ObjectRepo != nil {
		objectRepo = opts.ObjectRepo
	} else {
		objectRepo = memoryrepo.NewObjectRepository()
	}

	userAlbumsService := services.NewUserAlbumsService(albumRepo, queue)
	userAlbumsHandler := cloud.NewUserAlbumsHandler(userAlbumsService)

	albumManifestUploadService := services.NewAlbumManifestUploadService(albumRepo, albumVideoRepo, queue, clock)
	albumManifestUploadHandler := cloud.NewAlbumManifestUploadHandler(albumManifestUploadService)

	videoUploadService := services.NewVideoUploadService(albumRepo, albumVideoRepo, videoRepo, objectRepo, clock)
	videoUploadHandler := cloud.NewVideoUploadHandler(videoUploadService)

	eventualConsistencyWorker := services.NewEventualConsistencyWorker(albumRepo, queue, clock)
	eventualConsistencyCheckConsumer := services.NewEventualConsistencyCheckConsumer(albumRepo, queue, clock)

	mux := http.NewServeMux()
	mux.Handle("/v1/useralbums", userAlbumsHandler)
	mux.Handle("/v1/albummanifestupload", albumManifestUploadHandler)
	mux.Handle("/v1/album/", videoUploadHandler)

	return &App{
		Handler:                      mux,
		Queue:                        queue,
		Clock:                        clock,
		AlbumRepo:                    albumRepo,
		AlbumVideoRepo:               albumVideoRepo,
		VideoRepo:                    videoRepo,
		ObjectRepo:                   objectRepo,
		EventualConsistencyWorker:        eventualConsistencyWorker,
		EventualConsistencyCheckConsumer: eventualConsistencyCheckConsumer,
	}
}

func (a *App) SubscribeEventualConsistencyCheck(ctx context.Context) error {
	return a.Queue.Subscribe(ctx, "cloud:syncconsistencycheck", "syncconsistencycheck", "", a.EventualConsistencyCheckConsumer.Handle)
}
