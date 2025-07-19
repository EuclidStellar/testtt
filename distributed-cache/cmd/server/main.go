package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/yourusername/distributed-cache/internal/cache"
	"github.com/yourusername/distributed-cache/internal/config"
	"github.com/yourusername/distributed-cache/internal/server"
	"github.com/yourusername/distributed-cache/pkg/types"
)

func main() {
    var configPath = flag.String("config", "", "Path to configuration file")
    flag.Parse()

    // Load configuration
    cfg, err := config.LoadConfig(*configPath)
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }

    // Create cache instance
    var evictionPolicy types.EvictionPolicy
    switch cfg.Cache.EvictionPolicy {
    case "lru":
        evictionPolicy = types.LRU
    case "lfu":
        evictionPolicy = types.LFU
    default:
        evictionPolicy = types.LRU
    }

    cacheInstance := cache.NewMemoryStorage(cfg.Cache.MaxSize, evictionPolicy)

    // Start cleanup routine
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    cacheInstance.StartCleanup(ctx, cfg.Cache.CleanupInterval)

    // Create and start server
    srv := server.NewServer(cacheInstance, cfg)

    // Graceful shutdown handling
    go func() {
        sigint := make(chan os.Signal, 1)
        signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
        <-sigint

        fmt.Println("\nShutting down server...")
        shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
        defer shutdownCancel()

        if err := srv.Shutdown(shutdownCtx); err != nil {
            log.Printf("Server shutdown error: %v", err)
        }
        cancel() // Cancel the cleanup routine
    }()

    // Start server
    if err := srv.Start(ctx); err != nil && err != http.ErrServerClosed {
        log.Fatalf("Server failed to start: %v", err)
    }

    fmt.Println("Server stopped")
}