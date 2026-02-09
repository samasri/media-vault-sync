package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	onpremapp "github.com/media-vault-sync/internal/app/onprem"
)

func main() {
	cfg := onpremapp.LoadConfig()

	if cfg.ProviderID == "" {
		log.Fatal("PROVIDER_ID environment variable is required")
	}

	app := onpremapp.Wire(cfg, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := app.SubscribeAll(ctx); err != nil {
		log.Fatalf("failed to subscribe to topics: %v", err)
	}

	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: app.Handler,
	}

	go func() {
		log.Printf("on-prem receiver starting on :%s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	go runQueueProcessor(ctx, app, cfg.QueueTickInterval)

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

func runQueueProcessor(ctx context.Context, app *onpremapp.App, interval time.Duration) {
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
