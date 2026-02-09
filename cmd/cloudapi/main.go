package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	cloudapp "github.com/media-vault-sync/internal/app/cloud"
)

func main() {
	cfg := cloudapp.LoadConfig()
	app := cloudapp.Wire(cfg, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := app.SubscribeEventualConsistencyCheck(ctx); err != nil {
		log.Fatalf("failed to subscribe to syncconsistencycheck: %v", err)
	}

	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: app.Handler,
	}

	go func() {
		log.Printf("cloud API server starting on :%s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	go runQueueProcessor(ctx, app, cfg.QueueTickInterval)
	go runPeriodicScanner(ctx, app, cfg.ScanInterval)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("shutting down...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}
	log.Println("shutdown complete")
}

func runQueueProcessor(ctx context.Context, app *cloudapp.App, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			app.Queue.Tick(ctx)
		}
	}
}

func runPeriodicScanner(ctx context.Context, app *cloudapp.App, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := app.EventualConsistencyWorker.Scan(ctx); err != nil {
				log.Printf("scan error: %v", err)
			}
		}
	}
}
